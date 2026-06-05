package targetexecutor

import "helm/internal/ir"

func TargetExecutorEvaluateCondition(
	condition ir.HelmCondition,
	resolved TargetResolvedParameters,
) bool {
	defined := targetExecutorResolvedParameterDefined(resolved, condition.Parameter)
	value := resolved.Scalars[condition.Parameter]

	switch condition.ConditionType {
	case ir.ConditionDefined:
		return defined
	case ir.ConditionNotDefined:
		return !defined
	case ir.ConditionEquals:
		return defined && value == condition.TargetValue
	case ir.ConditionNotEquals:
		return defined && value != condition.TargetValue
	default:
		return false
	}
}

func targetExecutorResolvedParameterDefined(
	resolved TargetResolvedParameters,
	paramName string,
) bool {
	if value, ok := resolved.Scalars[paramName]; ok && value != "" {
		return true
	}
	if paths, ok := resolved.PathLists[paramName]; ok && len(paths) > 0 {
		return true
	}
	return false
}
