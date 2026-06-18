package entityexecutor

import (
	"testing"

	"helm/internal/ir"
)

func TestEntityPropertyBagReadsInterfaceKeys(t *testing.T) {
	entity := ir.HelmEntity{
		InterfaceBag: map[string]ir.HelmStringListExpr{
			"INCLUDE_PATHS": {{Kind: ir.StringListLiteral, Literal: "-Ilibs/splash/src"}},
			"LINK_LIBS":     {{Kind: ir.StringListLiteral, Literal: "-lsplash"}},
		},
	}

	includePaths := entityBagFragments(entity, "INCLUDE_PATHS")
	if len(includePaths) != 1 || includePaths[0] != "-Ilibs/splash/src" {
		t.Fatalf("INCLUDE_PATHS = %#v", includePaths)
	}

	linkLibs := entityBagFragments(entity, "LINK_LIBS")
	if len(linkLibs) != 1 || linkLibs[0] != "-lsplash" {
		t.Fatalf("LINK_LIBS = %#v", linkLibs)
	}

	missing := entityBagFragments(entity, "UNKNOWN_KEY")
	if len(missing) != 0 {
		t.Fatalf("UNKNOWN_KEY = %#v", missing)
	}
}
