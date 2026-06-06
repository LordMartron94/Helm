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

	plan, err := EntityBuildExecutionPlan(builtIR, []ir.HelmLabel{{Path: "libs/splash", Name: "splash"}})
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

func TestEntityFlattenBagsMergesCPPFLAGS(t *testing.T) {
	builtIR := ir.HelmIR{
		Entities: map[string]ir.HelmEntity{
			"//libs/math:math": {
				InterfaceBag: map[string]ir.HelmStringListExpr{
					"CPPFLAGS": {{Kind: ir.StringListLiteral, Literal: "-Imath"}},
				},
			},
		},
	}
	flat := EntityFlattenBags(builtIR, []ir.HelmLabel{{Path: "libs/math", Name: "math"}}, "CPPFLAGS")
	if len(flat) != 1 || flat[0] != "-Imath" {
		t.Fatalf("flat = %#v", flat)
	}
}
