package agent

import "time"

// ConnectionState describes the agent-edge connectivity lifecycle.
type ConnectionState string

const (
	ConnectionStateConnecting   ConnectionState = "connecting"
	ConnectionStateOnline       ConnectionState = "online"
	ConnectionStateReconnecting ConnectionState = "reconnecting"
	ConnectionStateStopping     ConnectionState = "stopping"
)

// RuntimeEventType categorizes dashboard/runtime events.
type RuntimeEventType string

const (
	RuntimeEventConnectionState RuntimeEventType = "connection_state"
	RuntimeEventRequestStart    RuntimeEventType = "request_start"
	RuntimeEventRequestComplete RuntimeEventType = "request_complete"
	RuntimeEventRequestError    RuntimeEventType = "request_error"
	RuntimeEventMemorySnapshot  RuntimeEventType = "memory_snapshot"
)

// RuntimeEvent carries connection and request lifecycle updates for terminal UX.
type RuntimeEvent struct {
	Type RuntimeEventType
	Time time.Time

	State   ConnectionState
	Reason  string
	Backoff time.Duration

	RequestID string
	Method    string
	Path      string
	Status    int
	Duration  time.Duration
	Error     string

	AllocBytes uint64
	HeapBytes  uint64
	SysBytes   uint64
	NumGC      uint32
	Goroutines int
}

// EventObserver receives runtime events from the agent loop.
type EventObserver func(RuntimeEvent)

// RunOptions configures optional runtime hooks for RunAgentLoop.
type RunOptions struct {
	Observer EventObserver
}

func (o RunOptions) emit(ev RuntimeEvent) {
	if o.Observer == nil {
		return
	}
	o.Observer(ev)
}
