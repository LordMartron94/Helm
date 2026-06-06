package expand

import (
	"fmt"
	"helm/internal/ir"
	"strings"
)

// EvaluateStringListExpr flattens a Helm string-list expression into concrete strings.
func EvaluateStringListExpr(
	expr ir.HelmStringListExpr,
	targets map[string]ir.HelmTarget,
	paramValues map[string]ir.HelmParameterValue,
	scalarParams map[string]string,
	stringListParams map[string][]string,
	pathListParams map[string][]string,
	ctx InterpolationContext,
) ([]string, error) {
	if len(expr) == 0 {
		return nil, nil
	}

	out := make([]string, 0, len(expr))
	for _, element := range expr {
		switch element.Kind {
		case ir.StringListLiteral:
			text := InterpolationContextExpandLiteral(ctx, element.Literal)
			if text != "" {
				out = append(out, text)
			}
		case ir.StringListParamRef:
			fragment, err := evaluateStringListParamRef(
				element.ParamName,
				paramValues,
				scalarParams,
				stringListParams,
				pathListParams,
			)
			if err != nil {
				return nil, err
			}
			out = append(out, fragment...)
		case ir.StringListCollect:
			if element.Collect == nil {
				return nil, fmt.Errorf("collect element is missing call data")
			}
			fragment, err := evaluateStringListCollect(
				*element.Collect,
				targets,
				paramValues,
				scalarParams,
				stringListParams,
				pathListParams,
				ctx,
			)
			if err != nil {
				return nil, err
			}
			out = append(out, fragment...)
		default:
			return nil, fmt.Errorf("unknown string-list element kind")
		}
	}

	return out, nil
}

// JoinStringListExpr evaluates expr and joins fragments with sep (used for env values).
func JoinStringListExpr(
	expr ir.HelmStringListExpr,
	targets map[string]ir.HelmTarget,
	paramValues map[string]ir.HelmParameterValue,
	scalarParams map[string]string,
	stringListParams map[string][]string,
	pathListParams map[string][]string,
	ctx InterpolationContext,
	sep string,
) (string, error) {
	fragments, err := EvaluateStringListExpr(
		expr,
		targets,
		paramValues,
		scalarParams,
		stringListParams,
		pathListParams,
		ctx,
	)
	if err != nil {
		return "", err
	}
	return strings.Join(fragments, sep), nil
}

func evaluateStringListParamRef(
	paramName string,
	paramValues map[string]ir.HelmParameterValue,
	scalarParams map[string]string,
	stringListParams map[string][]string,
	pathListParams map[string][]string,
) ([]string, error) {
	if fragments, ok := stringListParams[paramName]; ok {
		return append([]string(nil), fragments...), nil
	}
	if paths, ok := pathListParams[paramName]; ok {
		return append([]string(nil), paths...), nil
	}
	if scalar, ok := scalarParams[paramName]; ok {
		return scalarTextStringListFragments(scalar), nil
	}
	if value, ok := paramValues[paramName]; ok {
		switch value.Kind {
		case ir.HelmParameterScalar:
			return scalarTextStringListFragments(value.Scalar), nil
		case ir.HelmParameterStringList:
			return EvaluateStringListExpr(
				value.StringList,
				nil,
				paramValues,
				scalarParams,
				stringListParams,
				pathListParams,
				InterpolationContext{},
			)
		default:
			return nil, fmt.Errorf(
				"parameter '%s' cannot be used in a string-list expression",
				paramName,
			)
		}
	}
	return nil, fmt.Errorf("parameter '%s' is not bound", paramName)
}

func scalarTextStringListFragments(text string) []string {
	if text == "" {
		return nil
	}
	if parts, ok := PathsFromShellParameterList(text); ok {
		return parts
	}
	return []string{text}
}

func evaluateStringListCollect(
	collect ir.HelmCollectExpr,
	targets map[string]ir.HelmTarget,
	paramValues map[string]ir.HelmParameterValue,
	scalarParams map[string]string,
	stringListParams map[string][]string,
	pathListParams map[string][]string,
	ctx InterpolationContext,
) ([]string, error) {
	value, ok := paramValues[collect.DependenciesParam]
	if !ok || value.Kind != ir.HelmParameterDependencyList {
		return nil, fmt.Errorf(
			"collect() parameter '%s' is not a dependency list",
			collect.DependenciesParam,
		)
	}

	out := make([]string, 0)
	for _, dep := range value.Dependencies {
		targetName, ok := ir.IRResolveTargetName(targets, dep.TargetName)
		if !ok {
			return nil, fmt.Errorf("collect() dependency target '%s' does not exist", dep.TargetName)
		}
		producer, ok := targets[targetName]
		if !ok {
			return nil, fmt.Errorf("collect() dependency target '%s' does not exist", dep.TargetName)
		}
		exportExpr, ok := producer.Export[collect.ExportKey]
		if !ok || len(exportExpr) == 0 {
			continue
		}
		fragment, err := EvaluateStringListExpr(
			exportExpr,
			targets,
			dep.Parameters,
			scalarParams,
			stringListParams,
			pathListParams,
			ctx,
		)
		if err != nil {
			return nil, fmt.Errorf("collect() from '%s' export '%s': %w", targetName, collect.ExportKey, err)
		}
		out = append(out, fragment...)
	}

	return out, nil
}
