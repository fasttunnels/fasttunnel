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

func TestBuildAPIErrorExplainsDeletedLeaseRace(t *testing.T) {
	raw := []byte(`{"detail":"Tunnel has been deleted","code":"TUNNEL_DELETED","status_code":400}`)

	apiErr := buildAPIError(http.MethodPost, "/api/v1/sessions/lease", http.StatusBadRequest, raw)
	if apiErr == nil {
		t.Fatal("expected API error")
	}
	if apiErr.UserMsg != "Tunnel was already closed. Try running the command again." {
		t.Fatalf("expected deleted lease guidance, got %q", apiErr.UserMsg)
	}
}

func TestBuildAPIErrorDeviceAuthorizationPending(t *testing.T) {
	raw := []byte(`{"code":"authorization_pending","detail":"Authorization pending"}`)

	apiErr := buildAPIError(http.MethodPost, "/api/v1/auth/device/token", http.StatusBadRequest, raw)
	if apiErr == nil {
		t.Fatal("expected API error")
	}
	if apiErr.Code != "authorization_pending" {
		t.Fatalf("expected code authorization_pending, got %q", apiErr.Code)
	}
	if apiErr.Detail != "Authorization pending" {
		t.Fatalf("expected detail 'Authorization pending', got %q", apiErr.Detail)
	}
}
