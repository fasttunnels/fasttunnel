package agent

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/fasttunnels/fasttunnel/internal/telemetry"
	"github.com/gorilla/websocket"
)

// ── Frame protocol (mirrors data_plane/internal/tunnel) ──────────────────────

const (
	// framePing is sent by either side as a heartbeat.
	framePing = "ping"
	// framePong is the heartbeat reply.
	framePong = "pong"
	// frameHTTPRequest is sent by the edge to the CLI agent when a public HTTP
	// request arrives for a tunnel subdomain.
	frameHTTPRequest = "http_request"
	// frameHTTPResponse is sent by the CLI agent back to the edge after it has
	// proxied the request to the local application.
	frameHTTPResponse = "http_response"
	// frameHTTPResponseStart begins a streamed HTTP response.
	frameHTTPResponseStart = "http_response_start"
	// frameHTTPResponseChunk carries a chunk of a streamed HTTP response body.
	frameHTTPResponseChunk = "http_response_chunk"
	// frameHTTPResponseEnd ends a streamed HTTP response.
	frameHTTPResponseEnd = "http_response_end"
	// frameWSOpen asks the agent to open a websocket stream to the local app.
	frameWSOpen = "ws_open"
	// frameWSOpenAck confirms whether opening a websocket stream succeeded.
	frameWSOpenAck = "ws_open_ack"
	// frameWSData carries websocket payloads.
	frameWSData = "ws_data"
	// frameWSClose carries websocket close metadata.
	frameWSClose = "ws_close"
	// frameWSError carries stream-level websocket errors.
	frameWSError = "ws_error"
)

// frame is the envelope for every message crossing the WebSocket connection.
type frame struct {
	// Type is always required. See the frame* constants above.
	Type string `json:"type"`

	// RequestID ties an http_request frame to its http_response.
	RequestID string `json:"request_id,omitempty"`
	// StreamID ties websocket frames to a single bidirectional ws stream.
	StreamID string `json:"stream_id,omitempty"`

	// ── HTTP request fields (edge -> CLI) ────────────────────────────────────

	Method  string              `json:"method,omitempty"`
	Path    string              `json:"path,omitempty"`
	Query   string              `json:"query,omitempty"`
	Headers map[string][]string `json:"headers,omitempty"`
	// Body carries the request or response body as a base64-encoded string.
	Body string `json:"body,omitempty"`
	// MessageType carries websocket message type constants for ws_data.
	MessageType int `json:"message_type,omitempty"`

	// ── HTTP response fields (CLI -> edge) ───────────────────────────────────

	Status int `json:"status,omitempty"`

	// OK marks a successful ws_open_ack.
	OK bool `json:"ok,omitempty"`
	// Error carries stream-level websocket errors.
	Error string `json:"error,omitempty"`
	// CloseCode carries websocket close status code for ws_close.
	CloseCode int `json:"close_code,omitempty"`
	// CloseReason carries websocket close reason text for ws_close.
	CloseReason string `json:"close_reason,omitempty"`
	// Subprotocol carries selected websocket subprotocol for ws_open_ack.
	Subprotocol string `json:"subprotocol,omitempty"`
}

type localWSStream struct {
	conn    *websocket.Conn
	writeMu sync.Mutex
}

type wsStreamRegistry struct {
	mu      sync.RWMutex
	streams map[string]*localWSStream
}

func newWSStreamRegistry() *wsStreamRegistry {
	return &wsStreamRegistry{streams: make(map[string]*localWSStream)}
}

func (r *wsStreamRegistry) set(streamID string, stream *localWSStream) {
	r.mu.Lock()
	old := r.streams[streamID]
	r.streams[streamID] = stream
	r.mu.Unlock()

	if old != nil {
		_ = old.conn.Close()
	}
}

func (r *wsStreamRegistry) get(streamID string) (*localWSStream, bool) {
	r.mu.RLock()
	stream, ok := r.streams[streamID]
	r.mu.RUnlock()
	return stream, ok
}

func (r *wsStreamRegistry) remove(streamID string) (*localWSStream, bool) {
	r.mu.Lock()
	stream, ok := r.streams[streamID]
	if ok {
		delete(r.streams, streamID)
	}
	r.mu.Unlock()
	return stream, ok
}

func (r *wsStreamRegistry) closeAll() {
	r.mu.Lock()
	streams := r.streams
	r.streams = make(map[string]*localWSStream)
	r.mu.Unlock()

	for _, stream := range streams {
		_ = stream.conn.Close()
	}
}

