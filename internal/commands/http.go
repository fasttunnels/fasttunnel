package commands

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/fasttunnel/fasttunnel/cli/internal/agent"
	"github.com/fasttunnel/fasttunnel/cli/internal/config"
	"github.com/fasttunnel/fasttunnel/cli/internal/tunnel"
)

// RunHTTP handles the `http` and `https` subcommands.
//
// It resolves the active auth state, calls the tunnel service to create and
// register a new tunnel, prints the public URL, then runs the WebSocket agent
// loop until the user presses Ctrl+C or a system signal is received.
func RunHTTP(svc *tunnel.Service, protocol string, args []string) error {
	fs := flag.NewFlagSet(protocol, flag.ExitOnError)
	localPort := fs.Int("port", 8080, "local port to forward")
	subdomain := fs.String("subdomain", "", "optional vanity subdomain (random if omitted)")
	_ = fs.Parse(args)

	authState, err := config.LoadAuth()
	if err != nil {
		return fmt.Errorf("not logged in (%w)\nRun: fasttunnel login", err)
	}

	lease, err := svc.CreateAndRegister(*subdomain, protocol, *localPort, authState.AccessToken)
	if err != nil {
		return err
	}

	// Graceful cleanup: delete the tunnel (and cascade-disconnect the session)
	// when the CLI exits normally.  For hard kills (SIGKILL) the edge handles
	// session cleanup independently via NotifyDisconnect.
	defer func() {
		if err := svc.Cleanup(lease.TunnelID, authState.AccessToken); err != nil {
			log.Printf("cleanup tunnel %s: %v", lease.TunnelID, err)
		}
	}()

	fmt.Printf("\nfasttunnel %s tunnel active\n", protocol)
	fmt.Printf("  public url  : %s\n", lease.PublicURL)
	fmt.Printf("  local target: http://localhost:%d\n", *localPort)
	fmt.Printf("  subdomain   : %s\n", lease.Subdomain)
	fmt.Println("\nForwarding requests — press Ctrl+C to stop.")

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := agent.RunAgentLoop(ctx, lease.EdgeURL, lease.SessionToken, *localPort); err != nil && err != context.Canceled {
		log.Printf("agent loop exited: %v", err)
	}
	fmt.Println("\nTunnel closed.")
	return nil
}
