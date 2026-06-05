package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunLogOpenTruncatesEachRun(t *testing.T) {
	dir := t.TempDir()

	first, err := RunLogOpen(dir, "entry_a")
	if err != nil {
		t.Fatalf("open first: %v", err)
	}
	if _, err := first.Writer().Write([]byte("first-run\n")); err != nil {
		t.Fatalf("write first: %v", err)
	}
	_ = first.Close()

	second, err := RunLogOpen(dir, "entry_b")
	if err != nil {
		t.Fatalf("open second: %v", err)
	}
	_ = second.AppendDiagnostics("diag block")
	_ = second.Close()

	content, err := os.ReadFile(RunLogPath(dir))
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	text := string(content)
	if strings.Contains(text, "first-run") {
		t.Fatalf("log was not truncated between runs: %s", text)
	}
	if !strings.Contains(text, "entry_b") {
		t.Fatalf("missing second entry header: %s", text)
	}
	if !strings.Contains(text, "diag block") {
		t.Fatalf("missing diagnostics section: %s", text)
	}
	if filepath.Base(RunLogPath(dir)) != runLogFileName {
		t.Fatalf("unexpected log file name: %s", runLogFileName)
	}
}
