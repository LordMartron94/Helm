package targetexecutor

import (
	"fmt"
	"helm/internal/artifactresolve"
	"helm/internal/expand"
	"helm/internal/ir"
	"helm/internal/workspacepath"
	"sort"
)

type TargetMatrixInstance struct {
	Bindings map[string]string
	CacheKey string
}

func TargetExecutorMatrixInstances(
	helmBaseDir string,
	target ir.HelmTarget,
	paramValues map[string]ir.HelmParameterValue,
	globals map[string]ir.HelmGlobalVariable,
	interpCtx expand.InterpolationContext,
) ([]TargetMatrixInstance, error) {
	if target.Matrix == nil {
		return []TargetMatrixInstance{{
			Bindings: map[string]string{},
			CacheKey: "",
		}}, nil
	}

	matrix := target.Matrix
	var instances []TargetMatrixInstance
	seen := make(map[string]struct{})

	for _, value := range matrix.Values {
		switch value.Kind {
		case ir.MatrixValueLiteral:
			bindings, err := targetExecutorMatrixLiteralBindings(
				helmBaseDir,
				value.Literal,
				interpCtx,
			)
			if err != nil {
				return nil, fmt.Errorf("target '%s': %w", target.Name, err)
			}
			for _, bindingValue := range bindings {
				instance, createErr := targetExecutorMatrixInstanceCreate(
					matrix.VariableName,
					bindingValue,
					seen,
				)
				if createErr != nil {
					return nil, createErr
				}
				instances = append(instances, instance)
			}
		case ir.MatrixValueParameterRef:
			paths, err := targetExecutorResolveParameterPaths(
				helmBaseDir,
				value.ParameterName,
				globals,
				paramValues,
				interpCtx,
			)
			if err != nil {
				return nil, fmt.Errorf("target '%s': matrix parameter '%s': %w", target.Name, value.ParameterName, err)
			}
			if len(paths) == 0 {
				return nil, fmt.Errorf(
					"target '%s': matrix parameter '%s' produced no paths",
					target.Name,
					value.ParameterName,
				)
			}
			for _, path := range paths {
				bindingValue := workspacepath.WorkspaceNormalize(path)
				instance, createErr := targetExecutorMatrixInstanceCreate(
					matrix.VariableName,
					bindingValue,
					seen,
				)
				if createErr != nil {
					return nil, createErr
				}
				instances = append(instances, instance)
			}
		case ir.MatrixValueGlob:
			if value.Glob == nil {
				return nil, fmt.Errorf("target '%s': matrix glob value is missing configuration", target.Name)
			}
			glob := artifactresolve.ArtifactGlobWithInterpolatedBaseContext(value.Glob, interpCtx)
			paths, err := artifactresolve.ArtifactWalkGlob(helmBaseDir, glob)
			if err != nil {
				return nil, fmt.Errorf("target '%s': matrix glob: %w", target.Name, err)
			}
			for _, path := range paths {
				bindingValue := workspacepath.WorkspaceNormalize(path)
				instance, createErr := targetExecutorMatrixInstanceCreate(
					matrix.VariableName,
					bindingValue,
					seen,
				)
				if createErr != nil {
					return nil, createErr
				}
				instances = append(instances, instance)
			}
		default:
			return nil, fmt.Errorf("target '%s': unknown matrix value kind", target.Name)
		}
	}

	if len(instances) == 0 {
		return nil, fmt.Errorf("target '%s': matrix produced no execution instances", target.Name)
	}

	return instances, nil
}

func targetExecutorMatrixLiteralBindings(
	helmBaseDir string,
	literal string,
	interpCtx expand.InterpolationContext,
) ([]string, error) {
	text := expand.InterpolationContextExpandLiteral(interpCtx, literal)
	if text == "" {
		return nil, fmt.Errorf("matrix literal resolved to empty binding")
	}
	if paths, ok := expand.PathsFromShellParameterList(text); ok {
		bindings := make([]string, 0, len(paths))
		for _, path := range paths {
			bindings = append(bindings, workspacepath.WorkspaceNormalize(path))
		}
		return bindings, nil
	}
	return []string{workspacepath.WorkspaceNormalize(text)}, nil
}

func targetExecutorMatrixInstanceCreate(
	variableName string,
	bindingValue string,
	seen map[string]struct{},
) (TargetMatrixInstance, error) {
	cacheKey := TargetExecutorMatrixCacheKey(variableName, bindingValue)
	if _, exists := seen[cacheKey]; exists {
		return TargetMatrixInstance{}, fmt.Errorf(
			"target matrix: duplicate binding %s",
			cacheKey,
		)
	}
	seen[cacheKey] = struct{}{}

	return TargetMatrixInstance{
		Bindings: map[string]string{variableName: bindingValue},
		CacheKey: cacheKey,
	}, nil
}

func TargetExecutorMatrixCacheKey(variableName, bindingValue string) string {
	return variableName + "=" + bindingValue
}

func targetExecutorEffectiveParameterValues(
	target ir.HelmTarget,
	inv TargetInvocation,
	bindings map[string]string,
) map[string]ir.HelmParameterValue {
	merged := TargetExecutorParametersForTarget(target, inv)
	for key, value := range bindings {
		merged[key] = ir.HelmParameterValue{
			Kind:   ir.HelmParameterScalar,
			Scalar: value,
		}
	}
	return merged
}

func TargetExecutorSortedInstanceKeys(instances []TargetMatrixInstance) []string {
	keys := make([]string, 0, len(instances))
	for _, instance := range instances {
		keys = append(keys, instance.CacheKey)
	}
	sort.Strings(keys)
	return keys
}
