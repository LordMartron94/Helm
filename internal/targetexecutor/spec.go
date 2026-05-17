package targetexecutor

type TargetInvocation struct {
	Parameters map[string]string
}

func TargetInvocationParameters(inv TargetInvocation) map[string]string {
	if inv.Parameters == nil {
		return map[string]string{}
	}
	return inv.Parameters
}
