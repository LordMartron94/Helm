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
					runTemplate.Argv,
					legCtx,
					resolved,
					nil,
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

	linkEnv, envErr := entityResolveEnv(builtIR, entity, adapter.Env, interpCtx, resolved)
	if envErr != nil {
		return EntityAdapterPlan{}, fmt.Errorf("entity '%s': %w", entityKey, envErr)
	}

	for _, runTemplate := range adapter.Runs {
		argv, argvErr := entityResolveRunArgv(
			helmBaseDir,
			builtIR,
			entity,
			runTemplate.Argv,
			interpCtx,
			resolved,
			matrixOutputs,
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
	template []ir.HelmRunArgvElement,
	interpCtx expand.InterpolationContext,
	resolved EntityResolvedParameters,
	matrixOutputs []string,
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
		case element.ParamName != "":
			fragment, err := entityResolveParamFragment(
				element.ParamName,
				resolved,
				matrixOutputs,
				interpCtx,
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
	resolved EntityResolvedParameters,
	matrixOutputs []string,
	interpCtx expand.InterpolationContext,
) ([]string, error) {
	if paramName == "MATRIX_OUTPUTS" {
		return append([]string(nil), matrixOutputs...), nil
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
	return nil, fmt.Errorf("run argv: undeclared parameter '%s'", paramName)
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
	envDecl map[string]ir.HelmStringListExpr,
	interpCtx expand.InterpolationContext,
	resolved EntityResolvedParameters,
) (map[string]string, error) {
	if len(envDecl) == 0 {
		return nil, nil
	}

	out := make(map[string]string, len(envDecl))
	for key, expr := range envDecl {
		fragments, err := entityEvaluateStringListExpr(builtIR, entity, expr, interpCtx, resolved)
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
