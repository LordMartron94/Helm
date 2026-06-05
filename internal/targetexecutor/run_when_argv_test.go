package targetexecutor

import (
	"helm/internal/ir"
	"os"
	"path/filepath"
	"testing"
)

func TestTargetExecutorResolveWhenArgvRun(t *testing.T) {
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

	target := ir.HelmTarget{
		Name: "link_when",
		Steps: []ir.HelmTargetStep{
			{
				Kind: ir.TargetStepWhen,
				When: &ir.HelmCondition{
					ConditionType: ir.ConditionDefined,
					Parameter:     "FILES",
					Runs: []ir.HelmRunCommand{
						{
							Argv: []ir.HelmRunArgvElement{
								{Literal: "tools/link.sh"},
								{ParamName: "FILES"},
							},
						},
					},
				},
			},
		},
	}

	steps, err := targetExecutorResolveTargetRunSteps(
		dir,
		target,
		globals,
		paramValues,
		nil,
		map[string]string{"FILES": "bound"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(steps) != 1 {
		t.Fatalf("steps = %#v", steps)
	}

	want := []string{"tools/link.sh", first, second}
	if len(steps[0].Argv) != len(want) {
		t.Fatalf("argv = %#v, want %#v", steps[0].Argv, want)
	}
	for i := range want {
		if steps[0].Argv[i] != want[i] {
			t.Fatalf("argv[%d] = %q, want %q", i, steps[0].Argv[i], want[i])
		}
	}
}

func TestTargetExecutorResolveWhenSkipsArgvWhenConditionFalse(t *testing.T) {
	t.Parallel()

	target := ir.HelmTarget{
		Name: "skip_when",
		Steps: []ir.HelmTargetStep{
			{
				Kind: ir.TargetStepWhen,
				When: &ir.HelmCondition{
					ConditionType: ir.ConditionDefined,
					Parameter:     "MISSING",
					Runs: []ir.HelmRunCommand{
						{Argv: []ir.HelmRunArgvElement{{Literal: "never"}}},
					},
				},
			},
		},
	}

	steps, err := targetExecutorResolveTargetRunSteps(
		t.TempDir(),
		target,
		nil,
		nil,
		nil,
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(steps) != 0 {
		t.Fatalf("expected no steps, got %#v", steps)
	}
}
