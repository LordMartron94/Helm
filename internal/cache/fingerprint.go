package cache

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"foundation/hash"
	"foundation/system"
	"helm/internal/artifactresolve"
	"helm/internal/ir"
	"sort"
)

var (
	cacheFileHasher      = hash.XXH3HasherCreateWithSeed(0)
	cacheAggregateHasher = hash.XXH3HasherCreateWithSeed(1)
)

func CacheFingerprintState(
	helmBaseDir string,
	target ir.HelmTarget,
	targets map[string]ir.HelmTarget,
	globalVars map[string]string,
	parameters map[string]string,
	depStateFingerprints map[string]uint64,
	depOutputFingerprints map[string]uint64,
) (uint64, error) {
	if target.Artifacts == nil {
		return 0, fmt.Errorf("target '%s' has no artifacts block", target.Name)
	}

	inputPaths, err := artifactresolve.ArtifactResolveInputPaths(helmBaseDir, target.Artifacts, globalVars, parameters)
	if err != nil {
		return 0, err
	}

	var buffer bytes.Buffer
	buffer.WriteString("state")
	cacheWritePaths(&buffer, inputPaths)

	depNames := make([]string, 0, len(target.DependsOn))
	for _, dep := range target.DependsOn {
		canonical, ok := ir.IRResolveTargetName(targets, dep.TargetName)
		if !ok {
			continue
		}
		depNames = append(depNames, canonical)
	}
	sort.Strings(depNames)
	for _, depName := range depNames {
		buffer.WriteString(depName)
		binary.Write(&buffer, binary.LittleEndian, depStateFingerprints[depName])
		binary.Write(&buffer, binary.LittleEndian, depOutputFingerprints[depName])
	}

	paramKeys := make([]string, 0, len(parameters))
	for key := range parameters {
		paramKeys = append(paramKeys, key)
	}
	sort.Strings(paramKeys)
	for _, key := range paramKeys {
		buffer.WriteString(key)
		buffer.WriteString(parameters[key])
	}

	return hash.XXH3HasherHash64(cacheAggregateHasher, buffer.Bytes()), nil
}

func CacheFingerprintOutput(
	helmBaseDir string,
	target ir.HelmTarget,
	globalVars map[string]string,
	parameters map[string]string,
) (uint64, error) {
	if target.Artifacts == nil {
		return 0, fmt.Errorf("target '%s' has no artifacts block", target.Name)
	}

	outputPaths, err := artifactresolve.ArtifactResolveOutputPaths(
		helmBaseDir,
		target.Artifacts.Outputs,
		globalVars,
		parameters,
	)
	if err != nil {
		return 0, err
	}

	if len(outputPaths) == 0 {
		return 0, nil
	}

	var buffer bytes.Buffer
	buffer.WriteString("output")
	if err := cacheWritePaths(&buffer, outputPaths); err != nil {
		return 0, err
	}

	return hash.XXH3HasherHash64(cacheAggregateHasher, buffer.Bytes()), nil
}

func CacheAggregateInstanceFingerprints(fingerprints []uint64) uint64 {
	if len(fingerprints) == 0 {
		return 0
	}
	sorted := append([]uint64(nil), fingerprints...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })

	var buffer bytes.Buffer
	buffer.WriteString("aggregate")
	for _, fingerprint := range sorted {
		binary.Write(&buffer, binary.LittleEndian, fingerprint)
	}
	return hash.XXH3HasherHash64(cacheAggregateHasher, buffer.Bytes())
}

func cacheWritePaths(buffer *bytes.Buffer, paths []string) error {
	for _, path := range paths {
		fileHash, err := cacheFileContentHash(path)
		if err != nil {
			return err
		}
		buffer.WriteString(path)
		binary.Write(buffer, binary.LittleEndian, fileHash)
	}
	return nil
}

func cacheFileContentHash(path string) (uint64, error) {
	content, err := system.FileReadAllBytes(path)
	if err != nil {
		return 0, fmt.Errorf("fingerprint file '%s': %w", path, err)
	}
	return hash.XXH3HasherHash64(cacheFileHasher, content), nil
}
