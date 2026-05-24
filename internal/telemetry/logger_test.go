package telemetry

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
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

func TestSetModeWithVersionTreatsLegacyReleaseAsProd(t *testing.T) {
	t.Setenv("FASTTUNNEL_LOG_MODE", "")
	t.Setenv("FASTTUNNEL_MODE", "")

	SetModeWithVersion("dev", "v0.1.3")
	if IsDev() {
		t.Fatal("expected legacy release build to resolve as prod when version is non-dev")
	}

	SetModeWithVersion("dev", "dev")
	if !IsDev() {
		t.Fatal("expected local dev build to remain dev")
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
	t.Setenv("FASTTUNNEL_LOG_DIR", t.TempDir())
	t.Setenv("FASTTUNNEL_LOG_MODE", "")
	t.Setenv("FASTTUNNEL_MODE", "")
	defer Shutdown()
	SetMode("prod")

	out := captureStdout(t, func() {
		LogForwardStart("tunnel", "GET", "/", "", "localhost:3000")
	})

	if strings.TrimSpace(out) != "" {
		t.Fatalf("expected no forward-start output in prod, got %q", out)
	}
}

func TestLogResponseUsesNgrokStyleInProd(t *testing.T) {
	t.Setenv("FASTTUNNEL_LOG_DIR", t.TempDir())
	t.Setenv("FASTTUNNEL_LOG_MODE", "")
	t.Setenv("FASTTUNNEL_MODE", "")
	defer Shutdown()
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
	if !strings.Contains(out, "304") {
		t.Fatalf("expected status code in prod line, got %q", out)
	}
	if !strings.Contains(out, "Not Modified") {
		t.Fatalf("expected status text in prod line, got %q", out)
	}
	if !strings.Contains(out, "\x1b[") {
		t.Fatalf("expected ANSI colors in prod line, got %q", out)
	}
}

func TestForwardErrorGoesToFileInProd(t *testing.T) {
	logDir := t.TempDir()
	t.Setenv("FASTTUNNEL_LOG_DIR", logDir)
	t.Setenv("FASTTUNNEL_LOG_MODE", "")
	t.Setenv("FASTTUNNEL_MODE", "")
	defer Shutdown()
	SetMode("prod")

	stdout := strings.TrimSpace(captureStdout(t, func() {
		LogForwardError("tunnel", "GET", "/broken", "", "localhost:3000", "dial failed")
	}))
	if stdout != "" {
		t.Fatalf("expected no stdout in prod for forward errors, got %q", stdout)
	}

	Shutdown()

	entries, err := os.ReadDir(logDir)
	if err != nil {
		t.Fatalf("read log dir: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("expected at least one log file")
	}

	payload, err := os.ReadFile(filepath.Join(logDir, entries[0].Name()))
	if err != nil {
		t.Fatalf("read log file: %v", err)
	}
	if !bytes.Contains(payload, []byte(`"category":"forward_error"`)) {
		t.Fatalf("expected forward_error category in file, got %q", string(payload))
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

func TestLogAPIErrorShowsActionHintInProd(t *testing.T) {
	t.Setenv("FASTTUNNEL_LOG_DIR", t.TempDir())
	t.Setenv("FASTTUNNEL_LOG_MODE", "")
	t.Setenv("FASTTUNNEL_MODE", "")
	defer Shutdown()
	SetMode("prod")

	out := captureStdout(t, func() {
		LogAPIError(&APIError{UserMsg: "Session expired.", ActionHint: "Run: fasttunnel login", Endpoint: "/api/v1/tunnels", Code: "INVALID_TOKEN", StatusCode: 401})
	})

	if !strings.Contains(out, "Session expired.") {
		t.Fatalf("expected primary user message in prod output, got %q", out)
	}
	if !strings.Contains(out, "Run: fasttunnel login") {
		t.Fatalf("expected action hint in prod output, got %q", out)
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
