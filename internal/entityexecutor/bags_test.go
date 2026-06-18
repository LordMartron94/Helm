package entityexecutor

import (
	"testing"

	"helm/internal/ir"
)

func TestEntityFlattenBagsPreservesDeclarationOrder(t *testing.T) {
	builtIR := ir.HelmIR{
		Entities: map[string]ir.HelmEntity{
			"//libs/echo:echo": {
				InterfaceBag: map[string]ir.HelmStringListExpr{
					"LINK_LIBS": {{Kind: ir.StringListLiteral, Literal: "-lecho"}},
				},
			},
			"//libs/loom:loom": {
				InterfaceBag: map[string]ir.HelmStringListExpr{
					"LINK_LIBS": {{Kind: ir.StringListLiteral, Literal: "-lloom"}},
				},
			},
		},
	}

	flat := EntityFlattenBags(
		builtIR,
		[]ir.HelmLabel{
			{Path: "libs/echo", Name: "echo"},
			{Path: "libs/loom", Name: "loom"},
		},
		"LINK_LIBS",
	)
	if len(flat) != 2 || flat[0] != "-lecho" || flat[1] != "-lloom" {
		t.Fatalf("flat = %#v", flat)
	}
}

func TestEntityFlattenBagsClosureOrdersDependentsBeforeDependencies(t *testing.T) {
	builtIR := ir.HelmIR{
		Entities: map[string]ir.HelmEntity{
			"//libs/nexus:nexus": {
				Label: ir.HelmLabel{Path: "libs/nexus", Name: "nexus"},
				InterfaceBag: map[string]ir.HelmStringListExpr{
					"LINK_LIBS": {{Kind: ir.StringListLiteral, Literal: "-lnexus"}},
				},
			},
			"//libs/echo:echo": {
				Label: ir.HelmLabel{Path: "libs/echo", Name: "echo"},
				Deps:  []ir.HelmLabel{{Path: "libs/nexus", Name: "nexus"}},
				InterfaceBag: map[string]ir.HelmStringListExpr{
					"LINK_LIBS": {{Kind: ir.StringListLiteral, Literal: "-lecho"}},
				},
			},
			"//libs/loom:loom": {
				Label: ir.HelmLabel{Path: "libs/loom", Name: "loom"},
				Deps:  []ir.HelmLabel{{Path: "libs/nexus", Name: "nexus"}},
				InterfaceBag: map[string]ir.HelmStringListExpr{
					"LINK_LIBS": {{Kind: ir.StringListLiteral, Literal: "-lloom"}},
				},
			},
		},
	}

	flat := EntityFlattenBagsClosure(
		builtIR,
		[]ir.HelmLabel{
			{Path: "libs/echo", Name: "echo"},
			{Path: "libs/loom", Name: "loom"},
		},
		"LINK_LIBS",
	)
	if len(flat) != 3 {
		t.Fatalf("flat = %#v", flat)
	}
	if flat[0] != "-lecho" && flat[0] != "-lloom" {
		t.Fatalf("first consumer flag = %q", flat[0])
	}
	if flat[1] != "-lecho" && flat[1] != "-lloom" {
		t.Fatalf("second consumer flag = %q", flat[1])
	}
	if flat[2] != "-lnexus" {
		t.Fatalf("foundation flag = %q, want -lnexus", flat[2])
	}
}
