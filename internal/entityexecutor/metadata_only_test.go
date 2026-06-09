package entityexecutor

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"helm/internal/ir"
)

func TestEntityExpandAdapterMetadataOnlyProducesNoSteps(t *testing.T) {
	dir := t.TempDir()
	builtIR := ir.HelmIR{
		SourceDirectory: dir,
		Entities: map[string]ir.HelmEntity{
			"//libs/nexus:nexus": {
				Name:  "nexus",
				Kind:  ir.HelmEntityKindInterface,
				Label: ir.HelmLabel{Path: "libs/nexus", Name: "nexus"},
				InterfaceBag: map[string]ir.HelmStringListExpr{
					"CPPFLAGS": {{Kind: ir.StringListLiteral, Literal: "-Ilibs/nexus/include"}},
				},
			},
		},
	}

	plan, err := EntityExpandAdapter(dir, builtIR, "//libs/nexus:nexus", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Steps) != 0 {
		t.Fatalf("steps = %#v, want none", plan.Steps)
	}
	if plan.PrimaryOutput != "" {
		t.Fatalf("primary output = %q, want empty", plan.PrimaryOutput)
	}
}

func TestEntityExportGraphMetadataOnlyEntity(t *testing.T) {
	dir := t.TempDir()
	builtIR := ir.HelmIR{
		SourceDirectory: dir,
		Entities: map[string]ir.HelmEntity{
			"//libs/nexus:nexus": {
				Name:  "nexus",
				Kind:  ir.HelmEntityKindInterface,
				Label: ir.HelmLabel{Path: "libs/nexus", Name: "nexus"},
				InterfaceBag: map[string]ir.HelmStringListExpr{
					"CPPFLAGS": {{Kind: ir.StringListLiteral, Literal: "-Ilibs/nexus/include"}},
					"HEADERS":  {{Kind: ir.StringListLiteral, Literal: "libs/nexus/include/nexus.h"}},
				},
			},
		},
	}

	export, err := EntityExportExecutionGraph(builtIR, []ir.HelmLabel{{Path: "libs/nexus", Name: "nexus"}})
	if err != nil {
		t.Fatal(err)
	}

	entry, ok := export.Entities["//libs/nexus:nexus"]
	if !ok {
		t.Fatalf("entities = %#v", export.Entities)
	}
	if entry.Adapter != "" {
		t.Fatalf("adapter = %q, want empty", entry.Adapter)
	}
	if len(entry.RunArgvs) != 0 {
		t.Fatalf("run_argvs = %#v, want empty", entry.RunArgvs)
	}
	if entry.OutputPath != "" {
		t.Fatalf("output_path = %q, want empty", entry.OutputPath)
	}
	if entry.PropertyBag["CPPFLAGS"][0] != "-Ilibs/nexus/include" {
		t.Fatalf("CPPFLAGS = %#v", entry.PropertyBag["CPPFLAGS"])
	}
	if entry.PropertyBag["HEADERS"][0] != "libs/nexus/include/nexus.h" {
		t.Fatalf("HEADERS = %#v", entry.PropertyBag["HEADERS"])
	}

	raw, err := EntityExportExecutionGraphJSON(builtIR, []ir.HelmLabel{{Path: "libs/nexus", Name: "nexus"}})
	if err != nil {
		t.Fatal(err)
	}
	var decoded EntityExecutionGraphExport
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
}

func TestEntityFlattenBagsCollectsFromMetadataOnlyDependency(t *testing.T) {
	builtIR := ir.HelmIR{
		Entities: map[string]ir.HelmEntity{
			"//libs/nexus:nexus": {
				Kind: ir.HelmEntityKindHeaderOnly,
				InterfaceBag: map[string]ir.HelmStringListExpr{
					"CPPFLAGS": {{Kind: ir.StringListLiteral, Literal: "-Ilibs/nexus/include"}},
				},
			},
			"//apps/demo:demo": {
				AdapterName: "c_executable",
				Deps: []ir.HelmLabel{
					{Path: "libs/nexus", Name: "nexus"},
				},
			},
		},
	}

	flat := EntityFlattenBags(
		builtIR,
		builtIR.Entities["//apps/demo:demo"].Deps,
		"CPPFLAGS",
	)
	if len(flat) != 1 || flat[0] != "-Ilibs/nexus/include" {
		t.Fatalf("flat = %#v", flat)
	}
}

