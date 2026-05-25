package commands

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/fasttunnels/fasttunnel/internal/agent"
	"github.com/fasttunnels/fasttunnel/internal/cmdparse"
	"github.com/fasttunnels/fasttunnel/internal/config"
	"github.com/fasttunnels/fasttunnel/internal/dashboard"
	"github.com/fasttunnels/fasttunnel/internal/telemetry"
	"github.com/fasttunnels/fasttunnel/internal/tunnel"
)

// RunHTTP handles the http and https subcommands.
//
// args is pre-parsed by cmdparse — no flag handling here.
func RunHTTP(svc *tunnel.Service, parsed cmdparse.Tunnel) error {
	authState, err := config.LoadAuth()
	if err != nil {
		return fmt.Errorf("not logged in...\n\nRun: fasttunnel login")
	}

	lease, err := svc.CreateAndRegister(parsed.Subdomain, parsed.Protocol, parsed.Port, authState.AccessToken)
	if err != nil {
		return err
	}

	// Graceful cleanup: delete the tunnel (and cascade-disconnect the session)
	// when the CLI exits normally. For hard kills (SIGKILL) the edge handles
	// session cleanup independently via NotifyDisconnect.
	defer func() {
		if err := svc.Cleanup(lease.TunnelID, authState.AccessToken); err != nil {
			// Silently ignore errors on cleanup
			// TODO: Clean up with edge secret if client error (e.g. invalid token) since auth state may be stale
			telemetry.SilentLogProdError(err)
		}
	}()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	stdoutInfo, stdoutErr := os.Stdout.Stat()
	stdinInfo, stdinErr := os.Stdin.Stat()
	useDashboard := parsed.UIEnabled && stdoutErr == nil && stdinErr == nil &&
		(stdoutInfo.Mode()&os.ModeCharDevice) != 0 &&
		(stdinInfo.Mode()&os.ModeCharDevice) != 0
	if useDashboard {
		telemetry.SetTerminalOutputMuted(true)
		defer telemetry.SetTerminalOutputMuted(false)

		ui := dashboard.NewController(dashboard.SessionInfo{
			Protocol:    parsed.Protocol,
			PublicURL:   lease.PublicURL,
			LocalTarget: fmt.Sprintf("http://localhost:%d", parsed.Port),
			Subdomain:   lease.Subdomain,
			MaxRows:     25,
		})

		errCh := make(chan error, 1)
		go func() {
			errCh <- agent.RunAgentLoop(ctx, lease.EdgeURL, lease.SessionToken, parsed.Port, agent.RunOptions{
				Observer: ui.Observer(),
			})
		}()

		if err := ui.Run(ctx, stop); err != nil {
			telemetry.SilentLogProdError(err)
		}

		err = <-errCh
		if err != nil && err != context.Canceled {
			telemetry.SilentLogProdError(err)
		}
		return nil
	}

	telemetry.LogInfo(fmt.Sprintf("\nfasttunnel %s tunnel active", parsed.Protocol))
	telemetry.LogInfo(fmt.Sprintf("  public url  : %s", lease.PublicURL))
	telemetry.LogInfo(fmt.Sprintf("  local target: http://localhost:%d", parsed.Port))
	telemetry.LogInfo(fmt.Sprintf("  subdomain   : %s", lease.Subdomain))
	telemetry.LogInfo("\nForwarding requests — press Ctrl+C to stop.")

	if err := agent.RunAgentLoop(ctx, lease.EdgeURL, lease.SessionToken, parsed.Port, agent.RunOptions{}); err != nil && err != context.Canceled {
		// Agent loop errors are already logged by telemetry in agent/tunnel.go
		telemetry.SilentLogProdError(err)
	}
	telemetry.LogInfo("\nTunnel closed.")
	return nil
}
