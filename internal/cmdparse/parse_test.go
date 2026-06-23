package cmdparse

import (
	"testing"
	"time"
)

func TestParseTunnelUIEnabledByDefault(t *testing.T) {
	parsed, err := Parse([]string{"http", "3000"})
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if parsed.Name != CmdHTTP {
		t.Fatalf("Name = %q, want %q", parsed.Name, CmdHTTP)
	}
	if !parsed.Tunnel.UIEnabled {
		t.Fatalf("UIEnabled = false, want true")
	}
}

func TestParseTunnelNoUIFlag(t *testing.T) {
	parsed, err := Parse([]string{"https", "3000", "--no-ui"})
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if parsed.Name != CmdHTTPS {
		t.Fatalf("Name = %q, want %q", parsed.Name, CmdHTTPS)
	}
	if parsed.Tunnel.UIEnabled {
		t.Fatalf("UIEnabled = true, want false")
	}
}

func TestParseTunnelNoUIOverridesUI(t *testing.T) {
	parsed, err := Parse([]string{"http", "--ui", "--no-ui", "-p", "3000"})
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if parsed.Tunnel.UIEnabled {
		t.Fatalf("UIEnabled = true, want false when --no-ui is set")
	}
}

func TestParseTunnelProtocolPathSupportsUIFlags(t *testing.T) {
	parsed, err := Parse([]string{"--protocol", "http", "--port", "3000", "--ui"})
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if parsed.Name != CmdHTTP {
		t.Fatalf("Name = %q, want %q", parsed.Name, CmdHTTP)
	}
	if !parsed.Tunnel.UIEnabled {
		t.Fatalf("UIEnabled = false, want true")
	}
}

func TestParseTunnelDiagnosticsFlags(t *testing.T) {
	parsed, err := Parse([]string{
		"http",
		"3000",
		"--memstats",
		"--memstats-interval", "5s",
		"--pprof-addr", "127.0.0.1:6060",
		"--cpu-profile", "/tmp/fasttunnel.cpu.pprof",
		"--heap-profile", "/tmp/fasttunnel.heap.pprof",
	})
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	if !parsed.Tunnel.MemoryStatsEnabled {
		t.Fatalf("MemoryStatsEnabled = false, want true")
	}
	if parsed.Tunnel.MemoryStatsInterval != 5*time.Second {
		t.Fatalf("MemoryStatsInterval = %s, want 5s", parsed.Tunnel.MemoryStatsInterval)
	}
	if parsed.Tunnel.PprofAddr != "127.0.0.1:6060" {
		t.Fatalf("PprofAddr = %q, want 127.0.0.1:6060", parsed.Tunnel.PprofAddr)
	}
	if parsed.Tunnel.CPUProfilePath != "/tmp/fasttunnel.cpu.pprof" {
		t.Fatalf("CPUProfilePath = %q, want /tmp/fasttunnel.cpu.pprof", parsed.Tunnel.CPUProfilePath)
	}
	if parsed.Tunnel.HeapProfilePath != "/tmp/fasttunnel.heap.pprof" {
		t.Fatalf("HeapProfilePath = %q, want /tmp/fasttunnel.heap.pprof", parsed.Tunnel.HeapProfilePath)
	}
}
