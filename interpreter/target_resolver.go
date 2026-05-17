package interpreter

import (
	"helm/internal/ir"
	"helm/internal/targetexecutor"
)

func executionChainForTarget(builtIR ir.HelmIR, targetName string) ([][]string, error) {
	return targetexecutor.TargetExecutorExecutionChain(builtIR, targetName)
}
