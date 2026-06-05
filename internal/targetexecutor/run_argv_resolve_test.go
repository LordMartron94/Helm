package targetexecutor

import (
	"helm/internal/expand"
	"helm/internal/ir"
	"os"
	"path/filepath"
	"testing"
)

func TestTargetExecutorResolveRunArgvSplicesParameterPaths(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeFixture := func(name, content string) string {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write fixture: %v", err)
		}
		return path
	}

	first := writeFixture("a.c", "a")
	second := writeFixture("b.c", "b")

	globals := map[string]ir.HelmGlobalVariable{
		"FILES": {
			Kind: ir.HelmGlobalVarArtifactArray,
			ArtifactItems: []ir.HelmArtifactInput{
				{Kind: ir.ArtifactInputString, Literal: first},
				{Kind: ir.ArtifactInputString, Literal: second},
			},
		},
	}

	paramValues := map[string]ir.HelmParameterValue{
		"FILES": {Kind: ir.HelmParameterGlobalRef, GlobalName: "FILES"},
	}

	argv, err := targetExecutorResolveRunArgv(
		dir,
		[]ir.HelmRunArgvElement{
			{Literal: "tools/link.sh"},
			{Literal: "build/out"},
			{ParamName: "FILES"},
		},
		nil,
		globals,
		paramValues,
		expand.InterpolationContext{},
		TargetResolvedParametersEmpty(),
	)
	if err != nil {
		t.Fatal(err)
	}

	want := []string{"tools/link.sh", "build/out", "a.c", "b.c"}
	if len(argv) != len(want) {
		t.Fatalf("argv = %#v, want %#v", argv, want)
	}
	for i := range want {
		if argv[i] != want[i] {
			t.Fatalf("argv[%d] = %q, want %q (full %#v)", i, argv[i], want[i], argv)
		}
	}
}

func TestTargetExecutorResolveRunArgvSplicesMultilineScalarFlags(t *testing.T) {
	t.Parallel()

	flags := "-g -std=c89 -pedantic\n-fwrapv -fno-strict-aliasing\n-Wall"
	resolved := TargetResolvedParameters{
		Scalars: map[string]string{
			"COMPILER_FLAGS": flags,
		},
	}

	argv, err := targetExecutorResolveRunArgv(
		t.TempDir(),
		[]ir.HelmRunArgvElement{
			{Literal: "tools/scripts/compile_object.sh"},
			{Literal: "main.c"},
			{ParamName: "COMPILER_FLAGS"},
		},
		nil,
		nil,
		map[string]ir.HelmParameterValue{},
		expand.InterpolationContext{},
		resolved,
	)
	if err != nil {
		t.Fatal(err)
	}

	wantPrefix := []string{
		"tools/scripts/compile_object.sh",
		"main.c",
		"-g",
		"-std=c89",
		"-pedantic",
		"-fwrapv",
		"-fno-strict-aliasing",
		"-Wall",
	}
	if len(argv) < len(wantPrefix) {
		t.Fatalf("argv = %#v, want at least %#v", argv, wantPrefix)
	}
	for i := range wantPrefix {
		if argv[i] != wantPrefix[i] {
			t.Fatalf("argv[%d] = %q, want %q (full %#v)", i, argv[i], wantPrefix[i], argv)
		}
	}
}

func TestTargetExecutorResolveRunArgvSplicesStringListAndScalarInclude(t *testing.T) {
	t.Parallel()

	resolved := TargetResolvedParameters{
		Scalars: map[string]string{
			"DEFINES": "",
		},
		StringLists: map[string][]string{
			"INCLUDE_FLAGS": {"-Itestbed"},
		},
	}

	argv, err := targetExecutorResolveRunArgv(
		t.TempDir(),
		[]ir.HelmRunArgvElement{
			{ParamName: "DEFINES"},
			{ParamName: "INCLUDE_FLAGS"},
		},
		nil,
		nil,
		map[string]ir.HelmParameterValue{},
		expand.InterpolationContext{},
		resolved,
	)
	if err != nil {
		t.Fatal(err)
	}

	want := []string{"-Itestbed"}
	if len(argv) != len(want) {
		t.Fatalf("argv = %#v, want %#v", argv, want)
	}
	if argv[0] != want[0] {
		t.Fatalf("argv = %#v", argv)
	}
}

func TestTargetExecutorRunRequestArgvBypassesShlex(t *testing.T) {
	t.Parallel()

	argv, err := targetExecutorRunRequestArgv(TargetRunRequest{
		TargetName: "demo",
		StepIndex:  0,
		Argv:       []string{"/bin/echo", "path with spaces", "tail"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(argv) != 3 || argv[1] != "path with spaces" {
		t.Fatalf("argv = %#v", argv)
	}
}
