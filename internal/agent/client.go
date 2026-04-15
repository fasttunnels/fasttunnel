package agent

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/fasttunnels/fasttunnel/internal/config"
	"github.com/fasttunnels/fasttunnel/internal/telemetry"
)

type Client struct {
	httpClient *http.Client
	config     config.Config
	version    string
}

// ── PKCE CLI auth responses ────────────────────────────────────────────────────

// CliInitResponse is returned by POST /auth/cli/init.
type CliInitResponse struct {
	RequestID string `json:"request_id"`
	LoginURL  string `json:"login_url"`
}

// CliTokenResponse is returned by POST /auth/cli/token.
type CliTokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
}

// ── Tunnel / session responses ─────────────────────────────────────────────────

type CreateTunnelResponse struct {
	TunnelID  string `json:"tunnel_id"`
	Subdomain string `json:"subdomain"`
	PublicURL string `json:"public_url"`
	Protocol  string `json:"protocol"`
}

type LeaseResponse struct {
	SessionID    string `json:"session_id"`
	SessionToken string `json:"session_token"`
	EdgeURL      string `json:"edge_url"`
	ExpiresIn    int    `json:"expires_in"`
}

type apiErrorPayload struct {
	Detail     string `json:"detail"`
	Code       string `json:"code"`
	StatusCode int    `json:"status_code"`
}

func NewClient(version string) *Client {
	return &Client{
		httpClient: &http.Client{},
		config:     config.Default(),
		version:    version,
	}
}

// ── Auth methods ───────────────────────────────────────────────────────────────

// InitLogin creates a PKCE login intent on the control plane.
// codeChallenge is the S256 hash of the code verifier.
// state is a random anti-CSRF token.
// redirectURI is the local callback server URL.
func (c *Client) InitLogin(codeChallenge, state, redirectURI string) (CliInitResponse, error) {
	payload := map[string]any{
		"code_challenge":        codeChallenge,
		"code_challenge_method": "S256",
		"state":                 state,
		"redirect_uri":          redirectURI,
	}
	var resp CliInitResponse
	if err := c.postJSON(c.controlURL("/api/v1/auth/cli/init"), payload, "", &resp); err != nil {
		return CliInitResponse{}, err
	}
	return resp, nil
}

// ExchangeCliToken exchanges a PKCE authorization code for access + refresh tokens.
func (c *Client) ExchangeCliToken(code, codeVerifier, redirectURI string) (CliTokenResponse, error) {
	payload := map[string]any{
		"code":          code,
		"code_verifier": codeVerifier,
		"redirect_uri":  redirectURI,
	}
	var resp CliTokenResponse
	if err := c.postJSON(c.controlURL("/api/v1/auth/cli/token"), payload, "", &resp); err != nil {
		return CliTokenResponse{}, err
	}
	return resp, nil
}

// ── Tunnel methods ─────────────────────────────────────────────────────────────

func (c *Client) CreateTunnel(subdomain string, localPort int, protocol string, accessToken string) (CreateTunnelResponse, error) {
	if localPort <= 0 || localPort > 65535 {
		return CreateTunnelResponse{}, fmt.Errorf("invalid local port")
	}

	payload := map[string]any{
		"local_port": localPort,
		"protocol":   protocol,
	}
	// Only include requested_subdomain when the user explicitly provided one.
	if subdomain != "" {
		payload["requested_subdomain"] = subdomain
	}

	var resp CreateTunnelResponse
	if err := c.postJSON(c.controlURL("/api/v1/tunnels"), payload, accessToken, &resp); err != nil {
		return CreateTunnelResponse{}, err
	}
	return resp, nil
}

func (c *Client) CreateLease(tunnelID string, protocol string, accessToken string) (LeaseResponse, error) {
	payload := map[string]any{
		"tunnel_id": tunnelID,
		"protocol":  protocol,
	}

	var resp LeaseResponse
	if err := c.postJSON(c.controlURL("/api/v1/sessions/lease"), payload, accessToken, &resp); err != nil {
		return LeaseResponse{}, err
	}
	return resp, nil
}

func (c *Client) RegisterEdge(lease LeaseResponse, tunnelID string, subdomain string, protocol string) error {
	payload := map[string]any{
		"session_id": lease.SessionID,
		"tunnel_id":  tunnelID,
		"subdomain":  subdomain,
		"protocol":   protocol,
	}
	return c.postJSON(c.edgeURL("/v1/register"), payload, lease.SessionToken, nil)
}

