package entityexecutor

import (
	"testing"

	"helm/internal/ir"
)

// TestEntityPropertyBagParitySplash documents v1→v2 bag key mapping for vertex-siege.
func TestEntityPropertyBagParitySplash(t *testing.T) {
	entity := ir.HelmEntity{
		InterfaceBag: map[string]ir.HelmStringListExpr{
			"CPPFLAGS": {{Kind: ir.StringListLiteral, Literal: "-Ilibs/splash/src"}},
			"LDFLAGS":  {{Kind: ir.StringListLiteral, Literal: "-lsplash"}},
		},
	}

	cpp := entityBagFragments(entity, "CPPFLAGS")
	if len(cpp) != 1 || cpp[0] != "-Ilibs/splash/src" {
		t.Fatalf("CPPFLAGS = %#v", cpp)
	}

	// v1.7 export key alias
	v1Includes := entityBagFragments(entity, "C_INCLUDES")
	if len(v1Includes) != 0 {
		t.Fatalf("unexpected C_INCLUDES without alias setup: %#v", v1Includes)
	}

	entity.InterfaceBag["C_INCLUDES"] = entity.InterfaceBag["CPPFLAGS"]
	alias := entityBagFragments(entity, "C_INCLUDES")
	if len(alias) != 1 || alias[0] != "-Ilibs/splash/src" {
		t.Fatalf("C_INCLUDES alias = %#v", alias)
	}
}
