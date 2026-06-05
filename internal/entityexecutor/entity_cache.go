package entityexecutor

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"helm/internal/cache"
	"helm/internal/ir"
)

// EntityCacheFingerprint hashes entity_id, property bags, and source paths.
func EntityCacheFingerprint(
	builtIR ir.HelmIR,
	entityKey string,
	outPath string,
	sourcePaths []string,
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

	sortedSources := append([]string(nil), sourcePaths...)
	sort.Strings(sortedSources)

	hash := sha256.New()
	hash.Write([]byte(entityKey))
	hash.Write([]byte{0})
	hash.Write([]byte(fmt.Sprintf("%d", EntityBagFingerprint(bag))))
	hash.Write([]byte{0})
	for _, path := range sortedSources {
		hash.Write([]byte(path))
		hash.Write([]byte{0})
	}
	hash.Write([]byte(outPath))

	sum := hash.Sum(nil)
	return binary.BigEndian.Uint64(sum[:8]), nil
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

func entityCacheRoot(builtIR ir.HelmIR, cacheRoot string) string {
	if cacheRoot != "" {
		return cacheRoot
	}
	return filepath.Join(builtIR.SourceDirectory, ".helm", "cache")
}
