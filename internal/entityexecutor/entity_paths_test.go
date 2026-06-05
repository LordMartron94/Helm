package entityexecutor

import (
	"path/filepath"
	"testing"

	"helm/internal/ir"
)

func TestEntityPathsToWorkspaceFromDeclarationBase(t *testing.T) {
	workspaceRoot := filepath.Join("/ws", "vertex-siege")
	declarationBase := filepath.Join(workspaceRoot, "libs", "splash")
	paths := EntityPathsToWorkspace(workspaceRoot, declarationBase, []string{"src/splash.c"})
	if len(paths) != 1 || paths[0] != "libs/splash/src/splash.c" {
		t.Fatalf("paths = %#v", paths)
	}
}

func TestEntityDeclarationBaseDirUsesSourceFile(t *testing.T) {
	workspaceRoot := "/ws"
	entity := ir.HelmEntity{
		SourceFile: filepath.Join(workspaceRoot, "libs", "splash", "splash.helm"),
	}
	base := EntityDeclarationBaseDir(workspaceRoot, entity)
	want := filepath.Join(workspaceRoot, "libs", "splash")
	if base != want {
		t.Fatalf("base = %q, want %q", base, want)
	}
}
