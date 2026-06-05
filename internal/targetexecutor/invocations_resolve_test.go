package targetexecutor

import (
	"helm/internal/ir"
	"testing"
)

func TestTargetExecutorBindDependencyParamsTargetParamRef(t *testing.T) {
	t.Parallel()

	parentResolved := TargetResolvedParameters{
		Scalars: map[string]string{
			"OUT_NAME": "libsplash.so",
		},
		PathLists: map[string][]string{
			"SOURCE_FILES": {"a.c", "b.c"},
		},
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

	parentParameters := map[string]ir.HelmParameterValue{
		"SOURCE_FILES": {Kind: ir.HelmParameterGlobalRef, GlobalName: "TEST_SOURCE_FILES"},
		"OUT_NAME":     {Kind: ir.HelmParameterScalar, Scalar: parentResolved.Scalars["OUT_NAME"]},
	}

	bound, err := targetExecutorBindDependencyParams(nil, parentResolved, parentParameters, raw)
	if err != nil {
		t.Fatal(err)
	}
	if bound["SOURCE_FILES"].Kind != ir.HelmParameterGlobalRef {
		t.Fatalf("SOURCE_FILES: got kind %v", bound["SOURCE_FILES"].Kind)
	}
	if bound["OUT_NAME"].Scalar != "libsplash.so" {
		t.Fatalf("OUT_NAME: got %q", bound["OUT_NAME"].Scalar)
	}
}
