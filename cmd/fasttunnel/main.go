// fasttunnel is the CLI companion for the fasttunnel tunneling platform.
//
// All argument / flag parsing is handled by the cmdparse package.
// This file is responsible only for wiring dependencies and dispatching.
package main

import (
	"fmt"
	"log"
	"os"

	"github.com/fasttunnels/fasttunnel/cli/internal/agent"
	"github.com/fasttunnels/fasttunnel/cli/internal/cmdparse"
	"github.com/fasttunnels/fasttunnel/cli/internal/commands"
	"github.com/fasttunnels/fasttunnel/cli/internal/tunnel"
)

// Build-time version info — injected by GoReleaser via ldflags:
//
//	-X main.version=v1.2.3
//	-X main.commit=abc1234
//	-X main.buildDate=2026-04-14
//
// A plain `go build` without ldflags leaves these as "dev".
var (
	version   = "dev"
	commit    = "none"
	buildDate = "unknown"
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
	case cmdparse.CmdVersion:
		commands.RunVersion(version, commit, buildDate)
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
