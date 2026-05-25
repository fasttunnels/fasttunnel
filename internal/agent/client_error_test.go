package agent

import (
	"net/http"
	"testing"
)

func TestBuildAPIErrorExpiredTokenGetsLoginHint(t *testing.T) {
	raw := []byte(`{"detail":"Token has expired","code":"INVALID_TOKEN","status_code":401}`)

	apiErr := buildAPIError(http.MethodPost, "/api/v1/tunnels", http.StatusUnauthorized, raw)
	if apiErr == nil {
		t.Fatal("expected API error")
	}
	if apiErr.UserMsg != "Session expired." {
		t.Fatalf("expected session-expired message, got %q", apiErr.UserMsg)
	}
	if apiErr.ActionHint != "Run: fasttunnel login" {
		t.Fatalf("expected login action hint, got %q", apiErr.ActionHint)
	}
}

func TestBuildAPIErrorLoginRequiredMessage(t *testing.T) {
	raw := []byte(`{"detail":"Login required. Run: fasttunnel login","code":"LOGIN_REQUIRED","status_code":401}`)

	apiErr := buildAPIError(http.MethodPost, "/api/v1/tunnels", http.StatusUnauthorized, raw)
	if apiErr == nil {
		t.Fatal("expected API error")
	}
	if apiErr.UserMsg != "You are not logged in." {
		t.Fatalf("expected login-required message, got %q", apiErr.UserMsg)
	}
	if apiErr.ActionHint != "Run: fasttunnel login" {
		t.Fatalf("expected login action hint, got %q", apiErr.ActionHint)
	}
}

func TestBuildAPIErrorKeepsEndpointMappings(t *testing.T) {
	raw := []byte(`{"detail":"subdomain already taken","code":"SUBDOMAIN_TAKEN","status_code":409}`)

	apiErr := buildAPIError(http.MethodPost, "/api/v1/tunnels", http.StatusConflict, raw)
	if apiErr == nil {
		t.Fatal("expected API error")
	}
	if apiErr.UserMsg != "Domain is already taken, try a different domain" {
		t.Fatalf("expected subdomain message mapping, got %q", apiErr.UserMsg)
	}
	if apiErr.ActionHint != "" {
		t.Fatalf("expected empty action hint, got %q", apiErr.ActionHint)
	}
}

func TestBuildAPIErrorRespectsSilentTunnelNotFound(t *testing.T) {
	raw := []byte(`{"detail":"Tunnel not found","code":"TUNNEL_NOT_FOUND","status_code":404}`)

	apiErr := buildAPIError(http.MethodDelete, "/api/v1/tunnels/abc123", http.StatusNotFound, raw)
	if apiErr == nil {
		t.Fatal("expected API error")
	}
	if !apiErr.Silent {
		t.Fatal("expected silent API error for tunnel cleanup not found")
	}
}
