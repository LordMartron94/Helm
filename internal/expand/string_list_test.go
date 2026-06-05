package expand

import (
	"helm/internal/ir"
	"testing"
)

func TestEvaluateStringListCollectMergesExportFlags(t *testing.T) {
	t.Parallel()

	targets := map[string]ir.HelmTarget{
		"lib_a": {
			Name: "lib_a",
			Export: map[string]ir.HelmStringListExpr{
				"LD_FLAGS": {
					{Kind: ir.StringListLiteral, Literal: "-la"},
				},
			},
		},
		"lib_b": {
			Name: "lib_b",
			Export: map[string]ir.HelmStringListExpr{
				"LD_FLAGS": {
					{Kind: ir.StringListLiteral, Literal: "-lb"},
				},
			},
		},
	}

	paramValues := map[string]ir.HelmParameterValue{
		"DEPS": {
			Kind: ir.HelmParameterDependencyList,
			Dependencies: []ir.HelmTargetDependency{
				{TargetName: "lib_a"},
				{TargetName: "lib_b"},
			},
		},
	}

	got, err := EvaluateStringListExpr(
		ir.HelmStringListExpr{{
			Kind: ir.StringListCollect,
			Collect: &ir.HelmCollectExpr{
				DependenciesParam: "DEPS",
				ExportKey:         "LD_FLAGS",
			},
		}},
		targets,
		paramValues,
		nil,
		nil,
		nil,
		InterpolationContext{},
	)
	if err != nil {
		t.Fatal(err)
	}

	want := []string{"-la", "-lb"}
	if len(got) != len(want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestJoinStringListExprCombinesParamAndCollect(t *testing.T) {
	t.Parallel()

	targets := map[string]ir.HelmTarget{
		"splash": {
			Name: "splash",
			Export: map[string]ir.HelmStringListExpr{
				"LD_FLAGS": {{Kind: ir.StringListLiteral, Literal: "-lsplash"}},
			},
		},
	}

	paramValues := map[string]ir.HelmParameterValue{
		"DEPS": {
			Kind: ir.HelmParameterDependencyList,
			Dependencies: []ir.HelmTargetDependency{
				{TargetName: "splash"},
			},
		},
	}

	joined, err := JoinStringListExpr(
		ir.HelmStringListExpr{
			{Kind: ir.StringListLiteral, Literal: "-Lbuild/lib"},
			{
				Kind: ir.StringListCollect,
				Collect: &ir.HelmCollectExpr{
					DependenciesParam: "DEPS",
					ExportKey:         "LD_FLAGS",
				},
			},
		},
		targets,
		paramValues,
		nil,
		nil,
		nil,
		InterpolationContext{},
		" ",
	)
	if err != nil {
		t.Fatal(err)
	}

	want := "-Lbuild/lib -lsplash"
	if joined != want {
		t.Fatalf("joined = %q, want %q", joined, want)
	}
}

func TestEvaluateStringListParamRefUsesResolvedPathLists(t *testing.T) {
	t.Parallel()

	joined, err := JoinStringListExpr(
		ir.HelmStringListExpr{
			{Kind: ir.StringListParamRef, ParamName: "LINKER_FLAGS"},
		},
		nil,
		map[string]ir.HelmParameterValue{
			"LINKER_FLAGS": {Kind: ir.HelmParameterGlobalRef, GlobalName: "TEST_LINKER_FLAGS"},
		},
		nil,
		nil,
		map[string][]string{
			"LINKER_FLAGS": {"-Lbuild/lib", "-Wl,-rpath,'$ORIGIN/../lib'"},
		},
		InterpolationContext{},
		" ",
	)
	if err != nil {
		t.Fatal(err)
	}

	want := "-Lbuild/lib -Wl,-rpath,'$ORIGIN/../lib'"
	if joined != want {
		t.Fatalf("joined = %q, want %q", joined, want)
	}
}

func TestEvaluateStringListParamRefSplitsScalarLinkerFlags(t *testing.T) {
	t.Parallel()

	joined, err := JoinStringListExpr(
		ir.HelmStringListExpr{
			{Kind: ir.StringListParamRef, ParamName: "LINKER_FLAGS"},
		},
		nil,
		nil,
		map[string]string{
			"LINKER_FLAGS": "-lvulkan -lxcb -lX11",
		},
		nil,
		nil,
		InterpolationContext{},
		" ",
	)
	if err != nil {
		t.Fatal(err)
	}

	want := "-lvulkan -lxcb -lX11"
	if joined != want {
		t.Fatalf("joined = %q, want %q", joined, want)
	}
}
