package entityexecutor

import (
	"strings"
	"testing"

	"helm/internal/ir"
)

func TestEntityBuildUsagePropagationTransitive(t *testing.T) {
	t.Parallel()

	builtIR := ir.HelmIR{
		Entities: map[string]ir.HelmEntity{
			"//testbed:testbed": {
				Label: ir.HelmLabel{Path: "testbed", Name: "testbed"},
				Deps:  []ir.HelmLabel{{Path: "libs/echo", Name: "echo"}},
				UsageBag: map[string]ir.HelmStringListExpr{
					"CPPFLAGS": {{Kind: ir.StringListLiteral, Literal: "-DECHO_MAX_SYSTEM_LABEL_LENGTH=15"}},
				},
			},
			"//libs/echo:echo": {
				Label: ir.HelmLabel{Path: "libs/echo", Name: "echo"},
				Deps:  []ir.HelmLabel{{Path: "libs/nexus", Name: "nexus"}},
			},
			"//libs/nexus:nexus": {
				Label: ir.HelmLabel{Path: "libs/nexus", Name: "nexus"},
			},
		},
	}

	propagation := EntityBuildUsagePropagation(
		builtIR,
		EntityLegacyRootsFromLabels([]ir.HelmLabel{{Path: "testbed", Name: "testbed"}}),
	)

	if _, ok := propagation["//testbed:testbed"]; ok {
		t.Fatal("usage must not apply to the declaring entity")
	}

	echoUsage := propagation["//libs/echo:echo"]["CPPFLAGS"]
	if len(echoUsage) != 1 || echoUsage[0] != "-DECHO_MAX_SYSTEM_LABEL_LENGTH=15" {
		t.Fatalf("echo usage = %#v", echoUsage)
	}

	nexusUsage := propagation["//libs/nexus:nexus"]["CPPFLAGS"]
	if len(nexusUsage) != 1 || nexusUsage[0] != "-DECHO_MAX_SYSTEM_LABEL_LENGTH=15" {
		t.Fatalf("nexus usage = %#v", nexusUsage)
	}
}

func TestEntityExpandAdapterAppliesUsageToCompileArgv(t *testing.T) {
	t.Parallel()

	builtIR := ir.HelmIR{
		SourceDirectory: "/ws",
		GlobalVariables: map[string]ir.HelmGlobalVariable{
			"OBJ_DIR": {Kind: ir.HelmGlobalVarString, StringValue: "build/obj"},
			"LIB_DIR": {Kind: ir.HelmGlobalVarString, StringValue: "build/lib"},
			"LIBRARY_COMPILER_FLAGS": {
				Kind:        ir.HelmGlobalVarStringList,
				StringList:  []string{"-shared"},
			},
		},
		Adapters: map[string]ir.HelmAdapterDecl{
			"c_shared_library": {
				Name: "c_shared_library",
				Parameters: []ir.HelmTargetParameter{
					{Name: "SOURCE_FILES"},
					{Name: "OUT_NAME"},
					{Name: "CPPFLAGS", Optional: true},
				},
				Matrix: &ir.HelmMatrix{
					VariableName: "SRC",
					Values: []ir.HelmMatrixValue{
						{Kind: ir.MatrixValueParameterRef, ParameterName: "SOURCE_FILES"},
					},
				},
				MatrixRuns: []ir.HelmRunCommand{
					{
						Argv: []ir.HelmRunArgvElement{
							{Literal: "compile.sh"},
							{Literal: "${SRC}"},
							{ParamName: "CPPFLAGS"},
						},
					},
				},
				Runs: []ir.HelmRunCommand{
					{Argv: []ir.HelmRunArgvElement{{Literal: "link.sh"}}},
				},
			},
		},
		Entities: map[string]ir.HelmEntity{
			"//libs/echo:echo": {
				Name:        "echo",
				AdapterName: "c_shared_library",
				Parameters: map[string]ir.HelmParameterValue{
					"SOURCE_FILES": {
						Kind:          ir.HelmParameterArtifactItems,
						ArtifactItems: []ir.HelmArtifactInput{{Kind: ir.ArtifactInputString, Literal: "src/e.c"}},
					},
					"OUT_NAME": {Kind: ir.HelmParameterScalar, Scalar: "echo"},
					"CPPFLAGS": {
						Kind: ir.HelmParameterStringList,
						StringList: ir.HelmStringListExpr{
							{Kind: ir.StringListLiteral, Literal: "-I."},
						},
					},
				},
			},
		},
	}

	usage := EntityPropertyBag{
		"CPPFLAGS": {"-DECHO_MAX_SYSTEM_LABEL_LENGTH=15"},
	}

	plan, err := EntityExpandAdapter("/ws", builtIR, "//libs/echo:echo", usage)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Steps) == 0 {
		t.Fatal("expected compile step")
	}

	argv := strings.Join(plan.Steps[0].Argv, " ")
	if !strings.Contains(argv, "-I.") {
		t.Fatalf("local CPPFLAGS missing: %q", argv)
	}
	if !strings.Contains(argv, "-DECHO_MAX_SYSTEM_LABEL_LENGTH=15") {
		t.Fatalf("usage CPPFLAGS missing: %q", argv)
	}
	if strings.Index(argv, "-I.") > strings.Index(argv, "-DECHO_MAX_SYSTEM_LABEL_LENGTH=15") {
		t.Fatalf("usage CPPFLAGS should follow local flags: %q", argv)
	}
}

func TestEntityCacheFingerprintIncludesUsage(t *testing.T) {
	t.Parallel()

	builtIR := ir.HelmIR{
		SourceDirectory: "/ws",
		Entities: map[string]ir.HelmEntity{
			"//libs/echo:echo": {
				Name:        "echo",
				AdapterName: "c_shared_library",
			},
		},
	}

	fpNone, err := EntityCacheFingerprint(
		builtIR,
		"//libs/echo:echo",
		"build/lib/libecho.so",
		[]string{"src/e.c"},
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}

	fpWithUsage, err := EntityCacheFingerprint(
		builtIR,
		"//libs/echo:echo",
		"build/lib/libecho.so",
		[]string{"src/e.c"},
		EntityUsagePropagation{
			"//libs/echo:echo": {
				"CPPFLAGS": {"-DECHO_MAX_SYSTEM_LABEL_LENGTH=15"},
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	if fpNone == fpWithUsage {
		t.Fatalf("usage must affect cache fingerprint: %d vs %d", fpNone, fpWithUsage)
	}
}
