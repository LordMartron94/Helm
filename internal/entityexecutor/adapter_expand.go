package entityexecutor

import (
	"fmt"
	"path/filepath"
	"strings"

	"helm/internal/artifactresolve"
	"helm/internal/expand"
	"helm/internal/ir"
	"helm/internal/workspacepath"
)

// EntityAdapterStep is one spawn request produced by adapter template expansion.
type EntityAdapterStep struct {
	Argv        []string
	Env         map[string]string
	OutputPaths []string
}

// EntityAdapterPlan is the full build plan for one entity (adapter-driven).
type EntityAdapterPlan struct {
	EntityKey     string
	Steps         []EntityAdapterStep
	PrimaryOutput string
	SourcePaths   []string
}

// EntityExpandAdapter resolves adapter templates into concrete spawn steps.
func EntityExpandAdapter(
	helmBaseDir string,
	builtIR ir.HelmIR,
	entityKey string,
) (EntityAdapterPlan, error) {
	entity, ok := builtIR.Entities[entityKey]
	if !ok {
		return EntityAdapterPlan{}, fmt.Errorf("entity '%s' not found", entityKey)
	}

	adapter, ok := builtIR.Adapters[entity.AdapterName]
	if !ok {
		return EntityAdapterPlan{}, fmt.Errorf("entity '%s': unknown adapter '%s'", entityKey, entity.AdapterName)
	}

	resolved, err := EntityResolveParameters(helmBaseDir, builtIR, entity)
	if err != nil {
		return EntityAdapterPlan{}, fmt.Errorf("entity '%s': %w", entityKey, err)
	}

	fileGlobals := ir.IRGlobalsForFile(builtIR, entity.SourceFile)
	scalarGlobals := ir.InterpolationGlobalsFromHelmGlobals(fileGlobals)
	interpCtx := resolved.InterpolationContext(scalarGlobals)
	paramValues := entityAdapterParameterValues(builtIR, entity)

	if ir.HelmAdapterDeclUsesPhases(adapter) {
		return entityExpandAdapterPhases(
			helmBaseDir,
			builtIR,
			entityKey,
			entity,
			adapter,
			resolved,
			interpCtx,
			fileGlobals,
			paramValues,
		)
	}

	plan := EntityAdapterPlan{
		EntityKey:   entityKey,
		SourcePaths: entityResolvedSourcePaths(resolved),
	}

	var matrixOutputs []string
	if adapter.Matrix != nil {
		instances, matrixErr := EntityMatrixInstances(
			helmBaseDir,
			entity,
			adapter.Matrix,
			paramValues,
			fileGlobals,
			interpCtx,
		)
		if matrixErr != nil {
			return EntityAdapterPlan{}, fmt.Errorf("entity '%s': %w", entityKey, matrixErr)
		}

		for _, instance := range instances {
			legCtx := interpCtx
			legCtx.Scalars = entityCopyScalars(interpCtx.Scalars)
			for key, value := range instance.Bindings {
				legCtx.Scalars[key] = value
			}

			legOutputs, outputErr := entityExpandOutputPaths(
				helmBaseDir,
				adapter.MatrixLegOutputs,
				legCtx,
			)
			if outputErr != nil {
				return EntityAdapterPlan{}, fmt.Errorf("entity '%s': %w", entityKey, outputErr)
			}
			matrixOutputs = append(matrixOutputs, legOutputs...)

			for _, runTemplate := range adapter.MatrixRuns {
				argv, argvErr := entityResolveRunArgv(
					helmBaseDir,
					builtIR,
					entity,
					adapter.Parameters,
					runTemplate.Argv,
					legCtx,
					resolved,
					nil,
					fileGlobals,
				)
				if argvErr != nil {
					return EntityAdapterPlan{}, fmt.Errorf("entity '%s': %w", entityKey, argvErr)
				}
				plan.Steps = append(plan.Steps, EntityAdapterStep{
					Argv:        argv,
					OutputPaths: legOutputs,
				})
			}
		}
	}

	linkEnv, envErr := entityResolveEnv(builtIR, entity, adapter.Parameters, adapter.Env, interpCtx, resolved)
	if envErr != nil {
		return EntityAdapterPlan{}, fmt.Errorf("entity '%s': %w", entityKey, envErr)
	}

	legacyPhaseOutputs := entityLegacyPhaseOutputs(matrixOutputs)
	for _, runTemplate := range adapter.Runs {
		argv, argvErr := entityResolveRunArgv(
			helmBaseDir,
			builtIR,
			entity,
			adapter.Parameters,
			runTemplate.Argv,
			interpCtx,
			resolved,
			legacyPhaseOutputs,
			fileGlobals,
		)
		if argvErr != nil {
			return EntityAdapterPlan{}, fmt.Errorf("entity '%s': %w", entityKey, argvErr)
		}
		step := EntityAdapterStep{Argv: argv, Env: linkEnv}
		if len(adapter.Outputs) > 0 {
			primaryOutputs, outputErr := entityExpandOutputPaths(helmBaseDir, adapter.Outputs, interpCtx)
			if outputErr != nil {
				return EntityAdapterPlan{}, fmt.Errorf("entity '%s': %w", entityKey, outputErr)
			}
			step.OutputPaths = primaryOutputs
			if plan.PrimaryOutput == "" && len(primaryOutputs) > 0 {
				plan.PrimaryOutput = primaryOutputs[0]
			}
		}
		plan.Steps = append(plan.Steps, step)
	}

	if plan.PrimaryOutput == "" && len(adapter.Outputs) > 0 {
		primaryOutputs, outputErr := entityExpandOutputPaths(helmBaseDir, adapter.Outputs, interpCtx)
		if outputErr != nil {
			return EntityAdapterPlan{}, fmt.Errorf("entity '%s': %w", entityKey, outputErr)
		}
		if len(primaryOutputs) > 0 {
			plan.PrimaryOutput = primaryOutputs[0]
		}
	}

	return plan, nil
}

