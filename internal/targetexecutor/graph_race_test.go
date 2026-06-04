package targetexecutor

import (
	"helm/internal/ir"
	"sync"
	"testing"
)

// TestGraphParallelFingerprintMapsRace stresses the same map access pattern as
// TargetExecutorRunGraph when many targets in one phase read dependency fingerprints.
func TestGraphParallelFingerprintMapsRace(t *testing.T) {
	t.Parallel()

	depStateFingerprints := make(map[string]uint64)
	depOutputFingerprints := make(map[string]uint64)
	var fingerprintMu sync.RWMutex

	const workers = 32
	var wg sync.WaitGroup
	wg.Add(workers * 2)

	for i := 0; i < workers; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				fingerprintMu.RLock()
				_ = depStateFingerprints["dep-a"]
				_ = depOutputFingerprints["dep-b"]
				fingerprintMu.RUnlock()
			}
		}(i)

		go func(id int) {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				fingerprintMu.Lock()
				depStateFingerprints["node"] = uint64(id)
				depOutputFingerprints["node"] = uint64(j)
				fingerprintMu.Unlock()
			}
		}(i)
	}

	wg.Wait()
}

func TestTargetExecutorRunGraphRaceWithMatrix(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode")
	}

	builtIR := ir.HelmIR{
		SourceDirectory: t.TempDir(),
		Targets: map[string]ir.HelmTarget{
			"entry": {
				Name: "entry",
				Artifacts: &ir.HelmArtifacts{
					Volatile: true,
				},
				DependsOn: []ir.HelmTargetDependency{
					{TargetName: "gen_a"},
					{TargetName: "gen_b"},
				},
				Steps: []ir.HelmTargetStep{{Kind: ir.TargetStepRun, Run: "echo entry"}},
			},
			"gen_a": {
				Name: "gen_a",
				Matrix: &ir.HelmMatrix{
					VariableName: "M",
					Values: []ir.HelmMatrixValue{
						{Kind: ir.MatrixValueLiteral, Literal: "a1"},
						{Kind: ir.MatrixValueLiteral, Literal: "a2"},
					},
				},
				Artifacts: &ir.HelmArtifacts{
					Volatile: true,
				},
				Steps: []ir.HelmTargetStep{{Kind: ir.TargetStepRun, Run: "echo ${M}"}},
			},
			"gen_b": {
				Name: "gen_b",
				Matrix: &ir.HelmMatrix{
					VariableName: "M",
					Values: []ir.HelmMatrixValue{
						{Kind: ir.MatrixValueLiteral, Literal: "b1"},
						{Kind: ir.MatrixValueLiteral, Literal: "b2"},
					},
				},
				Artifacts: &ir.HelmArtifacts{
					Volatile: true,
				},
				Steps: []ir.HelmTargetStep{{Kind: ir.TargetStepRun, Run: "echo ${M}"}},
			},
		},
	}

	opts := TargetExecutorOptions{
		DisableArtifactCache: true,
		RunHandler: func(req TargetRunRequest) (TargetRunResult, error) {
			return TargetRunResult{}, nil
		},
	}

	if err := TargetExecutorRunGraph(builtIR, "entry", nil, opts); err != nil {
		t.Fatal(err)
	}
}
