package entityexecutor

import (
	"os"
	"path/filepath"
	"testing"

	"helm/internal/ir"
)

func TestEntityCacheInputPathsIncludeDependencySources(t *testing.T) {
	builtIR := ir.HelmIR{
		SourceDirectory: "/ws",
		Entities: map[string]ir.HelmEntity{
			"//libs/echo:echo": {
				Name:        "echo",
				AdapterName: "c_shared_library",
				SourceFile:  "/ws/libs/echo/echo.helm",
				Parameters: map[string]ir.HelmParameterValue{
					"SOURCE_FILES": {
						Kind: ir.HelmParameterArtifactItems,
						ArtifactItems: []ir.HelmArtifactInput{
							{Kind: ir.ArtifactInputString, Literal: "libs/echo/src/e.c"},
						},
					},
				},
			},
			"//testbed:testbed": {
				Name:        "testbed",
				AdapterName: "c_executable",
				SourceFile:  "/ws/testbed/testbed.helm",
				Deps:        []ir.HelmLabel{{Path: "libs/echo", Name: "echo"}},
				Parameters: map[string]ir.HelmParameterValue{
					"SOURCE_FILES": {
						Kind: ir.HelmParameterArtifactItems,
						ArtifactItems: []ir.HelmArtifactInput{
							{Kind: ir.ArtifactInputString, Literal: "testbed/main.c"},
						},
					},
				},
			},
		},
	}

	paths := EntityCacheInputPaths(builtIR, "//testbed:testbed", []string{"testbed/main.c"})
	if len(paths) != 2 {
		t.Fatalf("paths = %#v", paths)
	}
}

func TestEntityCacheFingerprintChangesWhenSourceChanges(t *testing.T) {
	dir := t.TempDir()
	srcDir := filepath.Join(dir, "libs", "echo", "src")
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		t.Fatal(err)
	}
	srcFile := filepath.Join(srcDir, "e.c")
	if err := os.WriteFile(srcFile, []byte("int echo;\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	builtIR := ir.HelmIR{
		SourceDirectory: dir,
		Entities: map[string]ir.HelmEntity{
			"//libs/echo:echo": {
				Name:        "echo",
				AdapterName: "c_shared_library",
				SourceFile:  filepath.Join(dir, "libs/echo/echo.helm"),
				Parameters: map[string]ir.HelmParameterValue{
					"SOURCE_FILES": {
						Kind: ir.HelmParameterArtifactItems,
						ArtifactItems: []ir.HelmArtifactInput{
							{Kind: ir.ArtifactInputString, Literal: "libs/echo/src/e.c"},
						},
					},
					"OUT_NAME": {Kind: ir.HelmParameterScalar, Scalar: "echo"},
				},
			},
		},
		Adapters: map[string]ir.HelmAdapterDecl{
			"c_shared_library": {
				Name: "c_shared_library",
				Phases: []ir.HelmAdapterPhase{
					{Name: "_compile"},
					{Name: "_link", DependsOn: []string{"_compile"}, Outputs: []ir.HelmArtifactInput{
						{Kind: ir.ArtifactInputString, Literal: "build/lib/lib${OUT_NAME}.so"},
					}},
				},
			},
		},
	}

	fp1, err := EntityCacheFingerprint(builtIR, "//libs/echo:echo", "build/lib/libecho.so", []string{"libs/echo/src/e.c"})
	if err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(srcFile, []byte("int echo_changed;\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	fp2, err := EntityCacheFingerprint(builtIR, "//libs/echo:echo", "build/lib/libecho.so", []string{"libs/echo/src/e.c"})
	if err != nil {
		t.Fatal(err)
	}
	if fp1 == fp2 {
		t.Fatalf("fingerprint should change when source changes: %d == %d", fp1, fp2)
	}
}
