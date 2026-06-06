package interpreter

import (
	"os"
	"path/filepath"
	"testing"

	"helm/internal/ir"
	"lingua/helm"
	"signal"
)

func TestInterpretSingleLineStringListInEnvAndDependencyParams(t *testing.T) {
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
	helmPath := filepath.Join(dir, "single_line.helm")
	content := `LIB_DIR = "build/lib"

target lib() {
	help = "library"
	artifacts {
		outputs = [ "lib.so" ]
	}
	export {
		LD_FLAGS = [ "-lmylib" ]
	}
	run "true"
}

target child(FLAGS, DEPS?) {
	help = "child"
	artifacts {
		outputs = [ "out" ]
	}
	env {
		LDFLAGS = [ "-L${LIB_DIR}", collect(DEPS, "LD_FLAGS") ]
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
				DEPS = [ lib ]
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

	child := result.BuiltIR.Targets["child"]
	if len(child.Env["LDFLAGS"]) != 2 {
		t.Fatalf("child env LDFLAGS = %#v", child.Env["LDFLAGS"])
	}

	parent := result.BuiltIR.Targets["parent"]
	flags := parent.DependsOn[0].Parameters["FLAGS"]
	if flags.Kind != ir.HelmParameterStringList || len(flags.StringList) != 2 {
		t.Fatalf("FLAGS = %#v", flags)
	}

	deps := parent.DependsOn[0].Parameters["DEPS"]
	if deps.Kind != ir.HelmParameterDependencyList || len(deps.Dependencies) != 1 {
		t.Fatalf("DEPS = %#v", deps)
	}
}
