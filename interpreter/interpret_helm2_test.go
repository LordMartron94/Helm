package interpreter

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"helm/internal/entityexecutor"
	"helm/internal/ir"
	"lingua/helm"
	"signal"
)

func TestHelm2EntityAndWorkspaceSegregation(t *testing.T) {
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

	dir := t.TempDir()
	helmPath := filepath.Join(dir, "workspace.helm")
	writeFile(t, helmPath, `workspace {
}

adapter c_shared_library(SOURCE_FILES, OUT_NAME) {
    matrix SRC in SOURCE_FILES
    run [ "true" ]
    run [ "true" ]
}

entity splash {
    kind = "lib"
    use c_shared_library {
        params {
            SOURCE_FILES = [ "src/a.c" ]
            OUT_NAME = "splash"
        }
    }
    interface {
        CPPFLAGS = [ "-Isrc" ]
        LDFLAGS = [ "-lsplash" ]
    }
}

target run_app() {
    help = "run application smoke target"
    run "true"
}
`)

	dispatcher := signal.SignalDispatcherCreate(signal.DiagnosticCategoryManifest{
		{Label: "ERROR", Weight: 20},
	})
	ctx := signal.SignalContextCreate(dispatcher)

	merged, err := HelmInterpreterLoadWorkspace(interp, helmPath, ctx)
	if err != nil {
		t.Fatal(err)
	}
	if merged.Mode != ir.HelmModeWorkspace {
		t.Fatalf("mode = %v", merged.Mode)
	}
	if len(merged.Entities) != 1 {
		t.Fatalf("entities = %#v", merged.Entities)
	}
	if _, ok := merged.Targets["run_app"]; !ok {
		t.Fatal("missing run_app target")
	}
	for name, target := range merged.Targets {
		if target.Artifacts != nil {
			t.Fatalf("target %s has artifacts in workspace mode", name)
		}
	}
}

func TestHelm2LegacyModeWithoutWorkspace(t *testing.T) {
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

	dir := t.TempDir()
	helmPath := filepath.Join(dir, "legacy.helm")
	writeFile(t, helmPath, `target build() {
    help = "legacy build target"
    artifacts {
        inputs = []
        outputs = [ "out.bin" ]
    }
    run "true"
}
`)

	dispatcher := signal.SignalDispatcherCreate(signal.DiagnosticCategoryManifest{
		{Label: "ERROR", Weight: 20},
	})
	ctx := signal.SignalContextCreate(dispatcher)

	merged, err := HelmInterpreterLoadWorkspace(interp, helmPath, ctx)
	if err != nil {
		t.Fatal(err)
	}
	if merged.Mode != ir.HelmModeLegacy {
		t.Fatalf("mode = %v", merged.Mode)
	}
}

func TestHelm2WorkspaceCaseFile(t *testing.T) {
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

	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "Helmfile"), `OBJ_DIR = "build/obj"
LIB_DIR = "build/lib"
LIBRARY_COMPILER_FLAGS = "-g"

workspace {
    globals {
        OBJ_DIR = OBJ_DIR
        LIB_DIR = LIB_DIR
    }
}

adapter c_shared_library(SOURCE_FILES, OUT_NAME) {
    matrix SRC in SOURCE_FILES
    outputs = [ "${OBJ_DIR}/${OUT_NAME}_obj/${SRC}.o" ]
    run [ "true" ]
    outputs = [ "${LIB_DIR}/lib${OUT_NAME}.so" ]
    run [ "true" ]
}

entity sample {
    kind = "lib"
    use c_shared_library {
        params {
            SOURCE_FILES = [ "src/sample.c" ]
            OUT_NAME = "sample"
        }
    }
    interface {
        CPPFLAGS = [ "-Isrc" ]
    }
}

target smoke() {
    help = "workspace smoke target"
    depends_on [ "//sample:sample" ]
    run "true"
}
`)

	dispatcher := signal.SignalDispatcherCreate(signal.DiagnosticCategoryManifest{
		{Label: "ERROR", Weight: 20},
	})
	ctx := signal.SignalContextCreate(dispatcher)

	merged, err := HelmInterpreterLoadWorkspace(interp, filepath.Join(dir, "Helmfile"), ctx)
	if err != nil {
		t.Fatal(err)
	}
	if merged.Mode != ir.HelmModeWorkspace {
		t.Fatalf("mode = %v", merged.Mode)
	}
	if _, ok := merged.Adapters["c_shared_library"]; !ok {
		t.Fatal("missing c_shared_library adapter")
	}
	if _, ok := merged.Entities["//sample:sample"]; !ok {
		t.Fatalf("entities = %#v", merged.Entities)
	}
}

