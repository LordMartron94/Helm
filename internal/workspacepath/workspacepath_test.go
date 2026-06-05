package workspacepath

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWorkspaceRelativeInsideRoot(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	inside := filepath.Join(root, "testbed", "main.c")
	if err := os.MkdirAll(filepath.Dir(inside), 0o755); err != nil {
		t.Fatal(err)
	}

	rel, ok := WorkspaceRelative(root, inside)
	if !ok {
		t.Fatal("expected inside path")
	}
	if rel != "testbed/main.c" {
		t.Fatalf("rel = %q", rel)
	}
}

func TestWorkspaceRelativeOutsideRoot(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	rel, ok := WorkspaceRelative(root, "/usr/include/stdio.h")
	if ok {
		t.Fatalf("expected outside path to be rejected, got %q", rel)
	}
}

func TestWorkspaceAnchorRoundTrip(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	rel := "build/obj/test.o"
	anchored := WorkspaceAnchor(root, rel)
	back, ok := WorkspaceRelative(root, anchored)
	if !ok || back != rel {
		t.Fatalf("round trip: %q ok=%v", back, ok)
	}
}
