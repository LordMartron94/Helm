package cli

import "testing"

func TestStripTerminalEscapeSequences(t *testing.T) {
	input := "\x1b[38;2;127;140;141m=== Execution summary ===\x1b[0m\n  \x1b[38;2;46;204;113mOK\x1b[0m: 1"
	want := "=== Execution summary ===\n  OK: 1"

	got := StripTerminalEscapeSequences(input)
	if got != want {
		t.Fatalf("strip ANSI: got %q want %q", got, want)
	}
}
