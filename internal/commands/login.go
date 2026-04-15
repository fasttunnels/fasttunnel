// Package commands contains the runnable subcommand implementations.
// Each exported Run* function maps 1-to-1 to a CLI subcommand.
// Dependencies are injected via parameters; no global state.
package commands

import (
	"context"
	"fmt"
	"time"

	"github.com/fasttunnels/fasttunnel/internal/agent"
	"github.com/fasttunnels/fasttunnel/internal/auth"
	"github.com/fasttunnels/fasttunnel/internal/browser"
	"github.com/fasttunnels/fasttunnel/internal/callback"
	"github.com/fasttunnels/fasttunnel/internal/cmdparse"
	"github.com/fasttunnels/fasttunnel/internal/config"
)

// RunLogin handles the login subcommand.
//
// It runs the OAuth 2.0 Authorization Code + PKCE flow:
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
	fmt.Printf("\nOpen this URL to authenticate:\n%s\n\n", init.LoginURL)
	if browser.Open(init.LoginURL) {
		fmt.Println("Browser opened automatically.")
	} else {
		fmt.Println("Could not open browser — open the URL above manually.")
	}
	fmt.Println("Waiting for browser callback...")

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

	fmt.Println("Logged in successfully.")
	return nil
}
