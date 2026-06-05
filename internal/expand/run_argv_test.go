package expand

import (
	"helm/internal/ir"
	"testing"
)

func TestExpandFingerprintRunCommandArgv(t *testing.T) {
	t.Parallel()

	command := ir.HelmRunCommand{
		Argv: []ir.HelmRunArgvElement{
			{Literal: "link.sh"},
			{ParamName: "SOURCE_FILES"},
		},
	}

	got := ExpandFingerprintRunCommand(command, nil, nil)
	want := "argv\x00link.sh\x00param:SOURCE_FILES"
	if got != want {
		t.Fatalf("fingerprint = %q, want %q", got, want)
	}
}
