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
	globalVars map[string]string,
	parameters map[string]string,
	depExecutionNodeIDs []string,
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

	discoveredPaths, err := DynamicManifestDiscoveredPaths(
		helmBaseDir,
		target.Artifacts,
		globalVars,
		parameters,
	)
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

	paramKeys := make([]string, 0, len(parameters))
	for key := range parameters {
		paramKeys = append(paramKeys, key)
	}
	sort.Strings(paramKeys)
	for _, key := range paramKeys {
		buffer.WriteString(key)
		buffer.WriteString(parameters[key])
	}

	cacheWriteTargetExecution(&buffer, target, globalVars, parameters)

	return hash.XXH3HasherHash64(cacheAggregateHasher, buffer.Bytes()), nil
}

func cacheWriteTargetExecution(
	buffer *bytes.Buffer,
	target ir.HelmTarget,
	globalVars map[string]string,
	parameters map[string]string,
) {
	buffer.WriteString("execution")
	buffer.WriteString(expand.ExpandInterpolateLiteral(target.WorkDir, globalVars, parameters))

	envKeys := make([]string, 0, len(target.Env))
	for key := range target.Env {
		envKeys = append(envKeys, key)
	}
	sort.Strings(envKeys)
	for _, key := range envKeys {
		buffer.WriteString(key)
		buffer.WriteString(expand.ExpandInterpolateLiteral(target.Env[key], globalVars, parameters))
	}

	for _, step := range target.Steps {
		switch step.Kind {
		case ir.TargetStepRun:
			buffer.WriteString("run")
			buffer.WriteString(expand.ExpandInterpolateLiteral(step.Run, globalVars, parameters))
		case ir.TargetStepWhen:
			if step.When == nil {
				continue
			}
			condition := step.When
			buffer.WriteString("when")
			binary.Write(buffer, binary.LittleEndian, int32(condition.ConditionType))
			buffer.WriteString(condition.Parameter)
			buffer.WriteString(condition.TargetValue)
			for _, runLiteral := range condition.Runs {
				buffer.WriteString(expand.ExpandInterpolateLiteral(runLiteral, globalVars, parameters))
			}
		}
	}
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
		return 0, &CacheFingerprintFileError{Path: path, Cause: err}
	}
	return hash.XXH3HasherHash64(cacheFileHasher, content), nil
}
