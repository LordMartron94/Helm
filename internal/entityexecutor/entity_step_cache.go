package entityexecutor

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

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

	inputPaths := entityStepInputPaths(builtIR.SourceDirectory, step)
	if err := cache.EntityCacheWriteInputPaths(builtIR.SourceDirectory, &buffer, inputPaths); err != nil {
		return 0, err
	}

	manifestPath := entityStepDynamicManifestPath(step.Argv)
	if manifestPath != "" {
		discovered, err := cache.DynamicManifestDiscoveredPathsFromManifestPaths(
			builtIR.SourceDirectory,
			[]string{manifestPath},
		)
		if err != nil {
			return 0, err
		}
		if len(discovered) > 0 {
			buffer.WriteString("dynamic-discovered")
			if err := cache.EntityCacheWriteInputPaths(builtIR.SourceDirectory, &buffer, discovered); err != nil {
				return 0, err
			}
		}
	}

	return cache.EntityCacheHashStateBuffer(buffer.Bytes()), nil
}

func entityStepCacheRecordKey(entityKey, outputPath string) string {
	return entityKey + entityStepCacheKeySeparator + outputPath
}

func entityStepDynamicManifestPath(argv []string) string {
	_, objectPath, manifestPath, ok := entityStepCompileObjectPaths(argv)
	if !ok {
		return ""
	}
	_ = objectPath
	return manifestPath
}

func entityStepCompileObjectPaths(argv []string) (sourcePath, objectPath, manifestPath string, ok bool) {
	if len(argv) < 4 {
		return "", "", "", false
	}
	if filepath.Base(argv[0]) != "compile_object.sh" {
		return "", "", "", false
	}

	sourcePath = argv[1]
	objectPath = argv[2]
	if !strings.HasSuffix(objectPath, ".o") {
		return "", "", "", false
	}
	manifestPath = strings.TrimSuffix(objectPath, ".o") + ".deps"
	return sourcePath, objectPath, manifestPath, true
}

func entityStepInputPaths(workspaceRoot string, step EntityAdapterStep) []string {
	if sourcePath, _, _, ok := entityStepCompileObjectPaths(step.Argv); ok {
		return []string{sourcePath}
	}
	return entityStepArtifactPathsFromArgv(workspaceRoot, step.Argv)
}

func entityStepArtifactPathsFromArgv(workspaceRoot string, argv []string) []string {
	seen := make(map[string]struct{})
	var paths []string
	for _, arg := range argv {
		if !entityStepArgvLooksLikeArtifactPath(arg) {
			continue
		}
		rel, ok := entityStepWorkspaceRelativePath(workspaceRoot, arg)
		if !ok {
			continue
		}
		if _, exists := seen[rel]; exists {
			continue
		}
		seen[rel] = struct{}{}
		paths = append(paths, rel)
	}
	return paths
}

func entityStepArgvLooksLikeArtifactPath(arg string) bool {
	switch {
	case strings.HasSuffix(arg, ".o"),
		strings.HasSuffix(arg, ".a"),
		strings.HasSuffix(arg, ".so"),
		strings.HasSuffix(arg, ".c"):
		return true
	default:
		return false
	}
}

func entityStepWorkspaceRelativePath(workspaceRoot, arg string) (string, bool) {
	if filepath.IsAbs(arg) {
		rel, err := filepath.Rel(workspaceRoot, arg)
		if err != nil {
			return "", false
		}
		arg = rel
	}
	arg = filepath.ToSlash(arg)
	if strings.HasPrefix(arg, "../") || arg == ".." {
		return "", false
	}
	return arg, true
}

func entityStepDynamicBootstrapMiss(workspaceRoot, manifestPath string, outputPaths []string) bool {
	if manifestPath == "" {
		return false
	}
	absManifest := filepath.Join(workspaceRoot, filepath.FromSlash(manifestPath))
	if _, err := os.Stat(absManifest); err == nil {
		return false
	} else if !os.IsNotExist(err) {
		return true
	}
	absOutputs := make([]string, len(outputPaths))
	for index, relPath := range outputPaths {
		absOutputs[index] = entityCacheAbsOutputPath(workspaceRoot, relPath)
	}
	return !entityStepOutputsExist(absOutputs)
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
) (bool, error) {
	if cacheStore == nil || len(step.OutputPaths) == 0 {
		return false, nil
	}

	outputPath := step.OutputPaths[0]
	outputAbs := entityCacheAbsOutputPath(builtIR.SourceDirectory, outputPath)
	if outputAbs == "" {
		return false, nil
	}

	manifestPath := entityStepDynamicManifestPath(step.Argv)
	if entityStepDynamicBootstrapMiss(builtIR.SourceDirectory, manifestPath, step.OutputPaths) {
		return false, nil
	}

	stateFingerprint, err := EntityStepCacheFingerprint(builtIR, entityKey, step, usagePropagation)
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
	if !entityStepInputsExist(builtIR.SourceDirectory, step) {
		return false, nil
	}

	return true, nil
}

func entityStepInputsExist(workspaceRoot string, step EntityAdapterStep) bool {
	inputPaths := entityStepInputPaths(workspaceRoot, step)
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
) error {
	if cacheStore == nil || len(step.OutputPaths) == 0 {
		return nil
	}

	outputPath := step.OutputPaths[0]
	outputAbs := entityCacheAbsOutputPath(builtIR.SourceDirectory, outputPath)
	if outputAbs == "" {
		return nil
	}

	stateFingerprint, err := EntityStepCacheFingerprint(builtIR, entityKey, step, usagePropagation)
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
	entityKey string,
	usagePropagation EntityUsagePropagation,
) error {
	entity, ok := builtIR.Entities[entityKey]
	if !ok {
		return fmt.Errorf("entity '%s' not found", entityKey)
	}

	bag := EntityPropertyBag{}
	for key := range entity.InterfaceBag {
		bag[key] = entityBagFragments(entity, key)
	}
	for _, key := range []string{"CPPFLAGS", "LDFLAGS"} {
		merged := EntityFlattenBags(builtIR, entity.Deps, key)
		if len(merged) > 0 {
			bag["dep:"+key] = merged
		}
	}
	if usagePropagation != nil {
		if usage := usagePropagation[entityKey]; len(usage) > 0 {
			for key, fragments := range usage {
				bag["usage:"+key] = append([]string(nil), fragments...)
			}
		}
	}

	buffer.WriteString(entityKey)
	buffer.WriteByte(0)
	buffer.WriteString(fmt.Sprintf("%d", EntityBagFingerprint(bag)))
	buffer.WriteByte(0)
	buffer.WriteString(entity.AdapterName)
	buffer.WriteByte(0)

	for _, dep := range entity.Deps {
		depKey := ir.HelmLabelCanonical(dep)
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
