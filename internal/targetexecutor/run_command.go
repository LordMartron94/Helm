package targetexecutor

import (
	"fmt"
	"helm/internal/ir"
	"strings"
)

// TargetResolvedRun is one resolved execution step: either a string command (shlex-split at spawn)
// or a native argv slice (passed directly to the OS spawner).
type TargetResolvedRun struct {
	Command string
	Argv    []string
}

func TargetResolvedRunDisplay(step TargetResolvedRun) string {
	if len(step.Argv) > 0 {
		return strings.Join(step.Argv, " ")
	}
	return step.Command
}

func TargetExecutorExportRunSteps(steps []TargetResolvedRun) (commands []string, argvs [][]string) {
	if len(steps) == 0 {
		return nil, nil
	}

	commands = make([]string, len(steps))
	argvs = make([][]string, len(steps))
	hasArgv := false

	for i, step := range steps {
		commands[i] = TargetResolvedRunDisplay(step)
		if len(step.Argv) > 0 {
			argvs[i] = append([]string(nil), step.Argv...)
			hasArgv = true
		}
	}

	if !hasArgv {
		return commands, nil
	}

	return commands, argvs
}

func TargetExecutorResolveRunStep(
	helmBaseDir string,
	command ir.HelmRunCommand,
	globals map[string]ir.HelmGlobalVariable,
	paramValues map[string]ir.HelmParameterValue,
	globalVars map[string]string,
	resolvedParams map[string]string,
) (TargetResolvedRun, error) {
	if ir.HelmRunCommandIsArgv(command) {
		argv, err := targetExecutorResolveRunArgv(
			helmBaseDir,
			command.Argv,
			globals,
			paramValues,
			globalVars,
			resolvedParams,
		)
		if err != nil {
			return TargetResolvedRun{}, err
		}
		return TargetResolvedRun{Argv: argv}, nil
	}

	return TargetResolvedRun{
		Command: TargetExecutorResolveRunCommand(command.String, globalVars, resolvedParams),
	}, nil
}

func targetExecutorResolveTargetRunSteps(
	helmBaseDir string,
	target ir.HelmTarget,
	globals map[string]ir.HelmGlobalVariable,
	paramValues map[string]ir.HelmParameterValue,
	globalVars map[string]string,
	resolvedParams map[string]string,
) ([]TargetResolvedRun, error) {
	steps := make([]TargetResolvedRun, 0, len(target.Steps))

	for _, step := range target.Steps {
		switch step.Kind {
		case ir.TargetStepRun:
			resolved, err := TargetExecutorResolveRunStep(
				helmBaseDir,
				step.Run,
				globals,
				paramValues,
				globalVars,
				resolvedParams,
			)
			if err != nil {
				return nil, fmt.Errorf("target '%s': %w", target.Name, err)
			}
			steps = append(steps, resolved)
		case ir.TargetStepWhen:
			if step.When == nil {
				continue
			}
			if !TargetExecutorEvaluateCondition(*step.When, resolvedParams) {
				continue
			}
			for _, runCommand := range step.When.Runs {
				resolved, err := TargetExecutorResolveRunStep(
					helmBaseDir,
					runCommand,
					globals,
					paramValues,
					globalVars,
					resolvedParams,
				)
				if err != nil {
					return nil, fmt.Errorf("target '%s': %w", target.Name, err)
				}
				steps = append(steps, resolved)
			}
		default:
			return nil, fmt.Errorf("target '%s': unknown step kind", target.Name)
		}
	}

	return steps, nil
}
