package interpreter

import (
	"testing"

	"helm/internal/entityexecutor"
	"helm/internal/ir"
	"lingua/helm"
	"signal"
)

func TestVertexSiegeAndroidConfigurationGlobals(t *testing.T) {
	specPath, releaseSpec, err := helm.ResolveHelmLSpecPath()
	if err != nil {
		t.Fatal(err)
	}
	defer releaseSpec()

	interp, err := HelmInterpreterTryCreate(specPath)
	if err != nil {
		t.Fatal(err)
	}
	defer HelmInterpreterDestroy(interp)

	root := "/home/user/git-repos/Projects/vertex-siege/Helmfile"
	dispatcher := signal.SignalDispatcherCreate(signal.DiagnosticCategoryManifest{
		{Label: "ERROR", Weight: 20},
	})
	ctx := signal.SignalContextCreate(dispatcher)

	built, err := HelmInterpreterLoadWorkspace(interp, root, ctx)
	if err != nil {
		t.Skipf("vertex-siege Helmfile unavailable: %v", err)
	}

	decl, ok := built.Configurations["android"]
	if !ok {
		t.Fatal("missing android configuration in IR")
	}
	if decl.Globals["OBJ_DIR"].StringValue != "build/android/aarch64/obj" {
		t.Fatalf("android OBJ_DIR = %q", decl.Globals["OBJ_DIR"].StringValue)
	}

	entity, ok := built.Entities["//testing:testing"]
	if !ok {
		t.Fatal("missing testing entity")
	}

	globals := ir.IRGlobalsForFileConfiguration(built, entity.SourceFile, "android")
	if globals["OBJ_DIR"].StringValue != "build/android/aarch64/obj" {
		t.Fatalf("IRGlobals OBJ_DIR = %q", globals["OBJ_DIR"].StringValue)
	}

	adapter := built.Adapters["c_shared_library"]
	if len(adapter.Phases) == 0 || len(adapter.Phases[0].MatrixRuns) == 0 {
		t.Fatal("missing compile phase template")
	}
	if adapter.Phases[0].MatrixRuns[0].Argv[2].Literal != "${OBJ_DIR}/${SRC}.o" {
		t.Fatalf("adapter template not deferred: %q", adapter.Phases[0].MatrixRuns[0].Argv[2].Literal)
	}

	plan, err := entityexecutor.EntityExpandAdapter(built.SourceDirectory, built, "//testing:testing@android", nil)
	if err != nil {
		t.Fatalf("expand adapter: %v", err)
	}
	if len(plan.Steps) == 0 {
		t.Fatal("expected adapter steps")
	}
	wantObject := "build/android/aarch64/obj/testing/src/testing.c.o"
	if plan.Steps[0].Argv[2] != wantObject {
		t.Fatalf("expand adapter object = %q, want %q", plan.Steps[0].Argv[2], wantObject)
	}

	roots := entityexecutor.EntityRootsFromTargetDeps(built, "build_testbed_android")
	export, err := entityexecutor.EntityExportExecutionGraph(built, roots)
	if err != nil {
		t.Fatalf("export graph: %v", err)
	}

	entry, ok := export.Entities["//testing:testing@android"]
	if !ok {
		t.Fatal("missing testing@android in export")
	}
	if len(entry.RunArgvs) == 0 {
		t.Fatal("expected run argvs")
	}
	if entry.RunArgvs[0][2] != wantObject {
		t.Fatalf("export compile object = %q, want %q", entry.RunArgvs[0][2], wantObject)
	}
}
