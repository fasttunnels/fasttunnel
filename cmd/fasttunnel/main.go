// fasttunnel is the CLI companion for the fasttunnel tunneling platform.
//
// All argument / flag parsing is handled by the cmdparse package.
// This file is responsible only for wiring dependencies and dispatching.
package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/fasttunnels/fasttunnel/internal/agent"
	"github.com/fasttunnels/fasttunnel/internal/cmdparse"
	"github.com/fasttunnels/fasttunnel/internal/commands"
	"github.com/fasttunnels/fasttunnel/internal/telemetry"
	"github.com/fasttunnels/fasttunnel/internal/tunnel"
)

// Build-time version info — injected by GoReleaser via ldflags:
//
//	-X main.version=v1.2.3
//	-X main.commit=abc1234
//	-X main.buildDate=2026-04-14
//	-X main.buildChannel=prod
//
// A plain `go build` without ldflags leaves these as "dev".
var (
	version      = "dev"
	commit       = "none"
	buildDate    = "unknown"
	buildChannel = "dev"
)

func main() {
	os.Exit(run())
}

func run() int {
	parsed, err := cmdparse.Parse(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	telemetry.SetModeWithVersion(buildChannel, version)
	defer telemetry.Shutdown()

	client := agent.NewClient(version)
	svc := tunnel.New(client)

	switch parsed.Name {
	case cmdparse.CmdVersion:
		commands.RunVersion(version, commit, buildDate)
	case cmdparse.CmdCompletion:
		if err := commands.RunCompletion(parsed.Completion.Shell); err != nil {
			telemetry.LogError(err.Error(), "")
			return 1
		}
	case cmdparse.CmdLogin:
		if err := commands.RunLogin(client, parsed.Login); err != nil {
			logCommandError(err)
			return 1
		}
	case cmdparse.CmdHTTP, cmdparse.CmdHTTPS:
		if err := commands.RunHTTP(svc, parsed.Tunnel); err != nil {
			logCommandError(err)
			return 1
		}
	default:
		fmt.Fprintf(os.Stderr, "unhandled command %q\n", parsed.Name)
		return 1
	}

	return 0
}

func logCommandError(err error) {
	if err == nil {
		return
	}

	var apiErr *telemetry.APIError
	if errors.As(err, &apiErr) {
		telemetry.LogAPIError(apiErr)
		return
	}

	telemetry.LogError(err.Error(), "")
}