func TestHelm2AdapterEnvBlockParsesParamAndCollect(t *testing.T) {
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

	dir := t.TempDir()
	adapterPath := filepath.Join(dir, "adapter.helm")
	writeFile(t, adapterPath, `adapter c_shared_library(SOURCE_FILES, OUT_NAME, DEPENDENCIES?) {
    matrix SRC in SOURCE_FILES
    run [ "compile" ]
    env {
        LDFLAGS = [ param LDFLAGS, collect(DEPENDENCIES, "LDFLAGS") ]
    }
    run [ "link" ]
}
`)

	dispatcher := signal.SignalDispatcherCreate(signal.DiagnosticCategoryManifest{
		{Label: "ERROR", Weight: 20},
	})
	ctx := signal.SignalContextCreate(dispatcher)

	result := HelmInterpreterInterpretFile(interp, adapterPath, ctx)
	if result.Error != nil {
		t.Fatal(result.Error)
	}
	decl := result.BuiltIR.Adapters["c_shared_library"]
	if !result.BuiltIR.Succeeded {
		t.Fatal("semantic errors in adapter env test")
	}
	if len(decl.Env["LDFLAGS"]) != 2 {
		t.Fatalf("env = %#v", decl.Env)
	}
}

func TestHelm2VertexSiegeSplashLinkEnv(t *testing.T) {
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

	manifestPath := filepath.Join("..", "..", "..", "..", "vertex-siege", "Helmfile")
	if _, err := os.Stat(manifestPath); err != nil {
		t.Skip("vertex-siege Helmfile not available:", err)
	}

	dispatcher := signal.SignalDispatcherCreate(signal.DiagnosticCategoryManifest{
		{Label: "ERROR", Weight: 20},
	})
	ctx := signal.SignalContextCreate(dispatcher)

	merged, err := HelmInterpreterLoadWorkspace(interp, manifestPath, ctx)
	if err != nil {
		t.Fatal(err)
	}

	adapter := merged.Adapters["c_shared_library"]
	if len(adapter.Phases) != 2 {
		t.Fatalf("adapter phases = %#v", adapter.Phases)
	}

	plan, err := entityexecutor.EntityExpandAdapter(merged.SourceDirectory, merged, "//libs/splash:splash")
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Steps) < 2 {
		t.Fatalf("steps = %d", len(plan.Steps))
	}
	linkStep := plan.Steps[len(plan.Steps)-1]
	if !strings.Contains(strings.Join(linkStep.Argv, " "), "libsplash.so") {
		t.Fatalf("link argv = %#v", linkStep.Argv)
	}
	linkArgv := strings.Join(linkStep.Argv, " ")
	if strings.Contains(linkArgv, "-lvulkan") || strings.Contains(linkArgv, "-lxcb") {
		t.Fatalf("producer link must not inherit own interface LDFLAGS: %#v", linkStep.Argv)
	}
}

