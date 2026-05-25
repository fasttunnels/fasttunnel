package cmdparse

import "testing"

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
