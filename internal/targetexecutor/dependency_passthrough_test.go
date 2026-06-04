package targetexecutor

import (
	"errors"
	"helm/internal/cache"
	"helm/internal/ir"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTargetExecutorDependencyBlockedTransitive(t *testing.T) {
	t.Parallel()

	plan := &TargetExecutionPlan{
		Outgoing: map[string][]string{
			"entry":  {"middle"},
			"middle": {"leaf"},
			"leaf":   nil,
		},
	}

	builtIR := ir.HelmIR{
		Targets: map[string]ir.HelmTarget{
			"entry": {
				Name:      "entry",
				DependsOn: []ir.HelmTargetDependency{{TargetName: "middle"}},
			},
			"middle": {
				Name:      "middle",
				DependsOn: []ir.HelmTargetDependency{{TargetName: "leaf"}},
			},
			"leaf": {Name: "leaf"},
		},
	}

	leafErr := errors.New("leaf compile failed")
	results := map[string]error{
		"leaf":   leafErr,
		"middle": nil,
	}

	blocked := targetExecutorDependencyBlocked(plan, builtIR, "entry", results)
	if blocked == nil {
		t.Fatal("expected transitive block")
	}
	if !strings.Contains(blocked.Error(), "leaf") {
		t.Fatalf("expected leaf in error, got %v", blocked)
	}
	if !strings.Contains(blocked.Error(), "leaf compile failed") {
		t.Fatalf("expected root cause preserved, got %v", blocked)
	}
}

func TestTargetExecutorPassthroughFingerprintError(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	outputPath := filepath.Join(dir, "missing.bin")
	_ = os.Mkdir(filepath.Join(dir, "build"), 0o755)

	builtIR := ir.HelmIR{
		SourceDirectory: dir,
		Targets: map[string]ir.HelmTarget{
			"producer": {
				Name: "producer",
				Artifacts: &ir.HelmArtifacts{
					Outputs: []ir.HelmArtifactInput{
						{Kind: ir.ArtifactInputString, Literal: "missing.bin"},
					},
				},
			},
			"consumer": {
				Name: "consumer",
				Artifacts: &ir.HelmArtifacts{
					Inputs: []ir.HelmArtifactInput{
						{Kind: ir.ArtifactInputString, Literal: "missing.bin"},
					},
				},
			},
		},
	}

	producerErr := errors.New("clang failed")
	results := map[string]error{
		"producer": producerErr,
	}

	fingerprintErr := &cache.CacheFingerprintFileError{
		Path:  outputPath,
		Cause: os.ErrNotExist,
	}

	remapped := targetExecutorPassthroughFingerprintError(
		builtIR,
		"consumer",
		fingerprintErr,
		results,
		map[string]TargetInvocation{},
	)
	if remapped == nil {
		t.Fatal("expected remapped dependency error")
	}
	if !strings.Contains(remapped.Error(), "producer") {
		t.Fatalf("expected producer dependency, got %v", remapped)
	}
	if !strings.Contains(remapped.Error(), "clang failed") {
		t.Fatalf("expected root cause, got %v", remapped)
	}
}
