package targetexecutor

import (
	"fmt"
	"helm/internal/expand"
)

func TargetExecutorInterpolateLiteral(
	literal string,
	globalVars map[string]string,
	parameters map[string]string,
) string {
	return expand.ExpandInterpolateLiteral(literal, globalVars, parameters)
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

func TargetExecutorFormatDependencyBlockedError(targetName string, dependencyName string, depErr error) error {
	return fmt.Errorf(
		"target '%s' blocked: dependency '%s' failed: %w",
		targetName,
		dependencyName,
		depErr,
	)
}
