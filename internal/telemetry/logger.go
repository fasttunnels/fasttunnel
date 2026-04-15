// Package telemetry provides mode-aware logging: dev mode logs full context with timestamps,
// production mode logs only user-facing messages without timestamps.
package telemetry

import (
	"fmt"
	"strings"
	"time"
)

// ANSI color codes
const (
	colorReset   = "\033[0m"
	colorBold    = "\033[1m"
	colorDim     = "\033[2m"
	colorRed     = "\033[31m"
	colorGreen   = "\033[32m"
	colorYellow  = "\033[33m"
	colorBlue    = "\033[34m"
	colorMagenta = "\033[35m"
	colorCyan    = "\033[36m"
)

type APIError struct {
	Method     string
	Endpoint   string
	StatusCode int
	Code       string
	Detail     string
	UserMsg    string
	Silent     bool
}

// mode controls whether to log full context (dev) or only user messages (prod).
var mode string = "dev"

// SetMode sets the logging mode: "dev" for development, anything else for production.
func SetMode(m string) {
	mode = m
}

// isDev returns true if we're in development mode.
func isDev() bool {
	return mode == "dev"
}

func IsDev() bool {
	return isDev()
}

func (e *APIError) Error() string {
	if e.UserMsg != "" {
		return fmt.Sprintf("%s failed: %s", e.Endpoint, e.UserMsg)
	}
	if e.Code != "" {
		return fmt.Sprintf("%s failed (%d %s)", e.Endpoint, e.StatusCode, e.Code)
	}
	return fmt.Sprintf("%s failed (%d)", e.Endpoint, e.StatusCode)
}

// TruncatePath keeps only the leftmost N characters of a path, adding "…" if truncated.
func TruncatePath(path string, maxLen int) string {
	if len(path) <= maxLen {
		return path
	}
	return path[:maxLen-1] + "…"
}

// ColorStatus returns a colored HTTP status code string.
// 2xx → green, 3xx → cyan, 4xx → yellow, 5xx → red, 0 → dim (pending).
func ColorStatus(status int) string {
	if status == 0 {
		return colorDim + "  ?" + colorReset
	}

	var color string
	switch {
	case status >= 200 && status < 300:
		color = colorGreen
	case status >= 300 && status < 400:
		color = colorCyan
	case status >= 400 && status < 500:
		color = colorYellow
	case status >= 500:
		color = colorRed
	default:
		color = colorDim
	}
	return color + fmt.Sprintf("%3d", status) + colorReset
}

// ColorMethod returns a colored HTTP method.
func ColorMethod(method string) string {
	methodMap := map[string]string{
		"GET":     colorCyan,
		"POST":    colorYellow,
		"PUT":     colorMagenta,
		"DELETE":  colorRed,
		"PATCH":   colorBlue,
		"HEAD":    colorDim,
		"OPTIONS": colorDim,
	}
	color, ok := methodMap[strings.ToUpper(method)]
	if !ok {
		color = colorBlue
	}
	return color + method + colorReset
}

// ColorDomain returns a colored subdomain/host.
func ColorDomain(domain string) string {
	return colorMagenta + domain + colorReset
}

// ColorError returns a colored error message.
func ColorError(msg string) string {
	return colorRed + "✗ " + colorReset + msg
}

// ColorSuccess returns a colored success indicator.
func ColorSuccess(msg string) string {
	return colorGreen + "✓ " + msg + colorReset
}

// FormatDuration returns a human-friendly duration with a color indicator.
// Quick (<50ms) → green, normal (50-200ms) → cyan, slow (200-1s) → yellow, very slow (>1s) → red.
func FormatDuration(d time.Duration) string {
	var color string
	switch {
	case d < 50*time.Millisecond:
		color = colorGreen
	case d < 200*time.Millisecond:
		color = colorCyan
	case d < 1*time.Second:
		color = colorYellow
	default:
		color = colorRed
	}
	return color + fmt.Sprintf("%.0fms", d.Seconds()*1000) + colorReset
}

// LogRequest logs an incoming HTTP request (CLI side: from edge).
func LogRequest(domain, method, path, query string) {
	if query != "" {
		path = path + "?" + query
	}
	path = TruncatePath(path, 60)

	fmt.Printf(
		"%s %s %s %s %s\n",
		colorBlue+"[REQ]"+colorReset,
		ColorDomain(domain),
		ColorMethod(method),
		path,
		colorDim+"(pending)"+colorReset,
	)
}