func entityResolvedSourcePaths(resolved EntityResolvedParameters) []string {
	var paths []string
	for _, list := range resolved.PathLists {
		paths = append(paths, list...)
	}
	return paths
}

func entityCopyScalars(scalars map[string]string) map[string]string {
	out := make(map[string]string, len(scalars))
	for key, value := range scalars {
		out[key] = value
	}
	return out
}

func entityExpandOutputPaths(
	helmBaseDir string,
	outputs []ir.HelmArtifactInput,
	interpCtx expand.InterpolationContext,
) ([]string, error) {
	if len(outputs) == 0 {
		return nil, nil
	}
	return artifactresolve.ArtifactResolveItemsContext(helmBaseDir, outputs, interpCtx, false)
}

func entityResolveRunArgv(
	helmBaseDir string,
	builtIR ir.HelmIR,
	entity ir.HelmEntity,
	adapterParams []ir.HelmTargetParameter,
	template []ir.HelmRunArgvElement,
	interpCtx expand.InterpolationContext,
	resolved EntityResolvedParameters,
	phaseOutputs map[string][]string,
	fileGlobals map[string]ir.HelmGlobalVariable,
) ([]string, error) {
	if len(template) == 0 {
		return nil, fmt.Errorf("run argv: empty command array")
	}

	argv := make([]string, 0, len(template))
	for _, element := range template {
		switch {
		case element.Collect != nil:
			fragment, err := entityEvaluateCollect(builtIR, entity, *element.Collect)
			if err != nil {
				return nil, err
			}
			argv = append(argv, fragment...)
		case element.PhaseOutputs != "":
			outputs, ok := phaseOutputs[element.PhaseOutputs]
			if !ok || len(outputs) == 0 {
				return nil, fmt.Errorf("run argv: phase '%s' produced no outputs", element.PhaseOutputs)
			}
			argv = append(argv, outputs...)
		case element.ParamName != "":
			fragment, err := entityResolveParamFragment(
				element.ParamName,
				adapterParams,
				resolved,
				phaseOutputs,
				interpCtx,
				fileGlobals,
			)
			if err != nil {
				return nil, err
			}
			argv = append(argv, fragment...)
		case element.AbsPath != "":
			rel := expand.InterpolationContextExpandLiteral(interpCtx, element.AbsPath)
			argv = append(argv, workspacepath.WorkspaceAnchor(helmBaseDir, rel))
		default:
			argv = append(argv, expand.InterpolationContextExpandLiteral(interpCtx, element.Literal))
		}
	}

	if len(argv) == 0 {
		return nil, fmt.Errorf("run argv: command array produced no arguments")
	}
	return argv, nil
}

