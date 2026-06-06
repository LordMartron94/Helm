package targetexecutor

import (
	"fmt"
	"helm/internal/artifactresolve"
	"helm/internal/expand"
	"helm/internal/ir"
	"sort"
	"strings"
)

func TargetInvocationWithScalars(parameters map[string]string) TargetInvocation {
	if len(parameters) == 0 {
		return TargetInvocation{}
	}
	out := make(map[string]ir.HelmParameterValue, len(parameters))
	for key, value := range parameters {
		out[key] = ir.HelmParameterValue{
			Kind:   ir.HelmParameterScalar,
			Scalar: value,
		}
	}
	return TargetInvocation{Parameters: out}
}

func TargetInvocationParameters(inv TargetInvocation) map[string]ir.HelmParameterValue {
	if inv.Parameters == nil {
		return map[string]ir.HelmParameterValue{}
	}
	return inv.Parameters
}

// TargetExecutorParametersForTarget returns invocation parameters with absent optional
// target parameters bound to empty scalar values.
func TargetExecutorParametersForTarget(target ir.HelmTarget, inv TargetInvocation) map[string]ir.HelmParameterValue {
	parameters := make(
		map[string]ir.HelmParameterValue,
		len(target.Parameters)+len(TargetInvocationParameters(inv)),
	)
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
		if param.DependencyList {
			parameters[param.Name] = ir.HelmParameterValue{
				Kind:         ir.HelmParameterDependencyList,
				Dependencies: nil,
			}
			continue
		}
		parameters[param.Name] = ir.HelmParameterValue{Kind: ir.HelmParameterScalar}
	}
	return parameters
}

// TargetExecutorResolveInvocationParameters expands HelmParameterValue entries into structured
// scalars and artifact path lists for run interpolation, matrix expansion, and cache fingerprints.
func TargetExecutorResolveInvocationParameters(
	helmBaseDir string,
	globals map[string]ir.HelmGlobalVariable,
	values map[string]ir.HelmParameterValue,
) (TargetResolvedParameters, error) {
	if len(values) == 0 {
		return TargetResolvedParametersEmpty(), nil
	}

	scalarGlobals := ir.InterpolationGlobalsFromHelmGlobals(globals)
	resolved := TargetResolvedParameters{
		Scalars:     make(map[string]string, len(values)),
		PathLists:   make(map[string][]string),
		StringLists: make(map[string][]string),
	}

	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	interpCtx := func() expand.InterpolationContext {
		return resolved.InterpolationContext(scalarGlobals)
	}

	for _, key := range keys {
		value := values[key]
		switch value.Kind {
		case ir.HelmParameterScalar:
			resolved.Scalars[key] = expand.InterpolationContextExpandLiteral(
				interpCtx(),
				value.Scalar,
			)
		case ir.HelmParameterGlobalRef:
			paths, scalar, err := targetExecutorResolveGlobalRefParameter(
				helmBaseDir,
				globals,
				interpCtx(),
				value.GlobalName,
			)
			if err != nil {
				return TargetResolvedParameters{}, fmt.Errorf("parameter '%s': %w", key, err)
			}
			if len(paths) > 0 {
				resolved.PathLists[key] = paths
				continue
			}
			resolved.Scalars[key] = scalar
		case ir.HelmParameterTargetParamRef:
			return TargetResolvedParameters{}, fmt.Errorf(
				"parameter '%s': caller parameter '%s' was not bound (internal error)",
				key,
				value.TargetParamName,
			)
		case ir.HelmParameterDependencyList:
			continue
		case ir.HelmParameterStringList:
			fragments, err := TargetExecutorEvaluateStringListExpr(
				value.StringList,
				nil,
				values,
				resolved,
				interpCtx(),
			)
			if err != nil {
				return TargetResolvedParameters{}, fmt.Errorf("parameter '%s': %w", key, err)
			}
			if len(fragments) > 0 {
				resolved.StringLists[key] = fragments
			}
		default:
			return TargetResolvedParameters{}, fmt.Errorf("parameter '%s': unknown parameter value kind", key)
		}
	}

	if len(resolved.PathLists) == 0 {
		resolved.PathLists = nil
	}
	if len(resolved.StringLists) == 0 {
		resolved.StringLists = nil
	}

	return resolved, nil
}

