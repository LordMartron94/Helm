package expand

import "testing"

func TestPathsFromShellParameterList(t *testing.T) {
	t.Parallel()

	list := JoinPathsForShell([]string{"/a/x.go", "/b/y.go"})
	paths, ok := PathsFromShellParameterList(list)
	if !ok || len(paths) != 2 {
		t.Fatalf("PathsFromShellParameterList(%q) = %v, %v; want 2 paths", list, paths, ok)
	}
	if paths[0] != "/a/x.go" || paths[1] != "/b/y.go" {
		t.Fatalf("got %v", paths)
	}

	if _, ok := PathsFromShellParameterList("build/out.so"); ok {
		t.Fatal("expected single path to stay unsplit")
	}
}
