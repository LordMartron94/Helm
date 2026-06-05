package targetexecutor

import (
	"helm/internal/expand"
	"helm/internal/ir"
)

func targetExecutorBindStringListLiterals(
	interpCtx expand.InterpolationContext,
	expr ir.HelmStringListExpr,
) ir.HelmStringListExpr {
	if len(expr) == 0 {
		return nil
	}
	out := make(ir.HelmStringListExpr, len(expr))
	for i, element := range expr {
		switch element.Kind {
		case ir.StringListLiteral:
			out[i] = ir.HelmStringListElement{
				Kind:    ir.StringListLiteral,
				Literal: expand.InterpolationContextExpandLiteral(interpCtx, element.Literal),
			}
		default:
			out[i] = element
		}
	}
	return out
}

func TargetExecutorEvaluateStringListExpr(
	expr ir.HelmStringListExpr,
	targets map[string]ir.HelmTarget,
	paramValues map[string]ir.HelmParameterValue,
	resolved TargetResolvedParameters,
	interpCtx expand.InterpolationContext,
) ([]string, error) {
	return expand.EvaluateStringListExpr(
		expr,
		targets,
		paramValues,
		resolved.Scalars,
		resolved.StringLists,
		resolved.PathLists,
		interpCtx,
	)
}

func targetExecutorInterpolateEnv(
	env map[string]ir.HelmStringListExpr,
	targets map[string]ir.HelmTarget,
	paramValues map[string]ir.HelmParameterValue,
	resolved TargetResolvedParameters,
	interpCtx expand.InterpolationContext,
) (map[string]string, error) {
	if len(env) == 0 {
		return nil, nil
	}

	out := make(map[string]string, len(env))
	for key, expr := range env {
		joined, err := expand.JoinStringListExpr(
			expr,
			targets,
			paramValues,
			resolved.Scalars,
			resolved.StringLists,
			resolved.PathLists,
			interpCtx,
			" ",
		)
		if err != nil {
			return nil, err
		}
		out[key] = joined
	}
	return out, nil
}

func targetExecutorResolvedEnvForFingerprint(
	target ir.HelmTarget,
	targets map[string]ir.HelmTarget,
	paramValues map[string]ir.HelmParameterValue,
	resolved TargetResolvedParameters,
	interpCtx expand.InterpolationContext,
) (map[string]string, error) {
	return targetExecutorInterpolateEnv(
		target.Env,
		targets,
		paramValues,
		resolved,
		interpCtx,
	)
}