// DeleteTunnel hard-deletes a tunnel from the control plane.
// Called by the CLI on graceful shutdown to clean up the DB row and release
// the subdomain for reuse.  The control plane cascades the delete to also
// mark any active session as disconnected.
func (c *Client) DeleteTunnel(tunnelID, accessToken string) error {
	req, err := http.NewRequest(http.MethodDelete, c.controlURL("/api/v1/tunnels/"+tunnelID), nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}

	defer func() {
		if err := resp.Body.Close(); err != nil {
			// Silently ignore errors closing response body
			telemetry.SilentLogProdError(err)
		}
	}()

	if resp.StatusCode >= 400 {
		raw, _ := io.ReadAll(resp.Body)
		apiErr := buildAPIError(req.Method, req.URL.Path, resp.StatusCode, raw)
		if apiErr.Silent {
			return nil
		}
		return apiErr
	}
	return nil
}

// ── Internal HTTP helpers ──────────────────────────────────────────────────────

func (c *Client) controlURL(path string) string {
	return c.config.ControlPlaneURL + path
}

func (c *Client) edgeURL(path string) string {
	return c.config.EdgeURL + path
}

func (c *Client) postJSON(url string, payload any, token string, out any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to encode request: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			fmt.Printf("failed to close response body: %v\n", err)
		}
	}()

	if resp.StatusCode >= 400 {
		raw, _ := io.ReadAll(resp.Body)
		apiErr := buildAPIError(req.Method, req.URL.Path, resp.StatusCode, raw)
		return apiErr
	}

	if out == nil {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}
	return nil
}

func buildAPIError(method, requestPath string, statusCode int, raw []byte) *telemetry.APIError {
	endpoint := normalizeEndpoint(method, requestPath)

	var payload apiErrorPayload
	_ = json.Unmarshal(raw, &payload)

	detail := payload.Detail
	if detail == "" {
		detail = strings.TrimSpace(string(raw))
	}

	userMsg, silent := mapAPIUserMessage(endpoint, payload.Code)
	if userMsg == "" {
		userMsg = detail
	}

	return telemetry.BuildAPIError(method, endpoint, statusCode, payload.Code, detail, userMsg, silent)
}

func normalizeEndpoint(method, requestPath string) string {
	switch {
	case method == http.MethodPost && requestPath == "/api/v1/auth/cli/init":
		return "/api/v1/auth/cli/init"
	case method == http.MethodPost && requestPath == "/api/v1/auth/cli/token":
		return "/api/v1/auth/cli/token"
	case method == http.MethodPost && requestPath == "/api/v1/tunnels":
		return "/api/v1/tunnels"
	case method == http.MethodPost && requestPath == "/api/v1/sessions/lease":
		return "/api/v1/sessions/lease"
	case method == http.MethodDelete && strings.HasPrefix(requestPath, "/api/v1/tunnels/"):
		return "/api/v1/tunnels/{tunnelId}"
	default:
		return requestPath
	}
}

func mapAPIUserMessage(endpoint, code string) (string, bool) {
	switch endpoint {
	case "/api/v1/auth/cli/init":
		if code == "UNSUPPORTED_CHALLENGE_METHOD" {
			return "Error logging in, try again later", false
		}

	case "/api/v1/auth/cli/token":
		switch code {
		case "PKCE_VERIFICATION_FAILED", "REDIRECT_URI_MISMATCH":
			return "Error logging in, verification failed", false
		}

	case "/api/v1/tunnels":
		switch code {
		case "UNSUPPORTED_PROTOCOL":
			return "Protocol not supported", false
		case "SUBDOMAIN_RESERVED":
			return "Cannot issue the requested domain, try a different domain", false
		case "SUBDOMAIN_TAKEN":
			return "Domain is already taken, try a different domain", false
		}

	case "/api/v1/sessions/lease":
		switch code {
		case "TUNNEL_OWNERSHIP_DENIED":
			return "Nice try..", false
		case "TUNNEL_DELETED":
			return "Tunnel not found", false
		}

	case "/api/v1/tunnels/{tunnelId}":
		if code == "TUNNEL_NOT_FOUND" {
			return "", true
		}
	}

	return "", false
}