// LogResponse logs an outgoing HTTP response (CLI side: to edge).
func LogResponse(domain, method, path, query string, status int, duration time.Duration) {
	if query != "" {
		path = path + "?" + query
	}
	path = TruncatePath(path, 60)

	fmt.Printf(
		"%s %s %s %s %s %s\n",
		colorGreen+"[RSP]"+colorReset,
		ColorDomain(domain),
		ColorMethod(method),
		path,
		ColorStatus(status),
		FormatDuration(duration),
	)
}

// LogForwardStart logs when the CLI starts forwarding to localhost.
func LogForwardStart(domain, method, path, query, localTarget string) {
	if query != "" {
		path = path + "?" + query
	}
	path = TruncatePath(path, 50)
	fmt.Printf(
		"%s %s %s → %s%s %s\n",
		colorCyan+"[FWD]"+colorReset,
		ColorDomain(domain),
		ColorMethod(method),
		colorDim+localTarget+colorReset,
		path,
		colorDim+"(pending)"+colorReset,
	)
}

// LogForwardError logs a forwarding error.
func LogForwardError(domain, method, path, query, localTarget, errMsg string) {
	if query != "" {
		path = path + "?" + query
	}
	path = TruncatePath(path, 50)
	fmt.Printf(
		"%s %s %s → %s%s %s\n",
		colorRed+"[ERR]"+colorReset,
		ColorDomain(domain),
		ColorMethod(method),
		colorDim+localTarget+colorReset,
		path,
		ColorError(errMsg),
	)
}

// LogEdgeForward logs when edge forwards a request to an agent.
func LogEdgeForward(domain, method, path, query string) {
	if query != "" {
		path = path + "?" + query
	}
	path = TruncatePath(path, 60)
	fmt.Printf(
		"%s %s %s %s %s\n",
		colorBlue+"[→AGENT]"+colorReset,
		ColorDomain(domain),
		ColorMethod(method),
		path,
		colorDim+"(pending)"+colorReset,
	)
}

// LogEdgeResponse logs when edge receives a response from agent.
func LogEdgeResponse(domain, method, path, query string, status int, duration time.Duration) {
	if query != "" {
		path = path + "?" + query
	}
	path = TruncatePath(path, 60)
	fmt.Printf(
		"%s %s %s %s %s %s\n",
		colorGreen+"[←AGENT]"+colorReset,
		ColorDomain(domain),
		ColorMethod(method),
		path,
		ColorStatus(status),
		FormatDuration(duration),
	)
}

// LogAgentDisconnect logs when an agent disconnects.
func LogAgentDisconnect(domain, reason string) {
	fmt.Printf(
		"%s %s %s\n",
		colorYellow+"[DISCO]"+colorReset,
		ColorDomain(domain),
		ColorError(reason),
	)
}

// LogAgentConnect logs when an agent connects.
func LogAgentConnect(domain string) {
	fmt.Printf(
		"%s %s %s\n",
		colorGreen+"[CONN]"+colorReset,
		ColorDomain(domain),
		ColorSuccess("connected"),
	)
}

// LogInfo logs an informational message (always timestamped in dev mode).
func LogInfo(msg string) {
	if isDev() {
		fmt.Printf("[INFO] %s\n", msg)
	} else {
		fmt.Printf("%s\n", msg)
	}
}

// LogError logs an error message (with full context in dev mode, user message only in prod).
// context is ignored in production mode.
func LogError(userMsg, context string) {
	if isDev() {
		if context != "" {
			fmt.Printf("[ERROR] %s (%s)\n", userMsg, context)
		} else {
			fmt.Printf("[ERROR] %s\n", userMsg)
		}
	} else {
		fmt.Printf("%s\n", ColorError(userMsg))
	}
}

func BuildAPIError(method, endpoint string, statusCode int, code string, detail string, userMsg string, silent bool) *APIError {
	return &APIError{
		Method:     method,
		Endpoint:   endpoint,
		StatusCode: statusCode,
		Code:       code,
		Detail:     detail,
		UserMsg:    userMsg,
		Silent:     silent,
	}
}

func SilentLogProdError(err error) {
	if isDev() {
		apiErr, ok := err.(*APIError)
		if ok {
			LogError(apiErr.UserMsg, apiErr.Error())
		} else {
			LogError(err.Error(), "")
		}
	}
}
