package interpreter

import (
	"os"
	"path/filepath"
	"testing"

	"lingua/helm"
	"signal"
)

func TestVertexSiegeFilesParse(t *testing.T) {
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

	root := filepath.Join("..", "..", "..", "..", "vertex-siege")
	files := []string{
		filepath.Join(root, "infra", "adapters", "c_shared_library.helm"),
		filepath.Join(root, "libs", "splash", "splash.helm"),
		filepath.Join(root, "Helmfile.v2"),
	}
	for _, file := range files {
		if _, err := os.Stat(file); err != nil {
			t.Skip(file, err)
		}
		t.Run(filepath.Base(file), func(t *testing.T) {
			dispatcher := signal.SignalDispatcherCreate(signal.DiagnosticCategoryManifest{
				{Label: "ERROR", Weight: 20},
			})
			ctx := signal.SignalContextCreate(dispatcher)
			result := HelmInterpreterInterpretFile(interp, file, ctx)
			if result.Error != nil {
				t.Fatal(result.Error)
			}
			if !result.BuiltIR.Succeeded {
				t.Fatal("semantic errors")
			}
		})
	}
}
