package targetexecutor

import (
	"helm/internal/ir"
	"testing"
)

func TestTargetExecutorBindDependencyParamsTargetParamRef(t *testing.T) {
	t.Parallel()

	parentResolved := map[string]string{
		"SOURCE_FILES": `"a.c" "b.c"`,
		"OUT_NAME":     "libsplash.so",
	}
	raw := map[string]ir.HelmParameterValue{
		"SOURCE_FILES": {
			Kind:            ir.HelmParameterTargetParamRef,
			TargetParamName: "SOURCE_FILES",
		},
		"OUT_NAME": {
			Kind:            ir.HelmParameterTargetParamRef,
			TargetParamName: "OUT_NAME",
		},
	}

	bound, err := targetExecutorBindDependencyParams(nil, parentResolved, raw)
	if err != nil {
		t.Fatal(err)
	}
	if bound["SOURCE_FILES"].Scalar != parentResolved["SOURCE_FILES"] {
		t.Fatalf("SOURCE_FILES: got %q", bound["SOURCE_FILES"].Scalar)
	}
	if bound["OUT_NAME"].Scalar != "libsplash.so" {
		t.Fatalf("OUT_NAME: got %q", bound["OUT_NAME"].Scalar)
	}
}
