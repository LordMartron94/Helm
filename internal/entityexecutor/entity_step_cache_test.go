package entityexecutor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"helm/internal/cache"
	"helm/internal/ir"
)

func TestEntityStepCompileObjectPaths(t *testing.T) {
	argv := []string{
		"tools/scripts/compile_object.sh",
		"libs/echo/src/e_message.c",
		"build/obj/libs/echo/src/e_message.c.o",
		"build/obj/libs/echo/src/e_message.c.d",
		"-g",
	}
	sourcePath, objectPath, manifestPath, ok := entityStepCompileObjectPaths(argv)
	if !ok {
		t.Fatal("expected compile_object.sh step")
	}
	if sourcePath != "libs/echo/src/e_message.c" {
		t.Fatalf("source = %q", sourcePath)
	}
	if objectPath != "build/obj/libs/echo/src/e_message.c.o" {
		t.Fatalf("object = %q", objectPath)
	}
	if manifestPath != "build/obj/libs/echo/src/e_message.c.deps" {
		t.Fatalf("manifest = %q", manifestPath)
	}
}

func TestEntityStepCacheFingerprintChangesWhenHeaderInManifestChanges(t *testing.T) {
	dir := t.TempDir()
	sourceRel := "libs/echo/src/e_message.c"
	headerRel := "libs/nexus/nexus.h"
	sourcePath := filepath.Join(dir, filepath.FromSlash(sourceRel))
	headerPath := filepath.Join(dir, filepath.FromSlash(headerRel))
	manifestRel := "build/obj/libs/echo/src/e_message.c.deps"
	manifestPath := filepath.Join(dir, filepath.FromSlash(manifestRel))

	for _, path := range []string{sourcePath, headerPath} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(sourcePath, []byte("#include <nexus/nexus.h>\nint x;\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(headerPath, []byte("int nexus_v1;\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(manifestPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifestPath, []byte(headerRel+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	builtIR := testEntityStepCacheBuiltIR(dir)
	step := EntityAdapterStep{
		Argv: []string{
			"tools/scripts/compile_object.sh",
			sourceRel,
			"build/obj/libs/echo/src/e_message.c.o",
			"build/obj/libs/echo/src/e_message.c.d",
			"-g",
		},
		OutputPaths: []string{"build/obj/libs/echo/src/e_message.c.o"},
	}

	fp1, err := EntityStepCacheFingerprint(builtIR, "//libs/echo:echo", step, nil)
	if err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(headerPath, []byte("int nexus_v2;\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	fp2, err := EntityStepCacheFingerprint(builtIR, "//libs/echo:echo", step, nil)
	if err != nil {
		t.Fatal(err)
	}
	if fp1 == fp2 {
		t.Fatalf("fingerprint should change when manifest-listed header changes: %d == %d", fp1, fp2)
	}
}

func TestEntityStepCacheSkipsUnchangedCompileLeg(t *testing.T) {
	dir := t.TempDir()
	builtIR := testEntityStepCacheBuiltIR(dir)

	sourceRel := "libs/echo/src/e_message.c"
	sourcePath := filepath.Join(dir, filepath.FromSlash(sourceRel))
	objectRel := "build/obj/libs/echo/src/e_message.c.o"
	objectPath := filepath.Join(dir, filepath.FromSlash(objectRel))

	if err := os.MkdirAll(filepath.Dir(sourcePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sourcePath, []byte("int x;\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(objectPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(objectPath, []byte("object"), 0o644); err != nil {
		t.Fatal(err)
	}

	step := EntityAdapterStep{
		Argv: []string{
			"tools/scripts/compile_object.sh",
			sourceRel,
			objectRel,
			"build/obj/libs/echo/src/e_message.c.d",
		},
		OutputPaths: []string{objectRel},
	}

	cacheStore, err := cache.EntityCacheStoreOpen(filepath.Join(dir, ".helm", "cache"))
	if err != nil {
		t.Fatal(err)
	}
	defer cache.EntityCacheStoreClose(cacheStore)

	if err := entityStepCacheRecord(builtIR, "//libs/echo:echo", step, nil, cacheStore); err != nil {
		t.Fatal(err)
	}

	skip, err := entityStepCacheShouldSkip(builtIR, "//libs/echo:echo", step, nil, cacheStore)
	if err != nil {
		t.Fatal(err)
	}
	if !skip {
		t.Fatal("expected unchanged compile leg to be skipped")
	}
}

func TestEntityExecutorRunGraphSkipsOnlyUnchangedMatrixLegs(t *testing.T) {
	dir := t.TempDir()
	builtIR := testEntityStepCacheBuiltIR(dir)

	firstSourceRel := "libs/echo/src/e_message.c"
	secondSourceRel := "libs/echo/src/e_color.c"
	firstObjectRel := "build/obj/libs/echo/src/e_message.c.o"
	secondObjectRel := "build/obj/libs/echo/src/e_color.c.o"

	for _, spec := range []struct {
		sourceRel string
		objectRel string
		body      string
	}{
		{firstSourceRel, firstObjectRel, "int message;\n"},
		{secondSourceRel, secondObjectRel, "int color;\n"},
	} {
		sourcePath := filepath.Join(dir, filepath.FromSlash(spec.sourceRel))
		objectPath := filepath.Join(dir, filepath.FromSlash(spec.objectRel))
		if err := os.MkdirAll(filepath.Dir(sourcePath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(sourcePath, []byte(spec.body), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Dir(objectPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(objectPath, []byte("object"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	writeCompileDeps := func(req EntityRunRequest) error {
		if len(req.Argv) < 3 || filepath.Base(req.Argv[0]) != "compile_object.sh" {
			return nil
		}
		manifestRel := strings.TrimSuffix(req.Argv[2], ".o") + ".deps"
		manifestPath := filepath.Join(dir, filepath.FromSlash(manifestRel))
		if err := os.MkdirAll(filepath.Dir(manifestPath), 0o755); err != nil {
			return err
		}
		return os.WriteFile(manifestPath, []byte(req.Argv[1]+"\n"), 0o644)
	}

	spawned := make(map[string]int)
	err := EntityExecutorRunGraph(
		builtIR,
		[]EntityInstance{{Label: ir.HelmLabel{Path: "libs/echo", Name: "echo"}, Configuration: ir.HelmConfigurationDefaultName}},
		EntityExecutorOptions{
			RunHandler: func(req EntityRunRequest) error {
				if len(req.Argv) > 1 && filepath.Base(req.Argv[0]) == "compile_object.sh" {
					spawned[req.Argv[1]]++
					return writeCompileDeps(req)
				}
				if len(req.Argv) > 0 && req.Argv[0] == "ar" {
					archivePath := filepath.Join(dir, filepath.FromSlash(req.Argv[2]))
					if err := os.MkdirAll(filepath.Dir(archivePath), 0o755); err != nil {
						return err
					}
					return os.WriteFile(archivePath, []byte("archive"), 0o644)
				}
				return nil
			},
			CacheRoot: filepath.Join(dir, ".helm", "cache"),
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(spawned) != 2 {
		t.Fatalf("first run spawned = %#v, want two compile legs", spawned)
	}

	spawned = make(map[string]int)
	err = EntityExecutorRunGraph(
		builtIR,
		[]EntityInstance{{Label: ir.HelmLabel{Path: "libs/echo", Name: "echo"}, Configuration: ir.HelmConfigurationDefaultName}},
		EntityExecutorOptions{
			RunHandler: func(req EntityRunRequest) error {
				if len(req.Argv) > 1 && filepath.Base(req.Argv[0]) == "compile_object.sh" {
					spawned[req.Argv[1]]++
					return writeCompileDeps(req)
				}
				return nil
			},
			CacheRoot: filepath.Join(dir, ".helm", "cache"),
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(spawned) != 0 {
		t.Fatalf("second run spawned = %#v, want all compile legs skipped", spawned)
	}

	headerRel := "libs/nexus/nexus.h"
	headerPath := filepath.Join(dir, filepath.FromSlash(headerRel))
	manifestRel := "build/obj/libs/echo/src/e_message.c.deps"
	manifestPath := filepath.Join(dir, filepath.FromSlash(manifestRel))
	if err := os.MkdirAll(filepath.Dir(headerPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(headerPath, []byte("int changed;\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(manifestPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifestPath, []byte(headerRel+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	spawned = make(map[string]int)
	err = EntityExecutorRunGraph(
		builtIR,
		[]EntityInstance{{Label: ir.HelmLabel{Path: "libs/echo", Name: "echo"}, Configuration: ir.HelmConfigurationDefaultName}},
		EntityExecutorOptions{
			RunHandler: func(req EntityRunRequest) error {
				if len(req.Argv) > 1 && filepath.Base(req.Argv[0]) == "compile_object.sh" {
					spawned[req.Argv[1]]++
					return writeCompileDeps(req)
				}
				return nil
			},
			CacheRoot: filepath.Join(dir, ".helm", "cache"),
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(spawned) != 1 || spawned[firstSourceRel] != 1 {
		t.Fatalf("third run spawned = %#v, want only %s", spawned, firstSourceRel)
	}
}

func testEntityStepCacheBuiltIR(dir string) ir.HelmIR {
	return ir.HelmIR{
		SourceDirectory: dir,
		Mode:            ir.HelmModeWorkspace,
		Entities: map[string]ir.HelmEntity{
			"//libs/echo:echo": {
				Name:        "echo",
				AdapterName: "c_static_library",
				SourceFile:  filepath.Join(dir, "libs/echo/echo.helm"),
				Parameters: map[string]ir.HelmParameterValue{
					"SOURCE_FILES": {
						Kind: ir.HelmParameterArtifactItems,
						ArtifactItems: []ir.HelmArtifactInput{
							{Kind: ir.ArtifactInputString, Literal: "src/e_message.c"},
							{Kind: ir.ArtifactInputString, Literal: "src/e_color.c"},
						},
					},
					"OUT_NAME": {Kind: ir.HelmParameterScalar, Scalar: "echo"},
				},
			},
		},
		Adapters: map[string]ir.HelmAdapterDecl{
			"c_static_library": {
				Name: "c_static_library",
				Phases: []ir.HelmAdapterPhase{
					{
						Name: "_compile",
						Matrix: &ir.HelmMatrix{
							VariableName: "SRC",
							Values: []ir.HelmMatrixValue{
								{Kind: ir.MatrixValueParameterRef, ParameterName: "SOURCE_FILES"},
							},
						},
						MatrixLegOutputs: []ir.HelmArtifactInput{
							{Kind: ir.ArtifactInputString, Literal: "build/obj/${SRC}.o"},
						},
						MatrixRuns: []ir.HelmRunCommand{
							{
								Argv: []ir.HelmRunArgvElement{
									{Literal: "tools/scripts/compile_object.sh"},
									{Literal: "${SRC}"},
									{Literal: "build/obj/${SRC}.o"},
									{Literal: "build/obj/${SRC}.d"},
								},
							},
						},
					},
					{
						Name:      "_archive",
						DependsOn: []string{"_compile"},
						Outputs: []ir.HelmArtifactInput{
							{Kind: ir.ArtifactInputString, Literal: "build/lib/lib${OUT_NAME}.a"},
						},
						Runs: []ir.HelmRunCommand{
							{
								Argv: []ir.HelmRunArgvElement{
									{Literal: "ar"},
									{Literal: "rcs"},
									{Literal: "build/lib/lib${OUT_NAME}.a"},
									{PhaseOutputs: "_compile"},
								},
							},
						},
					},
				},
			},
		},
	}
}
