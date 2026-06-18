package entityexecutor

import (
	"fmt"

	"helm/internal/expand"
	"helm/internal/ir"
)

func entityExpandAdapterPhases(
	helmBaseDir string,
	builtIR ir.HelmIR,
	entityKey string,
	entity ir.HelmEntity,
	adapter ir.HelmAdapterDecl,
	configuration string,
	resolved EntityResolvedParameters,
	interpCtx expand.InterpolationContext,
	fileGlobals map[string]ir.HelmGlobalVariable,
	paramValues map[string]ir.HelmParameterValue,
) (EntityAdapterPlan, error) {
	phases, err := ir.AdapterPhaseTopoOrder(adapter.Phases)
	if err != nil {
		return EntityAdapterPlan{}, fmt.Errorf("entity '%s': %w", entityKey, err)
	}

	plan := EntityAdapterPlan{
		EntityKey:   entityKey,
		SourcePaths: entityResolvedSourcePaths(resolved),
	}
	phaseOutputs := make(map[string][]string)

	for _, phase := range phases {
		linkEnv, envErr := entityResolveEnv(builtIR, entity, configuration, adapter.Parameters, phase.Env, interpCtx, resolved)
		if envErr != nil {
			return EntityAdapterPlan{}, fmt.Errorf("entity '%s': %w", entityKey, envErr)
		}

		if phase.Matrix != nil {
			instances, matrixErr := EntityMatrixInstances(
				helmBaseDir,
				entity,
				phase.Matrix,
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
					phase.MatrixLegOutputs,
					legCtx,
				)
				if outputErr != nil {
					return EntityAdapterPlan{}, fmt.Errorf("entity '%s': %w", entityKey, outputErr)
				}
				phaseOutputs[phase.Name] = append(phaseOutputs[phase.Name], legOutputs...)

				for _, runTemplate := range phase.MatrixRuns {
					argv, argvErr := entityResolveRunArgv(
						helmBaseDir,
						builtIR,
						entity,
						configuration,
						adapter.Parameters,
						runTemplate.Argv,
						legCtx,
						resolved,
						phaseOutputs,
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
			continue
		}

		for _, runTemplate := range phase.Runs {
			argv, argvErr := entityResolveRunArgv(
				helmBaseDir,
				builtIR,
				entity,
				configuration,
				adapter.Parameters,
				runTemplate.Argv,
				interpCtx,
				resolved,
				phaseOutputs,
				fileGlobals,
			)
			if argvErr != nil {
				return EntityAdapterPlan{}, fmt.Errorf("entity '%s': %w", entityKey, argvErr)
			}
			step := EntityAdapterStep{Argv: argv, Env: linkEnv}
			if len(phase.Outputs) > 0 {
				primaryOutputs, outputErr := entityExpandOutputPaths(helmBaseDir, phase.Outputs, interpCtx)
				if outputErr != nil {
					return EntityAdapterPlan{}, fmt.Errorf("entity '%s': %w", entityKey, outputErr)
				}
				step.OutputPaths = primaryOutputs
				phaseOutputs[phase.Name] = append(phaseOutputs[phase.Name], primaryOutputs...)
				if plan.PrimaryOutput == "" && len(primaryOutputs) > 0 {
					plan.PrimaryOutput = primaryOutputs[0]
				}
			}
			plan.Steps = append(plan.Steps, step)
		}
	}

	if plan.PrimaryOutput == "" {
		for index := len(phases) - 1; index >= 0; index-- {
			outputs := phaseOutputs[phases[index].Name]
			if len(outputs) > 0 {
				plan.PrimaryOutput = outputs[len(outputs)-1]
				break
			}
		}
	}

	return plan, nil
}
