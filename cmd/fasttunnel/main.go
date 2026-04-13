// fasttunnel is the CLI companion for the fasttunnel tunneling platform.
//
// Usage:
//
//	fasttunnel login [--callback-port <port>]
//	fasttunnel http  --port <port> [--subdomain <sub>]
//	fasttunnel https --port <port> [--subdomain <sub>]
package main

import (
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/fasttunnel/fasttunnel/cli/internal/agent"
	"github.com/fasttunnel/fasttunnel/cli/internal/commands"
	"github.com/fasttunnel/fasttunnel/cli/internal/tunnel"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	// ── Compose dependencies ──────────────────────────────────────────────────
	client := agent.NewClient()
	svc := tunnel.New(client)

	// ── Dispatch ──────────────────────────────────────────────────────────────
	switch cmd := strings.ToLower(os.Args[1]); cmd {
	case "login":
		if err := commands.RunLogin(client, os.Args[2:]); err != nil {
			log.Fatal(err)
		}
	case "http", "https":
		if err := commands.RunHTTP(svc, cmd, os.Args[2:]); err != nil {
			log.Fatal(err)
		}
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println("usage:")
	fmt.Println("  fasttunnel login [--callback-port 43001]")
	fmt.Println("  fasttunnel http  --port 8080 [--subdomain my-app]")
	fmt.Println("  fasttunnel https --port 8443 [--subdomain my-app]")
}
