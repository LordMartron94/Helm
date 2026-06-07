package targetexecutor

import (
	"helm/internal/expand"
	"helm/internal/ir"
	"testing"
)

func TestTargetExecutorEvaluateConditionGlobalString(t *testing.T) {
	t.Parallel()

	interpCtx := expand.InterpolationContext{
		Scalars: map[string]string{
			"VERSION": "2.1.5",
		},
	}

	cases := []struct {
		name      string
		condition ir.HelmCondition
		want      bool
	}{
		{
			name: "defined global string",
			condition: ir.HelmCondition{
				ConditionType: ir.ConditionDefined,
				Parameter:     "VERSION",
			},
			want: true,
		},
		{
			name: "equals global string",
			condition: ir.HelmCondition{
				ConditionType: ir.ConditionEquals,
				Parameter:     "VERSION",
				TargetValue:   "2.1.5",
			},
			want: true,
		},
		{
			name: "not_equals global string",
			condition: ir.HelmCondition{
				ConditionType: ir.ConditionNotEquals,
				Parameter:     "VERSION",
				TargetValue:   "0.0.0",
			},
			want: true,
		},
		{
			name: "not_defined missing global",
			condition: ir.HelmCondition{
				ConditionType: ir.ConditionNotDefined,
				Parameter:     "MISSING",
			},
			want: true,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := TargetExecutorEvaluateCondition(tc.condition, interpCtx)
			if got != tc.want {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestTargetExecutorEvaluateConditionGlobalArtifactArray(t *testing.T) {
	t.Parallel()

	interpCtx := expand.InterpolationContext{
		PathLists: map[string][]string{
			"FILES": {"a.c", "b.c"},
		},
	}

	condition := ir.HelmCondition{
		ConditionType: ir.ConditionDefined,
		Parameter:     "FILES",
	}
	if !TargetExecutorEvaluateCondition(condition, interpCtx) {
		t.Fatal("expected artifact-array global to be defined")
	}

	emptyCtx := expand.InterpolationContext{
		PathLists: map[string][]string{
			"FILES": {},
		},
	}
	if TargetExecutorEvaluateCondition(condition, emptyCtx) {
		t.Fatal("expected empty artifact-array global to be undefined")
	}
}

func TestTargetExecutorEvaluateConditionParameterViaInterpCtx(t *testing.T) {
	t.Parallel()

	resolved := TargetResolvedParameters{
		Scalars: map[string]string{
			"TAG": "release",
		},
	}
	interpCtx := resolved.InterpolationContext(nil)

	condition := ir.HelmCondition{
		ConditionType: ir.ConditionEquals,
		Parameter:     "TAG",
		TargetValue:   "release",
	}
	if !TargetExecutorEvaluateCondition(condition, interpCtx) {
		t.Fatal("expected bound parameter to satisfy equals via interpolation context")
	}
}