func requestEventID(req frame) string {
	if req.RequestID != "" {
		return req.RequestID
	}
	if req.StreamID != "" {
		return req.StreamID
	}
	return fmt.Sprintf("%s-%d", strings.ToUpper(req.Method), time.Now().UnixNano())
}

// ── Agent loop ────────────────────────────────────────────────────────────────

// RunAgentLoop connects to the edge WebSocket at wsURL, authenticates with
// sessionToken, and forwards tunnel requests to localhost:localPort.
//
// It never returns unless ctx is cancelled or an unrecoverable error occurs.
// Transient connection errors trigger an exponential-backoff reconnect.
func RunAgentLoop(ctx context.Context, edgeHTTPURL, sessionToken string, localPort int, opts RunOptions) error {
	wsURL := httpToWS(edgeHTTPURL) + "/connect"
	backoff := 1 * time.Second

	for {
		select {
		case <-ctx.Done():
			opts.emit(RuntimeEvent{
				Type:  RuntimeEventConnectionState,
				Time:  time.Now(),
				State: ConnectionStateStopping,
			})
			return ctx.Err()
		default:
		}

		opts.emit(RuntimeEvent{
			Type:  RuntimeEventConnectionState,
			Time:  time.Now(),
			State: ConnectionStateConnecting,
		})

		err := runOnce(ctx, wsURL, sessionToken, localPort, opts)
		if err != nil {
			if ctx.Err() != nil {
				opts.emit(RuntimeEvent{
					Type:  RuntimeEventConnectionState,
					Time:  time.Now(),
					State: ConnectionStateStopping,
				})
				return ctx.Err()
			}
			opts.emit(RuntimeEvent{
				Type:    RuntimeEventConnectionState,
				Time:    time.Now(),
				State:   ConnectionStateReconnecting,
				Reason:  err.Error(),
				Backoff: backoff,
			})
		}

		select {
		case <-ctx.Done():
			opts.emit(RuntimeEvent{
				Type:  RuntimeEventConnectionState,
				Time:  time.Now(),
				State: ConnectionStateStopping,
			})
			return ctx.Err()
		case <-time.After(backoff):
		}

		// Exponential backoff: 1s, 2s, 4s, … capped at 30s.
		if backoff < 30*time.Second {
			backoff *= 2
		}
	}
}

// runOnce establishes one WebSocket connection and services it until it closes.
func runOnce(ctx context.Context, wsURL, sessionToken string, localPort int, opts RunOptions) error {
	header := http.Header{"Authorization": {"Bearer " + sessionToken}}

	conn, _, err := websocket.DefaultDialer.DialContext(ctx, wsURL, header)
	if err != nil {
		return fmt.Errorf("dial %s: %w", wsURL, err)
	}

	defer func() {
		if err := conn.Close(); err != nil {
			// Silently ignore close errors
			telemetry.SilentLogProdError(err)
		}
	}()

	streams := newWSStreamRegistry()
	defer streams.closeAll()

	telemetry.LogInfo(fmt.Sprintf("connected to %s", wsURL))
	opts.emit(RuntimeEvent{
		Type:  RuntimeEventConnectionState,
		Time:  time.Now(),
		State: ConnectionStateOnline,
	})

	var writeMu sync.Mutex

	// conn.ReadMessage() is blocking and does not respect context cancellation.
	// This goroutine watches for ctx cancellation and closes the connection,
	// which causes ReadMessage to return immediately with an error.
	go func() {
		<-ctx.Done()
		_ = writeJSON(conn, &writeMu,
			websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, "shutting down"),
		)
		func() {
			if err := conn.Close(); err != nil {
				telemetry.SilentLogProdError(err)
			}
		}()
	}()

	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			// If the context was cancelled this is an intentional shutdown, not an error.
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return fmt.Errorf("read: %w", err)
		}

		var f frame
		if err := json.Unmarshal(msg, &f); err != nil {
			continue
		}

		switch f.Type {
		case framePing:
			pongMsg, _ := json.Marshal(frame{Type: framePong})
			_ = writeJSON(conn, &writeMu, websocket.TextMessage, pongMsg)
		case frameHTTPRequest:
			go handleHTTPRequest(ctx, conn, &writeMu, f, localPort, opts)
		case frameWSOpen:
			go handleWebSocketOpen(ctx, conn, &writeMu, f, localPort, streams, opts)
		case frameWSData:
			handleWebSocketData(conn, &writeMu, f, streams)
		case frameWSClose, frameWSError:
			handleWebSocketClose(f, streams)
		}
	}
}

