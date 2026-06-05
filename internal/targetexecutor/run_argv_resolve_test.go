package targetexecutor

import (
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
		globals,
		paramValues,
		nil,
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}

	want := []string{"tools/link.sh", "build/out", first, second}
	if len(argv) != len(want) {
		t.Fatalf("argv = %#v, want %#v", argv, want)
	}
	for i := range want {
		if argv[i] != want[i] {
			t.Fatalf("argv[%d] = %q, want %q (full %#v)", i, argv[i], want[i], argv)
		}
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
