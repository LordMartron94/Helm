package interpreter_test

import (
	"os"
	"path/filepath"
	"testing"

	"helm/interpreter"
	"lingua/helm"
	"signal"
)

func TestAdapterPhaseDependsOnParsed(t *testing.T) {
	specPath, release, err := helm.ResolveHelmLSpecPath()
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	interp, err := interpreter.HelmInterpreterTryCreate(specPath)
	if err != nil {
		t.Fatal(err)
	}
	defer interpreter.HelmInterpreterDestroy(interp)
	dir := t.TempDir()
	src := `workspace {
}

target generate_compilation_database() {
    help = "gen compile db"
    run "true"
}

adapter c_executable(SOURCE_FILES, OUT_NAME) {
    target _compile {
        matrix SRC in SOURCE_FILES

        depends_on [ generate_compilation_database ]

        outputs = [ "out.o" ]

        run [ "true" ]
    }

    target _link {
        depends_on [ _compile ]

        outputs = [ "bin" ]

        run [ "true" ]
    }
}`
	p := filepath.Join(dir, "t.helm")
	if err := os.WriteFile(p, []byte(src), 0644); err != nil {
		t.Fatal(err)
	}
	d := signal.SignalDispatcherCreate(signal.DiagnosticCategoryManifest{{Label: "ERROR", Weight: 20}})
	ctx := signal.SignalContextCreate(d)
	res := interpreter.HelmInterpreterInterpretFileWithOptions(interp, p, ctx, interpreter.HelmInterpretOptions{})
	if res.Error != nil {
		t.Fatal(res.Error)
	}
	adapter := res.BuiltIR.Adapters["c_executable"]
	if len(adapter.Phases[0].TargetDependsOn) != 1 ||
		adapter.Phases[0].TargetDependsOn[0] != "generate_compilation_database" {
		t.Fatalf("compile phase target hooks = %#v", adapter.Phases[0].TargetDependsOn)
	}
	if len(adapter.Phases[1].DependsOn) != 1 || adapter.Phases[1].DependsOn[0] != "_compile" {
		t.Fatalf("link phase depends = %#v", adapter.Phases[1].DependsOn)
	}
}
