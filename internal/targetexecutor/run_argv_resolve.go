package targetexecutor

import (
	"fmt"
	"helm/internal/artifactresolve"
	"helm/internal/expand"
	"helm/internal/ir"
)

func targetExecutorResolveRunArgv(
	helmBaseDir string,
	template []ir.HelmRunArgvElement,
	globals map[string]ir.HelmGlobalVariable,
	paramValues map[string]ir.HelmParameterValue,
	interpolationGlobals map[string]string,
	resolvedScalars map[string]string,
) ([]string, error) {
	if len(template) == 0 {
		return nil, fmt.Errorf("run argv: empty command array")
	}

	argv := make([]string, 0, len(template))
	for _, element := range template {
		if element.ParamName != "" {
			paths, err := targetExecutorRunArgvParameterPaths(
				helmBaseDir,
				element.ParamName,
				globals,
				paramValues,
				interpolationGlobals,
				resolvedScalars,
			)
			if err != nil {
				return nil, err
			}
			argv = append(argv, paths...)
			continue
		}

		argv = append(argv, expand.ExpandInterpolateLiteral(element.Literal, interpolationGlobals, resolvedScalars))
	}

	if len(argv) == 0 {
		return nil, fmt.Errorf("run argv: command array produced no arguments")
	}

	return argv, nil
}

func targetExecutorRunArgvParameterPaths(
	helmBaseDir string,
	paramName string,
	globals map[string]ir.HelmGlobalVariable,
	paramValues map[string]ir.HelmParameterValue,
	interpolationGlobals map[string]string,
	resolvedScalars map[string]string,
) ([]string, error) {
	value, ok := paramValues[paramName]
	if !ok {
		return nil, fmt.Errorf("run argv: parameter '%s' is not bound", paramName)
	}

	switch value.Kind {
	case ir.HelmParameterGlobalRef:
		return targetExecutorRunArgvPathsFromGlobal(
			helmBaseDir,
			globals,
			value.GlobalName,
			interpolationGlobals,
			resolvedScalars,
		)
	case ir.HelmParameterScalar:
		text := expand.ExpandInterpolateLiteral(value.Scalar, interpolationGlobals, resolvedScalars)
		if text == "" {
			return nil, nil
		}
		if paths, ok := expand.PathsFromShellParameterList(text); ok {
			return paths, nil
		}
		return []string{text}, nil
	case ir.HelmParameterTargetParamRef:
		return nil, fmt.Errorf(
			"run argv: parameter '%s' was not bound (internal error)",
			paramName,
		)
	default:
		return nil, fmt.Errorf(
			"run argv: parameter '%s' cannot be spliced into a command array",
			paramName,
		)
	}
}

func targetExecutorRunArgvPathsFromGlobal(
	helmBaseDir string,
	globals map[string]ir.HelmGlobalVariable,
	globalName string,
	interpolationGlobals map[string]string,
	resolvedScalars map[string]string,
) ([]string, error) {
	variable, ok := globals[globalName]
	if !ok {
		return nil, fmt.Errorf("run argv: references undeclared global '%s'", globalName)
	}

	switch variable.Kind {
	case ir.HelmGlobalVarArtifactArray:
		return artifactresolve.ArtifactResolveItems(
			helmBaseDir,
			variable.ArtifactItems,
			interpolationGlobals,
			resolvedScalars,
			false,
		)
	case ir.HelmGlobalVarString:
		text := expand.ExpandInterpolateLiteral(variable.StringValue, interpolationGlobals, resolvedScalars)
		if text == "" {
			return nil, nil
		}
		if paths, ok := expand.PathsFromShellParameterList(text); ok {
			return paths, nil
		}
		return []string{artifactresolve.ArtifactAnchorPath(helmBaseDir, text)}, nil
	default:
		return nil, fmt.Errorf("run argv: global '%s' has unsupported kind", globalName)
	}
}
