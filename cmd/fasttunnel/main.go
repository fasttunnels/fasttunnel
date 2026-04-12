package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/fasttunnel/fasttunnel/cli/internal/agent"
	"github.com/fasttunnel/fasttunnel/cli/internal/auth"
	"github.com/fasttunnel/fasttunnel/cli/internal/config"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	cmd := strings.ToLower(os.Args[1])
	client := agent.NewClient()

	switch cmd {
	case "login":
		handleLogin(client, os.Args[2:])
	case "http", "https":
		handleTunnel(client, cmd, os.Args[2:])
	default:
		printUsage()
		os.Exit(1)
	}
}


func printUsage() {
	fmt.Println("usage:")
	fmt.Println("  fasttunnel login [--callback-port 43001]")
	fmt.Println("  fasttunnel http --port 8080 [--subdomain my-app]")
	fmt.Println("  fasttunnel https --port 8443 [--subdomain my-app]")
}

// handleLogin runs the OAuth 2.0 Authorization Code + PKCE login flow.
//
// Steps:
//  1. Generate code_verifier, code_challenge (S256), and anti-CSRF state.
//  2. Start a temporary local HTTP server to receive the callback.
//  3. Call /auth/cli/init → get request_id + login_url.
//  4. Open the login_url in the browser.
//  5. Wait for the callback: /callback?code=...&state=...
//  6. Validate state, exchange code for tokens via /auth/cli/token.
//  7. Save the access_token to disk.
func handleLogin(client *agent.Client, args []string) {
	loginFlags := flag.NewFlagSet("login", flag.ExitOnError)
	callbackPort := loginFlags.Int("callback-port", 0, "optional local callback port override")
	_ = loginFlags.Parse(args)

	// 1. Generate PKCE parameters
	verifier, err := auth.GenerateVerifier()
	if err != nil {
		log.Fatalf("failed to generate PKCE verifier: %v", err)
	}
	challenge := auth.ComputeChallenge(verifier)

	expectedState, err := auth.GenerateState()
	if err != nil {
		log.Fatalf("failed to generate state: %v", err)
	}

	// 2. Start local callback server
	callbackURL, waitForCallback, shutdown, err := startCallbackServer(*callbackPort, expectedState)
	if err != nil {
		log.Fatalf("failed to start local callback server: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = shutdown(ctx)
	}()

	// 3. Init PKCE login intent
	init, err := client.InitLogin(challenge, expectedState, callbackURL)
	if err != nil {
		log.Fatalf("failed to initiate login: %v", err)
	}

	// 4. Guide user to browser
	fmt.Printf("\nOpen this URL to authenticate:\n%s\n\n", init.LoginURL)
	if openBrowser(init.LoginURL) {
		fmt.Println("Browser opened automatically.")
	} else {
		fmt.Println("Could not open browser — open the URL above manually.")
	}
	fmt.Println("Waiting for browser callback...")

	// 5. Wait for callback (code + state)
	callbackResult, err := waitForCallback(600 * time.Second)
	if err != nil {
		log.Fatalf("login callback failed: %v", err)
	}

	// 6. Exchange code for tokens (PKCE-verified server-side)
	tokens, err := client.ExchangeCliToken(callbackResult.Code, verifier, callbackURL)
	if err != nil {
		log.Fatalf("token exchange failed: %v", err)
	}

	// this ignores the refresh token since the CLI currently only uses the access token,
	// but in a real implementation you'd want to save both and use the refresh token to get new access tokens when they expire.

	// 7. Save access token
	if err := config.SaveAuth(config.AuthState{AccessToken: tokens.AccessToken}); err != nil {
		log.Fatalf("failed to save auth state: %v", err)
	}

	fmt.Println("Login successful!")
}

// callbackResult holds the code and state received from the CLI callback server.
type callbackResult struct {
	Code  string
	State string
}

// startCallbackServer starts a temporary HTTP server on localhost, validates the
// incoming state against expectedState, and signals the code through a channel.
func startCallbackServer(port int, expectedState string) (
	string,
	func(time.Duration) (callbackResult, error),
	func(context.Context) error,
	error,
) {
	addr := "127.0.0.1:0"
	if port > 0 {
		addr = fmt.Sprintf("127.0.0.1:%d", port)
	}

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return "", nil, nil, fmt.Errorf("failed to bind callback port: %w", err)
	}

	results := make(chan callbackResult, 1)

	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		code := r.URL.Query().Get("code")
		state := r.URL.Query().Get("state")

		if state != expectedState {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte("Invalid state — possible CSRF attack. Close this window."))
			return
		}
		if code == "" {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte("Missing authorization code."))
			return
		}

		select {
		case results <- callbackResult{Code: code, State: state}:
		default:
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(
			`<html><head><title>fasttunnel login</title></head>` +
				`<body style="font-family:system-ui;padding:40px;text-align:center">` +
				`<h2>✓ Login successful</h2>` +
				`<p>Your fasttunnel CLI is now authenticated.</p>` +
				`<p>You can close this tab and return to your terminal.</p>` +
				`</body></html>`,
		))
	})

	server := &http.Server{Handler: mux}
	go func() { _ = server.Serve(listener) }()

	callbackURL := fmt.Sprintf("http://%s/callback", listener.Addr().String())

	wait := func(timeout time.Duration) (callbackResult, error) {
		select {
		case result := <-results:
			return result, nil
		case <-time.After(timeout):
			return callbackResult{}, fmt.Errorf("timed out waiting for browser callback")
		}
	}

	return callbackURL, wait, server.Shutdown, nil
}

func openBrowser(url string) bool {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		return false
	}
	if err := cmd.Start(); err != nil {
		return false
	}
	return true
}

func handleTunnel(client *agent.Client, protocol string, args []string) {
	tunnelFlags := flag.NewFlagSet(protocol, flag.ExitOnError)
	localPort := tunnelFlags.Int("port", 8080, "local port to forward")
	subdomain := tunnelFlags.String("subdomain", "", "optional vanity subdomain (random if omitted)")
	_ = tunnelFlags.Parse(args)

	authState, err := config.LoadAuth()
	if err != nil || authState.AccessToken == "" {
		log.Fatal("login required. Run: fasttunnel login")
	}

	tun, err := client.CreateTunnel(*subdomain, *localPort, protocol, authState.AccessToken)
	if err != nil {
		log.Fatalf("failed to create tunnel: %v", err)
	}

	lease, err := client.CreateLease(tun.TunnelID, protocol, authState.AccessToken)
	if err != nil {
		log.Fatalf("failed to create session lease: %v", err)
	}

	if err := client.RegisterEdge(lease, tun.TunnelID, tun.Subdomain, protocol); err != nil {
		log.Fatalf("failed to register tunnel with edge: %v", err)
	}

	fmt.Printf("\nfasttunnel %s tunnel active\n", protocol)
	fmt.Printf("  public url  : %s\n", tun.PublicURL)
	fmt.Printf("  local target: http://localhost:%d\n", *localPort)
	fmt.Printf("  subdomain   : %s\n", tun.Subdomain)
	fmt.Println("\nForwarding requests — press Ctrl+C to stop.")

	// Keep the process alive: open persistent WebSocket connection to the edge
	// and forward incoming HTTP request frames to the local app.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := agent.RunAgentLoop(ctx, lease.EdgeURL, lease.SessionToken, *localPort); err != nil && err != context.Canceled {
		log.Printf("agent loop exited: %v", err)
	}
	fmt.Println("\nTunnel closed.")
}
