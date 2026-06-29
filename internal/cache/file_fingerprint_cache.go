package cache

import (
	"fmt"
	"helm/internal/workspacepath"
	"os"
	"persistence"
	"sync"
	"syscall"
)

const fileFingerprintTableName = "file_fingerprints"

type FileFingerprintRecord struct {
	Path          string
	Size          int64
	ModTimeNano   int64
	ContentHash   uint64
}

// FileFingerprintCache memoizes per-path content hashes across cache checks.
type FileFingerprintCache struct {
	store *persistence.JSONStore
	mu    sync.Mutex
	hot   map[string]FileFingerprintRecord
}

func FileFingerprintCacheOpen(cacheRoot string) (*FileFingerprintCache, error) {
	store, err := persistence.JSONStoreCreate(cacheRoot)
	if err != nil {
		return nil, fmt.Errorf("open file fingerprint cache: %w", err)
	}

	_, err = persistence.JSONStoreRegister[FileFingerprintRecord](
		store,
		fileFingerprintTableName,
		func(record FileFingerprintRecord) string {
			return record.Path
		},
		func(a, b FileFingerprintRecord) bool {
			return a.Path < b.Path
		},
	)
	if err != nil {
		return nil, fmt.Errorf("register file fingerprint table: %w", err)
	}

	cache := &FileFingerprintCache{
		store: store,
		hot:   make(map[string]FileFingerprintRecord),
	}

	records, err := persistence.JSONStoreItems[FileFingerprintRecord](store, fileFingerprintTableName)
	if err != nil {
		return nil, fmt.Errorf("load file fingerprint table: %w", err)
	}
	for _, record := range records {
		cache.hot[record.Path] = record
	}

	return cache, nil
}

func FileFingerprintCacheClose(fileCache *FileFingerprintCache) error {
	if fileCache == nil {
		return nil
	}
	return FileFingerprintCacheSaveAll(fileCache)
}

func FileFingerprintCacheSaveAll(fileCache *FileFingerprintCache) error {
	if fileCache == nil {
		return nil
	}
	fileCache.mu.Lock()
	defer fileCache.mu.Unlock()
	return persistence.JSONStoreSaveAll(fileCache.store)
}

// CacheFileFingerprint returns a stable content hash for one workspace-relative path.
// When fileCache is nil, the file is read and hashed on every call.
func CacheFileFingerprint(helmBaseDir, relPath string, fileCache *FileFingerprintCache) (uint64, error) {
	absPath := workspacepath.WorkspaceAnchor(helmBaseDir, relPath)
	info, statErr := os.Stat(absPath)
	if statErr != nil {
		if os.IsNotExist(statErr) {
			return cacheMissingFileFingerprint, nil
		}
		return 0, &CacheFingerprintFileError{Path: relPath, Cause: statErr}
	}

	size := info.Size()
	modTimeNano := fileStatModTimeNano(info)

	if fileCache != nil {
		if cachedHash, ok := fileCache.lookup(relPath, size, modTimeNano); ok {
			return cachedHash, nil
		}
	}

	contentHash, err := cacheFileContentHashFromPath(absPath)
	if err != nil {
		return 0, &CacheFingerprintFileError{Path: relPath, Cause: err}
	}

	if fileCache != nil {
		fileCache.remember(relPath, size, modTimeNano, contentHash)
	}

	return contentHash, nil
}

func (fileCache *FileFingerprintCache) lookup(relPath string, size, modTimeNano int64) (uint64, bool) {
	fileCache.mu.Lock()
	defer fileCache.mu.Unlock()

	record, ok := fileCache.hot[relPath]
	if !ok {
		return 0, false
	}
	if record.Size != size || record.ModTimeNano != modTimeNano {
		return 0, false
	}
	return record.ContentHash, true
}

func (fileCache *FileFingerprintCache) remember(relPath string, size, modTimeNano int64, contentHash uint64) {
	record := FileFingerprintRecord{
		Path:        relPath,
		Size:        size,
		ModTimeNano: modTimeNano,
		ContentHash: contentHash,
	}

	fileCache.mu.Lock()
	defer fileCache.mu.Unlock()

	fileCache.hot[relPath] = record
	if fileCache.store == nil {
		return
	}

	if _, err := persistence.JSONStoreGet[FileFingerprintRecord](fileCache.store, fileFingerprintTableName, relPath); err != nil {
		_ = persistence.JSONStoreAppend(fileCache.store, fileFingerprintTableName, record)
		return
	}
	_ = persistence.JSONStoreReplace(fileCache.store, fileFingerprintTableName, relPath, record)
}

func fileStatModTimeNano(info os.FileInfo) int64 {
	if ts, ok := info.Sys().(*syscall.Stat_t); ok && ts != nil {
		return ts.Mtim.Nano()
	}
	return info.ModTime().UnixNano()
}
