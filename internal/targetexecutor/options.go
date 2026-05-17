package targetexecutor

import "helm/internal/ir"

type TargetExecutorOptions struct {
	RunHandler TargetRunHandler

	ConfirmDependency func(dependent string, dep ir.HelmTargetDependency) (proceed bool, err error)
}
