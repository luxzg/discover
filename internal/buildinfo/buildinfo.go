package buildinfo

import "fmt"

var (
	// Version is the app version string. Keep this in sync with CHANGELOG code entries.
	Version = "v2.19"
	// Commit and BuildTime can be injected at build time with -ldflags if desired.
	Commit    = "dev"
	BuildTime = "unknown"
)

func String() string {
	return fmt.Sprintf("%s (commit=%s, built=%s)", Version, Commit, BuildTime)
}
