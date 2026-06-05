package entityexecutor

import (
	"fmt"
	"path/filepath"
	"sort"

	"helm/internal/artifactresolve"
	"helm/internal/expand"
	"helm/internal/ir"
	"helm/internal/workspacepath"
)

// EntityResolvedParameters holds entity adapter invocation parameters after resolution.
type EntityResolvedParameters struct {
	Scalars     map[string]string
	PathLists   map[string][]string
	StringLists map[string][]string
}

func EntityResolvedParametersEmpty() EntityResolvedParameters {
	return EntityResolvedParameters{}
}

func (resolved EntityResolvedParameters) InterpolationContext(globalScalars map[string]string) expand.InterpolationContext {
	scalars := expand.InterpolationContextMergeScalars(nil, globalScalars)
	scalars = expand.InterpolationContextMergeScalars(scalars, resolved.Scalars)

	pathLists := map[string][]string{}
	for key, paths := range resolved.PathLists {
		pathLists[key] = append([]string(nil), paths...)
	}

	return expand.InterpolationContext{
		Scalars:   scalars,
		PathLists: pathLists,
	}
}

// EntityResolveParameters expands entity adapter parameter values for template execution.
func EntityResolveParameters(
	workspaceRoot string,
	builtIR ir.HelmIR,
	entity ir.HelmEntity,
) (EntityResolvedParameters, error) {
	values := entityAdapterParameterValues(builtIR, entity)
	if len(values) == 0 {
		return EntityResolvedParametersEmpty(), nil
	}

	declarationBase := EntityDeclarationBaseDir(workspaceRoot, entity)
	fileGlobals := ir.IRGlobalsForFile(builtIR, entity.SourceFile)
	scalarGlobals := ir.InterpolationGlobalsFromHelmGlobals(fileGlobals)
	resolved := EntityResolvedParameters{
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
			paths, scalar, err := entityResolveGlobalRefParameter(
				workspaceRoot,
				fileGlobals,
				interpCtx(),
				value.GlobalName,
			)
			if err != nil {
				return EntityResolvedParameters{}, fmt.Errorf("parameter '%s': %w", key, err)
			}
			if len(paths) > 0 {
				resolved.PathLists[key] = paths
				continue
			}
			resolved.Scalars[key] = scalar
		case ir.HelmParameterArtifactItems:
			paths, err := artifactresolve.ArtifactResolveItemsContext(
				declarationBase,
				value.ArtifactItems,
				interpCtx(),
				false,
			)
			if err != nil {
				return EntityResolvedParameters{}, fmt.Errorf("parameter '%s': %w", key, err)
			}
			if len(paths) > 0 {
				resolved.PathLists[key] = EntityPathsToWorkspace(workspaceRoot, declarationBase, paths)
			}
		case ir.HelmParameterStringList:
			fragments, err := entityEvaluateStringListExpr(
				builtIR,
				entity,
				value.StringList,
				interpCtx(),
				resolved,
			)
			if err != nil {
				return EntityResolvedParameters{}, fmt.Errorf("parameter '%s': %w", key, err)
			}
			if len(fragments) > 0 {
				resolved.StringLists[key] = fragments
			}
		case ir.HelmParameterDependencyList, ir.HelmParameterTargetParamRef:
			continue
		default:
			return EntityResolvedParameters{}, fmt.Errorf("parameter '%s': unknown parameter value kind", key)
		}
	}

	for key, fragments := range entityInterfaceStringLists(entity) {
		if _, exists := resolved.StringLists[key]; !exists && len(fragments) > 0 {
			resolved.StringLists[key] = fragments
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

func entityAdapterParameterValues(
	builtIR ir.HelmIR,
	entity ir.HelmEntity,
) map[string]ir.HelmParameterValue {
	values := make(map[string]ir.HelmParameterValue, len(entity.Parameters))
	for key, value := range entity.Parameters {
		values[key] = value
	}
	return values
}

func entityInterfaceStringLists(entity ir.HelmEntity) map[string][]string {
	out := make(map[string][]string)
	for key := range entity.InterfaceBag {
		fragments := entityBagFragments(entity, key)
		if len(fragments) > 0 {
			out[key] = fragments
		}
	}
	return out
}

func entityResolveGlobalRefParameter(
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

func entityResolveParameterPaths(
	workspaceRoot string,
	entity ir.HelmEntity,
	paramName string,
	globals map[string]ir.HelmGlobalVariable,
	values map[string]ir.HelmParameterValue,
	interpCtx expand.InterpolationContext,
) ([]string, error) {
	value, ok := values[paramName]
	if !ok {
		return nil, fmt.Errorf("parameter '%s' is not bound", paramName)
	}

	declarationBase := EntityDeclarationBaseDir(workspaceRoot, entity)

	switch value.Kind {
	case ir.HelmParameterGlobalRef:
		paths, _, err := entityResolveGlobalRefParameter(workspaceRoot, globals, interpCtx, value.GlobalName)
		return paths, err
	case ir.HelmParameterScalar:
		text := expand.InterpolationContextExpandLiteral(interpCtx, value.Scalar)
		if text == "" {
			return nil, nil
		}
		if parts, ok := expand.PathsFromShellParameterList(text); ok {
			return EntityPathsToWorkspace(workspaceRoot, declarationBase, parts), nil
		}
		rel, ok := workspacepath.WorkspaceRelative(
			workspaceRoot,
			filepath.Join(declarationBase, filepath.FromSlash(text)),
		)
		if ok {
			return []string{rel}, nil
		}
		return []string{workspacepath.WorkspaceNormalize(text)}, nil
	case ir.HelmParameterArtifactItems:
		paths, err := artifactresolve.ArtifactResolveItemsContext(
			declarationBase,
			value.ArtifactItems,
			interpCtx,
			false,
		)
		if err != nil {
			return nil, err
		}
		return EntityPathsToWorkspace(workspaceRoot, declarationBase, paths), nil
	default:
		return nil, fmt.Errorf("parameter '%s' cannot be resolved to artifact paths", paramName)
	}
}

func entityEvaluateStringListExpr(
	builtIR ir.HelmIR,
	entity ir.HelmEntity,
	expr ir.HelmStringListExpr,
	interpCtx expand.InterpolationContext,
	resolved EntityResolvedParameters,
) ([]string, error) {
	var out []string
	for _, element := range expr {
		switch element.Kind {
		case ir.StringListLiteral:
			text := expand.InterpolationContextExpandLiteral(interpCtx, element.Literal)
			if text != "" {
				out = append(out, text)
			}
		case ir.StringListParamRef:
			fileGlobals := ir.IRGlobalsForFile(builtIR, entity.SourceFile)
			fragment, err := entityResolveParamFragment(
				element.ParamName,
				resolved,
				nil,
				interpCtx,
				fileGlobals,
			)
			if err != nil {
				return nil, err
			}
			out = append(out, fragment...)
		case ir.StringListCollect:
			if element.Collect == nil {
				return nil, fmt.Errorf("collect element is missing call data")
			}
			fragment, err := entityEvaluateCollect(builtIR, entity, *element.Collect)
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
