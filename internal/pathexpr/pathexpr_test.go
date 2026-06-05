package pathexpr

import (
	"helm/internal/ir"
	"testing"
)

func TestPathExprJoinPrefixMapExt(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	scope := PathExprScope{
		PathLists: map[string][]string{
			"SOURCE_FILES": {"testbed/main.c", "testbed/util.c"},
		},
		Scalars: map[string]string{
			"OBJ_DIR":  "build/obj",
			"OUT_NAME": "testbed",
		},
	}

	joinExpr := ir.HelmPathExpr{
		Kind:     ir.PathExprCall,
		CallName: "join_prefix",
		CallArgs: []ir.HelmPathExpr{
			{Kind: ir.PathExprLetRef, Name: "SOURCE_FILES"},
			{Kind: ir.PathExprLiteral, Literal: "${OBJ_DIR}/${OUT_NAME}_obj"},
		},
	}
	joined, err := PathExprEvaluate(root, joinExpr, scope)
	if err != nil {
		t.Fatal(err)
	}

	mapExpr := ir.HelmPathExpr{
		Kind:     ir.PathExprCall,
		CallName: "map_ext",
		CallArgs: []ir.HelmPathExpr{
			{Kind: ir.PathExprLiteral, Literal: ""}, // replaced below
			{Kind: ir.PathExprLiteral, Literal: ".c"},
			{Kind: ir.PathExprLiteral, Literal: ".o"},
		},
	}
	scope.PathLists["JOINED"] = joined
	mapExpr.CallArgs[0] = ir.HelmPathExpr{Kind: ir.PathExprLetRef, Name: "JOINED"}

	objects, err := PathExprEvaluate(root, mapExpr, scope)
	if err != nil {
		t.Fatal(err)
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
