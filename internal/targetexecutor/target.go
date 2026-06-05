package targetexecutor

import (
	"helm/internal/ir"
	"io"
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

	workDir, steps, err := TargetExecutorResolveTargetRuns(helmBaseDir, target, globals, inv)
	if err != nil {
		return err
	}

	paramValues := TargetExecutorParametersForTarget(target, inv)
	resolved, err := TargetExecutorResolveInvocationParameters(helmBaseDir, globals, paramValues)
	if err != nil {
		return err
	}

	baseInterpCtx, err := TargetExecutorInterpolationGlobals(helmBaseDir, globals, resolved)
	if err != nil {
		return err
	}
	interpCtx, err := TargetExecutorEvaluateLetBindings(helmBaseDir, target, baseInterpCtx)
	if err != nil {
		return err
	}

	env := targetExecutorInterpolateEnv(target.Env, interpCtx)

	for stepIndex, step := range steps {
		req := targetExecutorRunRequestCreate(target.Name, stepIndex, step, workDir, env, target.Interactive, opts)
		if err := targetExecutorInvokeRun(handler, req, opts); err != nil {
			return err
		}
	}

	return nil
}

func targetExecutorRunRequestCreate(
	targetName string,
	stepIndex int,
	step TargetResolvedRun,
	workDir string,
	env map[string]string,
	interactive bool,
	opts TargetExecutorOptions,
) TargetRunRequest {
	req := TargetRunRequest{
		TargetName:  targetName,
		StepIndex:   stepIndex,
		Command:     TargetResolvedRunDisplay(step),
		Argv:        step.Argv,
		WorkDir:     workDir,
		Env:         env,
		Interactive: interactive,
	}

	if !interactive && opts.StreamRunOutput {
		req.LiveStdout = opts.StreamStdout
		if req.LiveStdout == nil {
			req.LiveStdout = os.Stdout
		}

		req.LiveStderr = opts.StreamStderr
		if req.LiveStderr == nil {
			req.LiveStderr = os.Stderr
		}
	}

	if opts.RunTranscript != nil && opts.TranscriptNodeID != "" && !interactive {
		if req.LiveStdout != nil {
			req.LiveStdout = io.MultiWriter(
				req.LiveStdout,
				opts.RunTranscript.StdoutWriter(opts.TranscriptNodeID),
			)
		} else {
			req.LiveStdout = opts.RunTranscript.StdoutWriter(opts.TranscriptNodeID)
		}
		if req.LiveStderr != nil {
			req.LiveStderr = io.MultiWriter(
				req.LiveStderr,
				opts.RunTranscript.StderrWriter(opts.TranscriptNodeID),
			)
		} else {
			req.LiveStderr = opts.RunTranscript.StderrWriter(opts.TranscriptNodeID)
		}
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
	if err == nil {
		return nil
	}
	if result.ExitCode != 0 {
		return TargetExecutorProcessExitError{
			Target:   req.TargetName,
			ExitCode: result.ExitCode,
			Err:      err,
		}
	}
	return err
}
