package entityexecutor

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"helm/internal/cache"
	"helm/internal/ir"
)

const entityStepCacheKeySeparator = "#"

// EntityStepCacheFingerprint hashes one adapter spawn step for per-leg caching.
func EntityStepCacheFingerprint(
	builtIR ir.HelmIR,
	entityKey string,
	step EntityAdapterStep,
	usagePropagation EntityUsagePropagation,
	fileCache *cache.FileFingerprintCache,
) (uint64, error) {
	var buffer bytes.Buffer
	if err := entityCacheWriteBaseState(&buffer, builtIR, entityKey, usagePropagation); err != nil {
		return 0, err
	}

	buffer.WriteString("step")
	for _, arg := range step.Argv {
		buffer.WriteString(arg)
		buffer.WriteByte(0)
	}

	entityStepCacheWriteEnv(&buffer, step.Env)

	inputPaths, err := entityStepCacheResolvedInputPaths(builtIR, entityKey, step)
	if err != nil {
		return 0, err
	}
	if err := cache.EntityCacheWriteInputPaths(builtIR.SourceDirectory, &buffer, inputPaths, fileCache); err != nil {
		return 0, err
	}

	if len(step.DynamicManifestPaths) > 0 {
		discovered, discoverErr := cache.DynamicManifestDiscoveredPathsFromManifestPaths(
			builtIR.SourceDirectory,
			step.DynamicManifestPaths,
		)
		if discoverErr != nil {
			return 0, discoverErr
		}
		if len(discovered) > 0 {
			buffer.WriteString("dynamic-discovered")
			if err := cache.EntityCacheWriteInputPaths(builtIR.SourceDirectory, &buffer, discovered, fileCache); err != nil {
				return 0, err
			}
		}
	}

	return cache.EntityCacheHashStateBuffer(buffer.Bytes()), nil
}

