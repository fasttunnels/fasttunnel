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
