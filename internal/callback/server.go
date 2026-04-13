// Package callback implements the transient local HTTP server used during the
// OAuth 2.0 + PKCE login flow.
//
// It listens for exactly one GET /callback request, validates the anti-CSRF
// state parameter, and surfaces the authorization code to the caller via Wait.
package callback

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"
)

// Result holds the code and validated state received from the OAuth callback.
type Result struct {
	Code  string
	State string
}

// Server is a short-lived HTTP listener that captures one OAuth redirect.
type Server struct {
	// URL is the full callback URL (http://127.0.0.1:<port>/callback) that must
	// be passed as redirect_uri to the authorization server.
	URL    string
	server *http.Server
	ch     chan Result
}

// Start binds a TCP listener on 127.0.0.1 (port if > 0, otherwise a free OS
// port), starts the callback server, and returns a ready *Server.
func Start(port int, expectedState string) (*Server, error) {
	addr := "127.0.0.1:0"
	if port > 0 {
		addr = fmt.Sprintf("127.0.0.1:%d", port)
	}

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("bind callback port: %w", err)
	}

	ch := make(chan Result, 1)

	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		code := r.URL.Query().Get("code")
		state := r.URL.Query().Get("state")

		if state != expectedState {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte("Invalid state — possible CSRF attack. Close this window."))
			return
		}
		if code == "" {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte("Missing authorization code."))
			return
		}

		select {
		case ch <- Result{Code: code, State: state}:
		default:
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(
			`<html><head><title>fasttunnel login</title></head>` +
				`<body style="font-family:system-ui;padding:40px;text-align:center">` +
				`<h2>&#10003; Login successful</h2>` +
				`<p>Your fasttunnel CLI is now authenticated.</p>` +
				`<p>You can close this tab and return to your terminal.</p>` +
				`</body></html>`,
		))
	})

	srv := &http.Server{Handler: mux}
	go func() { _ = srv.Serve(listener) }()

	return &Server{
		URL:    fmt.Sprintf("http://%s/callback", listener.Addr().String()),
		server: srv,
		ch:     ch,
	}, nil
}

// Wait blocks until the OAuth callback arrives or timeout elapses.
func (s *Server) Wait(timeout time.Duration) (Result, error) {
	select {
	case result := <-s.ch:
		return result, nil
	case <-time.After(timeout):
		return Result{}, fmt.Errorf("timed out waiting for browser callback")
	}
}

// Shutdown gracefully drains the callback server.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.server.Shutdown(ctx)
}
