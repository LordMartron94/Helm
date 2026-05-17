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

func TargetExecutorFormatDependencyBlockedError(targetName string, dependencyName string, depErr error) error {
	return fmt.Errorf(
		"target '%s' blocked: dependency '%s' failed: %w",
		targetName,
		dependencyName,
		depErr,
	)
}
