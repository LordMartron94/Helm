package cache

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"foundation/hash"
	"foundation/system"
	"helm/internal/artifactresolve"
	"helm/internal/expand"
	"helm/internal/ir"
	"os"
	"sort"
)

// cacheMissingFileFingerprint is the stable content hash for a referenced path that does not
// exist yet. The path itself is still written into the aggregate fingerprint so creation or
// removal of the file changes the cache state.
const cacheMissingFileFingerprint uint64 = 0

// CacheFingerprintFileError is returned when hashing an artifact path for a fingerprint fails.
type CacheFingerprintFileError struct {
	Path  string
	Cause error
}

func (e *CacheFingerprintFileError) Error() string {
	return fmt.Sprintf("fingerprint file '%s': %s", e.Path, e.Cause)
}

func (e *CacheFingerprintFileError) Unwrap() error {
	return e.Cause
}

func CacheFingerprintFileErrorPath(err error) (string, bool) {
	var fileErr *CacheFingerprintFileError
	if errors.As(err, &fileErr) {
		return fileErr.Path, true
	}
	return "", false
}

func CacheFingerprintFileErrorIsNotExist(err error) bool {
	var fileErr *CacheFingerprintFileError
	if !errors.As(err, &fileErr) {
		return false
	}
	return os.IsNotExist(fileErr.Cause)
}

var (
	cacheFileHasher      = hash.XXH3HasherCreateWithSeed(0)
	cacheAggregateHasher = hash.XXH3HasherCreateWithSeed(1)
)

func CacheFingerprintState(
	helmBaseDir string,
	target ir.HelmTarget,
	targets map[string]ir.HelmTarget,
	interpCtx expand.InterpolationContext,
	depExecutionNodeIDs []string,
	depStateFingerprints map[string]uint64,
	depOutputFingerprints map[string]uint64,
) (uint64, error) {
	if target.Artifacts == nil {
		return 0, fmt.Errorf("target '%s' has no artifacts block", target.Name)
	}

	inputPaths, err := artifactresolve.ArtifactResolveInputPathsContext(helmBaseDir, target.Artifacts, interpCtx)
	if err != nil {
		return 0, err
	}

	discoveredPaths, err := DynamicManifestDiscoveredPathsContext(helmBaseDir, target.Artifacts, interpCtx)
	if err != nil {
		return 0, err
	}

	var buffer bytes.Buffer
	buffer.WriteString("state")
	cacheWritePaths(&buffer, inputPaths)
	if len(discoveredPaths) > 0 {
		buffer.WriteString("dynamic-discovered")
		if err := cacheWritePaths(&buffer, discoveredPaths); err != nil {
			return 0, err
		}
	}

	for i, dep := range target.DependsOn {
		execNode := dep.TargetName
		if i < len(depExecutionNodeIDs) && depExecutionNodeIDs[i] != "" {
			execNode = depExecutionNodeIDs[i]
		} else {
			canonical, ok := ir.IRResolveTargetName(targets, dep.TargetName)
			if ok {
				execNode = canonical
			}
		}
		buffer.WriteString(execNode)
		binary.Write(&buffer, binary.LittleEndian, depStateFingerprints[execNode])
		binary.Write(&buffer, binary.LittleEndian, depOutputFingerprints[execNode])
	}

	cacheWriteInterpolationParameters(&buffer, interpCtx)
	cacheWriteTargetExecution(&buffer, target, interpCtx)

	return hash.XXH3HasherHash64(cacheAggregateHasher, buffer.Bytes()), nil
}

func cacheWriteInterpolationParameters(buffer *bytes.Buffer, ctx expand.InterpolationContext) {
	keys := make([]string, 0, len(ctx.Scalars)+len(ctx.PathLists))
	for key := range ctx.Scalars {
		keys = append(keys, key)
	}
	for key := range ctx.PathLists {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, key := range keys {
		buffer.WriteString(key)
		if paths, ok := ctx.PathLists[key]; ok {
			sortedPaths := append([]string(nil), paths...)
			sort.Strings(sortedPaths)
			for _, path := range sortedPaths {
				buffer.WriteString(path)
				buffer.WriteByte(0)
			}
			continue
		}
		buffer.WriteString(ctx.Scalars[key])
	}
}

func cacheWriteTargetExecution(
	buffer *bytes.Buffer,
	target ir.HelmTarget,
	ctx expand.InterpolationContext,
) {
	buffer.WriteString("execution")
	buffer.WriteString(expand.InterpolationContextExpandLiteral(ctx, target.WorkDir))

	envKeys := make([]string, 0, len(target.Env))
	for key := range target.Env {
		envKeys = append(envKeys, key)
	}
	sort.Strings(envKeys)
	for _, key := range envKeys {
		buffer.WriteString(key)
		buffer.WriteString(expand.InterpolationContextExpandLiteral(ctx, target.Env[key]))
	}

	for _, step := range target.Steps {
		switch step.Kind {
		case ir.TargetStepRun:
			buffer.WriteString("run")
			buffer.WriteString(expand.ExpandFingerprintRunCommand(step.Run, ctx))
		case ir.TargetStepWhen:
			if step.When == nil {
				continue
			}
			condition := step.When
			buffer.WriteString("when")
			binary.Write(buffer, binary.LittleEndian, int32(condition.ConditionType))
			buffer.WriteString(condition.Parameter)
			buffer.WriteString(condition.TargetValue)
			for _, runCommand := range condition.Runs {
				buffer.WriteString(expand.ExpandFingerprintRunCommand(runCommand, ctx))
			}
		}
	}
}

func CacheFingerprintOutput(
	helmBaseDir string,
	target ir.HelmTarget,
	interpCtx expand.InterpolationContext,
) (uint64, error) {
	if target.Artifacts == nil {
		return 0, fmt.Errorf("target '%s' has no artifacts block", target.Name)
	}

	outputPaths, err := artifactresolve.ArtifactResolveOutputPathsContext(
		helmBaseDir,
		target.Artifacts.Outputs,
		interpCtx,
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
	if _, statErr := os.Stat(path); statErr != nil {
		if os.IsNotExist(statErr) {
			return cacheMissingFileFingerprint, nil
		}
		return 0, &CacheFingerprintFileError{Path: path, Cause: statErr}
	}

	content, err := system.FileReadAllBytes(path)
	if err != nil {
		return 0, &CacheFingerprintFileError{Path: path, Cause: err}
	}
	return hash.XXH3HasherHash64(cacheFileHasher, content), nil
}
