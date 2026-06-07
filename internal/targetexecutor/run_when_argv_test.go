package targetexecutor

import (
	"helm/internal/expand"
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

	resolved, err := TargetExecutorResolveInvocationParameters(dir, globals, paramValues)
	if err != nil {
		t.Fatal(err)
	}
	interpCtx, err := TargetExecutorInterpolationGlobals(dir, globals, resolved)
	if err != nil {
		t.Fatal(err)
	}

	steps, err := targetExecutorResolveTargetRunSteps(
		dir,
		target,
		nil,
		globals,
		paramValues,
		interpCtx,
		resolved,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(steps) != 1 {
		t.Fatalf("steps = %#v", steps)
	}

	want := []string{"tools/link.sh", "a.c", "b.c"}
	if len(steps[0].Argv) != len(want) {
		t.Fatalf("argv = %#v, want %#v", steps[0].Argv, want)
	}
	for i := range want {
		if steps[0].Argv[i] != want[i] {
			t.Fatalf("argv[%d] = %q, want %q", i, steps[0].Argv[i], want[i])
		}
	}
}

func TestTargetExecutorResolveWhenGlobalArgvRun(t *testing.T) {
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

	target := ir.HelmTarget{
		Name: "link_when_global",
		Steps: []ir.HelmTargetStep{
			{
				Kind: ir.TargetStepWhen,
				When: &ir.HelmCondition{
					ConditionType: ir.ConditionDefined,
					Parameter:     "FILES",
					Runs: []ir.HelmRunCommand{
						{Argv: []ir.HelmRunArgvElement{{Literal: "tools/link-global.sh"}}},
					},
				},
			},
		},
	}

	resolved := TargetResolvedParametersEmpty()
	interpCtx, err := TargetExecutorInterpolationGlobals(dir, globals, resolved)
	if err != nil {
		t.Fatal(err)
	}

	steps, err := targetExecutorResolveTargetRunSteps(
		dir,
		target,
		nil,
		globals,
		nil,
		interpCtx,
		resolved,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(steps) != 1 {
		t.Fatalf("steps = %#v", steps)
	}

	if len(steps[0].Argv) != 1 || steps[0].Argv[0] != "tools/link-global.sh" {
		t.Fatalf("argv = %#v", steps[0].Argv)
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
		expand.InterpolationContext{},
		TargetResolvedParametersEmpty(),
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(steps) != 0 {
		t.Fatalf("expected no steps, got %#v", steps)
	}
}
