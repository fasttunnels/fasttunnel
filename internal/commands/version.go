package commands

import "fmt"

// RunVersion prints the build-time version info injected via ldflags.
// Values are "dev" / "none" / "unknown" when built without GoReleaser.
func RunVersion(version, commit, buildDate string) {
	fmt.Printf("fasttunnel %s (%s) built %s\n", version, commit, buildDate)
}
