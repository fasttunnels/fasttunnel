package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/fasttunnel/fasttunnel/cli/internal/agent"
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


	fmt.Println("Login successful!")
}

// callbackResult holds the code and state received from the CLI callback server.
type callbackResult struct {
	Code  string
	State string
}


func handleTunnel(client *agent.Client, protocol string, args []string) {
	tunnelFlags := flag.NewFlagSet(protocol, flag.ExitOnError)
	localPort := tunnelFlags.Int("port", 8080, "local port to forward")
	subdomain := tunnelFlags.String("subdomain", "", "optional vanity subdomain (random if omitted)")
	_ = tunnelFlags.Parse(args)


	fmt.Printf("\nfasttunnel %s tunnel active\n", protocol)

}
