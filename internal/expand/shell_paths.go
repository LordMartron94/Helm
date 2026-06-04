package expand

import (
	"github.com/google/shlex"
)

// PathsFromShellParameterList splits a dependency-parameter file list produced by
// JoinPathsForShell into individual paths. Returns false when the value should be
// treated as a single path string.
func PathsFromShellParameterList(value string) ([]string, bool) {
	if value == "" {
		return nil, false
	}

	parts, err := shlex.Split(value)
	if err != nil || len(parts) <= 1 {
		return nil, false
	}

	return parts, true
}