// handleHTTPRequest proxies a single tunnel HTTP request to the local app and
// sends the response frame back over the WebSocket connection.
func handleHTTPRequest(ctx context.Context, conn *websocket.Conn, writeMu *sync.Mutex, req frame, localPort int, opts RunOptions) {
	start := time.Now()
	requestID := requestEventID(req)
	if req.RequestID == "" {
		req.RequestID = requestID
	}
	path := req.Path
	if path == "" {
		path = "/"
	}
	if req.Query != "" {
		path += "?" + req.Query
	}

	opts.emit(RuntimeEvent{
		Type:      RuntimeEventRequestStart,
		Time:      start,
		RequestID: requestID,
		Method:    strings.ToUpper(req.Method),
		Path:      path,
	})

	// Log incoming request
	telemetry.LogForwardStart(
		"tunnel",
		req.Method,
		req.Path,
		req.Query,
		fmt.Sprintf("localhost:%d", localPort),
	)

	respStatus, _, respErr := forwardToLocalStream(ctx, req, localPort, func(f frame) error {
		return writeFrame(conn, writeMu, f)
	})
	duration := time.Since(start)

	// Always emit a response log line; status formatting is handled by telemetry.
	telemetry.LogResponse(
		"tunnel",
		req.Method,
		req.Path,
		req.Query,
		respStatus,
		duration,
	)

	responseEventType := RuntimeEventRequestComplete
	if respStatus >= http.StatusBadGateway || respErr != "" {
		responseEventType = RuntimeEventRequestError
	}
	opts.emit(RuntimeEvent{
		Type:      responseEventType,
		Time:      time.Now(),
		RequestID: requestID,
		Method:    strings.ToUpper(req.Method),
		Path:      path,
		Status:    respStatus,
		Duration:  duration,
		Error:     respErr,
	})
}

func writeJSON(conn *websocket.Conn, writeMu *sync.Mutex, messageType int, data []byte) error {
	writeMu.Lock()
	defer writeMu.Unlock()
	return conn.WriteMessage(messageType, data)
}

func writeFrame(conn *websocket.Conn, writeMu *sync.Mutex, f frame) error {
	data, err := json.Marshal(f)
	if err != nil {
		return err
	}
	return writeJSON(conn, writeMu, websocket.TextMessage, data)
}

func handleWebSocketOpen(ctx context.Context, conn *websocket.Conn, writeMu *sync.Mutex, req frame, localPort int, streams *wsStreamRegistry, opts RunOptions) {
	if req.StreamID == "" {
		return
	}

	start := time.Now()
	requestID := requestEventID(req)
	path := req.Path
	if path == "" {
		path = "/"
	}
	if req.Query != "" {
		path += "?" + req.Query
	}
	opts.emit(RuntimeEvent{
		Type:      RuntimeEventRequestStart,
		Time:      start,
		RequestID: requestID,
		Method:    strings.ToUpper(req.Method),
		Path:      path,
	})

	localTarget := fmt.Sprintf("localhost:%d", localPort)
	telemetry.LogForwardStart("tunnel", req.Method, req.Path, req.Query, localTarget)

	targetURL := fmt.Sprintf("ws://localhost:%d%s", localPort, path)
	if req.Query != "" {
		targetURL += "?" + req.Query
	}

	headers := cloneWebSocketHeaders(req.Headers)
	originalHost := firstHeaderValue(req.Headers, "Host")
	if rewrittenOrigin, ok := rewriteOriginForLocal(headers.Get("Origin"), originalHost, localPort); ok {
		headers.Set("Origin", rewrittenOrigin)
	}
	subprotocols := requestedWebSocketSubprotocols(req.Headers)

	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
		Subprotocols:     subprotocols,
	}
	localConn, resp, err := dialer.DialContext(ctx, targetURL, headers)
	if err != nil {
		reason := formatLocalWSError(err, resp)
		telemetry.LogForwardError("tunnel", req.Method, req.Path, req.Query, localTarget, reason)
		opts.emit(RuntimeEvent{
			Type:      RuntimeEventRequestError,
			Time:      time.Now(),
			RequestID: requestID,
			Method:    strings.ToUpper(req.Method),
			Path:      path,
			Status:    http.StatusBadGateway,
			Duration:  time.Since(start),
			Error:     reason,
		})
		_ = writeFrame(conn, writeMu, frame{
			Type:     frameWSOpenAck,
			StreamID: req.StreamID,
			OK:       false,
			Error:    reason,
		})
		return
	}

	streams.set(req.StreamID, &localWSStream{conn: localConn})
	duration := time.Since(start)
	telemetry.LogResponse("tunnel", req.Method, req.Path, req.Query, http.StatusSwitchingProtocols, duration)
	opts.emit(RuntimeEvent{
		Type:      RuntimeEventRequestComplete,
		Time:      time.Now(),
		RequestID: requestID,
		Method:    strings.ToUpper(req.Method),
		Path:      path,
		Status:    http.StatusSwitchingProtocols,
		Duration:  duration,
	})

	if err := writeFrame(conn, writeMu, frame{
		Type:        frameWSOpenAck,
		StreamID:    req.StreamID,
		OK:          true,
		Subprotocol: localConn.Subprotocol(),
	}); err != nil {
		if stream, ok := streams.remove(req.StreamID); ok {
			_ = stream.conn.Close()
		}
		return
	}

	go pumpLocalToEdge(conn, writeMu, req.StreamID, localConn, streams)
}

