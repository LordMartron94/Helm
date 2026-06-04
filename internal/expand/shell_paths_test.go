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

	single := JoinPathsForShell([]string{"/home/user/project/libs/splash/src/test.c"})
	paths, ok = PathsFromShellParameterList(single)
	if !ok || len(paths) != 1 {
		t.Fatalf("single quoted list: got %v, %v; want one path", paths, ok)
	}
	if paths[0] != "/home/user/project/libs/splash/src/test.c" {
		t.Fatalf("got %q", paths[0])
	}

	if _, ok := PathsFromShellParameterList("build/out.so"); ok {
		t.Fatal("expected plain relative path to stay unsplit")
	}
}
