package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

const runLogFileName = "last-run.log"

// RunLog is a truncated per-run transcript under .helm/last-run.log.
type RunLog struct {
	file   *os.File
	path   string
	closed bool
}

func RunLogOpen(helmDir string, entryTarget string) (*RunLog, error) {
	helmCacheDir := filepath.Join(helmDir, ".helm")
	if err := os.MkdirAll(helmCacheDir, 0o755); err != nil {
		return nil, fmt.Errorf("run log: %w", err)
	}

	path := filepath.Join(helmCacheDir, runLogFileName)
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return nil, fmt.Errorf("run log: %w", err)
	}

	log := &RunLog{file: file, path: path}
	if err := log.writeHeader(entryTarget); err != nil {
		_ = file.Close()
		return nil, err
	}
	return log, nil
}

func RunLogPath(helmDir string) string {
	return filepath.Join(helmDir, ".helm", runLogFileName)
}

func (log *RunLog) Writer() io.Writer {
	if log == nil || log.file == nil || log.closed {
		return nil
	}
	return log.file
}

func (log *RunLog) AppendSection(title string) error {
	if log == nil || log.file == nil || log.closed {
		return nil
	}
	_, err := fmt.Fprintf(log.file, "\n--- %s ---\n", title)
	return err
}

func (log *RunLog) AppendDiagnostics(text string) error {
	if log == nil || log.file == nil || log.closed || text == "" {
		return nil
	}
	if err := log.AppendSection("Diagnostics"); err != nil {
		return err
	}
	_, err := io.WriteString(log.file, text)
	return err
}

func (log *RunLog) Close() error {
	if log == nil || log.file == nil || log.closed {
		return nil
	}
	log.closed = true
	return log.file.Close()
}

func (log *RunLog) writeHeader(entryTarget string) error {
	_, err := fmt.Fprintf(
		log.file,
		"Helm run transcript\nentry_target: %s\nstarted_at: %s\n\n",
		entryTarget,
		time.Now().Format(time.RFC3339),
	)
	return err
}
