package targetexecutor

import (
	"testing"

	"helm/internal/expand"
	"helm/internal/ir"
)

func TestTargetExecutorInterpolateEnvLinkerFlagsFromPathLists(t *testing.T) {
	t.Parallel()

	target := ir.HelmTarget{
		Name: "_link_binary",
		Env: map[string]ir.HelmStringListExpr{
			"LDFLAGS": {
				{Kind: ir.StringListParamRef, ParamName: "LINKER_FLAGS"},
			},
		},
	}

	resolved := TargetResolvedParameters{
		PathLists: map[string][]string{
			"LINKER_FLAGS": {"-Lbuild/lib", "-Wl,-rpath,'$ORIGIN/../lib'"},
		},
	}

	env, err := targetExecutorInterpolateEnv(
		target.Env,
		nil,
		map[string]ir.HelmParameterValue{
			"LINKER_FLAGS": {Kind: ir.HelmParameterGlobalRef, GlobalName: "TEST_LINKER_FLAGS"},
		},
		resolved,
		expand.InterpolationContext{},
	)
	if err != nil {
		t.Fatal(err)
	}

	want := "-Lbuild/lib -Wl,-rpath,'$ORIGIN/../lib'"
	if env["LDFLAGS"] != want {
		t.Fatalf("LDFLAGS = %q, want %q", env["LDFLAGS"], want)
	}
}
