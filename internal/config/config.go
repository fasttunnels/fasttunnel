// Package config holds the CLI runtime configuration and auth-state persistence.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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

// Load resolves a Config from environment variables, falling back to defaults.
func Load() Config {
	return Config{
		ControlPlaneURL: envOr("FASTTUNNEL_CONTROL_URL", "https://api.fasttunnel.dev"),
		EdgeURL:         envOr("FASTTUNNEL_EDGE_URL", "https://edge.fasttunnel.dev"),
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
