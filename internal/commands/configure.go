package commands

import (
	"fmt"

	"github.com/fasttunnels/fasttunnel/internal/agent"
	"github.com/fasttunnels/fasttunnel/internal/cmdparse"
	"github.com/fasttunnels/fasttunnel/internal/config"
	"github.com/fasttunnels/fasttunnel/internal/telemetry"
)

// RunConfigure handles the `fasttunnel configure <auth_token>` command.
//
// It validates the token format, exchanges it for a short-lived access JWT,
// persists both credentials to disk, and prints a success confirmation.
//
// The two-file layout after success:
//
//	~/.fasttunnel/config.json       → { "auth_token": "ft_sk_..." }
//	~/.fasttunnel/credentials.json  → { "access_token": "<JWT>" }
//
// Subsequent `fasttunnel login` calls skip the browser if config.json exists.
// `fasttunnel http` silently re-exchanges if the JWT expires.
func RunConfigure(client *agent.Client, parsed cmdparse.Configure) error {
	telemetry.LogInfo("Exchanging auth token…")

	resp, err := client.ExchangeAuthToken(parsed.AuthToken)
	if err != nil {
		return fmt.Errorf("token exchange failed: %w\n\nCheck that the token is valid and not revoked.\nGenerate a new token at: https://app.fasttunnel.dev/cli-access", err)
	}

	// Persist auth token to config.json
	if err := config.SaveAuthConfig(config.AuthConfig{AuthToken: parsed.AuthToken}); err != nil {
		return fmt.Errorf("failed to save auth config: %w", err)
	}

	// Persist access JWT to credentials.json
	if err := config.SaveAuth(config.AuthState{AccessToken: resp.AccessToken}); err != nil {
		return fmt.Errorf("failed to save credentials: %w", err)
	}

	msg := "Configured successfully."
	if resp.Email != "" {
		msg = fmt.Sprintf("Configured successfully. Authenticated as %s", resp.Email)
	}
	telemetry.LogInfo(msg)
	return nil
}