func formatLocalWSError(err error, resp *http.Response) string {
	if resp == nil {
		return fmt.Sprintf("local websocket unreachable: %v", err)
	}

	bodySnippet := ""
	if resp.Body != nil {
		defer func() { _ = resp.Body.Close() }()
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 256))
		if readErr == nil {
			bodySnippet = strings.Join(strings.Fields(string(body)), " ")
			if len(bodySnippet) > 120 {
				bodySnippet = bodySnippet[:120] + "..."
			}
		}
	}

	if bodySnippet != "" {
		return fmt.Sprintf("local websocket handshake failed: HTTP %d (%v) body=%q", resp.StatusCode, err, bodySnippet)
	}

	return fmt.Sprintf("local websocket handshake failed: HTTP %d (%v)", resp.StatusCode, err)
}

func handleWebSocketData(conn *websocket.Conn, writeMu *sync.Mutex, f frame, streams *wsStreamRegistry) {
	stream, ok := streams.get(f.StreamID)
	if !ok {
		return
	}

	payload, err := base64.StdEncoding.DecodeString(f.Body)
	if err != nil {
		_ = writeFrame(conn, writeMu, frame{
			Type:     frameWSError,
			StreamID: f.StreamID,
			Error:    "invalid ws payload encoding",
		})
		return
	}

	messageType := f.MessageType
	if messageType == 0 {
		messageType = websocket.TextMessage
	}

	stream.writeMu.Lock()
	err = stream.conn.WriteMessage(messageType, payload)
	stream.writeMu.Unlock()
	if err != nil {
		if removed, ok := streams.remove(f.StreamID); ok {
			_ = removed.conn.Close()
		}
		_ = writeFrame(conn, writeMu, frame{
			Type:      frameWSClose,
			StreamID:  f.StreamID,
			CloseCode: websocket.CloseAbnormalClosure,
			Error:     "failed writing to local websocket",
		})
	}
}

func handleWebSocketClose(f frame, streams *wsStreamRegistry) {
	stream, ok := streams.remove(f.StreamID)
	if !ok {
		return
	}

	closeCode := f.CloseCode
	if closeCode == 0 {
		closeCode = websocket.CloseNormalClosure
	}

	_ = stream.conn.WriteControl(
		websocket.CloseMessage,
		websocket.FormatCloseMessage(closeCode, f.CloseReason),
		time.Now().Add(2*time.Second),
	)
	_ = stream.conn.Close()
}

