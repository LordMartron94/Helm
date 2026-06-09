package entityexecutor

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"helm/internal/cache"
	"helm/internal/ir"
)

// EntityCacheFingerprint hashes entity inputs, dependency outputs, and adapter state.
func EntityCacheFingerprint(
	builtIR ir.HelmIR,
	entityKey string,
	outPath string,
	sourcePaths []string,
	usagePropagation EntityUsagePropagation,
) (uint64, error) {
	entity, ok := builtIR.Entities[entityKey]
	if !ok {
		return 0, fmt.Errorf("entity '%s' not found", entityKey)
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

	var buffer bytes.Buffer
	buffer.WriteString(entityKey)
	buffer.WriteByte(0)
	buffer.WriteString(fmt.Sprintf("%d", EntityBagFingerprint(bag)))
	buffer.WriteByte(0)
	buffer.WriteString(entity.AdapterName)
	buffer.WriteByte(0)

	inputPaths := EntityCacheInputPaths(builtIR, entityKey, sourcePaths)
	if err := cache.EntityCacheWriteInputPaths(builtIR.SourceDirectory, &buffer, inputPaths); err != nil {
		return 0, err
	}

	for _, dep := range entity.Deps {
		depKey := ir.HelmLabelCanonical(dep)
		var depUsage EntityPropertyBag
		if usagePropagation != nil {
			depUsage = usagePropagation[depKey]
		}
		depPlan, expandErr := EntityExpandAdapter(builtIR.SourceDirectory, builtIR, depKey, depUsage)
		if expandErr != nil {
			return 0, expandErr
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
		binary.Write(&buffer, binary.LittleEndian, depOutputFP)
	}

	buffer.WriteString(outPath)
	return cache.EntityCacheHashStateBuffer(buffer.Bytes()), nil
}

// EntityCacheInputPaths returns source paths that invalidate an entity build when changed.
func EntityCacheInputPaths(
	builtIR ir.HelmIR,
	entityKey string,
	sourcePaths []string,
) []string {
	seen := make(map[string]struct{})
	var paths []string
	entityAppendUniquePaths(&paths, seen, sourcePaths)

	entity, ok := builtIR.Entities[entityKey]
	if !ok {
		return paths
	}

	for _, dep := range entity.Deps {
		depKey := ir.HelmLabelCanonical(dep)
		depEntity, depOK := builtIR.Entities[depKey]
		if !depOK {
			continue
		}
		resolved, resolveErr := EntityResolveParameters(builtIR.SourceDirectory, builtIR, depEntity)
		if resolveErr != nil {
			continue
		}
		entityAppendUniquePaths(&paths, seen, entityResolvedSourcePaths(resolved))
	}

	sort.Strings(paths)
	return paths
}

func entityAppendUniquePaths(paths *[]string, seen map[string]struct{}, candidates []string) {
	for _, path := range candidates {
		if path == "" {
			continue
		}
		if _, exists := seen[path]; exists {
			continue
		}
		seen[path] = struct{}{}
		*paths = append(*paths, path)
	}
}

func entityCacheOutputFingerprint(outputPath string) (uint64, error) {
	info, err := os.Stat(outputPath)
	if err != nil {
		return 0, err
	}
	hash := sha256.New()
	hash.Write([]byte(outputPath))
	hash.Write([]byte{0})
	fmt.Fprintf(hash, "%d|%d", info.Size(), info.ModTime().UnixNano())
	sum := hash.Sum(nil)
	return binary.BigEndian.Uint64(sum[:8]), nil
}

func entityCacheGate(
	cacheStore *cache.EntityCacheStore,
	entityKey string,
	stateFingerprint uint64,
	outputPath string,
) (bool, error) {
	if cacheStore == nil {
		return false, nil
	}

	record, found, err := cache.EntityCacheStoreGet(cacheStore, entityKey)
	if err != nil {
		return false, err
	}
	if !found || record.StateFingerprint != stateFingerprint {
		return false, nil
	}

	outputFingerprint, statErr := entityCacheOutputFingerprint(outputPath)
	if statErr != nil {
		return false, nil
	}
	if record.OutputFingerprint != outputFingerprint {
		return false, nil
	}
	return true, nil
}

func entityCacheRecord(
	cacheStore *cache.EntityCacheStore,
	entityKey string,
	stateFingerprint uint64,
	outputPath string,
) error {
	if cacheStore == nil {
		return nil
	}

	outputFingerprint, err := entityCacheOutputFingerprint(outputPath)
	if err != nil {
		return err
	}

	return cache.EntityCacheStorePut(cacheStore, cache.EntityCacheRecord{
		EntityKey:         entityKey,
		StateFingerprint:  stateFingerprint,
		OutputFingerprint: outputFingerprint,
	})
}

func entityCacheAbsOutputPath(workspaceRoot, relPath string) string {
	if relPath == "" {
		return ""
	}
	return filepath.Join(workspaceRoot, filepath.FromSlash(relPath))
}

func entityCacheRoot(builtIR ir.HelmIR, cacheRoot string) string {
	if cacheRoot != "" {
		return cacheRoot
	}
	return filepath.Join(builtIR.SourceDirectory, ".helm", "cache")
}