func TestHelm2VertexSiegeEchoInterfaceNotSelfLinked(t *testing.T) {
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

	manifestPath := filepath.Join("..", "..", "..", "..", "vertex-siege", "Helmfile")
	if _, err := os.Stat(manifestPath); err != nil {
		t.Skip("vertex-siege Helmfile not available:", err)
	}

	dispatcher := signal.SignalDispatcherCreate(signal.DiagnosticCategoryManifest{
		{Label: "ERROR", Weight: 20},
	})
	ctx := signal.SignalContextCreate(dispatcher)

	merged, err := HelmInterpreterLoadWorkspace(interp, manifestPath, ctx)
	if err != nil {
		t.Fatal(err)
	}

	rootFlags := ir.IRGlobalsForRootManifest(merged)["LIBRARY_COMPILER_FLAGS"]
	if len(rootFlags.ArtifactItems) < 3 {
		t.Fatalf("root LIBRARY_COMPILER_FLAGS items = %d", len(rootFlags.ArtifactItems))
	}

	echoPlan, err := entityexecutor.EntityExpandAdapter(merged.SourceDirectory, merged, "//libs/echo:echo")
	if err != nil {
		t.Fatal(err)
	}
	echoLink := echoPlan.Steps[len(echoPlan.Steps)-1].Argv
	echoArgv := strings.Join(echoLink, " ")
	if strings.Contains(echoArgv, "-lecho") {
		t.Fatalf("echo producer link must not include own interface -lecho: %#v", echoLink)
	}
	if !strings.Contains(echoArgv, "-shared") {
		t.Fatalf("echo shared-library link must include -shared from LIBRARY_COMPILER_FLAGS: %#v", echoLink)
	}

	testbedPlan, err := entityexecutor.EntityExpandAdapter(merged.SourceDirectory, merged, "//testbed:testbed")
	if err != nil {
		t.Fatal(err)
	}
	testbedLink := testbedPlan.Steps[len(testbedPlan.Steps)-1].Argv
	testbedArgv := strings.Join(testbedLink, " ")
	if !strings.Contains(testbedArgv, "-lecho") {
		t.Fatalf("testbed consumer link must collect echo LDFLAGS: %#v", testbedLink)
	}
	if !strings.Contains(testbedArgv, "-Lbuild/lib") {
		t.Fatalf("testbed link must get workspace -L from c_executable adapter: %#v", testbedLink)
	}
	if !strings.Contains(testbedArgv, "-Wl,-rpath,$ORIGIN/../lib") {
		t.Fatalf("testbed link must get workspace rpath from c_executable adapter: %#v", testbedLink)
	}
}

func TestHelm2ParseVertexSiegeWorkspace(t *testing.T) {
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

	manifestPath := filepath.Join("..", "..", "..", "..", "vertex-siege", "Helmfile")
	if _, err := os.Stat(manifestPath); err != nil {
		t.Skip("vertex-siege Helmfile not available:", err)
	}

	dispatcher := signal.SignalDispatcherCreate(signal.DiagnosticCategoryManifest{
		{Label: "ERROR", Weight: 20},
	})
	ctx := signal.SignalContextCreate(dispatcher)

	merged, err := HelmInterpreterLoadWorkspace(interp, manifestPath, ctx)
	if err != nil {
		t.Fatal(err)
	}
	if merged.Mode != ir.HelmModeWorkspace {
		t.Fatalf("mode = %v", merged.Mode)
	}
	if _, ok := merged.Entities["//libs/splash:splash"]; !ok {
		t.Fatalf("entities = %#v", merged.Entities)
	}
}

func TestHelm2WorkspaceGlobalsAcceptLiteralValues(t *testing.T) {
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

	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "libs", "sample", "sample.helm"), `entity sample {
    use noop {
        params {
            OUT = "${LIB_DIR}/libsample.so"
        }
    }
}
`)
	writeFile(t, filepath.Join(dir, "Helmfile"), `workspace {
    globals {
        LIB_DIR = "build/lib"
    }
}

adapter noop(OUT) {
    run [ "true" ]
}
`)

	dispatcher := signal.SignalDispatcherCreate(signal.DiagnosticCategoryManifest{
		{Label: "ERROR", Weight: 20},
	})
	ctx := signal.SignalContextCreate(dispatcher)

	merged, err := HelmInterpreterLoadWorkspace(interp, filepath.Join(dir, "Helmfile"), ctx)
	if err != nil {
		t.Fatal(err)
	}

	if merged.Workspace.Globals["LIB_DIR"].StringValue != "build/lib" {
		t.Fatalf("LIB_DIR = %#v", merged.Workspace.Globals["LIB_DIR"])
	}
}

