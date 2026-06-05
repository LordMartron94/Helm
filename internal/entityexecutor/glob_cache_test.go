package entityexecutor

import (
	"os"
	"path/filepath"
	"testing"

	"helm/internal/ir"
)

func TestEntityCacheInputPathsFromGlobbedSourceFiles(t *testing.T) {
	dir := t.TempDir()
	srcDir := filepath.Join(dir, "libs", "echo", "src")
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		t.Fatal(err)
	}
	first := filepath.Join(srcDir, "a.c")
	second := filepath.Join(srcDir, "b.c")
	for _, file := range []string{first, second} {
		if err := os.WriteFile(file, []byte("int x;\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	helmFile := filepath.Join(dir, "libs", "echo", "echo.helm")
	if err := os.MkdirAll(filepath.Dir(helmFile), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(helmFile, []byte("entity echo {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	builtIR := ir.HelmIR{
		SourceDirectory: dir,
		Entities: map[string]ir.HelmEntity{
			"//libs/echo:echo": {
				Name:        "echo",
				Label:       ir.HelmLabel{Path: "libs/echo", Name: "echo"},
				AdapterName: "c_shared_library",
				SourceFile:  helmFile,
				Parameters: map[string]ir.HelmParameterValue{
					"SOURCE_FILES": {
						Kind: ir.HelmParameterArtifactItems,
						ArtifactItems: []ir.HelmArtifactInput{
							{
								Kind: ir.ArtifactInputGlob,
								Glob: &ir.HelmGlob{
									BaseDirectory: "src",
									Includes:      []string{"**/*.c"},
									Recursive:     true,
								},
							},
						},
					},
				},
			},
		},
	}

	resolved, err := EntityResolveParameters(dir, builtIR, builtIR.Entities["//libs/echo:echo"])
	if err != nil {
		t.Fatal(err)
	}
	paths := entityResolvedSourcePaths(resolved)
	if len(paths) != 2 {
		t.Fatalf("resolved source paths = %#v", paths)
	}

	cachePaths := EntityCacheInputPaths(builtIR, "//libs/echo:echo", paths)
	if len(cachePaths) != 2 {
		t.Fatalf("cache input paths = %#v", cachePaths)
	}
}
