package agent

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/fasttunnel/fasttunnel/cli/internal/config"
)

type Client struct {
	httpClient *http.Client
	config     config.Config
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

func NewClient() *Client {
	return &Client{
		httpClient: &http.Client{},
		config:     config.Default(),
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
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		raw, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("api error (%d): %s", resp.StatusCode, string(raw))
	}

	if out == nil {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}
	return nil
}
