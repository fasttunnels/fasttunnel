// Package commands contains the runnable subcommand implementations.
// Each exported Run* function maps 1-to-1 to a CLI subcommand.
// Dependencies are injected via parameters; no global state.
package commands

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/fasttunnels/fasttunnel/internal/agent"
	"github.com/fasttunnels/fasttunnel/internal/auth"
	"github.com/fasttunnels/fasttunnel/internal/browser"
	"github.com/fasttunnels/fasttunnel/internal/callback"
	"github.com/fasttunnels/fasttunnel/internal/cmdparse"
	"github.com/fasttunnels/fasttunnel/internal/config"
	"github.com/fasttunnels/fasttunnel/internal/telemetry"
)

// RunLogin handles the login subcommand.
//
// Silent path: if ~/.fasttunnel/config.json holds an auth token, it is
// exchanged for a short-lived access JWT — no browser required.
//
// Device path: runs the RFC 8628 headless device code flow when
// --device or -d is passed.
//
// Browser path (default fallback): runs the standard OAuth 2.0 PKCE flow:
//  1. Generate code_verifier, code_challenge (S256), anti-CSRF state.
//  2. Start a temporary local callback server.
//  3. Call /auth/cli/init → receive login_url.
//  4. Open the login_url in the default browser.
//  5. Wait for the callback (code + validated state).
//  6. Exchange code for tokens via /auth/cli/token.
//  7. Persist the access token to disk.
//
// parsed is pre-resolved by cmdparse — no flag handling here.
func RunLogin(client *agent.Client, parsed cmdparse.Login) error {
	// ── Device path: RFC 8628 headless code flow ─────────────────────────────
	if parsed.Device {
		return runDeviceLogin(client)
	}

	// ── Silent path: auth token configured — skip browser entirely ────────────
	authCfg, _ := config.LoadAuthConfig()
	if authCfg.AuthToken != "" {
		resp, err := client.ExchangeAuthToken(authCfg.AuthToken)
		if err == nil {
			if saveErr := config.SaveAuth(config.AuthState{AccessToken: resp.AccessToken}); saveErr != nil {
				return fmt.Errorf("save credentials: %w", saveErr)
			}
			telemetry.LogInfo("Authenticated via auth token.")
			return nil
		}
		// Exchange failed (revoked or expired) — fall through to browser login
		telemetry.LogInfo("Auth token invalid or revoked — falling back to browser login…")
	}

	// ── PKCE browser flow ─────────────────────────────────────────────────────

	// 1. PKCE parameters.
	verifier, err := auth.GenerateVerifier()
	if err != nil {
		return fmt.Errorf("generate pkce verifier: %w", err)
	}
	challenge := auth.ComputeChallenge(verifier)

	expectedState, err := auth.GenerateState()
	if err != nil {
		return fmt.Errorf("generate state: %w", err)
	}

	// 2. Start callback server on the resolved port.
	cbSrv, err := callback.Start(parsed.CallbackPort, expectedState)
	if err != nil {
		return fmt.Errorf("start callback server: %w", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = cbSrv.Shutdown(ctx)
	}()

	// 3. Init PKCE login intent on the control plane.
	init, err := client.InitLogin(challenge, expectedState, cbSrv.URL)
	if err != nil {
		return fmt.Errorf("init login: %w", err)
	}

	// 4. Guide user to the browser.
	telemetry.LogInfo(fmt.Sprintf("\nOpen this URL to authenticate:\n%s\n", init.LoginURL))
	if browser.Open(init.LoginURL) {
		telemetry.LogInfo("Browser opened automatically.")
	} else {
		telemetry.LogInfo("Could not open browser — open the URL above manually.")
	}
	telemetry.LogInfo("Waiting for browser callback...")

	// 5. Wait for the redirect (state validated inside callback.Server).
	result, err := cbSrv.Wait(600 * time.Second)
	if err != nil {
		return fmt.Errorf("browser callback: %w", err)
	}

	// 6. Exchange code for tokens.
	tokens, err := client.ExchangeCliToken(result.Code, verifier, cbSrv.URL)
	if err != nil {
		return fmt.Errorf("token exchange: %w", err)
	}

	// 7. Persist access token.
	if err := config.SaveAuth(config.AuthState{AccessToken: tokens.AccessToken}); err != nil {
		return fmt.Errorf("save auth: %w", err)
	}

	telemetry.LogInfo("Logged in successfully.")
	return nil
}

// runDeviceLogin implements the RFC 8628 OAuth 2.0 Device Authorization Grant.
// Used for headless systems where automatic browser launch is impossible.
func runDeviceLogin(client *agent.Client) error {
	deviceCodeResp, err := client.RequestDeviceCode()
	if err != nil {
		return fmt.Errorf("request device code: %w", err)
	}

	telemetry.LogInfo("\nTo authenticate FastTunnel CLI on this device:")
	telemetry.LogInfo(fmt.Sprintf("  1. Visit:      %s", deviceCodeResp.VerificationURI))
	telemetry.LogInfo(fmt.Sprintf("  2. Enter code: %s\n", deviceCodeResp.UserCode))
	telemetry.LogInfo(fmt.Sprintf("Direct link:\n  %s\n", deviceCodeResp.VerificationURIComplete))
	telemetry.LogInfo("Waiting for authorization (press Ctrl+C to cancel)...")

	interval := time.Duration(deviceCodeResp.Interval) * time.Second
	if interval <= 0 {
		interval = 5 * time.Second
	}
	expiresIn := time.Duration(deviceCodeResp.ExpiresIn) * time.Second
	if expiresIn <= 0 {
		expiresIn = 15 * time.Minute
	}
	deadline := time.Now().Add(expiresIn)

	networkFailures := 0
	for {
		time.Sleep(interval)
		if time.Now().After(deadline) {
			return fmt.Errorf("device login timed out; please try again")
		}

		tokens, err := client.PollDeviceToken(deviceCodeResp.DeviceCode)
		if err == nil {
			if saveErr := config.SaveAuth(config.AuthState{AccessToken: tokens.AccessToken}); saveErr != nil {
				return fmt.Errorf("save auth credentials: %w", saveErr)
			}
			telemetry.LogInfo("\n✓ Successfully authenticated! Credentials saved.")
			return nil
		}

		var apiErr *telemetry.APIError
		if errors.As(err, &apiErr) {
			networkFailures = 0
			switch apiErr.Code {
			case "authorization_pending":
				continue
			case "slow_down":
				interval += 5 * time.Second
				continue
			case "expired_token":
				return fmt.Errorf("device code expired; please run `fasttunnel login --device` again")
			case "access_denied":
				return fmt.Errorf("login request was denied by user")
			}
			if strings.EqualFold(apiErr.Detail, "authorization pending") || strings.EqualFold(apiErr.UserMsg, "authorization pending") {
				continue
			}
		}

		errLower := strings.ToLower(err.Error())
		if strings.Contains(errLower, "authorization_pending") || strings.Contains(errLower, "authorization pending") {
			networkFailures = 0
			continue
		}
		if strings.Contains(errLower, "slow_down") {
			networkFailures = 0
			interval += 5 * time.Second
			continue
		}
		if strings.Contains(errLower, "expired_token") || strings.Contains(errLower, "expired") {
			return fmt.Errorf("device code expired; please run `fasttunnel login --device` again")
		}
		if strings.Contains(errLower, "access_denied") || strings.Contains(errLower, "denied") {
			return fmt.Errorf("login request was denied by user")
		}

		// Handle transient network hiccups (e.g. connection reset, EOF, timeout) during the 15-minute polling window
		if strings.Contains(errLower, "connection reset") || strings.Contains(errLower, "eof") || strings.Contains(errLower, "connection refused") || strings.Contains(errLower, "timeout") {
			networkFailures++
			if networkFailures <= 5 {
				continue
			}
		}

		return fmt.Errorf("device authentication failed: %w", err)
	}
}
