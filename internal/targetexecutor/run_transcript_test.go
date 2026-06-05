package targetexecutor

import (
	"bytes"
	"strings"
	"testing"
)

func TestTargetExecutorRunTranscriptLogOnly(t *testing.T) {
	var logBuffer bytes.Buffer
	transcript := TargetExecutorRunTranscriptCreate(&logBuffer)

	transcript.PhaseStart(1)
	transcript.TargetStart("leaf", false)
	_, _ = transcript.StdoutWriter("leaf").Write([]byte("hello\n"))
	transcript.TargetEnd("leaf", nil)
	transcript.PhaseEnd(1)

	logText := logBuffer.String()
	if !strings.Contains(logText, "=== START PHASE 1 ===") {
		t.Fatalf("missing phase start in log: %s", logText)
	}
	if !strings.Contains(logText, ">>> START TARGET leaf") {
		t.Fatalf("missing target start in log: %s", logText)
	}
	if !strings.Contains(logText, "hello") {
		t.Fatalf("missing captured stdout in log: %s", logText)
	}
}
