package entityexecutor

import (
	"fmt"
	"sort"

	"helm/internal/expand"
	"helm/internal/ir"
	"helm/internal/workspacepath"
)

// EntityMatrixInstance is one matrix expansion leg for an adapter.
type EntityMatrixInstance struct {
	Bindings map[string]string
}

// EntityMatrixInstances expands an adapter matrix using resolved entity parameters.
func EntityMatrixInstances(
	workspaceRoot string,
	entity ir.HelmEntity,
	matrix *ir.HelmMatrix,
	values map[string]ir.HelmParameterValue,
	globals map[string]ir.HelmGlobalVariable,
	interpCtx expand.InterpolationContext,
) ([]EntityMatrixInstance, error) {
	if matrix == nil {
		return []EntityMatrixInstance{{Bindings: map[string]string{}}}, nil
	}

	var instances []EntityMatrixInstance
	seen := make(map[string]struct{})

	for _, value := range matrix.Values {
		switch value.Kind {
		case ir.MatrixValueLiteral:
			bindingValue := workspacepath.WorkspaceNormalize(
				expand.InterpolationContextExpandLiteral(interpCtx, value.Literal),
			)
			instance, err := entityMatrixInstanceCreate(matrix.VariableName, bindingValue, seen)
			if err != nil {
				return nil, err
			}
			instances = append(instances, instance)
		case ir.MatrixValueParameterRef:
			paths, err := entityResolveParameterPaths(
				workspaceRoot,
				entity,
				value.ParameterName,
				globals,
				values,
				interpCtx,
			)
			if err != nil {
				return nil, fmt.Errorf("matrix parameter '%s': %w", value.ParameterName, err)
			}
			if len(paths) == 0 {
				return nil, fmt.Errorf("matrix parameter '%s' produced no paths", value.ParameterName)
			}
			for _, path := range paths {
				instance, createErr := entityMatrixInstanceCreate(
					matrix.VariableName,
					workspacepath.WorkspaceNormalize(path),
					seen,
				)
				if createErr != nil {
					return nil, createErr
				}
				instances = append(instances, instance)
			}
		case ir.MatrixValueGlob:
			return nil, fmt.Errorf("matrix glob values are not supported in entity adapters")
		}
	}

	if len(instances) == 0 {
		return nil, fmt.Errorf("matrix produced no instances")
	}
	return instances, nil
}

func entityMatrixInstanceCreate(
	variableName string,
	bindingValue string,
	seen map[string]struct{},
) (EntityMatrixInstance, error) {
	if bindingValue == "" {
		return EntityMatrixInstance{}, fmt.Errorf("matrix binding for '%s' is empty", variableName)
	}
	if _, exists := seen[bindingValue]; exists {
		return EntityMatrixInstance{}, fmt.Errorf("matrix: duplicate binding %q", bindingValue)
	}
	seen[bindingValue] = struct{}{}
	return EntityMatrixInstance{
		Bindings: map[string]string{variableName: bindingValue},
	}, nil
}

func entityMatrixInstanceKeys(instances []EntityMatrixInstance) []string {
	if len(instances) == 0 {
		return nil
	}
	keys := make([]string, 0, len(instances))
	for _, instance := range instances {
		for _, value := range instance.Bindings {
			keys = append(keys, value)
		}
	}
	sort.Strings(keys)
	return keys
}
