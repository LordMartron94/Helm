package targetexecutor

import (
	"helm/internal/cache"
	"helm/internal/ir"
	"io"
	"signal"
)

type TargetExecutorOptions struct {
	RunHandler TargetRunHandler

	// StreamRunOutput writes subprocess stdout/stderr as they are produced (see LiveStdout/LiveStderr).
	StreamRunOutput bool
	// StreamStdout and StreamStderr default to os.Stdout and os.Stderr when StreamRunOutput is true.
	StreamStdout io.Writer
	StreamStderr io.Writer

	ConfirmDependency func(dependent string, dep ir.HelmTargetDependency) (proceed bool, err error)

	// When set, each run step emits EXEC_OK / EXEC_FAIL signals with captured output payloads.
	SignalContext *signal.SignalContext

	// CacheRoot is the directory for persisted target cache records (.helm/cache by default).
	CacheRoot string
	// DisableArtifactCache skips cache open, lookup, and persistence (for execution-only tests).
	DisableArtifactCache bool
	// BypassCache forces every target to run; successful runs still update the cache store.
	BypassCache bool
	// CacheStore is opened from CacheRoot by the graph runner when nil.
	CacheStore *cache.TargetCacheStore

	// RunTranscript records phase/target boundaries and captured subprocess I/O in a log file only.
	RunTranscript *TargetExecutorRunTranscript
	// RunState is optional per-run state (entry target reach tracking).
	RunState *TargetExecutorRunState
	// TranscriptNodeID is the active execution node for subprocess capture.
	TranscriptNodeID string
}
