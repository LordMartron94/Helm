package targetexecutor

import (
	"helm/internal/ir"
	"signal"
)

type TargetExecutorOptions struct {
	RunHandler TargetRunHandler

	ConfirmDependency func(dependent string, dep ir.HelmTargetDependency) (proceed bool, err error)

	// When set, each run step emits EXEC_OK / EXEC_FAIL signals with captured output payloads.
	SignalContext *signal.SignalContext
}
