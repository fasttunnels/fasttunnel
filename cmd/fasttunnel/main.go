// fasttunnel is the CLI companion for the fasttunnel tunneling platform.
//
// All argument / flag parsing is handled by the cmdparse package.
// This file is responsible only for wiring dependencies and dispatching.
package main

import (
	"fmt"
	"log"
	"os"

	"github.com/fasttunnel/fasttunnel/cli/internal/agent"
	"github.com/fasttunnel/fasttunnel/cli/internal/cmdparse"
	"github.com/fasttunnel/fasttunnel/cli/internal/commands"
	"github.com/fasttunnel/fasttunnel/cli/internal/tunnel"
)

func main() {
	parsed, err := cmdparse.Parse(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	// ── Compose dependencies ──────────────────────────────────────────────────
	client := agent.NewClient()
	svc := tunnel.New(client)

	// ── Dispatch ──────────────────────────────────────────────────────────────
	switch parsed.Name {
	case cmdparse.CmdLogin:
		if err := commands.RunLogin(client, parsed.Login); err != nil {
			log.Fatal(err)
		}
	case cmdparse.CmdHTTP, cmdparse.CmdHTTPS:
		if err := commands.RunHTTP(svc, parsed.Tunnel); err != nil {
			log.Fatal(err)
		}
	default:
		fmt.Fprintf(os.Stderr, "unhandled command %q\n", parsed.Name)
		os.Exit(1)
	}
}
