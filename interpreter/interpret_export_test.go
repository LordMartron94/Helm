package interpreter

import (
	"os"
	"path/filepath"
	"testing"

	"helm/internal/ir"
	"lingua/helm"
	"signal"
)

func TestInterpretExportBlockAndCollectEnv(t *testing.T) {
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
	helmPath := filepath.Join(dir, "export.helm")
	content := `LIB_DIR = "build/lib"

target lib() {
	help = "library"
	artifacts {
		outputs = [ "lib.so" ]
	}
	export {
		LD_FLAGS = [ "-lmylib" ]
		C_INCLUDES = [ "-Iinclude" ]
	}
	run "true"
}

target app(DEPS?) {
	help = "application"
	artifacts {
		outputs = [ "app" ]
	}
	env {
		LDFLAGS = [ "-L${LIB_DIR}", collect(DEPS, "LD_FLAGS") ]
	}
	run "true"
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
		t.Fatal("expected successful IR build")
	}

	lib, ok := result.BuiltIR.Targets["lib"]
	if !ok {
		t.Fatal("missing lib target")
	}
	if len(lib.Export["LD_FLAGS"]) != 1 || lib.Export["LD_FLAGS"][0].Literal != "-lmylib" {
		t.Fatalf("lib export LD_FLAGS = %#v", lib.Export["LD_FLAGS"])
	}

	app := result.BuiltIR.Targets["app"]
	if len(app.Env["LDFLAGS"]) != 2 {
		t.Fatalf("app env LDFLAGS = %#v", app.Env["LDFLAGS"])
	}
	if app.Env["LDFLAGS"][1].Collect == nil || app.Env["LDFLAGS"][1].Collect.ExportKey != "LD_FLAGS" {
		t.Fatalf("app env collect = %#v", app.Env["LDFLAGS"][1])
	}
}

func TestInterpretDependencyStringListParameter(t *testing.T) {
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
	helmPath := filepath.Join(dir, "string_list_param.helm")
	content := `target child(FLAGS) {
	help = "child"
	artifacts {
		outputs = [ "out" ]
	}
	run "true"
}

target parent() {
	help = "parent"
	artifacts {
		outputs = [ "out" ]
	}
	depends_on [
		child {
			params {
				FLAGS = [ "-Ione", "-Itwo" ]
			}
		}
	]
	run "true"
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
		t.Fatal("expected successful IR build")
	}

	parent := result.BuiltIR.Targets["parent"]
	if len(parent.DependsOn) != 1 {
		t.Fatal("expected one dependency")
	}
	flags := parent.DependsOn[0].Parameters["FLAGS"]
	if flags.Kind != ir.HelmParameterStringList {
		t.Fatalf("FLAGS kind = %v", flags.Kind)
	}
	if len(flags.StringList) != 2 {
		t.Fatalf("FLAGS = %#v", flags.StringList)
	}
}
