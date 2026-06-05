package cache

import (
	"helm/internal/expand"
	"helm/internal/ir"
	"os"
	"path/filepath"
	"testing"
)

func TestDynamicManifestDiscoveredPathsFiltersOutsideWorkspace(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	manifestRel := "build/obj.deps"
	manifestPath := filepath.Join(root, manifestRel)
	if err := os.MkdirAll(filepath.Dir(manifestPath), 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := "testbed/main.c\n/usr/include/stdio.h\n# comment\n\n"
	if err := os.WriteFile(manifestPath, []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}

	artifacts := &ir.HelmArtifacts{
		Dynamic: []ir.HelmArtifactInput{
			{Kind: ir.ArtifactInputString, Literal: manifestRel},
		},
	}

	paths, err := DynamicManifestDiscoveredPathsContext(
		root,
		artifacts,
		expand.InterpolationContext{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 1 || paths[0] != "testbed/main.c" {
		t.Fatalf("paths = %#v", paths)
	}
}
