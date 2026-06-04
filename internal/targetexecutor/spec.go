package targetexecutor

import "helm/internal/ir"

type TargetInvocation struct {
	Parameters map[string]ir.HelmParameterValue
}
