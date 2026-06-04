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
		parameters[param.Name] = ir.HelmParameterValue{Kind: ir.HelmParameterScalar}
	}
	return parameters
}

// TargetExecutorResolveInvocationParameters expands HelmParameterValue entries into strings
// for run interpolation, artifact resolution, and cache fingerprints.
func TargetExecutorResolveInvocationParameters(
	helmBaseDir string,
	globals map[string]ir.HelmGlobalVariable,
	values map[string]ir.HelmParameterValue,
) (map[string]string, error) {
	if len(values) == 0 {
		return nil, nil
	}

	scalarGlobals := ir.InterpolationGlobalsFromHelmGlobals(globals)
	resolved := make(map[string]string, len(values))

	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, key := range keys {
		value := values[key]
		switch value.Kind {
		case ir.HelmParameterScalar:
			resolved[key] = expand.ExpandInterpolateLiteral(value.Scalar, scalarGlobals, resolved)
		case ir.HelmParameterGlobalRef:
			text, err := targetExecutorResolveGlobalRefParameter(
				helmBaseDir,
				globals,
				scalarGlobals,
				resolved,
				value.GlobalName,
			)
			if err != nil {
				return nil, fmt.Errorf("parameter '%s': %w", key, err)
			}
			resolved[key] = text
		default:
			return nil, fmt.Errorf("parameter '%s': unknown parameter value kind", key)
		}
	}

	return resolved, nil
}

func targetExecutorResolveGlobalRefParameter(
	helmBaseDir string,
	globals map[string]ir.HelmGlobalVariable,
	scalarGlobals map[string]string,
	parameters map[string]string,
	globalName string,
) (string, error) {
	variable, ok := globals[globalName]
	if !ok {
		return "", fmt.Errorf("references undeclared global '%s'", globalName)
	}

	switch variable.Kind {
	case ir.HelmGlobalVarString:
		return expand.ExpandInterpolateLiteral(variable.StringValue, scalarGlobals, parameters), nil
	case ir.HelmGlobalVarArtifactArray:
		mergedGlobals, err := TargetExecutorInterpolationGlobals(helmBaseDir, globals, parameters)
		if err != nil {
			return "", err
		}
		paths, err := artifactresolve.ArtifactResolveItems(
			helmBaseDir,
			variable.ArtifactItems,
			mergedGlobals,
			parameters,
			false,
		)
		if err != nil {
			return "", err
		}
		return expand.JoinPathsForShell(paths), nil
	default:
		return "", fmt.Errorf("global '%s' has unsupported kind", globalName)
	}
}

func targetExecutorBindDependencyParams(
	globals map[string]ir.HelmGlobalVariable,
	parentResolved map[string]string,
	raw map[string]ir.HelmParameterValue,
) (map[string]ir.HelmParameterValue, error) {
	if len(raw) == 0 {
		return map[string]ir.HelmParameterValue{}, nil
	}

	scalarGlobals := ir.InterpolationGlobalsFromHelmGlobals(globals)
	out := make(map[string]ir.HelmParameterValue, len(raw))

	for key, value := range raw {
		switch value.Kind {
		case ir.HelmParameterScalar:
			out[key] = ir.HelmParameterValue{
				Kind:   ir.HelmParameterScalar,
				Scalar: expand.ExpandInterpolateLiteral(value.Scalar, scalarGlobals, parentResolved),
			}
		case ir.HelmParameterGlobalRef:
			if _, ok := globals[value.GlobalName]; !ok {
				return nil, fmt.Errorf("parameter '%s' references undeclared global '%s'", key, value.GlobalName)
			}
			out[key] = value
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
		}
		buffer.WriteByte(0)
	}
	return buffer.String()
}
