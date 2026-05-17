package version

import "fmt"

// Set at link time via -ldflags; defaults suit local go run / go build.
var (
	Version = "dev"
	Commit  = "unknown"
)

func String() string {
	if Commit == "" || Commit == "unknown" {
		return fmt.Sprintf("helm %s", Version)
	}
	return fmt.Sprintf("helm %s (%s)", Version, Commit)
}
