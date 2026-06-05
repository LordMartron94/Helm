package targetexecutor

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"sync"
)

const (
	runTranscriptTargetIndent  = 1
	runTranscriptContentIndent = 2
)

// TargetExecutorRunTranscript records phase/target boundaries and per-target subprocess I/O in a log only.
type TargetExecutorRunTranscript struct {
	log io.Writer
	mu  sync.Mutex

	activeNode string
	buffers    map[string]*targetExecutorTranscriptBuffer
}

type targetExecutorTranscriptBuffer struct {
	stdout bytes.Buffer
	stderr bytes.Buffer
}

func TargetExecutorRunTranscriptCreate(log io.Writer) *TargetExecutorRunTranscript {
	if log == nil {
		return nil
	}
	return &TargetExecutorRunTranscript{
		log:     log,
		buffers: make(map[string]*targetExecutorTranscriptBuffer),
	}
}

func (t *TargetExecutorRunTranscript) PhaseStart(phaseIndex int) {
	if t == nil {
		return
	}
	t.writeLine(fmt.Sprintf("=== PHASE %d ===", phaseIndex))
}

func (t *TargetExecutorRunTranscript) PhaseEnd(int) {
	if t == nil {
		return
	}
	t.writeLine("")
}

func (t *TargetExecutorRunTranscript) TargetStart(nodeID string, interactive bool) {
	if t == nil {
		return
	}
	t.mu.Lock()
	t.activeNode = nodeID
	if _, exists := t.buffers[nodeID]; !exists {
		t.buffers[nodeID] = &targetExecutorTranscriptBuffer{}
	}
	t.mu.Unlock()

	t.writeIndented(runTranscriptTargetIndent, nodeID)
	if interactive {
		t.writeIndented(runTranscriptContentIndent, "[interactive TTY — subprocess I/O not captured]")
	}
}

func (t *TargetExecutorRunTranscript) TargetEnd(nodeID string, runErr error) {
	if t == nil {
		return
	}

	t.mu.Lock()
	buffer := t.buffers[nodeID]
	delete(t.buffers, nodeID)
	if t.activeNode == nodeID {
		t.activeNode = ""
	}
	t.mu.Unlock()

	if buffer != nil {
		if buffer.stdout.Len() > 0 {
			t.writeIndented(runTranscriptContentIndent, "[stdout]")
			t.writeIndentedBytes(runTranscriptContentIndent, buffer.stdout.Bytes())
		}
		if buffer.stderr.Len() > 0 {
			t.writeIndented(runTranscriptContentIndent, "[stderr]")
			t.writeIndentedBytes(runTranscriptContentIndent, buffer.stderr.Bytes())
		}
	}

	if runErr != nil {
		t.writeIndented(runTranscriptContentIndent, fmt.Sprintf("error: %v", runErr))
	}
}

func (t *TargetExecutorRunTranscript) StdoutWriter(nodeID string) io.Writer {
	if t == nil {
		return nil
	}
	return targetExecutorTranscriptStreamWriter{t: t, nodeID: nodeID, stderr: false}
}

func (t *TargetExecutorRunTranscript) StderrWriter(nodeID string) io.Writer {
	if t == nil {
		return nil
	}
	return targetExecutorTranscriptStreamWriter{t: t, nodeID: nodeID, stderr: true}
}

func (t *TargetExecutorRunTranscript) writeLine(line string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	_, _ = io.WriteString(t.log, line+"\n")
}

func (t *TargetExecutorRunTranscript) writeIndented(level int, line string) {
	t.writeLine(runTranscriptIndent(level) + line)
}

func (t *TargetExecutorRunTranscript) writeIndentedBytes(level int, data []byte) {
	text := string(data)
	if text == "" {
		return
	}
	text = strings.TrimRight(text, "\n")
	if text == "" {
		return
	}
	prefix := runTranscriptIndent(level)
	for _, line := range strings.Split(text, "\n") {
		t.writeLine(prefix + line)
	}
}

func runTranscriptIndent(level int) string {
	if level <= 0 {
		return ""
	}
	return strings.Repeat("  ", level)
}

func (t *TargetExecutorRunTranscript) bufferFor(nodeID string) *targetExecutorTranscriptBuffer {
	t.mu.Lock()
	defer t.mu.Unlock()
	buffer, ok := t.buffers[nodeID]
	if !ok {
		buffer = &targetExecutorTranscriptBuffer{}
		t.buffers[nodeID] = buffer
	}
	return buffer
}

type targetExecutorTranscriptStreamWriter struct {
	t      *TargetExecutorRunTranscript
	nodeID string
	stderr bool
}

func (w targetExecutorTranscriptStreamWriter) Write(p []byte) (int, error) {
	if w.t == nil || len(p) == 0 {
		return len(p), nil
	}
	buffer := w.t.bufferFor(w.nodeID)
	if w.stderr {
		_, err := buffer.stderr.Write(p)
		if err != nil {
			return 0, err
		}
		return len(p), nil
	}
	_, err := buffer.stdout.Write(p)
	if err != nil {
		return 0, err
	}
	return len(p), nil
}
