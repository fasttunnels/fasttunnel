package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type Config struct {
	ControlPlaneURL string
	EdgeURL         string
}

type AuthState struct {
	AccessToken string `json:"access_token"`
}

func Default() Config {
	return Config{
		ControlPlaneURL: "http://localhost:8000",
		EdgeURL:         "http://localhost:8081",
	}
}

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