func targetExecutorResolveGlobalRefParameter(
	helmBaseDir string,
	globals map[string]ir.HelmGlobalVariable,
	interpCtx expand.InterpolationContext,
	globalName string,
) (paths []string, scalar string, err error) {
	variable, ok := globals[globalName]
	if !ok {
		return nil, "", fmt.Errorf("references undeclared global '%s'", globalName)
	}

	switch variable.Kind {
	case ir.HelmGlobalVarString:
		return nil, expand.InterpolationContextExpandLiteral(interpCtx, variable.StringValue), nil
	case ir.HelmGlobalVarArtifactArray:
		paths, err = artifactresolve.ArtifactResolveItemsContext(
			helmBaseDir,
			variable.ArtifactItems,
			interpCtx,
			false,
		)
		if err != nil {
			return nil, "", err
		}
		return paths, "", nil
	default:
		return nil, "", fmt.Errorf("global '%s' has unsupported kind", globalName)
	}
}

func targetExecutorBindDependencyParams(
	globals map[string]ir.HelmGlobalVariable,
	parentResolved TargetResolvedParameters,
	parentParameters map[string]ir.HelmParameterValue,
	raw map[string]ir.HelmParameterValue,
) (map[string]ir.HelmParameterValue, error) {
	if len(raw) == 0 {
		return map[string]ir.HelmParameterValue{}, nil
	}

	scalarGlobals := ir.InterpolationGlobalsFromHelmGlobals(globals)
	parentInterp := parentResolved.InterpolationContext(scalarGlobals)
	out := make(map[string]ir.HelmParameterValue, len(raw))

	for key, value := range raw {
		switch value.Kind {
		case ir.HelmParameterScalar:
			out[key] = ir.HelmParameterValue{
				Kind:   ir.HelmParameterScalar,
				Scalar: expand.InterpolationContextExpandLiteral(parentInterp, value.Scalar),
			}
		case ir.HelmParameterGlobalRef:
			if _, ok := globals[value.GlobalName]; !ok {
				return nil, fmt.Errorf("parameter '%s' references undeclared global '%s'", key, value.GlobalName)
			}
			out[key] = value
		case ir.HelmParameterTargetParamRef:
			parentValue, ok := parentParameters[value.TargetParamName]
			if !ok {
				return nil, fmt.Errorf(
					"parameter '%s' references caller parameter '%s' which is not in scope",
					key,
					value.TargetParamName,
				)
			}
			switch parentValue.Kind {
			case ir.HelmParameterDependencyList:
				out[key] = parentValue
			case ir.HelmParameterScalar:
				out[key] = ir.HelmParameterValue{
					Kind:   ir.HelmParameterScalar,
					Scalar: expand.InterpolationContextExpandLiteral(parentInterp, parentValue.Scalar),
				}
			case ir.HelmParameterGlobalRef:
				out[key] = parentValue
			case ir.HelmParameterStringList:
				out[key] = parentValue
			default:
				return nil, fmt.Errorf(
					"parameter '%s' cannot forward caller parameter '%s' of that kind",
					key,
					value.TargetParamName,
				)
			}
		case ir.HelmParameterDependencyList:
			out[key] = value
		case ir.HelmParameterStringList:
			out[key] = ir.HelmParameterValue{
				Kind:       ir.HelmParameterStringList,
				StringList: targetExecutorBindStringListLiterals(parentInterp, value.StringList),
			}
		default:
			return nil, fmt.Errorf("parameter '%s': unknown parameter value kind", key)
		}
	}

	return out, nil
}

func targetExecutorParameterMapFingerprint(parameters map[string]ir.HelmParameterValue) string {
	if len(parameters) == 0 {
		return ""
	}

	keys := make([]string, 0, len(parameters))
	for key := range parameters {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var buffer strings.Builder
	for _, key := range keys {
		value := parameters[key]
		buffer.WriteString(key)
		buffer.WriteByte(0)
		buffer.WriteByte(byte(value.Kind))
		buffer.WriteByte(0)
		switch value.Kind {
		case ir.HelmParameterScalar:
			buffer.WriteString(value.Scalar)
		case ir.HelmParameterGlobalRef:
			buffer.WriteString(value.GlobalName)
		case ir.HelmParameterTargetParamRef:
			buffer.WriteString(value.TargetParamName)
		case ir.HelmParameterDependencyList:
			buffer.WriteString("deps:")
			for _, dep := range value.Dependencies {
				buffer.WriteString(dep.TargetName)
				buffer.WriteByte(0)
				buffer.WriteString(targetExecutorParameterMapFingerprint(dep.Parameters))
				buffer.WriteByte(0)
			}
		case ir.HelmParameterStringList:
			for _, element := range value.StringList {
				buffer.WriteByte(byte(element.Kind))
				buffer.WriteString(element.Literal)
				buffer.WriteString(element.ParamName)
				if element.Collect != nil {
					buffer.WriteString(element.Collect.DependenciesParam)
					buffer.WriteString(element.Collect.ExportKey)
				}
			}
		}
		buffer.WriteByte(0)
	}
	return buffer.String()
}