func TestEntityExecutorRunGraphSkipsMetadataOnlySpawn(t *testing.T) {
	dir := t.TempDir()
	builtIR := ir.HelmIR{
		SourceDirectory: dir,
		Entities: map[string]ir.HelmEntity{
			"//libs/nexus:nexus": {
				Name:  "nexus",
				Kind:  ir.HelmEntityKindInterface,
				Label: ir.HelmLabel{Path: "libs/nexus", Name: "nexus"},
				InterfaceBag: map[string]ir.HelmStringListExpr{
					"CPPFLAGS": {{Kind: ir.StringListLiteral, Literal: "-Ilibs/nexus/include"}},
				},
			},
		},
	}

	spawnCount := 0
	err := EntityExecutorRunGraph(
		builtIR,
		[]ir.HelmLabel{{Path: "libs/nexus", Name: "nexus"}},
		EntityExecutorOptions{
			RunHandler: func(req EntityRunRequest) error {
				spawnCount++
				return nil
			},
			DisableCache: true,
			CacheRoot:    filepath.Join(dir, ".helm", "cache"),
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if spawnCount != 0 {
		t.Fatalf("spawn count = %d, want 0", spawnCount)
	}
}

func TestEntityBuildExecutionPlanIncludesMetadataOnlyEntity(t *testing.T) {
	builtIR := ir.HelmIR{
		Entities: map[string]ir.HelmEntity{
			"//libs/nexus:nexus": {
				Kind: ir.HelmEntityKindInterface,
			},
			"//libs/splash:splash": {
				AdapterName: "c_shared_library",
				Deps: []ir.HelmLabel{
					{Path: "libs/nexus", Name: "nexus"},
				},
			},
		},
	}

	plan, err := EntityBuildExecutionPlan(builtIR, []ir.HelmLabel{{Path: "libs/splash", Name: "splash"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Order) != 2 {
		t.Fatalf("order = %#v", plan.Order)
	}
	if plan.Order[0] != "//libs/nexus:nexus" {
		t.Fatalf("nexus should be first, got %#v", plan.Order)
	}
}

func TestEntityCacheFingerprintMetadataOnlyDependency(t *testing.T) {
	dir := t.TempDir()
	builtIR := ir.HelmIR{
		SourceDirectory: dir,
		Entities: map[string]ir.HelmEntity{
			"//libs/nexus:nexus": {
				Kind: ir.HelmEntityKindInterface,
				InterfaceBag: map[string]ir.HelmStringListExpr{
					"CPPFLAGS": {{Kind: ir.StringListLiteral, Literal: "-Ilibs/nexus/include"}},
				},
			},
			"//libs/splash:splash": {
				AdapterName: "c_shared_library",
				Deps: []ir.HelmLabel{
					{Path: "libs/nexus", Name: "nexus"},
				},
				Parameters: map[string]ir.HelmParameterValue{
					"SOURCE_FILES": {
						Kind:          ir.HelmParameterArtifactItems,
						ArtifactItems: []ir.HelmArtifactInput{{Kind: ir.ArtifactInputString, Literal: "src/a.c"}},
					},
				},
			},
		},
		Adapters: map[string]ir.HelmAdapterDecl{
			"c_shared_library": {
				Name: "c_shared_library",
				Matrix: &ir.HelmMatrix{
					VariableName: "SRC",
					Values: []ir.HelmMatrixValue{
						{Kind: ir.MatrixValueParameterRef, ParameterName: "SOURCE_FILES"},
					},
				},
				MatrixRuns: []ir.HelmRunCommand{
					{Argv: []ir.HelmRunArgvElement{{Literal: "compile"}}},
				},
				Runs: []ir.HelmRunCommand{
					{Argv: []ir.HelmRunArgvElement{{Literal: "link"}}},
				},
				Outputs: []ir.HelmArtifactInput{
					{Kind: ir.ArtifactInputString, Literal: "build/lib/libsplash.so"},
				},
			},
		},
	}

	_, err := EntityCacheFingerprint(builtIR, "//libs/splash:splash", "build/lib/libsplash.so", []string{"src/a.c"}, nil)
	if err != nil {
		t.Fatal(err)
	}
}
