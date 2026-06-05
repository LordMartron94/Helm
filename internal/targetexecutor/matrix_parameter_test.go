package targetexecutor

import (
	"helm/internal/expand"
	"helm/internal/ir"
	"os"
	"path/filepath"
	"testing"
)

func TestTargetExecutorMatrixInstancesExpandParameterArtifactPaths(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeFixture := func(rel, content string) string {
		path := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write fixture: %v", err)
		}
		return path
	}

	first := writeFixture("testbed/a.c", "a")
	second := writeFixture("testbed/b.c", "b")

	globals := map[string]ir.HelmGlobalVariable{
		"SOURCE_FILES": {
			Kind: ir.HelmGlobalVarArtifactArray,
			ArtifactItems: []ir.HelmArtifactInput{
				{Kind: ir.ArtifactInputString, Literal: first},
				{Kind: ir.ArtifactInputString, Literal: second},
			},
		},
	}

	target := ir.HelmTarget{
		Name: "compile_objects",
		Matrix: &ir.HelmMatrix{
			VariableName: "SRC",
			Values: []ir.HelmMatrixValue{
				{Kind: ir.MatrixValueParameterRef, ParameterName: "SOURCE_FILES"},
			},
		},
	}

	paramValues := map[string]ir.HelmParameterValue{
		"SOURCE_FILES": {Kind: ir.HelmParameterGlobalRef, GlobalName: "SOURCE_FILES"},
	}

	resolved, err := TargetExecutorResolveInvocationParameters(dir, globals, paramValues)
	if err != nil {
		t.Fatal(err)
	}
	interpCtx, err := TargetExecutorInterpolationGlobals(dir, globals, resolved)
	if err != nil {
		t.Fatal(err)
	}

	instances, err := TargetExecutorMatrixInstances(
		dir,
		target,
		paramValues,
		globals,
		interpCtx,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(instances) != 2 {
		t.Fatalf("instances = %#v", instances)
	}

	wantBindings := map[string]struct{}{
		"testbed/a.c": {},
		"testbed/b.c": {},
	}
	for _, instance := range instances {
		src, ok := instance.Bindings["SRC"]
		if !ok {
			t.Fatalf("missing SRC binding: %#v", instance)
		}
		if _, exists := wantBindings[src]; !exists {
			t.Fatalf("unexpected SRC binding %q", src)
		}
		delete(wantBindings, src)
		if src[0] == '"' || filepath.IsAbs(src) {
			t.Fatalf("SRC binding must be a clean relative path, got %q", src)
		}
	}
	if len(wantBindings) != 0 {
		t.Fatalf("missing bindings: %#v", wantBindings)
	}
}

func TestTargetExecutorMatrixLiteralInterpolatesCleanBinding(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	mainPath := filepath.Join(dir, "testbed", "main.c")
	if err := os.MkdirAll(filepath.Dir(mainPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(mainPath, []byte("c"), 0o644); err != nil {
		t.Fatal(err)
	}

	target := ir.HelmTarget{
		Name: "compile_one",
		Matrix: &ir.HelmMatrix{
			VariableName: "SRC",
			Values: []ir.HelmMatrixValue{
				{Kind: ir.MatrixValueLiteral, Literal: "${SRC_PATH}"},
			},
		},
	}

	interpCtx := expand.InterpolationContext{
		Scalars: map[string]string{
			"SRC_PATH": "testbed/main.c",
		},
	}

	instances, err := TargetExecutorMatrixInstances(
		dir,
		target,
		nil,
		nil,
		interpCtx,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(instances) != 1 {
		t.Fatalf("instances = %#v", instances)
	}
	if instances[0].Bindings["SRC"] != "testbed/main.c" {
		t.Fatalf("SRC = %q", instances[0].Bindings["SRC"])
	}
}

func TestTargetExecutorResolveMatrixRunUsesCleanSrcInterpolation(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	mainPath := filepath.Join(dir, "testbed", "main.c")
	if err := os.MkdirAll(filepath.Dir(mainPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(mainPath, []byte("c"), 0o644); err != nil {
		t.Fatal(err)
	}

	globals := map[string]ir.HelmGlobalVariable{
		"SOURCE_FILES": {
			Kind: ir.HelmGlobalVarArtifactArray,
			ArtifactItems: []ir.HelmArtifactInput{
				{Kind: ir.ArtifactInputString, Literal: mainPath},
			},
		},
	}

	target := ir.HelmTarget{
		Name: "_compile_objects",
		Matrix: &ir.HelmMatrix{
			VariableName: "SRC",
			Values: []ir.HelmMatrixValue{
				{Kind: ir.MatrixValueParameterRef, ParameterName: "SOURCE_FILES"},
			},
		},
		Steps: []ir.HelmTargetStep{
			{
				Kind: ir.TargetStepRun,
				Run: ir.HelmRunCommand{
					Argv: []ir.HelmRunArgvElement{
						{Literal: "tools/scripts/compile_object.sh"},
						{Literal: "${SRC}"},
						{Literal: "build/obj/${SRC}.o"},
					},
				},
			},
		},
	}

	inv := TargetInvocation{
		Parameters: map[string]ir.HelmParameterValue{
			"SOURCE_FILES": {Kind: ir.HelmParameterGlobalRef, GlobalName: "SOURCE_FILES"},
		},
	}

	instances, err := TargetExecutorMatrixInstances(
		dir,
		target,
		inv.Parameters,
		globals,
		expand.InterpolationContext{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(instances) != 1 {
		t.Fatalf("instances = %#v", instances)
	}

	effectiveInv := TargetInvocation{
		Parameters: targetExecutorEffectiveParameterValues(target, inv, instances[0].Bindings),
	}
	_, steps, err := TargetExecutorResolveTargetRuns(dir, target, nil, globals, effectiveInv)
	if err != nil {
		t.Fatal(err)
	}
	if len(steps) != 1 || len(steps[0].Argv) != 3 {
		t.Fatalf("steps = %#v", steps)
	}

	if steps[0].Argv[1] != "testbed/main.c" {
		t.Fatalf("SRC argv = %q", steps[0].Argv[1])
	}
	if steps[0].Argv[2] != "build/obj/testbed/main.c.o" {
		t.Fatalf("object argv = %q", steps[0].Argv[2])
	}
}
