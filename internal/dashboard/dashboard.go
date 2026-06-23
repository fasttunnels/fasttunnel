package dashboard

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/fasttunnels/fasttunnel/internal/agent"
	"github.com/fasttunnels/fasttunnel/internal/diagnostics"
)

const defaultMaxRows = 15

type SessionInfo struct {
	Protocol    string
	PublicURL   string
	LocalTarget string
	Subdomain   string
	MaxRows     int
	Diagnostics []string
}

type Controller struct {
	info   SessionInfo
	events chan agent.RuntimeEvent
}

func NewController(info SessionInfo) *Controller {
	if info.MaxRows <= 0 {
		info.MaxRows = defaultMaxRows
	}
	return &Controller{
		info:   info,
		events: make(chan agent.RuntimeEvent, 1024),
	}
}

func (c *Controller) Observer() agent.EventObserver {
	return func(ev agent.RuntimeEvent) {
		select {
		case c.events <- ev:
		default:
			// Keep tunnel hot paths non-blocking.
		}
	}
}

func (c *Controller) Run(ctx context.Context, cancel context.CancelFunc) error {
	m := newModel(c.info, c.events, cancel, ctx.Done())
	program := tea.NewProgram(m, tea.WithAltScreen())
	_, err := program.Run()
	return err
}

type runtimeEventMsg struct {
	event agent.RuntimeEvent
}

type contextDoneMsg struct{}

type activeRequest struct {
	Method string
	Path   string
	Start  time.Time
}

type requestRow struct {
	At       time.Time
	Method   string
	Path     string
	Status   int
	Duration time.Duration
	Error    string
}

type model struct {
	info   SessionInfo
	events <-chan agent.RuntimeEvent
	done   <-chan struct{}
	cancel context.CancelFunc

	width  int
	height int

	state       agent.ConnectionState
	stateReason string
	stateAt     time.Time
	backoff     time.Duration
	lastMemory  *agent.RuntimeEvent

	paused      bool
	showHelp    bool
	filterIndex int

	active map[string]activeRequest
	rows   []requestRow
}

func newModel(info SessionInfo, events <-chan agent.RuntimeEvent, cancel context.CancelFunc, done <-chan struct{}) model {
	return model{
		info:        info,
		events:      events,
		done:        done,
		cancel:      cancel,
		state:       agent.ConnectionStateConnecting,
		stateAt:     time.Now(),
		active:      make(map[string]activeRequest),
		filterIndex: 0,
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(waitForRuntimeEvent(m.events, m.done), waitForContextDone(m.done))
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			m.cancel()
			return m, tea.Quit
		case "p":
			m.paused = !m.paused
			return m, nil
		case "c":
			m.active = make(map[string]activeRequest)
			m.rows = nil
			return m, nil
		case "f":
			m.filterIndex = (m.filterIndex + 1) % 5
			return m, nil
		case "h":
			m.showHelp = !m.showHelp
			return m, nil
		}
	case runtimeEventMsg:
		m.applyEvent(msg.event)
		return m, waitForRuntimeEvent(m.events, m.done)
	case contextDoneMsg:
		return m, tea.Quit
	}
	return m, nil
}

func (m *model) applyEvent(ev agent.RuntimeEvent) {
	switch ev.Type {
	case agent.RuntimeEventConnectionState:
		m.state = ev.State
		m.stateReason = ev.Reason
		m.backoff = ev.Backoff
		m.stateAt = ev.Time
		return
	case agent.RuntimeEventMemorySnapshot:
		snapshot := ev
		m.lastMemory = &snapshot
		return
	case agent.RuntimeEventRequestStart, agent.RuntimeEventRequestComplete, agent.RuntimeEventRequestError:
		if m.paused {
			return
		}
	}

	switch ev.Type {
	case agent.RuntimeEventRequestStart:
		if ev.RequestID == "" {
			return
		}
		m.active[ev.RequestID] = activeRequest{
			Method: ev.Method,
			Path:   ev.Path,
			Start:  ev.Time,
		}
	case agent.RuntimeEventRequestComplete, agent.RuntimeEventRequestError:
		row := requestRow{
			At:       ev.Time,
			Method:   ev.Method,
			Path:     ev.Path,
			Status:   ev.Status,
			Duration: ev.Duration,
			Error:    ev.Error,
		}
		if active, ok := m.active[ev.RequestID]; ok {
			if row.Method == "" {
				row.Method = active.Method
			}
			if row.Path == "" {
				row.Path = active.Path
			}
			if row.Duration <= 0 {
				row.Duration = ev.Time.Sub(active.Start)
			}
			delete(m.active, ev.RequestID)
		}

		if row.Method == "" {
			row.Method = "GET"
		}
		if row.Path == "" {
			row.Path = "/"
		}
		if row.At.IsZero() {
			row.At = time.Now()
		}

		m.rows = append(m.rows, row)
		if len(m.rows) > m.info.MaxRows {
			m.rows = m.rows[len(m.rows)-m.info.MaxRows:]
		}
	}
}

