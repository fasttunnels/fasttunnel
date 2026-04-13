// Package browser provides a cross-platform utility for opening URLs in the
// user's default system browser.
package browser

import (
	"os/exec"
	"runtime"
)

// Open attempts to open url in the system browser.
// Returns true if the launch command started successfully; on failure (or an
// unsupported OS) it returns false so the caller can prompt the user to open
// the URL manually.
func Open(url string) bool {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		return false
	}
	return cmd.Start() == nil
}
