package entityexecutor

import (
	"path/filepath"
	"testing"

	"helm/internal/ir"
)

func TestEntityExecutorTargetHooksRunOncePerGraph(t *testing.T) {
	dir := t.TempDir()
	objDir := filepath.Join(dir, "build", "obj")
	compilePhase := ir.HelmAdapterPhase{
		Name:            "_compile",
		TargetDependsOn: []string{"generate_compilation_database"},
		Matrix: &ir.HelmMatrix{
			VariableName: "SRC",
			Values: []ir.HelmMatrixValue{
				{Kind: ir.MatrixValueParameterRef, ParameterName: "SOURCE_FILES"},
			},
		},
		MatrixLegOutputs: []ir.HelmArtifactInput{
			{Kind: ir.ArtifactInputString, Literal: objDir + "/${SRC}.o"},
		},
		MatrixRuns: []ir.HelmRunCommand{
			{
				Argv: []ir.HelmRunArgvElement{
					{Literal: "tools/scripts/compile_object.sh"},
					{Literal: "${SRC}"},
					{Literal: objDir + "/${SRC}.o"},
					{Literal: objDir + "/${SRC}.d"},
				},
			},
		},
	}
	linkPhase := ir.HelmAdapterPhase{
		Name:      "_link",
		DependsOn: []string{"_compile"},
		Outputs: []ir.HelmArtifactInput{
			{Kind: ir.ArtifactInputString, Literal: objDir + "/${OUT_NAME}"},
		},
		Runs: []ir.HelmRunCommand{
			{
				Argv: []ir.HelmRunArgvElement{
					{Literal: "true"},
					{PhaseOutputs: "_compile"},
				},
			},
		},
	}

	builtIR := ir.HelmIR{
		SourceDirectory: dir,
		Mode:            ir.HelmModeWorkspace,
		Targets: map[string]ir.HelmTarget{
			"generate_compilation_database": {Name: "generate_compilation_database"},
		},
		Adapters: map[string]ir.HelmAdapterDecl{
			"c_executable": {
				Name:   "c_executable",
				Phases: []ir.HelmAdapterPhase{compilePhase, linkPhase},
			},
		},
		Entities: map[string]ir.HelmEntity{
			"//apps/alpha:alpha": {
				Name:        "alpha",
				AdapterName: "c_executable",
				SourceFile:  filepath.Join(dir, "alpha.helm"),
				Parameters: map[string]ir.HelmParameterValue{
					"SOURCE_FILES": {
						Kind:          ir.HelmParameterArtifactItems,
						ArtifactItems: []ir.HelmArtifactInput{{Kind: ir.ArtifactInputString, Literal: "alpha.c"}},
					},
					"OUT_NAME": {Kind: ir.HelmParameterScalar, Scalar: "alpha"},
				},
			},
			"//apps/beta:beta": {
				Name:        "beta",
				AdapterName: "c_executable",
				SourceFile:  filepath.Join(dir, "beta.helm"),
				Parameters: map[string]ir.HelmParameterValue{
					"SOURCE_FILES": {
						Kind:          ir.HelmParameterArtifactItems,
						ArtifactItems: []ir.HelmArtifactInput{{Kind: ir.ArtifactInputString, Literal: "beta.c"}},
					},
					"OUT_NAME": {Kind: ir.HelmParameterScalar, Scalar: "beta"},
				},
			},
		},
	}

	hookRuns := 0
	err := EntityExecutorRunGraph(
		builtIR,
		[]EntityInstance{
			{Label: ir.HelmLabel{Path: "apps/alpha", Name: "alpha"}},
			{Label: ir.HelmLabel{Path: "apps/beta", Name: "beta"}},
		},
		EntityExecutorOptions{
			DisableCache: true,
			TargetHookRunner: func(targetName string) error {
				if targetName != "generate_compilation_database" {
					t.Fatalf("unexpected hook %q", targetName)
				}
				hookRuns++
				return nil
			},
			RunHandler: func(req EntityRunRequest) error {
				return nil
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if hookRuns != 1 {
		t.Fatalf("hookRuns = %d, want 1", hookRuns)
	}
}
