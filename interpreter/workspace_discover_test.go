package interpreter

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWorkspaceDiscoverHelmFilesSkipsRootAndExcludes(t *testing.T) {
	root := t.TempDir()
	rootManifest := filepath.Join(root, "Helmfile")
	writeDiscoverFile(t, rootManifest, "workspace {}\n")
	writeDiscoverFile(t, filepath.Join(root, "libs", "splash", "splash.helm"), "entity splash {}\n")
	writeDiscoverFile(t, filepath.Join(root, "build", "stale.helm"), "entity stale {}\n")
	writeDiscoverFile(t, filepath.Join(root, "skip.helm"), "entity skip {}\n")

	discovered, err := WorkspaceDiscoverHelmFiles(root, rootManifest, []string{"build", "skip.helm"})
	if err != nil {
		t.Fatal(err)
	}
	if len(discovered) != 1 {
		t.Fatalf("discovered = %#v", discovered)
	}
	want := filepath.Join(root, "libs", "splash", "splash.helm")
	if discovered[0] != want {
		t.Fatalf("got %q want %q", discovered[0], want)
	}
}

func writeDiscoverFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
