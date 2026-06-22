package entityexecutor

import (
	"path/filepath"
	"testing"

	"helm/internal/ir"
)

func TestEntityFlattenBagsResolvesRelPaths(t *testing.T) {
	workspaceRoot := filepath.Clean("/workspace")
	sourceFile := filepath.Join(workspaceRoot, "libs", "echo", "echo.helm")

	builtIR := ir.HelmIR{
		SourceDirectory: workspaceRoot,
		Entities: map[string]ir.HelmEntity{
			"//libs/echo:echo": {
				SourceFile: sourceFile,
				InterfaceBag: map[string]ir.HelmStringListExpr{
					"INCLUDE_PATHS": {{Kind: ir.StringListRel, RelPath: ".."}},
				},
			},
		},
	}

	flat := EntityFlattenBags(
		builtIR,
		ir.HelmEntityDepsFromLabels([]ir.HelmLabel{{Path: "libs/echo", Name: "echo"}}),
		ir.HelmConfigurationDefaultName,
		"INCLUDE_PATHS",
	)
	if len(flat) != 1 || flat[0] != "libs" {
		t.Fatalf("flat = %#v, want [libs]", flat)
	}
}

func TestEntityEvaluateFormatFlagsPrefixesCollectedPaths(t *testing.T) {
	workspaceRoot := filepath.Clean("/workspace")
	echoFile := filepath.Join(workspaceRoot, "libs", "echo", "echo.helm")
	testbedFile := filepath.Join(workspaceRoot, "testbed", "testbed.helm")

	builtIR := ir.HelmIR{
		SourceDirectory: workspaceRoot,
		Entities: map[string]ir.HelmEntity{
			"//libs/echo:echo": {
				SourceFile: echoFile,
				InterfaceBag: map[string]ir.HelmStringListExpr{
					"INCLUDE_PATHS": {{Kind: ir.StringListRel, RelPath: ".."}},
				},
			},
			"//testbed:testbed": {
				SourceFile: testbedFile,
				Deps:       []ir.HelmLabel{{Path: "libs/echo", Name: "echo"}},
			},
		},
	}

	entity := builtIR.Entities["//testbed:testbed"]
	expr := ir.HelmStringListExpr{{
		Kind: ir.StringListFormatFlags,
		FormatFlags: &ir.HelmFormatFlagsExpr{
			Prefix: "-I",
			Inner: ir.HelmStringListElement{
				Kind: ir.StringListCollect,
				Collect: &ir.HelmCollectExpr{
					DependenciesParam: "DEPENDENCIES",
					ExportKey:         "INCLUDE_PATHS",
				},
			},
		},
	}}

	out, err := entityEvaluateStringListExpr(
		builtIR,
		entity,
		ir.HelmConfigurationDefaultName,
		nil,
		expr,
		EntityResolvedParametersEmpty().InterpolationContext(nil),
		EntityResolvedParametersEmpty(),
	)
	if err != nil {
		t.Fatalf("entityEvaluateStringListExpr: %v", err)
	}
	if len(out) != 1 || out[0] != "-Ilibs" {
		t.Fatalf("out = %#v, want [-Ilibs]", out)
	}
}
