package targetexecutor

import (
	"helm/internal/expand"
	"helm/internal/ir"
)

func TargetExecutorEvaluateCondition(
	condition ir.HelmCondition,
	interpCtx expand.InterpolationContext,
) bool {
	defined := targetExecutorConditionVariableDefined(interpCtx, condition.Parameter)
	value := expand.InterpolationContextExpandLiteral(
		interpCtx,
		interpCtx.Scalars[condition.Parameter],
	)

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

func targetExecutorConditionVariableDefined(
	interpCtx expand.InterpolationContext,
	name string,
) bool {
	if value, ok := interpCtx.Scalars[name]; ok && value != "" {
		return true
	}
	if paths, ok := interpCtx.PathLists[name]; ok && len(paths) > 0 {
		return true
	}
	return false
}
