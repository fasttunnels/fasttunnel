package diagnostics

import (
	"context"
	"fmt"
	"net"
	"net/http"
	netpprof "net/http/pprof"
	"os"
	"runtime"
	runtimepprof "runtime/pprof"
	"time"
)

const defaultMemoryInterval = 15 * time.Second

type Config struct {
	MemoryStatsEnabled  bool
	MemoryStatsInterval time.Duration
	PprofAddr           string
	CPUProfilePath      string
	HeapProfilePath     string
}

type MemorySnapshot struct {
	Time       time.Time
	AllocBytes uint64
	HeapBytes  uint64
	SysBytes   uint64
	NumGC      uint32
	Goroutines int
}

type Observer func(MemorySnapshot)

type Session struct {
	cfg        Config
	cancel     context.CancelFunc
	monitorDone chan struct{}
	server     *http.Server
	serverDone chan error
	cpuFile    *os.File
}

func Start(ctx context.Context, cfg Config, observer Observer) (*Session, error) {
	session := &Session{
		cfg:         cfg,
		monitorDone: make(chan struct{}),
	}

	if cfg.CPUProfilePath != "" {
		file, err := os.Create(cfg.CPUProfilePath)
		if err != nil {
			return nil, fmt.Errorf("create CPU profile: %w", err)
		}
		if err := runtimepprof.StartCPUProfile(file); err != nil {
			_ = file.Close()
			return nil, fmt.Errorf("start CPU profile: %w", err)
		}
		session.cpuFile = file
	}

	if cfg.PprofAddr != "" {
		mux := http.NewServeMux()
		mux.HandleFunc("/debug/pprof/", netpprof.Index)
		mux.HandleFunc("/debug/pprof/cmdline", netpprof.Cmdline)
		mux.HandleFunc("/debug/pprof/profile", netpprof.Profile)
		mux.HandleFunc("/debug/pprof/symbol", netpprof.Symbol)
		mux.HandleFunc("/debug/pprof/trace", netpprof.Trace)
		mux.Handle("/debug/pprof/allocs", netpprof.Handler("allocs"))
		mux.Handle("/debug/pprof/block", netpprof.Handler("block"))
		mux.Handle("/debug/pprof/goroutine", netpprof.Handler("goroutine"))
		mux.Handle("/debug/pprof/heap", netpprof.Handler("heap"))
		mux.Handle("/debug/pprof/mutex", netpprof.Handler("mutex"))
		mux.Handle("/debug/pprof/threadcreate", netpprof.Handler("threadcreate"))

		session.server = &http.Server{
			Addr:              cfg.PprofAddr,
			Handler:           mux,
			ReadHeaderTimeout: 5 * time.Second,
		}
		session.serverDone = make(chan error, 1)

		ln, err := net.Listen("tcp", cfg.PprofAddr)
		if err != nil {
			session.closeCPUProfile()
			return nil, fmt.Errorf("listen on %s: %w", cfg.PprofAddr, err)
		}

		go func() {
			err := session.server.Serve(ln)
			if err != nil && err != http.ErrServerClosed {
				session.serverDone <- err
				return
			}
			session.serverDone <- nil
		}()
	}

	if cfg.MemoryStatsEnabled {
		interval := cfg.MemoryStatsInterval
		if interval <= 0 {
			interval = defaultMemoryInterval
		}
		monitorCtx, cancel := context.WithCancel(ctx)
		session.cancel = cancel

		go func() {
			defer close(session.monitorDone)
			emitMemorySnapshot(observer)

			ticker := time.NewTicker(interval)
			defer ticker.Stop()

			for {
				select {
				case <-monitorCtx.Done():
					return
				case <-ticker.C:
					emitMemorySnapshot(observer)
				}
			}
		}()
	} else {
		close(session.monitorDone)
	}

	return session, nil
}

func (s *Session) Close() error {
	var errs []error

	if s.cancel != nil {
		s.cancel()
	}
	<-s.monitorDone

	if s.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		if err := s.server.Shutdown(ctx); err != nil {
			errs = append(errs, fmt.Errorf("stop pprof server: %w", err))
		}
		cancel()
		if err := <-s.serverDone; err != nil {
			errs = append(errs, fmt.Errorf("pprof server: %w", err))
		}
	}

	if err := s.closeCPUProfile(); err != nil {
		errs = append(errs, err)
	}
	if err := s.writeHeapProfile(); err != nil {
		errs = append(errs, err)
	}

	if len(errs) == 0 {
		return nil
	}
	return errs[0]
}

func (s *Session) writeHeapProfile() error {
	if s.cfg.HeapProfilePath == "" {
		return nil
	}

	file, err := os.Create(s.cfg.HeapProfilePath)
	if err != nil {
		return fmt.Errorf("create heap profile: %w", err)
	}
	defer func() { _ = file.Close() }()

	runtime.GC()
	if err := runtimepprof.WriteHeapProfile(file); err != nil {
		return fmt.Errorf("write heap profile: %w", err)
	}
	return nil
}

func (s *Session) closeCPUProfile() error {
	if s.cpuFile == nil {
		return nil
	}
	runtimepprof.StopCPUProfile()
	err := s.cpuFile.Close()
	s.cpuFile = nil
	if err != nil {
		return fmt.Errorf("close CPU profile: %w", err)
	}
	return nil
}

func emitMemorySnapshot(observer Observer) {
	if observer == nil {
		return
	}

	var stats runtime.MemStats
	runtime.ReadMemStats(&stats)
	observer(MemorySnapshot{
		Time:       time.Now(),
		AllocBytes: stats.Alloc,
		HeapBytes:  stats.HeapAlloc,
		SysBytes:   stats.Sys,
		NumGC:      stats.NumGC,
		Goroutines: runtime.NumGoroutine(),
	})
}

func FormatBytes(value uint64) string {
	const unit = 1024
	if value < unit {
		return fmt.Sprintf("%d B", value)
	}

	div := float64(unit)
	suffix := "KiB"
	for _, next := range []string{"MiB", "GiB", "TiB"} {
		if float64(value) < div*unit {
			break
		}
		div *= unit
		suffix = next
	}

	return fmt.Sprintf("%.1f %s", float64(value)/div, suffix)
}
