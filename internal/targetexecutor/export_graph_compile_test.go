package targetexecutor

import (
	"testing"

	"helm/internal/ir"
)

func TestTargetExecutorExportExecutionGraphCompileArgvAndParameters(t *testing.T) {
	flags := "\n    -g -std=c89 -pedantic\n    -fwrapv -fno-strict-aliasing\n"

	builtIR := ir.HelmIR{
		SourceDirectory: "/tmp/vertex-siege",
		GlobalVariables: map[string]ir.HelmGlobalVariable{
			"APPLICATION_COMPILER_FLAGS": {
				Kind:        ir.HelmGlobalVarString,
				StringValue: flags,
			},
		},
		Targets: map[string]ir.HelmTarget{
			"_compile_objects": {
				Name: "_compile_objects",
				Parameters: []ir.HelmTargetParameter{
					{Name: "SOURCE_FILES"},
					{Name: "OBJ_DIR"},
					{Name: "OUT_NAME"},
					{Name: "COMPILER_FLAGS"},
					{Name: "INCLUDE_FLAGS"},
				},
				Matrix: &ir.HelmMatrix{
					VariableName: "SRC",
					Values: []ir.HelmMatrixValue{
						{Kind: ir.MatrixValueLiteral, Literal: "testbed/main.c"},
					},
				},
				Steps: []ir.HelmTargetStep{
					{
						Kind: ir.TargetStepRun,
						Run: ir.HelmRunCommand{
							Argv: []ir.HelmRunArgvElement{
								{Literal: "tools/scripts/compile_object.sh"},
								{Literal: "${SRC}"},
								{Literal: "${OBJ_DIR}/${OUT_NAME}_obj/${SRC}.o"},
								{Literal: "${OBJ_DIR}/${OUT_NAME}_obj/${SRC}.d"},
								{ParamName: "COMPILER_FLAGS"},
								{ParamName: "INCLUDE_FLAGS"},
							},
						},
					},
				},
			},
			"entry": {
				Name: "entry",
				DependsOn: []ir.HelmTargetDependency{
					{
						TargetName: "_compile_objects",
						Parameters: map[string]ir.HelmParameterValue{
							"SOURCE_FILES": {
								Kind:   ir.HelmParameterScalar,
								Scalar: "testbed/main.c",
							},
							"OBJ_DIR": {
								Kind:   ir.HelmParameterScalar,
								Scalar: "build/obj",
							},
							"OUT_NAME": {
								Kind:   ir.HelmParameterScalar,
								Scalar: "testbed",
							},
							"COMPILER_FLAGS": {
								Kind:       ir.HelmParameterGlobalRef,
								GlobalName: "APPLICATION_COMPILER_FLAGS",
							},
							"INCLUDE_FLAGS": {
								Kind: ir.HelmParameterStringList,
								StringList: ir.HelmStringListExpr{
									{Kind: ir.StringListLiteral, Literal: "-Itestbed"},
								},
							},
						},
					},
				},
				Steps: []ir.HelmTargetStep{
					{Kind: ir.TargetStepRun, Run: ir.HelmRunCommandLiteral("true")},
				},
			},
		},
		Succeeded: true,
	}

	export, err := TargetExecutorExportExecutionGraph(builtIR, "entry", nil)
	if err != nil {
		t.Fatalf("export: %v", err)
	}

	var compileEntry TargetGraphExportEntry
	for _, entry := range export.Targets {
		if entry.CanonicalTarget == "_compile_objects" {
			compileEntry = entry
			break
		}
	}
	if compileEntry.CanonicalTarget == "" {
		t.Fatalf("missing _compile_objects entry: %#v", export.Targets)
	}
	if len(compileEntry.RunArgvs) != 1 {
		t.Fatalf("run_argvs = %#v", compileEntry.RunArgvs)
	}

	argv := compileEntry.RunArgvs[0]
	if len(argv) < 6 {
		t.Fatalf("argv too short: %#v", argv)
	}
	if argv[0] != "tools/scripts/compile_object.sh" {
		t.Fatalf("argv[0] = %q", argv[0])
	}
	if argv[1] != "testbed/main.c" {
		t.Fatalf("argv[1] = %q", argv[1])
	}
	if argv[2] != "build/obj/testbed_obj/testbed/main.c.o" {
		t.Fatalf("argv[2] = %q", argv[2])
	}
	if argv[3] != "build/obj/testbed_obj/testbed/main.c.d" {
		t.Fatalf("argv[3] = %q", argv[3])
	}
	if argv[4] != "-g" || argv[5] != "-std=c89" {
		t.Fatalf("expected split compiler flags, got %#v", argv[4:])
	}
	if argv[len(argv)-1] != "-Itestbed" {
		t.Fatalf("expected include flag last, got %#v", argv)
	}

	includeFlags, ok := compileEntry.Parameters["INCLUDE_FLAGS"].([]string)
	if !ok || len(includeFlags) != 1 || includeFlags[0] != "-Itestbed" {
		t.Fatalf("INCLUDE_FLAGS parameters = %#v", compileEntry.Parameters["INCLUDE_FLAGS"])
	}

	compilerFlags, ok := compileEntry.Parameters["COMPILER_FLAGS"].([]string)
	if !ok || len(compilerFlags) < 2 || compilerFlags[0] != "-g" {
		t.Fatalf("COMPILER_FLAGS parameters = %#v", compileEntry.Parameters["COMPILER_FLAGS"])
	}
}
