package commands

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/fasttunnels/fasttunnel/cli/internal/agent"
	"github.com/fasttunnels/fasttunnel/cli/internal/cmdparse"
	"github.com/fasttunnels/fasttunnel/cli/internal/config"
	"github.com/fasttunnels/fasttunnel/cli/internal/tunnel"
)

// RunHTTP handles the http and https subcommands.
//
// args is pre-parsed by cmdparse — no flag handling here.
func RunHTTP(svc *tunnel.Service, parsed cmdparse.Tunnel) error {
	authState, err := config.LoadAuth()
	if err != nil {
		return fmt.Errorf("not logged in (%w)\nRun: fasttunnel login", err)
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
			log.Printf("cleanup tunnel %s: %v", lease.TunnelID, err)
		}
	}()

	fmt.Printf("\nfasttunnel %s tunnel active\n", parsed.Protocol)
	fmt.Printf("  public url  : %s\n", lease.PublicURL)
	fmt.Printf("  local target: http://localhost:%d\n", parsed.Port)
	fmt.Printf("  subdomain   : %s\n", lease.Subdomain)
	fmt.Println("\nForwarding requests — press Ctrl+C to stop.")

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := agent.RunAgentLoop(ctx, lease.EdgeURL, lease.SessionToken, parsed.Port); err != nil && err != context.Canceled {
		log.Printf("agent loop exited: %v", err)
	}
	fmt.Println("\nTunnel closed.")
	return nil
}
