package targetexecutor

import "helm/internal/ir"

func TargetExecutorEvaluateCondition(
	condition ir.HelmCondition,
	parameters map[string]string,
) bool {
	value, defined := parameters[condition.Parameter]

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
