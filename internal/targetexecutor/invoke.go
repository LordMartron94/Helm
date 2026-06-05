package targetexecutor

import (
	"errors"
	"fmt"
	"helm/internal/expand"
	"strings"
)

// TargetExecutorDependencyBlockedError records one blocked target and the dependency chain
// down to the root failure (no nested "blocked: dependency failed" wrapping).
type TargetExecutorDependencyBlockedError struct {
	BlockedTarget string
	Chain         []string
	RootCause     error
}

func (e *TargetExecutorDependencyBlockedError) Error() string {
	if e == nil {
		return ""
	}
	if len(e.Chain) == 0 {
		return fmt.Sprintf("target %q blocked: %v", e.BlockedTarget, e.RootCause)
	}
	return fmt.Sprintf(
		"target %q blocked (%s): %v",
		e.BlockedTarget,
		strings.Join(e.Chain, " → "),
		e.RootCause,
	)
}

func (e *TargetExecutorDependencyBlockedError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.RootCause
}

func TargetExecutorInterpolateLiteral(
	literal string,
	globalVars map[string]string,
	parameters map[string]string,
) string {
	return expand.ExpandInterpolateLiteral(literal, globalVars, parameters)
}

// TargetExecutorResolveRunCommand interpolates a run-command literal and folds multiline
// formatting into a single command line (see expand.ExpandInterpolateRunCommand).
func TargetExecutorResolveRunCommand(
	literal string,
	globalVars map[string]string,
	parameters map[string]string,
) string {
	return expand.ExpandInterpolateRunCommand(literal, globalVars, parameters)
}

func targetExecutorInterpolateEnv(
	env map[string]string,
	globalVars map[string]string,
	parameters map[string]string,
) map[string]string {
	if len(env) == 0 {
		return nil
	}

	out := make(map[string]string, len(env))
	for key, value := range env {
		out[key] = TargetExecutorInterpolateLiteral(value, globalVars, parameters)
	}
	return out
}

func TargetExecutorFormatDependencyBlockedError(
	targetName string,
	dependencyName string,
	depErr error,
) error {
	blocked := &TargetExecutorDependencyBlockedError{
		BlockedTarget: targetName,
		Chain:         []string{dependencyName},
		RootCause:     depErr,
	}

	var inner *TargetExecutorDependencyBlockedError
	if errors.As(depErr, &inner) {
		blocked.Chain = append([]string{dependencyName}, inner.Chain...)
		blocked.RootCause = inner.RootCause
	}

	return blocked
}
