package agent

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

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
)

// frame is the envelope for every message crossing the WebSocket connection.
type frame struct {
	// Type is always required. See the frame* constants above.
	Type string `json:"type"`

	// RequestID ties an http_request frame to its http_response.
	RequestID string `json:"request_id,omitempty"`

	// ── HTTP request fields (edge -> CLI) ────────────────────────────────────

	Method  string              `json:"method,omitempty"`
	Path    string              `json:"path,omitempty"`
	Query   string              `json:"query,omitempty"`
	Headers map[string][]string `json:"headers,omitempty"`
	// Body carries the request or response body as a base64-encoded string.
	Body string `json:"body,omitempty"`

	// ── HTTP response fields (CLI -> edge) ───────────────────────────────────

	Status int `json:"status,omitempty"`
}

// ── Agent loop ────────────────────────────────────────────────────────────────

// RunAgentLoop connects to the edge WebSocket at wsURL, authenticates with
// sessionToken, and forwards tunnel requests to localhost:localPort.
//
// It never returns unless ctx is cancelled or an unrecoverable error occurs.
// Transient connection errors trigger an exponential-backoff reconnect.
func RunAgentLoop(ctx context.Context, edgeHTTPURL, sessionToken string, localPort int) error {
	wsURL := httpToWS(edgeHTTPURL) + "/connect"
	backoff := 1 * time.Second

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		err := runOnce(ctx, wsURL, sessionToken, localPort)
		if err != nil {
			log.Printf("agent disconnected: %v — reconnecting in %s", err, backoff)
		}

		select {
		case <-ctx.Done():
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
func runOnce(ctx context.Context, wsURL, sessionToken string, localPort int) error {
	header := http.Header{"Authorization": {"Bearer " + sessionToken}}

	conn, _, err := websocket.DefaultDialer.DialContext(ctx, wsURL, header)
	if err != nil {
		return fmt.Errorf("dial %s: %w", wsURL, err)
	}
	defer conn.Close()

	log.Printf("agent connected to %s", wsURL)

	// Reset backoff on successful connect is handled by the caller.
	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			return fmt.Errorf("read: %w", err)
		}

		var f frame
		if err := json.Unmarshal(msg, &f); err != nil {
			continue
		}

		switch f.Type {
		case framePing:
			pongMsg, _ := json.Marshal(frame{Type: framePong})
			_ = conn.WriteMessage(websocket.TextMessage, pongMsg)
		case frameHTTPRequest:
			go handleHTTPRequest(ctx, conn, f, localPort)
		}
	}
}

// handleHTTPRequest proxies a single tunnel HTTP request to the local app and
// sends the response frame back over the WebSocket connection.
func handleHTTPRequest(ctx context.Context, conn *websocket.Conn, req frame, localPort int) {
	resp := forwardToLocal(ctx, req, localPort)
	data, err := json.Marshal(resp)
	if err != nil {
		log.Printf("marshal response frame: %v", err)
		return
	}
	if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
		log.Printf("write response frame: %v", err)
	}
}

// forwardToLocal sends the tunnel request to localhost:localPort and returns the response frame.
func forwardToLocal(ctx context.Context, req frame, localPort int) frame {
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
		return errorFrame(req.RequestID, http.StatusBadGateway, "failed to build request")
	}

	// Copy headers, skipping hop-by-hop headers.
	for k, vals := range req.Headers {
		switch strings.ToLower(k) {
		case "connection", "transfer-encoding", "host":
			continue
		}
		for _, v := range vals {
			httpReq.Header.Add(k, v)
		}
	}

	client := &http.Client{Timeout: 25 * time.Second}
	httpResp, err := client.Do(httpReq)
	if err != nil {
		log.Printf("local forward error: %v", err)
		return errorFrame(req.RequestID, http.StatusBadGateway, "local app unreachable")
	}
	defer httpResp.Body.Close()

	// Read response body; cap at 16 MiB.
	respBody, err := io.ReadAll(io.LimitReader(httpResp.Body, 16*1024*1024))
	if err != nil {
		return errorFrame(req.RequestID, http.StatusInternalServerError, "failed to read response")
	}

	respHeaders := make(map[string][]string)
	for k, vals := range httpResp.Header {
		switch strings.ToLower(k) {
		case "transfer-encoding":
			continue
		}
		respHeaders[k] = vals
	}

	return frame{
		Type:      frameHTTPResponse,
		RequestID: req.RequestID,
		Status:    httpResp.StatusCode,
		Headers:   respHeaders,
		Body:      base64.StdEncoding.EncodeToString(respBody),
	}
}

func errorFrame(requestID string, status int, msg string) frame {
	respBody := fmt.Sprintf(`{"error":%q}`, msg)
	return frame{
		Type:      frameHTTPResponse,
		RequestID: requestID,
		Status:    status,
		Headers:   map[string][]string{"Content-Type": {"application/json"}},
		Body:      base64.StdEncoding.EncodeToString([]byte(respBody)),
	}
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
