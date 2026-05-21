package agent

import (
	"net/http"
	"testing"
)

func TestCloneWebSocketHeaders(t *testing.T) {
	in := map[string][]string{
		"Host":                   {"abc.fasttunnel.dev"},
		"Connection":             {"Upgrade"},
		"Upgrade":                {"websocket"},
		"Sec-WebSocket-Key":      {"abc"},
		"Sec-WebSocket-Version":  {"13"},
		"Sec-WebSocket-Protocol": {"hmr"},
		"Origin":                 {"https://abc.fasttunnel.dev"},
		"Cookie":                 {"session=123"},
	}

	out := cloneWebSocketHeaders(in)

	if out.Get("Host") != "" {
		t.Fatal("expected Host to be stripped")
	}
	if out.Get("Connection") != "" {
		t.Fatal("expected Connection to be stripped")
	}
	if out.Get("Upgrade") != "" {
		t.Fatal("expected Upgrade to be stripped")
	}
	if out.Get("Sec-WebSocket-Key") != "" {
		t.Fatal("expected Sec-WebSocket-Key to be stripped")
	}
	if out.Get("Sec-WebSocket-Version") != "" {
		t.Fatal("expected Sec-WebSocket-Version to be stripped")
	}
	if out.Get("Sec-WebSocket-Protocol") != "hmr" {
		t.Fatal("expected Sec-WebSocket-Protocol to be preserved")
	}
	if out.Get("Origin") != "https://abc.fasttunnel.dev" {
		t.Fatal("expected Origin to be preserved")
	}
	if out.Get("Cookie") != "session=123" {
		t.Fatal("expected Cookie to be preserved")
	}
}

func TestWSStreamRegistrySetGetRemove(t *testing.T) {
	registry := newWSStreamRegistry()
	stream := &localWSStream{}

	registry.set("stream-1", stream)
	got, ok := registry.get("stream-1")
	if !ok {
		t.Fatal("expected stream to exist")
	}
	if got != stream {
		t.Fatal("expected same stream pointer")
	}

	removed, ok := registry.remove("stream-1")
	if !ok {
		t.Fatal("expected stream to be removed")
	}
	if removed != stream {
		t.Fatal("expected removed stream pointer to match")
	}
	if _, ok := registry.get("stream-1"); ok {
		t.Fatal("expected stream to be absent after remove")
	}
}

func TestCloneWebSocketHeadersReturnsHeaderMap(t *testing.T) {
	out := cloneWebSocketHeaders(map[string][]string{})
	if out == nil {
		t.Fatal("expected non-nil header map")
	}
	out.Add("X-Test", "value")
	if out.Get("X-Test") != "value" {
		t.Fatal("expected writable header map")
	}

	// Ensure the returned type is http.Header.
	_ = http.Header(out)
}
