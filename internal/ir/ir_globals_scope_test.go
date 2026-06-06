package ir

import "testing"

func TestIRGlobalsForFileMergesWorkspaceAndFileLocals(t *testing.T) {
	builtIR := HelmIR{
		RootManifestPath: "/ws/Helmfile",
		Workspace: &HelmWorkspace{
			Globals: map[string]HelmGlobalVariable{
				"LIB_DIR": {Kind: HelmGlobalVarString, StringValue: "build/lib"},
			},
		},
		FileGlobals: map[string]map[string]HelmGlobalVariable{
			"/ws/libs/splash/splash.helm": {
				"LOCAL_ONLY": {Kind: HelmGlobalVarString, StringValue: "yes"},
			},
			"/ws/Helmfile": {
				"BIN_DIR": {Kind: HelmGlobalVarString, StringValue: "build/bin"},
			},
		},
	}

	entityGlobals := IRGlobalsForFile(builtIR, "/ws/libs/splash/splash.helm")
	if entityGlobals["LIB_DIR"].StringValue != "build/lib" {
		t.Fatalf("LIB_DIR = %#v", entityGlobals["LIB_DIR"])
	}
	if entityGlobals["LOCAL_ONLY"].StringValue != "yes" {
		t.Fatalf("LOCAL_ONLY = %#v", entityGlobals["LOCAL_ONLY"])
	}
	if _, ok := entityGlobals["BIN_DIR"]; ok {
		t.Fatalf("root-only BIN_DIR leaked: %#v", entityGlobals)
	}

	rootGlobals := IRGlobalsForRootManifest(builtIR)
	if rootGlobals["BIN_DIR"].StringValue != "build/bin" {
		t.Fatalf("BIN_DIR = %#v", rootGlobals["BIN_DIR"])
	}
	if _, ok := rootGlobals["LIB_DIR"]; ok {
		t.Fatalf("workspace globals should not inject into root manifest: %#v", rootGlobals)
	}
}
