package telemetry

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	defaultQueueSize     = 512
	defaultRetentionHour = 24
	logFilePrefix        = "fasttunnel-"
	logFileSuffix        = ".jsonl"
)

type fileLogEvent struct {
	Timestamp  string `json:"timestamp"`
	Mode       string `json:"mode"`
	Category   string `json:"category"`
	Domain     string `json:"domain,omitempty"`
	Endpoint   string `json:"endpoint,omitempty"`
	Code       string `json:"code,omitempty"`
	Method     string `json:"method,omitempty"`
	Path       string `json:"path,omitempty"`
	Status     int    `json:"status,omitempty"`
	DurationMS int64  `json:"duration_ms,omitempty"`
	Target     string `json:"target,omitempty"`
	Message    string `json:"message,omitempty"`
	Action     string `json:"action,omitempty"`
	Error      string `json:"error,omitempty"`
}

type asyncFileSink struct {
	file *os.File
	enc  *json.Encoder
	// ch is the in-memory queue for non-blocking event ingestion.
	ch chan fileLogEvent
	// done is closed during shutdown to stop accepting new events and trigger drain.
	done chan struct{}
	// wg tracks the background writer goroutine lifetime.
	wg      sync.WaitGroup
	closed  atomic.Bool
	dropped atomic.Uint64
}

var (
	prodSinkMu         sync.Mutex
	prodSink           *asyncFileSink
	prodSinkInitFailed bool
)

func emitProdFileEvent(ev fileLogEvent) {
	if isDev() {
		return
	}

	s := getOrInitProdSink()
	if s == nil {
		return
	}

	ev.Timestamp = time.Now().UTC().Format(time.RFC3339Nano)
	ev.Mode = "prod"
	s.emit(ev)
}

func getOrInitProdSink() *asyncFileSink {
	prodSinkMu.Lock()
	defer prodSinkMu.Unlock()

	if prodSink != nil {
		return prodSink
	}
	if prodSinkInitFailed {
		return nil
	}

	logPath, err := resolveLogFilePath()
	if err != nil {
		prodSinkInitFailed = true
		fmt.Fprintf(os.Stderr, "fasttunnel: failed to resolve log file path: %v\n", err)
		return nil
	}

	sink, err := newAsyncFileSink(logPath, resolveQueueSize())
	if err != nil {
		prodSinkInitFailed = true
		fmt.Fprintf(os.Stderr, "fasttunnel: failed to initialize file logger: %v\n", err)
		return nil
	}

	prodSink = sink
	return prodSink
}

func resetProdSinkForMode(currentMode string) {
	prodSinkMu.Lock()
	defer prodSinkMu.Unlock()

	if currentMode == "dev" {
		if prodSink != nil {
			prodSink.close(500 * time.Millisecond)
			prodSink = nil
		}
	}

	// A mode reset gives the sink another chance to initialize if the previous
	// attempt failed due to transient filesystem issues.
	prodSinkInitFailed = false
}

func shutdownProdSink() {
	prodSinkMu.Lock()
	sink := prodSink
	prodSink = nil
	prodSinkMu.Unlock()

	if sink != nil {
		sink.close(1200 * time.Millisecond)
	}
}

func newAsyncFileSink(logPath string, queueSize int) (*asyncFileSink, error) {
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		return nil, fmt.Errorf("mkdir log dir: %w", err)
	}

	if err := cleanupOldLogFiles(filepath.Dir(logPath), resolveRetention()); err != nil {
		// Retention cleanup is best-effort; log and continue.
		fmt.Fprintf(os.Stderr, "fasttunnel: log cleanup warning: %v\n", err)
	}

	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open log file: %w", err)
	}

	s := &asyncFileSink{
		file: f,
		enc:  json.NewEncoder(f),
		ch:   make(chan fileLogEvent, queueSize),
		done: make(chan struct{}),
	}
	// Start one background writer. Add(1) means "one goroutine must call Done()"
	// before shutdown can proceed.
	s.wg.Add(1)
	go s.run()
	return s, nil
}

// emit is non-blocking by design: if the queue is full, the event is dropped
// and accounted for (best-effort logging on hot paths).
func (s *asyncFileSink) emit(ev fileLogEvent) {
	if s.closed.Load() {
		return
	}

	select {
	case <-s.done:
		return
	case s.ch <- ev:
	default:
		s.dropped.Add(1)
	}
}

// close performs a single shutdown sequence:
// 1) mark closed once, 2) signal done, 3) wait (bounded) for run() to drain,
// 4) close the file descriptor.
func (s *asyncFileSink) close(timeout time.Duration) {
	if !s.closed.CompareAndSwap(false, true) {
		return
	}

	close(s.done)
	waitDone := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(waitDone)
	}()

	select {
	case <-waitDone:
	case <-time.After(timeout):
	}

	_ = s.file.Close()
}

// run is the background writer goroutine.
// It keeps the file open for the sink lifetime and writes JSONL entries from ch.
// On shutdown, it drains queued events before returning.
func (s *asyncFileSink) run() {
	defer s.wg.Done()

	// dropped is incremented by emit() when ch is full.
	// We flush a compact summary event instead of one line per dropped event.
	flushDropped := func() {
		dropped := s.dropped.Swap(0)
		if dropped == 0 {
			return
		}
		_ = s.enc.Encode(fileLogEvent{
			Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
			Mode:      "prod",
			Category:  "logger_drop_summary",
			Message:   fmt.Sprintf("dropped %d log events due to full queue", dropped),
		})
	}

	for {
		select {
		case <-s.done:
			// Shutdown path: drain everything already queued, then exit.
			for {
				select {
				case ev := <-s.ch:
					_ = s.enc.Encode(ev)
				default:
					flushDropped()
					return
				}
			}
		case ev := <-s.ch:
			// Normal path: write one event and opportunistically flush drop summary.
			_ = s.enc.Encode(ev)
			flushDropped()
		}
	}
}

func resolveQueueSize() int {
	if raw := strings.TrimSpace(os.Getenv("FASTTUNNEL_LOG_QUEUE_SIZE")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			return parsed
		}
	}
	return defaultQueueSize
}

func resolveRetention() time.Duration {
	if raw := strings.TrimSpace(os.Getenv("FASTTUNNEL_LOG_RETENTION_HOURS")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			return time.Duration(parsed) * time.Hour
		}
	}
	return defaultRetentionHour * time.Hour
}

func resolveLogFilePath() (string, error) {
	if path := strings.TrimSpace(os.Getenv("FASTTUNNEL_LOG_FILE")); path != "" {
		return path, nil
	}

	baseDir := strings.TrimSpace(os.Getenv("FASTTUNNEL_LOG_DIR"))
	if baseDir == "" {
		cacheDir, err := os.UserCacheDir()
		if err == nil && cacheDir != "" {
			baseDir = filepath.Join(cacheDir, "fasttunnel", "logs")
		} else {
			home, homeErr := os.UserHomeDir()
			if homeErr != nil {
				return "", homeErr
			}
			baseDir = filepath.Join(home, ".fasttunnel", "logs")
		}
	}

	fileName := fmt.Sprintf("%s%s%s", logFilePrefix, time.Now().Format("20060102"), logFileSuffix)
	return filepath.Join(baseDir, fileName), nil
}

func cleanupOldLogFiles(dir string, retention time.Duration) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	cutoff := time.Now().Add(-retention)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasPrefix(name, logFilePrefix) || !strings.HasSuffix(name, logFileSuffix) {
			continue
		}
		info, infoErr := entry.Info()
		if infoErr != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			_ = os.Remove(filepath.Join(dir, name))
		}
	}

	return nil
}
