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
	var buffer bytes.Buffer
	if err := entityCacheWriteBaseState(&buffer, builtIR, entityKey, usagePropagation); err != nil {
		return 0, err
	}

	inputPaths := EntityCacheInputPaths(builtIR, entityKey, sourcePaths)
	if err := cache.EntityCacheWriteInputPaths(builtIR.SourceDirectory, &buffer, inputPaths); err != nil {
		return 0, err
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

	entity, inst, lookupErr := entityLookupForInstanceKey(builtIR, entityKey)
	if lookupErr != nil {
		return paths
	}

	deps, depErr := ir.HelmEntityDepsForConfiguration(entity, inst.Configuration)
	if depErr != nil {
		return paths
	}

	for _, dep := range deps {
		depInst := ir.HelmEntityDepResolveInstance(dep, inst.Configuration)
		_ = entityInstanceKey(depInst)
		depEntity, depOK := builtIR.Entities[entityInstanceBaseKey(depInst)]
		if !depOK {
			continue
		}
		resolved, resolveErr := EntityResolveParameters(
			builtIR.SourceDirectory,
			builtIR,
			depEntity,
			depInst.Configuration,
		)
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
