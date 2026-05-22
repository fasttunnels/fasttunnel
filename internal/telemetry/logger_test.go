package telemetry

import (
	"io"
	"os"
	"strings"
	"testing"
	"time"
)

func TestSetModeUsesBuildChannelByDefault(t *testing.T) {
	t.Setenv("FASTTUNNEL_LOG_MODE", "")
	t.Setenv("FASTTUNNEL_MODE", "")

	SetMode("dev")
	if !IsDev() {
		t.Fatal("expected dev mode when build channel is dev")
	}

	SetMode("prod")
	if IsDev() {
		t.Fatal("expected prod mode when build channel is prod")
	}

	SetMode("v1.2.3")
	if IsDev() {
		t.Fatal("expected semantic build channel/version to resolve as prod")
	}
}

func TestSetModeHonorsEnvOverridePrecedence(t *testing.T) {
	t.Setenv("FASTTUNNEL_MODE", "dev")
	t.Setenv("FASTTUNNEL_LOG_MODE", "prod")

	SetMode("dev")
	if IsDev() {
		t.Fatal("expected FASTTUNNEL_LOG_MODE to override FASTTUNNEL_MODE")
	}

	t.Setenv("FASTTUNNEL_LOG_MODE", "")
	SetMode("prod")
	if !IsDev() {
		t.Fatal("expected FASTTUNNEL_MODE to override build channel when FASTTUNNEL_LOG_MODE is unset")
	}
}

func TestLogForwardStartSuppressedInProd(t *testing.T) {
	t.Setenv("FASTTUNNEL_LOG_MODE", "")
	t.Setenv("FASTTUNNEL_MODE", "")
	SetMode("prod")

	out := captureStdout(t, func() {
		LogForwardStart("tunnel", "GET", "/", "", "localhost:3000")
	})

	if strings.TrimSpace(out) != "" {
		t.Fatalf("expected no forward-start output in prod, got %q", out)
	}
}

func TestLogResponseUsesNgrokStyleInProd(t *testing.T) {
	t.Setenv("FASTTUNNEL_LOG_MODE", "")
	t.Setenv("FASTTUNNEL_MODE", "")
	SetMode("prod")

	out := strings.TrimSpace(captureStdout(t, func() {
		LogResponse("tunnel", "get", "/favicon.ico", "", 304, 25*time.Millisecond)
	}))

	if !strings.Contains(out, "GET") {
		t.Fatalf("expected uppercase method in prod line, got %q", out)
	}
	if !strings.Contains(out, "/favicon.ico") {
		t.Fatalf("expected path in prod line, got %q", out)
	}
	if !strings.Contains(out, "304 Not Modified") {
		t.Fatalf("expected status and reason in prod line, got %q", out)
	}
}

func TestLogResponseKeepsVerboseFormatInDev(t *testing.T) {
	t.Setenv("FASTTUNNEL_LOG_MODE", "")
	t.Setenv("FASTTUNNEL_MODE", "")
	SetMode("dev")

	out := captureStdout(t, func() {
		LogResponse("tunnel", "GET", "/", "", 200, 10*time.Millisecond)
	})

	if !strings.Contains(out, "[RSP]") {
		t.Fatalf("expected dev response tag, got %q", out)
	}
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}

	os.Stdout = w
	fn()

	if err := w.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}
	os.Stdout = oldStdout

	b, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read captured output: %v", err)
	}
	if err := r.Close(); err != nil {
		t.Fatalf("close reader: %v", err)
	}

	return string(b)
}
