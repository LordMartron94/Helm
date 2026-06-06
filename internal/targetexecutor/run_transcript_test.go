package targetexecutor

import (
	"bytes"
	"strings"
	"testing"
)

func TestTargetExecutorRunTranscriptNestedFormat(t *testing.T) {
	var logBuffer bytes.Buffer
	transcript := TargetExecutorRunTranscriptCreate(&logBuffer)

	transcript.PhaseStart(1)
	transcript.TargetStart("leaf", false)
	_, _ = transcript.StdoutWriter("leaf").Write([]byte("hello\n"))
	transcript.TargetEnd("leaf", nil)
	transcript.TargetStart("branch", true)
	transcript.TargetEnd("branch", nil)
	transcript.PhaseEnd(1)

	logText := logBuffer.String()
	if strings.Contains(logText, "START") || strings.Contains(logText, "END TARGET") {
		t.Fatalf("transcript should not use START/END markers: %s", logText)
	}
	if !strings.Contains(logText, "=== PHASE 1 ===") {
		t.Fatalf("missing phase header: %s", logText)
	}
	if !strings.Contains(logText, "  leaf\n") {
		t.Fatalf("missing nested target name: %s", logText)
	}
	if !strings.Contains(logText, "    [stdout]") {
		t.Fatalf("missing nested stdout header: %s", logText)
	}
	if !strings.Contains(logText, "    hello") {
		t.Fatalf("missing nested stdout payload: %s", logText)
	}
	if !strings.Contains(logText, "  branch") {
		t.Fatalf("missing second target: %s", logText)
	}
	if !strings.Contains(logText, "interactive TTY") {
		t.Fatalf("missing interactive note: %s", logText)
	}
}
