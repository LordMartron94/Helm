package targetexecutor

import (
	"fmt"
	"helm/internal/ir"
)

func TargetExecutorRunTarget(
	target ir.HelmTarget,
	globalVars map[string]string,
	inv TargetInvocation,
	opts TargetExecutorOptions,
) error {
	handler := opts.RunHandler
	if handler == nil {
		handler = TargetExecutorDefaultRunHandler
	}

	parameters := TargetInvocationParameters(inv)

	for stepIndex, step := range target.Steps {
		switch step.Kind {
		case ir.TargetStepRun:
			command := TargetExecutorInterpolateLiteral(step.Run, globalVars, parameters)
			req := TargetRunRequest{
				TargetName: target.Name,
				StepIndex:  stepIndex,
				Command:    command,
				WorkDir:    target.WorkDir,
				Env:        target.Env,
			}
			if err := handler(req); err != nil {
				return err
			}
		case ir.TargetStepWhen:
			if step.When == nil {
				continue
			}
			if !TargetExecutorEvaluateCondition(*step.When, parameters) {
				continue
			}
			for _, runLiteral := range step.When.Runs {
				command := TargetExecutorInterpolateLiteral(runLiteral, globalVars, parameters)
				req := TargetRunRequest{
					TargetName: target.Name,
					StepIndex:  stepIndex,
					Command:    command,
					WorkDir:    target.WorkDir,
					Env:        target.Env,
				}
				if err := handler(req); err != nil {
					return err
				}
			}
		default:
			return fmt.Errorf("target '%s' step %d: unknown step kind", target.Name, stepIndex)
		}
	}

	return nil
}
