package interpreter

import (
	"os"
	"path/filepath"
	"testing"

	"lingua/helm"
	"signal"
)

func TestDependsOnCommaSeparatedDependencies(t *testing.T) {
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
	helmPath := filepath.Join(dir, "comma.helm")
	if err := os.WriteFile(helmPath, []byte(`target first() {
    help = "first"
    artifacts { volatile = true }
    run "true"
}

target second() {
    help = "second"
    artifacts { volatile = true }
    run "true"
}

target root() {
    help = "root"
    artifacts { volatile = true }
    depends_on [ first, second ]
    run "true"
}
`), 0o644); err != nil {
		t.Fatal(err)
	}

	dispatcher := signal.SignalDispatcherCreate(signal.DiagnosticCategoryManifest{
		{Label: "ERROR", Weight: 20},
	})
	ctx := signal.SignalContextCreate(dispatcher)

	result := HelmInterpreterInterpretFile(interp, helmPath, ctx)
	if result.Error != nil {
		t.Fatalf("parse error: %v", result.Error)
	}
	if !result.BuiltIR.Succeeded {
		t.Fatal("semantic errors")
	}
}