func pumpLocalToEdge(conn *websocket.Conn, writeMu *sync.Mutex, streamID string, localConn *websocket.Conn, streams *wsStreamRegistry) {
	defer func() {
		if stream, ok := streams.remove(streamID); ok {
			_ = stream.conn.Close()
		}
	}()

	for {
		messageType, payload, err := localConn.ReadMessage()
		if err != nil {
			closeCode := websocket.CloseNormalClosure
			closeReason := "normal closure"

			if ce, ok := err.(*websocket.CloseError); ok {
				closeCode = ce.Code
				if ce.Text != "" {
					closeReason = ce.Text
				}
			}

			_ = writeFrame(conn, writeMu, frame{
				Type:        frameWSClose,
				StreamID:    streamID,
				CloseCode:   closeCode,
				CloseReason: closeReason,
			})
			return
		}

		if messageType != websocket.TextMessage && messageType != websocket.BinaryMessage {
			continue
		}

		if err := writeFrame(conn, writeMu, frame{
			Type:        frameWSData,
			StreamID:    streamID,
			MessageType: messageType,
			Body:        base64.StdEncoding.EncodeToString(payload),
		}); err != nil {
			return
		}
	}
}

func cloneWebSocketHeaders(headers map[string][]string) http.Header {
	cloned := make(http.Header)
	for key, vals := range headers {
		if strings.EqualFold(key, "Host") ||
			strings.EqualFold(key, "Connection") ||
			strings.EqualFold(key, "Upgrade") ||
			strings.EqualFold(key, "Sec-WebSocket-Key") ||
			strings.EqualFold(key, "Sec-WebSocket-Version") ||
			strings.EqualFold(key, "Sec-WebSocket-Extensions") ||
			strings.EqualFold(key, "Sec-WebSocket-Protocol") {
			continue
		}

		for _, v := range vals {
			cloned.Add(key, v)
		}
	}
	return cloned
}

func requestedWebSocketSubprotocols(headers map[string][]string) []string {
	seen := make(map[string]struct{})
	var protocols []string

	for key, values := range headers {
		if !strings.EqualFold(key, "Sec-WebSocket-Protocol") {
			continue
		}
		for _, raw := range values {
			for _, token := range strings.Split(raw, ",") {
				protocol := strings.TrimSpace(token)
				if protocol == "" {
					continue
				}
				if _, ok := seen[protocol]; ok {
					continue
				}
				seen[protocol] = struct{}{}
				protocols = append(protocols, protocol)
			}
		}
	}

	return protocols
}

const maxResponseChunkBytes = 512 * 1024

// forwardToLocalStream sends the tunnel request to localhost:localPort and streams the response.
func forwardToLocalStream(ctx context.Context, req frame, localPort int, send func(frame) error) (int, int, string) {
	// Build the target URL.
	path := req.Path
	if path == "" {
		path = "/"
	}
	targetURL := fmt.Sprintf("http://localhost:%d%s", localPort, path)
	if req.Query != "" {
		targetURL += "?" + req.Query
	}

	// Decode base64 body.
	var bodyReader io.Reader = strings.NewReader("")
	if req.Body != "" {
		decoded, err := base64.StdEncoding.DecodeString(req.Body)
		if err == nil {
			bodyReader = bytes.NewReader(decoded)
		}
	}

	httpReq, err := http.NewRequestWithContext(ctx, req.Method, targetURL, bodyReader)
	if err != nil {
		return sendErrorResponse(req.RequestID, http.StatusBadGateway, "failed to build request", send)
	}
	httpReq.Host = fmt.Sprintf("localhost:%d", localPort)

	originalHost := firstHeaderValue(req.Headers, "Host")
	originalProto := firstHeaderValue(req.Headers, "X-Forwarded-Proto")

	// Copy headers, skipping hop-by-hop headers.
	for k, vals := range req.Headers {
		if isHopByHopHeader(k) || strings.EqualFold(k, "host") {
			continue
		}
		for _, v := range vals {
			httpReq.Header.Add(k, v)
		}
	}

	if originalHost != "" {
		httpReq.Header.Set("X-Forwarded-Host", originalHost)
	}
	if originalProto != "" {
		httpReq.Header.Set("X-Forwarded-Proto", originalProto)
	}

	if rewrittenOrigin, ok := rewriteOriginForLocal(httpReq.Header.Get("Origin"), originalHost, localPort); ok {
		httpReq.Header.Set("Origin", rewrittenOrigin)
	}

	client := &http.Client{Timeout: 25 * time.Second}
	httpResp, err := client.Do(httpReq)
	if err != nil {
		return sendErrorResponse(req.RequestID, http.StatusBadGateway, "local app unreachable", send)
	}

	defer func() {
		if err := httpResp.Body.Close(); err != nil {
			// Silently ignore errors closing response body
			telemetry.SilentLogProdError(err)
		}
	}()

	respHeaders := make(map[string][]string)
	for k, vals := range httpResp.Header {
		if isHopByHopHeader(k) {
			continue
		}
		if strings.EqualFold(k, "content-length") {
			continue
		}
		respHeaders[k] = vals
	}

	startFrame := frame{
		Type:      frameHTTPResponseStart,
		RequestID: req.RequestID,
		Status:    httpResp.StatusCode,
		Headers:   respHeaders,
	}
	if err := send(startFrame); err != nil {
		telemetry.SilentLogProdError(err)
		return http.StatusBadGateway, 0, "failed to send response"
	}

	bytesSent := 0
	buf := make([]byte, maxResponseChunkBytes)
	for {
		readBytes, readErr := httpResp.Body.Read(buf)
		if readBytes > 0 {
			chunk := frame{
				Type:      frameHTTPResponseChunk,
				RequestID: req.RequestID,
				Body:      base64.StdEncoding.EncodeToString(buf[:readBytes]),
			}
			if err := send(chunk); err != nil {
				telemetry.SilentLogProdError(err)
				return http.StatusBadGateway, bytesSent, "failed to send response"
			}
			bytesSent += readBytes
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			endErr := send(frame{
				Type:      frameHTTPResponseEnd,
				RequestID: req.RequestID,
				Error:     "failed to read response",
			})
			if endErr != nil {
				telemetry.SilentLogProdError(endErr)
			}
			return httpResp.StatusCode, bytesSent, "failed to read response"
		}
	}

	if err := send(frame{Type: frameHTTPResponseEnd, RequestID: req.RequestID}); err != nil {
		telemetry.SilentLogProdError(err)
		return http.StatusBadGateway, bytesSent, "failed to send response"
	}

	return httpResp.StatusCode, bytesSent, ""
}

