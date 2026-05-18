package targetexecutor

import "helm/internal/ir"

type TargetInvocation struct {
	Parameters map[string]string
}

func TargetInvocationParameters(inv TargetInvocation) map[string]string {
	if inv.Parameters == nil {
		return map[string]string{}
	}
	return inv.Parameters
}

// TargetExecutorParametersForTarget returns invocation parameters with absent optional
// target parameters bound to the empty string so ${NAME} placeholders expand at runtime.
func TargetExecutorParametersForTarget(target ir.HelmTarget, inv TargetInvocation) map[string]string {
	parameters := make(map[string]string, len(target.Parameters)+len(TargetInvocationParameters(inv)))
	for key, value := range TargetInvocationParameters(inv) {
		parameters[key] = value
	}
	for _, param := range target.Parameters {
		if !param.Optional {
			continue
		}
		if _, exists := parameters[param.Name]; exists {
			continue
		}
		parameters[param.Name] = ""
	}
	return parameters
}
