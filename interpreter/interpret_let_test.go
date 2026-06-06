package interpreter

import (
	"os"
	"path/filepath"
	"testing"

	"helm/internal/ir"
	"lingua/helm"
	"signal"
)

func TestLetBindingExtraction(t *testing.T) {
	t.Parallel()

	specPath, releaseSpec, err := helm.ResolveHelmLSpecPath()
	if err != nil {
		t.Fatalf("resolve spec: %v", err)
	}
	defer releaseSpec()

	helmInterp, err := HelmInterpreterTryCreate(specPath)
	if err != nil {
		t.Fatalf("create interpreter: %v", err)
	}
	defer HelmInterpreterDestroy(helmInterp)

	dir := t.TempDir()
	helmPath := filepath.Join(dir, "let.helm")
	content := `SOURCE_FILES = [
    "testbed/main.c"
]

target link(SOURCE_FILES, OBJ_DIR, OUT_NAME, OUT_DIR) {
    help = "link with let-derived object paths"
    let OBJECT_FILES = map_ext(
        join_prefix(SOURCE_FILES, "${OBJ_DIR}/${OUT_NAME}_obj"),
        ".c",
        ".o"
    )
    artifacts {
        volatile = true
        inputs = OBJECT_FILES
        outputs = [ "${OUT_DIR}/${OUT_NAME}" ]
    }
    run [
        "tools/link_objects.sh",
        "${OUT_DIR}/${OUT_NAME}",
        param OBJECT_FILES
    ]
}
`
	if err := os.WriteFile(helmPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write helm: %v", err)
	}

	dispatcher := signal.SignalDispatcherCreate(signal.DiagnosticCategoryManifest{
		{Label: "ERROR", Weight: 20},
	})
	ctx := signal.SignalContextCreate(dispatcher)

	result := HelmInterpreterInterpretFile(helmInterp, helmPath, ctx)
	if result.Error != nil {
		t.Fatalf("interpret: %v", result.Error)
	}
	if !result.BuiltIR.Succeeded {
		t.Fatal("expected IR build to succeed")
	}

	target, ok := result.BuiltIR.Targets["link"]
	if !ok {
		t.Fatal("missing link target")
	}
	if len(target.LetBindings) != 1 {
		t.Fatalf("let bindings: %#v", target.LetBindings)
	}
	if target.LetBindings[0].Name != "OBJECT_FILES" {
		t.Fatalf("let name: %#v", target.LetBindings[0])
	}
	if target.LetBindings[0].Expr.Kind != ir.PathExprCall {
		t.Fatalf("let expr: %#v", target.LetBindings[0].Expr)
	}
	if target.Artifacts == nil || len(target.Artifacts.Inputs) != 1 {
		t.Fatalf("artifacts inputs: %#v", target.Artifacts)
	}
	if target.Artifacts.Inputs[0].Kind != ir.ArtifactInputLetRef {
		t.Fatalf("expected let ref input, got %#v", target.Artifacts.Inputs[0])
	}
}

func TestLetShadowsParameterRejected(t *testing.T) {
	t.Parallel()

	specPath, releaseSpec, err := helm.ResolveHelmLSpecPath()
	if err != nil {
		t.Fatalf("resolve spec: %v", err)
	}
	defer releaseSpec()

	helmInterp, err := HelmInterpreterTryCreate(specPath)
	if err != nil {
		t.Fatalf("create interpreter: %v", err)
	}
	defer HelmInterpreterDestroy(helmInterp)

	dir := t.TempDir()
	helmPath := filepath.Join(dir, "let_shadow.helm")
	content := `target build(SOURCE_FILES) {
    help = "shadowing let"
    let SOURCE_FILES = map_ext(SOURCE_FILES, ".c", ".o")
    artifacts { volatile = true }
    run "echo nope"
}
`
	if err := os.WriteFile(helmPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write helm: %v", err)
	}

	dispatcher := signal.SignalDispatcherCreate(signal.DiagnosticCategoryManifest{
		{Label: "ERROR", Weight: 20},
	})
	ctx := signal.SignalContextCreate(dispatcher)

	result := HelmInterpreterInterpretFile(helmInterp, helmPath, ctx)
	if result.BuiltIR.Succeeded {
		t.Fatal("expected IR build to fail on let shadowing parameter")
	}
}