func isHopByHopHeader(header string) bool {
	switch strings.ToLower(header) {
	case "connection", "proxy-connection", "keep-alive", "proxy-authenticate", "proxy-authorization", "te", "trailer", "transfer-encoding", "upgrade":
		return true
	default:
		return false
	}
}

func firstHeaderValue(headers map[string][]string, key string) string {
	for hKey, vals := range headers {
		if !strings.EqualFold(hKey, key) {
			continue
		}
		if len(vals) == 0 {
			return ""
		}
		return vals[0]
	}
	return ""
}

func rewriteOriginForLocal(origin, originalHost string, localPort int) (string, bool) {
	if origin == "" || originalHost == "" {
		return "", false
	}

	originURL, err := url.Parse(origin)
	if err != nil || originURL.Host == "" {
		return "", false
	}

	originHost := originURL.Hostname()
	forwardedHost, _, err := net.SplitHostPort(originalHost)
	if err != nil {
		forwardedHost = originalHost
	}

	if !strings.EqualFold(originHost, forwardedHost) {
		return "", false
	}

	originURL.Scheme = "http"
	originURL.Host = fmt.Sprintf("localhost:%d", localPort)
	return originURL.String(), true
}

func sendErrorResponse(requestID string, status int, msg string, send func(frame) error) (int, int, string) {
	respBody := fmt.Sprintf(`{"error":%q}`, msg)
	if err := send(frame{
		Type:      frameHTTPResponseStart,
		RequestID: requestID,
		Status:    status,
		Headers:   map[string][]string{"Content-Type": {"application/json"}},
	}); err != nil {
		telemetry.SilentLogProdError(err)
		return status, 0, msg
	}

	if err := send(frame{
		Type:      frameHTTPResponseChunk,
		RequestID: requestID,
		Body:      base64.StdEncoding.EncodeToString([]byte(respBody)),
	}); err != nil {
		telemetry.SilentLogProdError(err)
		return status, 0, msg
	}

	if err := send(frame{Type: frameHTTPResponseEnd, RequestID: requestID, Error: msg}); err != nil {
		telemetry.SilentLogProdError(err)
		return status, len(respBody), msg
	}

	return status, len(respBody), msg
}

// httpToWS converts an http(s):// URL to a ws(s):// URL.
// If the URL is already ws:// or wss:// it is returned unchanged.
func httpToWS(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return strings.Replace(rawURL, "http://", "ws://", 1)
	}
	switch u.Scheme {
	case "https":
		u.Scheme = "wss"
	case "http":
		u.Scheme = "ws"
		// "ws" and "wss" are already correct — leave them untouched.
	}
	return u.String()
}
