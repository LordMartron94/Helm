package targetexecutor

import (
	"fmt"
	"helm/internal/ir"
	"os"
	"time"
)

func TargetExecutorRunTarget(
	helmBaseDir string,
	target ir.HelmTarget,
	globals map[string]ir.HelmGlobalVariable,
	inv TargetInvocation,
	opts TargetExecutorOptions,
) error {
	handler := opts.RunHandler
	if handler == nil {
		handler = TargetExecutorDefaultRunHandler
	}

	parameters := TargetExecutorParametersForTarget(target, inv)
	globalVars, err := TargetExecutorInterpolationGlobals(helmBaseDir, globals, parameters)
	if err != nil {
		return err
	}

	workDir := TargetExecutorInterpolateLiteral(target.WorkDir, globalVars, parameters)
	env := targetExecutorInterpolateEnv(target.Env, globalVars, parameters)

	for stepIndex, step := range target.Steps {
		switch step.Kind {
		case ir.TargetStepRun:
			command := TargetExecutorInterpolateLiteral(step.Run, globalVars, parameters)
			req := targetExecutorRunRequestCreate(target.Name, stepIndex, command, workDir, env, target.Interactive, opts)
			if err := targetExecutorInvokeRun(handler, req, opts); err != nil {
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
				req := targetExecutorRunRequestCreate(target.Name, stepIndex, command, workDir, env, target.Interactive, opts)
				if err := targetExecutorInvokeRun(handler, req, opts); err != nil {
					return err
				}
			}
		default:
			return fmt.Errorf("target '%s' step %d: unknown step kind", target.Name, stepIndex)
		}
	}

	return nil
}

func targetExecutorRunRequestCreate(
	targetName string,
	stepIndex int,
	command string,
	workDir string,
	env map[string]string,
	interactive bool,
	opts TargetExecutorOptions,
) TargetRunRequest {
	req := TargetRunRequest{
		TargetName:  targetName,
		StepIndex:   stepIndex,
		Command:     command,
		WorkDir:     workDir,
		Env:         env,
		Interactive: interactive,
	}

	if interactive || !opts.StreamRunOutput {
		return req
	}

	req.LiveStdout = opts.StreamStdout
	if req.LiveStdout == nil {
		req.LiveStdout = os.Stdout
	}

	req.LiveStderr = opts.StreamStderr
	if req.LiveStderr == nil {
		req.LiveStderr = os.Stderr
	}

	return req
}

func targetExecutorInvokeRun(
	handler TargetRunHandler,
	req TargetRunRequest,
	opts TargetExecutorOptions,
) error {
	startedAt := time.Now()
	result, err := handler(req)
	if opts.SignalContext != nil {
		targetExecutorEmitRunSignals(opts.SignalContext, req, result, err, time.Since(startedAt))
	}
	return err
}
