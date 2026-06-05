package entityexecutor

import (
	"os"
	"path/filepath"
	"testing"

	"helm/internal/ir"
)

func TestEntityExpandAdapterUsesAdapterRunTemplates(t *testing.T) {
	builtIR := ir.HelmIR{
		SourceDirectory: "/ws",
		GlobalVariables: map[string]ir.HelmGlobalVariable{
			"OBJ_DIR": {Kind: ir.HelmGlobalVarString, StringValue: "build/obj"},
			"LIB_DIR": {Kind: ir.HelmGlobalVarString, StringValue: "build/lib"},
			"LIBRARY_COMPILER_FLAGS": {
				Kind:        ir.HelmGlobalVarString,
				StringValue: "-g",
			},
		},
		Adapters: map[string]ir.HelmAdapterDecl{
			"c_shared_library": {
				Name: "c_shared_library",
				Parameters: []ir.HelmTargetParameter{
					{Name: "SOURCE_FILES"},
					{Name: "OUT_NAME"},
				},
				Matrix: &ir.HelmMatrix{
					VariableName: "SRC",
					Values: []ir.HelmMatrixValue{
						{Kind: ir.MatrixValueParameterRef, ParameterName: "SOURCE_FILES"},
					},
				},
				MatrixLegOutputs: []ir.HelmArtifactInput{
					{Kind: ir.ArtifactInputString, Literal: "${OBJ_DIR}/${OUT_NAME}_obj/${SRC}.o"},
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
				Outputs: []ir.HelmArtifactInput{
					{Kind: ir.ArtifactInputString, Literal: "${LIB_DIR}/lib${OUT_NAME}.so"},
				},
				Runs: []ir.HelmRunCommand{
					{
						Argv: []ir.HelmRunArgvElement{
							{Literal: "link.sh"},
							{Literal: "${LIB_DIR}/lib${OUT_NAME}.so"},
							{ParamName: "MATRIX_OUTPUTS"},
						},
					},
				},
			},
		},
		Entities: map[string]ir.HelmEntity{
			"//lib:lib": {
				Name:        "lib",
				Label:       ir.HelmLabel{Path: "lib", Name: "lib"},
				AdapterName: "c_shared_library",
				Parameters: map[string]ir.HelmParameterValue{
					"SOURCE_FILES": {
						Kind: ir.HelmParameterArtifactItems,
						ArtifactItems: []ir.HelmArtifactInput{
							{Kind: ir.ArtifactInputString, Literal: "src/a.c"},
						},
					},
					"OUT_NAME": {Kind: ir.HelmParameterScalar, Scalar: "lib"},
				},
				InterfaceBag: map[string]ir.HelmStringListExpr{
					"CPPFLAGS": {{Kind: ir.StringListLiteral, Literal: "-I."}},
				},
			},
		},
	}

	plan, err := EntityExpandAdapter("/ws", builtIR, "//lib:lib")
	if err != nil {
		t.Fatal(err)
	}

	if len(plan.Steps) != 2 {
		t.Fatalf("steps = %d, want 2", len(plan.Steps))
	}
	if plan.Steps[0].Argv[0] != "compile.sh" {
		t.Fatalf("matrix argv = %#v", plan.Steps[0].Argv)
	}
	if plan.Steps[0].Argv[2] != "-I." {
		t.Fatalf("CPPFLAGS argv = %#v", plan.Steps[0].Argv)
	}
	if plan.Steps[1].Argv[0] != "link.sh" {
		t.Fatalf("link argv = %#v", plan.Steps[1].Argv)
	}
	if plan.PrimaryOutput != "build/lib/liblib.so" {
		t.Fatalf("primary output = %q", plan.PrimaryOutput)
	}
}

func TestEntityExpandAdapterGlobSourceFiles(t *testing.T) {
	dir := t.TempDir()
	srcDir := filepath.Join(dir, "libs", "splash", "src")
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "splash.c"), []byte("int splash;\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	builtIR := ir.HelmIR{
		SourceDirectory: dir,
		GlobalVariables: map[string]ir.HelmGlobalVariable{
			"OBJ_DIR": {Kind: ir.HelmGlobalVarString, StringValue: "build/obj"},
			"LIB_DIR": {Kind: ir.HelmGlobalVarString, StringValue: "build/lib"},
		},
		Adapters: map[string]ir.HelmAdapterDecl{
			"c_shared_library": {
				Name: "c_shared_library",
				Parameters: []ir.HelmTargetParameter{
					{Name: "SOURCE_FILES"},
					{Name: "OUT_NAME"},
				},
				Matrix: &ir.HelmMatrix{
					VariableName: "SRC",
					Values: []ir.HelmMatrixValue{
						{Kind: ir.MatrixValueParameterRef, ParameterName: "SOURCE_FILES"},
					},
				},
				MatrixLegOutputs: []ir.HelmArtifactInput{
					{Kind: ir.ArtifactInputString, Literal: "${OBJ_DIR}/${OUT_NAME}_obj/${SRC}.o"},
				},
				MatrixRuns: []ir.HelmRunCommand{
					{
						Argv: []ir.HelmRunArgvElement{
							{Literal: "compile.sh"},
							{Literal: "${SRC}"},
						},
					},
				},
				Outputs: []ir.HelmArtifactInput{
					{Kind: ir.ArtifactInputString, Literal: "${LIB_DIR}/lib${OUT_NAME}.so"},
				},
				Runs: []ir.HelmRunCommand{
					{Argv: []ir.HelmRunArgvElement{{Literal: "link.sh"}}},
				},
			},
		},
		Entities: map[string]ir.HelmEntity{
			"//libs/splash:splash": {
				Name:        "splash",
				Label:       ir.HelmLabel{Path: "libs/splash", Name: "splash"},
				AdapterName: "c_shared_library",
				SourceFile:  filepath.Join(dir, "libs", "splash", "splash.helm"),
				Parameters: map[string]ir.HelmParameterValue{
					"SOURCE_FILES": {
						Kind: ir.HelmParameterArtifactItems,
						ArtifactItems: []ir.HelmArtifactInput{
							{
								Kind: ir.ArtifactInputGlob,
								Glob: &ir.HelmGlob{
									BaseDirectory: "src",
									Includes:      []string{"**/*.c"},
									Recursive:     true,
									Types:         "files",
								},
							},
						},
					},
					"OUT_NAME": {Kind: ir.HelmParameterScalar, Scalar: "splash"},
				},
			},
		},
	}

	plan, err := EntityExpandAdapter(dir, builtIR, "//libs/splash:splash")
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Steps) != 2 {
		t.Fatalf("steps = %d, want 2", len(plan.Steps))
	}
	if plan.Steps[0].Argv[1] != "libs/splash/src/splash.c" {
		t.Fatalf("matrix SRC argv = %#v", plan.Steps[0].Argv)
	}
}