func (m model) View() string {
	var b strings.Builder

	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("87"))
	metaStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("250"))
	warnStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	errorStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("204"))
	successStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("42"))

	stateLabel := string(m.state)
	stateStyle := warnStyle
	switch m.state {
	case agent.ConnectionStateOnline:
		stateStyle = successStyle
	case agent.ConnectionStateStopping:
		stateStyle = errorStyle
	case agent.ConnectionStateConnecting, agent.ConnectionStateReconnecting:
		stateStyle = warnStyle
	}

	fmt.Fprintf(&b, "%s\n", headerStyle.Render(fmt.Sprintf("fasttunnel %s tunnel", strings.ToLower(m.info.Protocol))))
	fmt.Fprintf(&b, "  public url  : %s\n", m.info.PublicURL)
	fmt.Fprintf(&b, "  local target: %s\n", m.info.LocalTarget)
	fmt.Fprintf(&b, "  subdomain   : %s\n", m.info.Subdomain)
	for _, line := range m.info.Diagnostics {
		fmt.Fprintf(&b, "  %s\n", line)
	}

	stateDetail := ""
	if m.stateReason != "" {
		stateDetail = fmt.Sprintf(" (%s)", truncateASCII(strings.TrimSpace(m.stateReason), 80))
	}
	if m.backoff > 0 && m.state == agent.ConnectionStateReconnecting {
		stateDetail += fmt.Sprintf(" retry in %s", m.backoff.Round(100*time.Millisecond))
	}
	fmt.Fprintf(&b, "\nstatus: %s%s\n", stateStyle.Render(strings.ToUpper(stateLabel)), metaStyle.Render(stateDetail))
	if m.lastMemory != nil {
		fmt.Fprintf(
			&b,
			"%s\n",
			metaStyle.Render(fmt.Sprintf(
				"memory: alloc %s  heap %s  sys %s  goroutines %d  gc %d",
				diagnostics.FormatBytes(m.lastMemory.AllocBytes),
				diagnostics.FormatBytes(m.lastMemory.HeapBytes),
				diagnostics.FormatBytes(m.lastMemory.SysBytes),
				m.lastMemory.Goroutines,
				m.lastMemory.NumGC,
			)),
		)
	}

	fmt.Fprintf(&b, "%s\n", metaStyle.Render(fmt.Sprintf("active: %d  filter: %s   updates: %s",
		len(m.active), filterLabel(m.filterIndex), updateLabel(m.paused))))
	//fmt.Fprintf(&b, "%s\n", metaStyle.Render(fmt.Sprintf("active: %d   retained: %d/%d   filter: %s   updates: %s",
	// len(m.active), len(m.rows), m.info.MaxRows, filterLabel(m.filterIndex), updateLabel(m.paused))))

	fmt.Fprintf(&b, "\n%s\n", renderTable(m.visibleRows(), m.width, m.height))

	if m.showHelp {
		fmt.Fprintf(&b, "\n%s\n", metaStyle.Render("keys: q/Ctrl+C quit, p pause updates, c clear, f cycle filter, h toggle help"))
	} else {
		fmt.Fprintf(&b, "\n%s\n", metaStyle.Render("press h for controls"))
	}

	return b.String()
}

func (m model) visibleRows() []requestRow {
	if m.filterIndex == 0 {
		return trimForHeight(m.rows, m.height)
	}

	filtered := make([]requestRow, 0, len(m.rows))
	for _, row := range m.rows {
		if statusBucket(row.Status) == m.filterIndex {
			filtered = append(filtered, row)
		}
	}
	return trimForHeight(filtered, m.height)
}

func trimForHeight(rows []requestRow, height int) []requestRow {
	if height <= 0 {
		return rows
	}
	maxRows := height - 12
	if maxRows < 5 {
		maxRows = 5
	}
	if len(rows) <= maxRows {
		return rows
	}
	return rows[len(rows)-maxRows:]
}

func renderTable(rows []requestRow, width, _ int) string {
	if width <= 0 {
		width = 120
	}
	pathWidth := width - 44
	if pathWidth < 20 {
		pathWidth = 20
	}

	var b strings.Builder
	headStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("111"))
	fmt.Fprintf(&b, "%s\n", headStyle.Render(fmt.Sprintf("%-12s %-6s %-*s %-4s %-8s", "Time", "Method", pathWidth, "Path", "Code", "Duration")))

	for _, row := range rows {
		fmt.Fprintf(&b, "%-12s %-6s %-*s %-4d %-8s\n",
			row.At.Format("15:04:05.000"),
			strings.ToUpper(row.Method),
			pathWidth,
			truncateASCII(row.Path, pathWidth),
			row.Status,
			formatDuration(row.Duration),
		)
	}

	if len(rows) == 0 {
		fmt.Fprintf(&b, "(no requests yet)\n")
	}

	return b.String()
}

func statusBucket(code int) int {
	switch {
	case code >= 200 && code < 300:
		return 1
	case code >= 300 && code < 400:
		return 2
	case code >= 400 && code < 500:
		return 3
	case code >= 500:
		return 4
	default:
		return 0
	}
}

func filterLabel(i int) string {
	switch i {
	case 1:
		return "2xx"
	case 2:
		return "3xx"
	case 3:
		return "4xx"
	case 4:
		return "5xx"
	default:
		return "all"
	}
}

func updateLabel(paused bool) string {
	if paused {
		return "paused"
	}
	return "live"
}

func truncateASCII(s string, max int) string {
	if max <= 0 {
		return ""
	}
	if len(s) <= max {
		return s
	}
	if max <= 3 {
		return s[:max]
	}
	return s[:max-3] + "..."
}

func formatDuration(d time.Duration) string {
	if d <= 0 {
		return "-"
	}
	return fmt.Sprintf("%dms", d.Milliseconds())
}

func waitForRuntimeEvent(events <-chan agent.RuntimeEvent, done <-chan struct{}) tea.Cmd {
	return func() tea.Msg {
		select {
		case <-done:
			return contextDoneMsg{}
		case ev := <-events:
			return runtimeEventMsg{event: ev}
		}
	}
}

func waitForContextDone(done <-chan struct{}) tea.Cmd {
	return func() tea.Msg {
		<-done
		return contextDoneMsg{}
	}
}
