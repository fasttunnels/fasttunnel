// Package config holds the CLI runtime configuration and auth-state persistence.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// DefaultControlPlaneURL and DefaultEdgeURL are the production endpoint
// addresses baked into the binary at build time via:
//
//	-ldflags "-X github.com/fasttunnels/fasttunnel/cli/internal/config.DefaultControlPlaneURL=https://api.fasttunnel.dev"
//
// GoReleaser sets these automatically on every tagged release (see .goreleaser.yml).
// A plain `go build` without ldflags falls back to the values below, which also
// point at production — so the binary always works out of the box.
//
// Self-hosters can override at runtime without rebuilding:
//
//	export FASTTUNNEL_CONTROL_URL=https://api.mycompany.com
//	export FASTTUNNEL_EDGE_URL=https://edge.mycompany.com
var (
	DefaultControlPlaneURL = "https://api.fasttunnel.dev"
	DefaultEdgeURL         = "https://edge.fasttunnel.dev"
)

// Config holds the remote endpoint addresses the CLI communicates with.
// Values are resolved from environment variables at startup via Load().
type Config struct {
	ControlPlaneURL string // FASTTUNNEL_CONTROL_URL, default http://localhost:8000
	EdgeURL         string // FASTTUNNEL_EDGE_URL,    default http://localhost:8081
}

// AuthState is the credential blob persisted to ~/.fasttunnel/credentials.json.
type AuthState struct {
	AccessToken string `json:"access_token"`
}

// Load resolves a Config from environment variables, falling back to the
// production defaults baked in at build time via ldflags.
func Load() Config {
	return Config{
		ControlPlaneURL: envOr("FASTTUNNEL_CONTROL_URL", DefaultControlPlaneURL),
		EdgeURL:         envOr("FASTTUNNEL_EDGE_URL", DefaultEdgeURL),
	}
}

// Default returns the default Config. Equivalent to Load() with no env vars set.
// Kept so existing callers (agent.NewClient) are not broken.
func Default() Config { return Load() }

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// ── Auth-state persistence ────────────────────────────────────────────────────

func authStateFilePath() (string, error) {
	if cfgDir := os.Getenv("FASTTUNNEL_CONFIG_DIR"); cfgDir != "" {
		return filepath.Join(cfgDir, "credentials.json"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to resolve home dir: %w", err)
	}
	return filepath.Join(home, ".fasttunnel", "credentials.json"), nil
}

// SaveAuth writes state to disk at the resolved credentials path.
func SaveAuth(state AuthState) error {
	path, err := authStateFilePath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("failed to create auth directory: %w", err)
	}
	payload, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to encode auth state: %w", err)
	}
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		return fmt.Errorf("failed to write auth state: %w", err)
	}
	return nil
}

// LoadAuth reads and decodes the persisted auth state.
func LoadAuth() (AuthState, error) {
	path, err := authStateFilePath()
	if err != nil {
		return AuthState{}, err
	}
	payload, err := os.ReadFile(path)
	if err != nil {
		return AuthState{}, err
	}
	var state AuthState
	if err := json.Unmarshal(payload, &state); err != nil {
		return AuthState{}, fmt.Errorf("failed to decode auth state: %w", err)
	}
	return state, nil
}
