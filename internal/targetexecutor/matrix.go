package targetexecutor

import (
	"fmt"
	"helm/internal/artifactresolve"
	"helm/internal/ir"
	"path/filepath"
	"sort"
	"strings"
)

type TargetMatrixInstance struct {
	Bindings map[string]string
	CacheKey string
}

func TargetExecutorMatrixInstances(
	helmBaseDir string,
	target ir.HelmTarget,
	globalVars map[string]string,
	invocationParams map[string]string,
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
			instance, err := targetExecutorMatrixInstanceCreate(
				matrix.VariableName,
				value.Literal,
				seen,
			)
			if err != nil {
				return nil, err
			}
			instances = append(instances, instance)
		case ir.MatrixValueGlob:
			if value.Glob == nil {
				return nil, fmt.Errorf("target '%s': matrix glob value is missing configuration", target.Name)
			}
			glob := artifactresolve.ArtifactGlobWithInterpolatedBase(value.Glob, globalVars, invocationParams)
			paths, err := artifactresolve.ArtifactWalkGlob(helmBaseDir, glob)
			if err != nil {
				return nil, fmt.Errorf("target '%s': matrix glob: %w", target.Name, err)
			}
			for _, path := range paths {
				bindingValue := targetExecutorMatrixBindingPath(helmBaseDir, path)
				instance, err := targetExecutorMatrixInstanceCreate(
					matrix.VariableName,
					bindingValue,
					seen,
				)
				if err != nil {
					return nil, err
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

func targetExecutorMatrixBindingPath(helmBaseDir, absolutePath string) string {
	if helmBaseDir == "" {
		return absolutePath
	}
	rel, err := filepath.Rel(helmBaseDir, absolutePath)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return absolutePath
	}
	return rel
}

func TargetExecutorEffectiveParameters(
	target ir.HelmTarget,
	inv TargetInvocation,
	bindings map[string]string,
) map[string]string {
	merged := TargetExecutorParametersForTarget(target, inv)
	for key, value := range bindings {
		merged[key] = value
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
