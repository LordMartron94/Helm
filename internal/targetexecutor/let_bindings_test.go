package targetexecutor

import (
	"helm/internal/ir"
	"os"
	"path/filepath"
	"testing"
)

func TestTargetExecutorEvaluateLetBindings(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeFixture := func(rel, content string) {
		path := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}
	}
	writeFixture("testbed/main.c", "c")
	writeFixture("testbed/util.c", "c")

	target := ir.HelmTarget{
		Name: "link",
		LetBindings: []ir.HelmPathBinding{
			{
				Name: "OBJECT_FILES",
				Expr: ir.HelmPathExpr{
					Kind:     ir.PathExprCall,
					CallName: "map_ext",
					CallArgs: []ir.HelmPathExpr{
						{
							Kind:     ir.PathExprCall,
							CallName: "join_prefix",
							CallArgs: []ir.HelmPathExpr{
								{Kind: ir.PathExprParamRef, Name: "SOURCE_FILES"},
								{Kind: ir.PathExprLiteral, Literal: "${OBJ_DIR}/${OUT_NAME}_obj"},
							},
						},
						{Kind: ir.PathExprLiteral, Literal: ".c"},
						{Kind: ir.PathExprLiteral, Literal: ".o"},
					},
				},
			},
		},
	}

	globals := map[string]ir.HelmGlobalVariable{}
	paramValues := map[string]ir.HelmParameterValue{
		"SOURCE_FILES": {
			Kind:       ir.HelmParameterGlobalRef,
			GlobalName: "SOURCE_FILES",
		},
		"OBJ_DIR":  {Kind: ir.HelmParameterScalar, Scalar: "build/obj"},
		"OUT_NAME": {Kind: ir.HelmParameterScalar, Scalar: "testbed"},
	}
	globals["SOURCE_FILES"] = ir.HelmGlobalVariable{
		Kind: ir.HelmGlobalVarArtifactArray,
		ArtifactItems: []ir.HelmArtifactInput{
			{Kind: ir.ArtifactInputString, Literal: "testbed/main.c"},
			{Kind: ir.ArtifactInputString, Literal: "testbed/util.c"},
		},
	}

	resolved, err := TargetExecutorResolveInvocationParameters(dir, globals, paramValues)
	if err != nil {
		t.Fatal(err)
	}
	baseInterpCtx, err := TargetExecutorInterpolationGlobals(dir, globals, resolved)
	if err != nil {
		t.Fatal(err)
	}
	interpCtx, err := TargetExecutorEvaluateLetBindings(dir, target, baseInterpCtx)
	if err != nil {
		t.Fatal(err)
	}

	objects, ok := interpCtx.PathLists["OBJECT_FILES"]
	if !ok {
		t.Fatal("missing OBJECT_FILES let binding")
	}
	want := []string{
		"build/obj/testbed_obj/testbed/main.c.o",
		"build/obj/testbed_obj/testbed/util.c.o",
	}
	if len(objects) != len(want) {
		t.Fatalf("objects = %#v", objects)
	}
	for i, path := range objects {
		if path != want[i] {
			t.Fatalf("objects[%d] = %q, want %q", i, path, want[i])
		}
	}
}