func TestHelm2WorkspaceGlobalsInjectIntoDiscoveredFiles(t *testing.T) {
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

	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "libs", "sample", "sample.helm"), `entity sample {
    use noop {
        params {
            OUT = "${LIB_DIR}/libsample.so"
        }
    }
}
`)
	writeFile(t, filepath.Join(dir, "Helmfile"), `LIB_DIR = "build/lib"

workspace {
    globals {
        LIB_DIR = LIB_DIR
    }
}

adapter noop(OUT) {
    run [ "true" ]
}
`)

	dispatcher := signal.SignalDispatcherCreate(signal.DiagnosticCategoryManifest{
		{Label: "ERROR", Weight: 20},
	})
	ctx := signal.SignalContextCreate(dispatcher)

	merged, err := HelmInterpreterLoadWorkspace(interp, filepath.Join(dir, "Helmfile"), ctx)
	if err != nil {
		t.Fatal(err)
	}

	entity, ok := merged.Entities["//libs/sample:sample"]
	if !ok {
		t.Fatalf("entities = %#v", merged.Entities)
	}
	param := entity.Parameters["OUT"]
	if param.Kind != ir.HelmParameterScalar || param.Scalar != "build/lib/libsample.so" {
		t.Fatalf("OUT = %#v", param)
	}
}

func TestHelm2EntityGlobParamIR(t *testing.T) {
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

	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "src", "a.c"), "int x;\n")
	helmPath := filepath.Join(dir, "workspace.helm")
	writeFile(t, helmPath, `workspace {
}

adapter c_shared_library(SOURCE_FILES, OUT_NAME) {
    matrix SRC in SOURCE_FILES
    run [ "true" ]
    run [ "true" ]
}

entity splash {
    use c_shared_library {
        params {
            SOURCE_FILES = [
                glob("src", include="**/*.c", recursive=true),
            ]
            OUT_NAME = "splash"
        }
    }
}
`)

	dispatcher := signal.SignalDispatcherCreate(signal.DiagnosticCategoryManifest{
		{Label: "ERROR", Weight: 20},
	})
	ctx := signal.SignalContextCreate(dispatcher)

	merged, err := HelmInterpreterLoadWorkspace(interp, helmPath, ctx)
	if err != nil {
		t.Fatal(err)
	}

	entity, ok := merged.Entities["//splash:splash"]
	if !ok {
		t.Fatalf("entities = %#v", merged.Entities)
	}
	param := entity.Parameters["SOURCE_FILES"]
	if param.Kind != ir.HelmParameterArtifactItems {
		t.Fatalf("SOURCE_FILES kind = %v, want HelmParameterArtifactItems; %#v", param.Kind, param)
	}
	if len(param.ArtifactItems) != 1 || param.ArtifactItems[0].Kind != ir.ArtifactInputGlob {
		t.Fatalf("SOURCE_FILES items = %#v", param.ArtifactItems)
	}
	if param.ArtifactItems[0].Glob == nil || param.ArtifactItems[0].Glob.BaseDirectory != "src" {
		t.Fatalf("glob = %#v", param.ArtifactItems[0].Glob)
	}
}

func TestHelm2WorkspaceRejectsTargetArtifacts(t *testing.T) {
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

	dir := t.TempDir()
	helmPath := filepath.Join(dir, "bad.helm")
	writeFile(t, helmPath, `workspace {
}

adapter c_shared_library(SOURCE_FILES) {
    matrix SRC in SOURCE_FILES
    run [ "true" ]
    run [ "true" ]
}

entity lib {
    use c_shared_library {
        params {
            SOURCE_FILES = [ "a.c" ]
        }
    }
}

target build() {
    help = "bad"
    artifacts { outputs = [ "out.bin" ] }
    run "true"
}
`)

	dispatcher := signal.SignalDispatcherCreate(signal.DiagnosticCategoryManifest{
		{Label: "ERROR", Weight: 20},
	})
	ctx := signal.SignalContextCreate(dispatcher)

	_, err = HelmInterpreterLoadWorkspace(interp, helmPath, ctx)
	if err == nil {
		t.Fatal("expected workspace segregation error")
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
