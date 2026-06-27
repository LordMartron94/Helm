package entityexecutor

import (
	"testing"

	"helm/internal/ir"
)

func TestEntityBuildExecutionPlanOrdersDependencies(t *testing.T) {
	builtIR := ir.HelmIR{
		Entities: map[string]ir.HelmEntity{
			"//libs/math:math": {
				Name:        "math",
				Label:       ir.HelmLabel{Path: "libs/math", Name: "math"},
				AdapterName: "c_shared_library",
			},
			"//libs/splash:splash": {
				Name:        "splash",
				Label:       ir.HelmLabel{Path: "libs/splash", Name: "splash"},
				AdapterName: "c_shared_library",
				Deps: []ir.HelmLabel{
					{Path: "libs/math", Name: "math"},
				},
			},
		},
	}

	plan, err := EntityBuildExecutionPlan(builtIR, EntityLegacyRootsFromLabels([]ir.HelmLabel{{Path: "libs/splash", Name: "splash"}}))
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Order) != 2 {
		t.Fatalf("order = %#v", plan.Order)
	}
	if plan.Order[0] != "//libs/math:math" {
		t.Fatalf("math should be first, got %#v", plan.Order)
	}
}

func TestEntityFlattenBagsMergesDirectDependencies(t *testing.T) {
	builtIR := ir.HelmIR{
		Entities: map[string]ir.HelmEntity{
			"//libs/math:math": {
				InterfaceBag: map[string]ir.HelmStringListExpr{
					"INCLUDE_PATHS": {{Kind: ir.StringListLiteral, Literal: "-Imath"}},
				},
			},
		},
	}
	flat := EntityFlattenBags(
		builtIR,
		ir.HelmEntityDepsFromLabels([]ir.HelmLabel{{Path: "libs/math", Name: "math"}}),
		ir.HelmConfigurationDefaultName,
		"INCLUDE_PATHS",
	)
	if len(flat) != 1 || flat[0] != "-Imath" {
		t.Fatalf("flat = %#v", flat)
	}
}

func TestEntityRootsFromTargetDepsCollectsTransitiveEntityLabels(t *testing.T) {
	mathLabel := ir.HelmLabel{Path: "libs/math", Name: "math"}
	splashLabel := ir.HelmLabel{Path: "libs/splash", Name: "splash"}
	testbedLabel := ir.HelmLabel{Path: "testbed", Name: "testbed"}

	builtIR := ir.HelmIR{
		Targets: map[string]ir.HelmTarget{
			"run_app": {
				Name: "run_app",
				DependsOn: []ir.HelmTargetDependency{
					{TargetName: "build_app"},
					{EntityLabel: &testbedLabel},
				},
			},
			"build_app": {
				Name: "build_app",
				DependsOn: []ir.HelmTargetDependency{
					{EntityLabel: &splashLabel},
					{EntityLabel: &mathLabel},
				},
			},
		},
	}

	roots := EntityRootsFromTargetDeps(builtIR, "run_app")
	if len(roots) != 3 {
		t.Fatalf("roots = %#v, want 3 entity instances", roots)
	}

	got := make(map[string]struct{}, len(roots))
	for _, root := range roots {
		got[entityInstanceKey(root)] = struct{}{}
	}

	for _, wantKey := range []string{
		"//testbed:testbed",
		"//libs/splash:splash",
		"//libs/math:math",
	} {
		if _, ok := got[wantKey]; !ok {
			t.Fatalf("missing root %q in %#v", wantKey, roots)
		}
	}
}

func TestEntityRootsFromTargetDepsSkipsRevisitedTargets(t *testing.T) {
	mathLabel := ir.HelmLabel{Path: "libs/math", Name: "math"}

	builtIR := ir.HelmIR{
		Targets: map[string]ir.HelmTarget{
			"entry": {
				Name: "entry",
				DependsOn: []ir.HelmTargetDependency{
					{TargetName: "left"},
					{TargetName: "right"},
				},
			},
			"left": {
				Name: "left",
				DependsOn: []ir.HelmTargetDependency{
					{TargetName: "shared"},
				},
			},
			"right": {
				Name: "right",
				DependsOn: []ir.HelmTargetDependency{
					{TargetName: "shared"},
				},
			},
			"shared": {
				Name: "shared",
				DependsOn: []ir.HelmTargetDependency{
					{EntityLabel: &mathLabel},
				},
			},
		},
	}

	roots := EntityRootsFromTargetDeps(builtIR, "entry")
	if len(roots) != 1 {
		t.Fatalf("roots = %#v, want one deduplicated entity root", roots)
	}
	if entityInstanceKey(roots[0]) != "//libs/math:math" {
		t.Fatalf("root = %#v", roots[0])
	}
}
