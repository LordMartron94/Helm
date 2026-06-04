package targetexecutor

import (
	"helm/internal/cache"
	"helm/internal/ir"
	"helm/shared"
	"signal"
)

type targetExecutorCacheDecision struct {
	Skip              bool
	OutputFingerprint uint64
	StateFingerprint  uint64
}

func targetExecutorEvaluateCache(
	builtIR ir.HelmIR,
	targetName string,
	instanceKey string,
	inv TargetInvocation,
	depExecutionNodeIDs []string,
	opts TargetExecutorOptions,
	depStateFingerprints map[string]uint64,
	depOutputFingerprints map[string]uint64,
) (targetExecutorCacheDecision, error) {
	target := builtIR.Targets[targetName]
	decision := targetExecutorCacheDecision{}

	if target.Artifacts == nil {
		return decision, nil
	}

	if target.Artifacts.Volatile || target.Interactive {
		return decision, nil
	}

	if opts.CacheStore == nil {
		return decision, nil
	}

	globalVars, parameters, interpErr := targetExecutorCacheInterpolationGlobals(builtIR, target, inv)
	if interpErr != nil {
		return decision, interpErr
	}

	bootstrapMiss, bootstrapErr := cache.DynamicManifestBootstrapMiss(
		builtIR.SourceDirectory,
		target.Artifacts,
		globalVars,
		parameters,
	)
	if bootstrapErr != nil {
		return decision, bootstrapErr
	}
	if bootstrapMiss {
		return decision, nil
	}

	stateFingerprint, err := cache.CacheFingerprintState(
		builtIR.SourceDirectory,
		target,
		builtIR.Targets,
		globalVars,
		parameters,
		depExecutionNodeIDs,
		depStateFingerprints,
		depOutputFingerprints,
	)
	if err != nil {
		return decision, err
	}
	decision.StateFingerprint = stateFingerprint

	if opts.BypassCache {
		return decision, nil
	}

	record, found, err := cache.TargetCacheStoreGet(opts.CacheStore, targetName, instanceKey)
	if err != nil {
		return decision, err
	}
	if !found {
		return decision, nil
	}

	if record.StateFingerprint != stateFingerprint {
		return decision, nil
	}

	decision.Skip = true
	decision.OutputFingerprint = record.OutputFingerprint
	if staleErr := targetExecutorInvalidateStaleCacheHit(builtIR, target, inv, &decision); staleErr != nil {
		return decision, staleErr
	}

	return decision, nil
}

func targetExecutorCommitCache(
	builtIR ir.HelmIR,
	targetName string,
	instanceKey string,
	inv TargetInvocation,
	depExecutionNodeIDs []string,
	opts TargetExecutorOptions,
	depStateFingerprints map[string]uint64,
	depOutputFingerprints map[string]uint64,
) (uint64, error) {
	target := builtIR.Targets[targetName]
	if target.Artifacts == nil {
		return 0, nil
	}

	outputFingerprint, err := targetExecutorOutputFingerprintAfterRun(builtIR, target, inv)
	if err != nil {
		return 0, err
	}

	if target.Artifacts.Volatile || target.Interactive || opts.CacheStore == nil {
		return outputFingerprint, nil
	}

	globalVars, parameters, interpErr := targetExecutorCacheInterpolationGlobals(builtIR, target, inv)
	if interpErr != nil {
		return 0, interpErr
	}

	stateFingerprint, err := cache.CacheFingerprintState(
		builtIR.SourceDirectory,
		target,
		builtIR.Targets,
		globalVars,
		parameters,
		depExecutionNodeIDs,
		depStateFingerprints,
		depOutputFingerprints,
	)
	if err != nil {
		return 0, err
	}

	record := cache.TargetCacheRecord{
		TargetName:        targetName,
		InstanceKey:       instanceKey,
		StateFingerprint:  stateFingerprint,
		OutputFingerprint: outputFingerprint,
	}

	if err := cache.TargetCacheStorePut(opts.CacheStore, record); err != nil {
		return 0, err
	}

	return outputFingerprint, nil
}

func targetExecutorOutputFingerprintAfterRun(
	builtIR ir.HelmIR,
	target ir.HelmTarget,
	inv TargetInvocation,
) (uint64, error) {
	if target.Artifacts == nil {
		return 0, nil
	}

	if len(target.Artifacts.Outputs) == 0 {
		return 0, nil
	}

	globalVars, parameters, err := targetExecutorCacheInterpolationGlobals(builtIR, target, inv)
	if err != nil {
		return 0, err
	}

	return cache.CacheFingerprintOutput(
		builtIR.SourceDirectory,
		target,
		globalVars,
		parameters,
	)
}

func targetExecutorCacheInterpolationGlobals(
	builtIR ir.HelmIR,
	target ir.HelmTarget,
	inv TargetInvocation,
) (map[string]string, map[string]string, error) {
	paramValues := TargetExecutorParametersForTarget(target, inv)
	resolvedParams, err := TargetExecutorResolveInvocationParameters(
		builtIR.SourceDirectory,
		builtIR.GlobalVariables,
		paramValues,
	)
	if err != nil {
		return nil, nil, err
	}
	globalVars, err := TargetExecutorInterpolationGlobals(
		builtIR.SourceDirectory,
		builtIR.GlobalVariables,
		resolvedParams,
	)
	if err != nil {
		return nil, nil, err
	}
	return globalVars, resolvedParams, nil
}

func targetExecutorEmitCacheSkipSignals(
	ctx *signal.SignalContext,
	targetName string,
	instanceKey string,
	stateFingerprint uint64,
) {
	if ctx == nil {
		return
	}

	builder := signal.SignalContextBuild(ctx, shared.SignalExecSkipped, "INFO").
		Payload(shared.PhasePayloadKey, shared.TargetExecutionPhase).
		Payload(shared.TargetPayloadKey, targetName).
		Payload(shared.ReasonPayloadKey, shared.CacheHitReason).
		Payload(shared.StateFingerprintPayloadKey, stateFingerprint)

	if instanceKey != "" {
		builder = builder.Payload(shared.MatrixInstancePayloadKey, instanceKey)
	}

	builder.Emit()
}

func targetExecutorEmitCacheUpdatedSignals(
	ctx *signal.SignalContext,
	targetName string,
	stateFingerprint uint64,
	outputFingerprint uint64,
) {
	if ctx == nil {
		return
	}

	builder := signal.SignalContextBuild(ctx, shared.SignalCacheUpdated, "INFO").
		Payload(shared.PhasePayloadKey, shared.TargetExecutionPhase).
		Payload(shared.TargetPayloadKey, targetName).
		Payload(shared.StateFingerprintPayloadKey, stateFingerprint)

	if outputFingerprint != 0 {
		builder = builder.Payload(shared.OutputFingerprintPayloadKey, outputFingerprint)
	}

	builder.Emit()
}
