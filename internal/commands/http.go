package commands

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/fasttunnels/fasttunnel/internal/agent"
	"github.com/fasttunnels/fasttunnel/internal/cmdparse"
	"github.com/fasttunnels/fasttunnel/internal/config"
	"github.com/fasttunnels/fasttunnel/internal/dashboard"
	"github.com/fasttunnels/fasttunnel/internal/diagnostics"
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

	diagLines := tunnelDiagnosticsSummary(parsed)

	var observer agent.EventObserver
	if useDashboard {
		ui := dashboard.NewController(dashboard.SessionInfo{
			Protocol:    parsed.Protocol,
			PublicURL:   lease.PublicURL,
			LocalTarget: fmt.Sprintf("http://localhost:%d", parsed.Port),
			Subdomain:   lease.Subdomain,
			MaxRows:     25,
			Diagnostics: diagLines,
		})
		observer = ui.Observer()

		telemetry.SetTerminalOutputMuted(true)
		defer telemetry.SetTerminalOutputMuted(false)

		diagSession, err := startTunnelDiagnostics(ctx, parsed, observer)
		if err != nil {
			return err
		}
		defer closeDiagnostics(diagSession)

		errCh := make(chan error, 1)
		go func() {
			errCh <- agent.RunAgentLoop(ctx, lease.EdgeURL, lease.SessionToken, parsed.Port, agent.RunOptions{
				Observer: observer,
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

	logTunnelBanner(parsed, lease, diagLines)

	diagSession, err := startTunnelDiagnostics(ctx, parsed, observer)
	if err != nil {
		return err
	}
	defer closeDiagnostics(diagSession)

	if err := agent.RunAgentLoop(ctx, lease.EdgeURL, lease.SessionToken, parsed.Port, agent.RunOptions{}); err != nil && err != context.Canceled {
		// Agent loop errors are already logged by telemetry in agent/tunnel.go
		telemetry.SilentLogProdError(err)
	}
	telemetry.LogInfo("\nTunnel closed.")
	return nil
}

func startTunnelDiagnostics(ctx context.Context, parsed cmdparse.Tunnel, observer agent.EventObserver) (*diagnostics.Session, error) {
	return diagnostics.Start(ctx, diagnostics.Config{
		MemoryStatsEnabled:  parsed.MemoryStatsEnabled,
		MemoryStatsInterval: parsed.MemoryStatsInterval,
		PprofAddr:           parsed.PprofAddr,
		CPUProfilePath:      parsed.CPUProfilePath,
		HeapProfilePath:     parsed.HeapProfilePath,
	}, func(snapshot diagnostics.MemorySnapshot) {
		telemetry.LogInfo(fmt.Sprintf(
			"memory footprint: alloc=%s heap=%s sys=%s goroutines=%d gc=%d",
			diagnostics.FormatBytes(snapshot.AllocBytes),
			diagnostics.FormatBytes(snapshot.HeapBytes),
			diagnostics.FormatBytes(snapshot.SysBytes),
			snapshot.Goroutines,
			snapshot.NumGC,
		))
		if observer != nil {
			observer(agent.RuntimeEvent{
				Type:       agent.RuntimeEventMemorySnapshot,
				Time:       snapshot.Time,
				AllocBytes: snapshot.AllocBytes,
				HeapBytes:  snapshot.HeapBytes,
				SysBytes:   snapshot.SysBytes,
				NumGC:      snapshot.NumGC,
				Goroutines: snapshot.Goroutines,
			})
		}
	})
}

func closeDiagnostics(session *diagnostics.Session) {
	if session == nil {
		return
	}
	if err := session.Close(); err != nil {
		telemetry.SilentLogProdError(err)
	}
}

func logTunnelBanner(parsed cmdparse.Tunnel, lease tunnel.Lease, diagLines []string) {
	telemetry.LogInfo(fmt.Sprintf("\nfasttunnel %s tunnel active", parsed.Protocol))
	telemetry.LogInfo(fmt.Sprintf("  public url  : %s", lease.PublicURL))
	telemetry.LogInfo(fmt.Sprintf("  local target: http://localhost:%d", parsed.Port))
	telemetry.LogInfo(fmt.Sprintf("  subdomain   : %s", lease.Subdomain))
	for _, line := range diagLines {
		telemetry.LogInfo("  " + line)
	}
	telemetry.LogInfo("\nForwarding requests — press Ctrl+C to stop.")
}

func tunnelDiagnosticsSummary(parsed cmdparse.Tunnel) []string {
	lines := make([]string, 0, 3)

	if parsed.MemoryStatsEnabled {
		interval := parsed.MemoryStatsInterval
		if interval <= 0 {
			interval = 15 * time.Second
		}
		lines = append(lines, fmt.Sprintf("memory      : every %s", interval))
	}
	if parsed.PprofAddr != "" {
		lines = append(lines, fmt.Sprintf("pprof       : %s", formatPprofURL(parsed.PprofAddr)))
	}
	if parsed.CPUProfilePath != "" || parsed.HeapProfilePath != "" {
		parts := make([]string, 0, 2)
		if parsed.CPUProfilePath != "" {
			parts = append(parts, "cpu="+parsed.CPUProfilePath)
		}
		if parsed.HeapProfilePath != "" {
			parts = append(parts, "heap="+parsed.HeapProfilePath)
		}
		lines = append(lines, "profiles    : "+strings.Join(parts, " "))
	}

	return lines
}

func formatPprofURL(addr string) string {
	host := addr
	if strings.HasPrefix(host, ":") {
		host = "127.0.0.1" + host
	}

	resolvedHost := host
	if tcpAddr, err := net.ResolveTCPAddr("tcp", host); err == nil {
		switch {
		case tcpAddr.IP == nil || tcpAddr.IP.IsUnspecified():
			resolvedHost = "127.0.0.1:" + strconv.Itoa(tcpAddr.Port)
		default:
			resolvedHost = net.JoinHostPort(tcpAddr.IP.String(), strconv.Itoa(tcpAddr.Port))
		}
	}

	return "http://" + resolvedHost + "/debug/pprof/"
}