func entityStepCacheWriteEnv(buffer *bytes.Buffer, env map[string]string) {
	if len(env) == 0 {
		return
	}

	keys := make([]string, 0, len(env))
	for key := range env {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	buffer.WriteString("env")
	for _, key := range keys {
		buffer.WriteString(key)
		buffer.WriteByte(0)
		buffer.WriteString(env[key])
		buffer.WriteByte(0)
	}
}

func entityStepCacheResolvedInputPaths(builtIR ir.HelmIR, entityKey string, step EntityAdapterStep) ([]string, error) {
	seen := make(map[string]struct{})
	var paths []string

	entityAppendUniquePaths(&paths, seen, step.CacheInputPaths)

	entity, inst, err := entityLookupForInstanceKey(builtIR, entityKey)
	if err != nil {
		return nil, err
	}

	resolved, err := EntityResolveParameters(builtIR.SourceDirectory, builtIR, entity, inst.Configuration)
	if err != nil {
		return nil, err
	}
	if cacheInputs, ok := resolved.PathLists["CACHE_INPUTS"]; ok {
		entityAppendUniquePaths(&paths, seen, cacheInputs)
	}

	sort.Strings(paths)
	return paths, nil
}

func entityStepCacheRecordKey(entityKey, outputPath string) string {
	return entityKey + entityStepCacheKeySeparator + outputPath
}

func entityStepDynamicBootstrapMiss(workspaceRoot string, manifestPaths []string, outputPaths []string) bool {
	if len(manifestPaths) == 0 {
		return false
	}

	absOutputs := make([]string, len(outputPaths))
	for index, relPath := range outputPaths {
		absOutputs[index] = entityCacheAbsOutputPath(workspaceRoot, relPath)
	}
	outputsExist := entityStepOutputsExist(absOutputs)

	for _, manifestPath := range manifestPaths {
		if manifestPath == "" {
			continue
		}
		absManifest := filepath.Join(workspaceRoot, filepath.FromSlash(manifestPath))
		if _, err := os.Stat(absManifest); err == nil {
			continue
		} else if !os.IsNotExist(err) {
			return true
		}
		if !outputsExist {
			return true
		}
	}

	return false
}

func entityStepOutputsExist(outputPaths []string) bool {
	if len(outputPaths) == 0 {
		return false
	}
	for _, path := range outputPaths {
		if path == "" {
			return false
		}
		if _, err := os.Stat(path); err != nil {
			if os.IsNotExist(err) {
				return false
			}
			return false
		}
	}
	return true
}

func entityStepCacheShouldSkip(
	builtIR ir.HelmIR,
	entityKey string,
	step EntityAdapterStep,
	usagePropagation EntityUsagePropagation,
	cacheStore *cache.EntityCacheStore,
	fileCache *cache.FileFingerprintCache,
) (bool, error) {
	if cacheStore == nil || len(step.OutputPaths) == 0 {
		return false, nil
	}

	outputPath := step.OutputPaths[0]
	outputAbs := entityCacheAbsOutputPath(builtIR.SourceDirectory, outputPath)
	if outputAbs == "" {
		return false, nil
	}

	if entityStepDynamicBootstrapMiss(builtIR.SourceDirectory, step.DynamicManifestPaths, step.OutputPaths) {
		return false, nil
	}

	stateFingerprint, err := EntityStepCacheFingerprint(builtIR, entityKey, step, usagePropagation, fileCache)
	if err != nil {
		return false, err
	}

	record, found, err := cache.EntityCacheStoreGet(cacheStore, entityStepCacheRecordKey(entityKey, outputPath))
	if err != nil {
		return false, err
	}
	if !found || record.StateFingerprint != stateFingerprint {
		return false, nil
	}

	outputFingerprint, statErr := entityCacheOutputFingerprint(outputAbs)
	if statErr != nil {
		return false, nil
	}
	if record.OutputFingerprint != outputFingerprint {
		return false, nil
	}

	absOutputs := make([]string, len(step.OutputPaths))
	for index, relPath := range step.OutputPaths {
		absOutputs[index] = entityCacheAbsOutputPath(builtIR.SourceDirectory, relPath)
	}
	if !entityStepOutputsExist(absOutputs) {
		return false, nil
	}
	if !entityStepInputsExist(builtIR.SourceDirectory, step, entityKey, builtIR) {
		return false, nil
	}

	return true, nil
}

func entityStepInputsExist(workspaceRoot string, step EntityAdapterStep, entityKey string, builtIR ir.HelmIR) bool {
	inputPaths, err := entityStepCacheResolvedInputPaths(builtIR, entityKey, step)
	if err != nil {
		return false
	}
	if len(inputPaths) == 0 {
		return true
	}
	for _, relPath := range inputPaths {
		absPath := entityCacheAbsOutputPath(workspaceRoot, relPath)
		if _, err := os.Stat(absPath); err != nil {
			return false
		}
	}
	return true
}

func entityStepCacheRecord(
	builtIR ir.HelmIR,
	entityKey string,
	step EntityAdapterStep,
	usagePropagation EntityUsagePropagation,
	cacheStore *cache.EntityCacheStore,
	fileCache *cache.FileFingerprintCache,
) error {
	if cacheStore == nil || len(step.OutputPaths) == 0 {
		return nil
	}

	outputPath := step.OutputPaths[0]
	outputAbs := entityCacheAbsOutputPath(builtIR.SourceDirectory, outputPath)
	if outputAbs == "" {
		return nil
	}

	stateFingerprint, err := EntityStepCacheFingerprint(builtIR, entityKey, step, usagePropagation, fileCache)
	if err != nil {
		return err
	}

	outputFingerprint, err := entityCacheOutputFingerprint(outputAbs)
	if err != nil {
		return err
	}

	return cache.EntityCacheStorePut(cacheStore, cache.EntityCacheRecord{
		EntityKey:         entityStepCacheRecordKey(entityKey, outputPath),
		StateFingerprint:  stateFingerprint,
		OutputFingerprint: outputFingerprint,
	})
}

func entityCacheWriteBaseState(
	buffer *bytes.Buffer,
	builtIR ir.HelmIR,
	instanceKey string,
	usagePropagation EntityUsagePropagation,
) error {
	entity, inst, err := entityLookupForInstanceKey(builtIR, instanceKey)
	if err != nil {
		return err
	}

	deps, depErr := ir.HelmEntityDepsForConfiguration(entity, inst.Configuration)
	if depErr != nil {
		return depErr
	}

	adapterName, adapterErr := ir.HelmEntityAdapterNameFor(entity, inst.Configuration)
	if adapterErr != nil {
		return adapterErr
	}

	bag := EntityPropertyBag{}
	for key := range entity.InterfaceBag {
		bag[key] = entityBagFragments(builtIR.SourceDirectory, entity, key)
	}
	for key := range entity.InterfaceBag {
		merged := EntityFlattenBags(builtIR, deps, inst.Configuration, key)
		if len(merged) > 0 {
			bag["dep:"+key] = merged
		}
		closureMerged := EntityFlattenBagsClosure(builtIR, deps, inst.Configuration, key)
		if len(closureMerged) > 0 {
			bag["dep-closure:"+key] = closureMerged
		}
	}
	if usagePropagation != nil {
		if usage := usagePropagation[instanceKey]; len(usage) > 0 {
			for key, fragments := range usage {
				bag["usage:"+key] = append([]string(nil), fragments...)
			}
		}
	}

	buffer.WriteString(instanceKey)
	buffer.WriteByte(0)
	buffer.WriteString(fmt.Sprintf("%d", EntityBagFingerprint(bag)))
	buffer.WriteByte(0)
	buffer.WriteString(adapterName)
	buffer.WriteByte(0)

	for _, dep := range deps {
		depInst := ir.HelmEntityDepResolveInstance(dep, inst.Configuration)
		depKey := entityInstanceKey(depInst)
		var depUsage EntityPropertyBag
		if usagePropagation != nil {
			depUsage = usagePropagation[depKey]
		}
		depPlan, expandErr := EntityExpandAdapter(builtIR.SourceDirectory, builtIR, depKey, depUsage)
		if expandErr != nil {
			return expandErr
		}
		depOutput := entityAdapterPrimaryOutputPath(depPlan)
		if depOutput == "" {
			continue
		}
		depOutputFP, fpErr := entityCacheOutputFingerprint(
			filepath.Join(builtIR.SourceDirectory, filepath.FromSlash(depOutput)),
		)
		if fpErr != nil {
			continue
		}
		buffer.WriteString(depKey)
		buffer.WriteString(fmt.Sprintf("%d", depOutputFP))
		buffer.WriteByte(0)
	}

	return nil
}
