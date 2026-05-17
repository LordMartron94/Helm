package targetexecutor

import (
	"helm/shared"
	"signal"
)

// Programmatic callers collect run output by registering a SignalDispatcher sink
// that reads EXEC_OK / EXEC_FAIL payloads (stdout, stderr, exit_code, command, target).
func targetExecutorEmitRunSignals(
	ctx *signal.SignalContext,
	req TargetRunRequest,
	result TargetRunResult,
	runErr error,
) {
	if ctx == nil {
		return
	}

	builder := signal.SignalContextBuild(ctx, shared.SignalExecOK, "INFO").
		Payload(shared.PhasePayloadKey, shared.TargetExecutionPhase).
		Payload(shared.TargetPayloadKey, req.TargetName).
		Payload(shared.CommandPayloadKey, req.Command).
		Payload(shared.StdoutPayloadKey, result.Stdout).
		Payload(shared.StderrPayloadKey, result.Stderr).
		Payload(shared.ExitCodePayloadKey, result.ExitCode)

	if runErr != nil {
		builder = signal.SignalContextBuild(ctx, shared.SignalExecFail, "ERROR").
			Payload(shared.PhasePayloadKey, shared.TargetExecutionPhase).
			Payload(shared.TargetPayloadKey, req.TargetName).
			Payload(shared.CommandPayloadKey, req.Command).
			Payload(shared.StdoutPayloadKey, result.Stdout).
			Payload(shared.StderrPayloadKey, result.Stderr).
			Payload(shared.ExitCodePayloadKey, result.ExitCode).
			Payload(shared.MessagePayloadKey, runErr.Error())
	}

	builder.Emit()
}
