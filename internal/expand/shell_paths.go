package expand

import (
	"github.com/google/shlex"
)

// PathsFromShellParameterList splits a value produced by JoinPathsForShell into paths.
// Returns false when the string is a plain unquoted path (e.g. build/out.so).
func PathsFromShellParameterList(value string) ([]string, bool) {
	if value == "" {
		return nil, false
	}

	parts, err := shlex.Split(value)
	if err != nil || len(parts) == 0 {
		return nil, false
	}

	// shlex leaves a single plain path unchanged; JoinPathsForShell always quotes each path.
	if len(parts) == 1 && parts[0] == value {
		return nil, false
	}

	return parts, true
}