func TestEntityExpandAdapterLinkEnvFromInterfaceLDFLAGS(t *testing.T) {
	builtIR := ir.HelmIR{
		SourceDirectory: "/ws",
		GlobalVariables: map[string]ir.HelmGlobalVariable{
			"OBJ_DIR": {Kind: ir.HelmGlobalVarString, StringValue: "build/obj"},
			"LIB_DIR": {Kind: ir.HelmGlobalVarString, StringValue: "build/lib"},
		},
		Adapters: map[string]ir.HelmAdapterDecl{
			"c_shared_library": {
				Name: "c_shared_library",
				Parameters: []ir.HelmTargetParameter{
					{Name: "SOURCE_FILES"},
					{Name: "OUT_NAME"},
					{Name: "DEPENDENCIES", Optional: true, DependencyList: true},
				},
				Matrix: &ir.HelmMatrix{
					VariableName: "SRC",
					Values: []ir.HelmMatrixValue{
						{Kind: ir.MatrixValueParameterRef, ParameterName: "SOURCE_FILES"},
					},
				},
				MatrixRuns: []ir.HelmRunCommand{
					{Argv: []ir.HelmRunArgvElement{{Literal: "compile.sh"}, {Literal: "${SRC}"}}},
				},
				Env: map[string]ir.HelmStringListExpr{
					"LDFLAGS": {
						{Kind: ir.StringListParamRef, ParamName: "LDFLAGS"},
					},
				},
				Outputs: []ir.HelmArtifactInput{
					{Kind: ir.ArtifactInputString, Literal: "${LIB_DIR}/lib${OUT_NAME}.so"},
				},
				Runs: []ir.HelmRunCommand{
					{Argv: []ir.HelmRunArgvElement{{Literal: "link.sh"}}},
				},
			},
		},
		Entities: map[string]ir.HelmEntity{
			"//libs/splash:splash": {
				Name:        "splash",
				AdapterName: "c_shared_library",
				Parameters: map[string]ir.HelmParameterValue{
					"SOURCE_FILES": {
						Kind:          ir.HelmParameterArtifactItems,
						ArtifactItems: []ir.HelmArtifactInput{{Kind: ir.ArtifactInputString, Literal: "a.c"}},
					},
					"OUT_NAME": {Kind: ir.HelmParameterScalar, Scalar: "splash"},
				},
				InterfaceBag: map[string]ir.HelmStringListExpr{
					"LDFLAGS": {
						{Kind: ir.StringListLiteral, Literal: "-lvulkan"},
						{Kind: ir.StringListLiteral, Literal: "-lxcb"},
					},
				},
			},
		},
	}

	plan, err := EntityExpandAdapter("/ws", builtIR, "//libs/splash:splash")
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Steps) != 2 {
		t.Fatalf("steps = %d", len(plan.Steps))
	}
	if plan.Steps[1].Env["LDFLAGS"] != "-lvulkan -lxcb" {
		t.Fatalf("link env LDFLAGS = %#v", plan.Steps[1].Env)
	}
}
