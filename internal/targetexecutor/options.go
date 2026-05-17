package targetexecutor

import (
	"helm/internal/cache"
	"helm/internal/ir"
	"signal"
)

type TargetExecutorOptions struct {
	RunHandler TargetRunHandler

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
}
