package cache

import (
	"os"
	"path/filepath"
	"testing"
)

/*
TestCacheFileContentHashMissingFile verifies that manifest-discovered (or other referenced) paths
that do not exist yet contribute a stable sentinel hash instead of aborting cache evaluation.
*/
func TestCacheFileContentHashMissingFile(t *testing.T) {
	t.Parallel()

	missingPath := filepath.Join(t.TempDir(), "not-created-yet.c")
	hash, err := cacheFileContentHash(missingPath)
	if err != nil {
		t.Fatalf("cacheFileContentHash(%q) error: %v", missingPath, err)
	}
	if hash != cacheMissingFileFingerprint {
		t.Fatalf("missing file hash = %d, want sentinel %d", hash, cacheMissingFileFingerprint)
	}
}

func TestCacheFileContentHashExistingFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "present.txt")
	if err := os.WriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	hash, err := cacheFileContentHash(path)
	if err != nil {
		t.Fatalf("cacheFileContentHash(%q) error: %v", path, err)
	}
	if hash == cacheMissingFileFingerprint {
		t.Fatalf("existing file hash must not use missing sentinel")
	}
}