func entityResolveParamFragment(
	paramName string,
	adapterParams []ir.HelmTargetParameter,
	resolved EntityResolvedParameters,
	phaseOutputs map[string][]string,
	interpCtx expand.InterpolationContext,
	fileGlobals map[string]ir.HelmGlobalVariable,
) ([]string, error) {
	if paramName == "MATRIX_OUTPUTS" {
		var flattened []string
		for _, outputs := range phaseOutputs {
			flattened = append(flattened, outputs...)
		}
		if len(flattened) == 0 {
			return nil, fmt.Errorf("run argv: MATRIX_OUTPUTS is empty")
		}
		return flattened, nil
	}
	if fragments, ok := entityGlobalStringListFragments(fileGlobals, paramName); ok {
		return fragments, nil
	}
	if fragments, ok := resolved.StringLists[paramName]; ok {
		return append([]string(nil), fragments...), nil
	}
	if paths, ok := resolved.PathLists[paramName]; ok {
		return append([]string(nil), paths...), nil
	}
	if scalar, ok := resolved.Scalars[paramName]; ok && scalar != "" {
		return entityAssignmentFragments(scalar), nil
	}
	if scalar, ok := interpCtx.Scalars[paramName]; ok && scalar != "" {
		return entityAssignmentFragments(scalar), nil
	}
	if entityAdapterParameterOptional(adapterParams, paramName) {
		return nil, nil
	}
	return nil, fmt.Errorf("run argv: undeclared parameter '%s'", paramName)
}

func entityAdapterParameterOptional(adapterParams []ir.HelmTargetParameter, paramName string) bool {
	for _, parameter := range adapterParams {
		if parameter.Name == paramName {
			return parameter.Optional
		}
	}
	return false
}

func entityLegacyPhaseOutputs(matrixOutputs []string) map[string][]string {
	if len(matrixOutputs) == 0 {
		return nil
	}
	return map[string][]string{"_matrix": matrixOutputs}
}

func entityGlobalStringListFragments(
	globals map[string]ir.HelmGlobalVariable,
	name string,
) ([]string, bool) {
	variable, ok := globals[name]
	if !ok {
		return nil, false
	}
	if variable.Kind == ir.HelmGlobalVarStringList {
		return append([]string(nil), variable.StringList...), true
	}
	if variable.Kind != ir.HelmGlobalVarArtifactArray {
		return nil, false
	}
	fragments := make([]string, 0, len(variable.ArtifactItems))
	for _, item := range variable.ArtifactItems {
		if item.Kind != ir.ArtifactInputString || item.Literal == "" {
			return nil, false
		}
		fragments = append(fragments, item.Literal)
	}
	if len(fragments) == 0 {
		return nil, false
	}
	return fragments, true
}

func entityEvaluateCollect(
	builtIR ir.HelmIR,
	entity ir.HelmEntity,
	collect ir.HelmCollectExpr,
) ([]string, error) {
	return EntityFlattenBags(builtIR, entity.Deps, collect.ExportKey), nil
}

func entityResolveEnv(
	builtIR ir.HelmIR,
	entity ir.HelmEntity,
	adapterParams []ir.HelmTargetParameter,
	envDecl map[string]ir.HelmStringListExpr,
	interpCtx expand.InterpolationContext,
	resolved EntityResolvedParameters,
) (map[string]string, error) {
	if len(envDecl) == 0 {
		return nil, nil
	}

	out := make(map[string]string, len(envDecl))
	for key, expr := range envDecl {
		fragments, err := entityEvaluateStringListExpr(
			builtIR,
			entity,
			adapterParams,
			expr,
			interpCtx,
			resolved,
		)
		if err != nil {
			return nil, fmt.Errorf("env %s: %w", key, err)
		}
		if len(fragments) > 0 {
			out[key] = stringsJoinFlags(fragments)
		}
	}
	return out, nil
}

func entityAssignmentFragments(text string) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	if parts, ok := expand.PathsFromShellParameterList(text); ok {
		normalized := make([]string, len(parts))
		for i, part := range parts {
			normalized[i] = workspacepath.WorkspaceNormalize(part)
		}
		return normalized
	}
	return []string{workspacepath.WorkspaceNormalize(text)}
}

func entityAdapterPrimaryOutputPath(plan EntityAdapterPlan) string {
	if plan.PrimaryOutput != "" {
		return plan.PrimaryOutput
	}
	for _, step := range plan.Steps {
		if len(step.OutputPaths) > 0 {
			return step.OutputPaths[len(step.OutputPaths)-1]
		}
	}
	return ""
}

func entityAdapterMkdirPaths(paths []string) error {
	seen := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		if path == "" {
			continue
		}
		dir := filepath.Dir(path)
		if _, exists := seen[dir]; exists {
			continue
		}
		seen[dir] = struct{}{}
		if err := EntityExecutorMkdirOutput(path); err != nil {
			return err
		}
	}
	return nil
}
