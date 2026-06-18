package ir

import "testing"

func TestHelmEntityConfigurationForFallsBackToDefaultVariant(t *testing.T) {
	entity := HelmEntity{
		Label: HelmLabel{Path: "libs/testing", Name: "testing"},
		Configurations: map[string]HelmEntityConfiguration{
			HelmConfigurationDefaultName: {
				AdapterName: "c_shared_library",
				Parameters:  map[string]HelmParameterValue{"OUT_NAME": {Kind: HelmParameterScalar, Scalar: "testing"}},
				Deps:        []HelmEntityDep{{Label: HelmLabel{Path: "libs/nexus", Name: "nexus"}}},
			},
		},
	}

	variant, err := HelmEntityConfigurationFor(entity, "android")
	if err != nil {
		t.Fatal(err)
	}
	if variant.AdapterName != "c_shared_library" {
		t.Fatalf("adapter = %q, want c_shared_library", variant.AdapterName)
	}
	if len(variant.Deps) != 1 || variant.Deps[0].Label.Name != "nexus" {
		t.Fatalf("deps = %#v", variant.Deps)
	}
}

func TestHelmEntityConfigurationForUsesExplicitOverride(t *testing.T) {
	entity := HelmEntity{
		Label: HelmLabel{Path: "libs/foo", Name: "foo"},
		Configurations: map[string]HelmEntityConfiguration{
			HelmConfigurationDefaultName: {
				AdapterName: "c_static_library",
			},
			"android": {
				AdapterName: "c_static_library",
				Parameters: map[string]HelmParameterValue{
					"CPPFLAGS": {Kind: HelmParameterStringList, StringList: HelmStringListExpr{
						{Kind: StringListLiteral, Literal: "-DFOO_ANDROID"},
					}},
				},
			},
		},
	}

	variant, err := HelmEntityConfigurationFor(entity, "android")
	if err != nil {
		t.Fatal(err)
	}
	if len(variant.Parameters["CPPFLAGS"].StringList) != 1 {
		t.Fatalf("android override params = %#v", variant.Parameters)
	}
}
