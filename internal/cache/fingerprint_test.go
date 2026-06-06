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

	dir := t.TempDir()
	missingRel := "not-created-yet.c"
	hash, err := cacheFileContentHash(dir, missingRel)
	if err != nil {
		t.Fatalf("cacheFileContentHash(%q) error: %v", missingRel, err)
	}
	if hash != cacheMissingFileFingerprint {
		t.Fatalf("missing file hash = %d, want sentinel %d", hash, cacheMissingFileFingerprint)
	}
}

func TestCacheFileContentHashExistingFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "present.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	hash, err := cacheFileContentHash(dir, "present.txt")
	if err != nil {
		t.Fatalf("cacheFileContentHash(%q) error: %v", "present.txt", err)
	}
	if hash == cacheMissingFileFingerprint {
		t.Fatalf("existing file hash must not use missing sentinel")
	}
}
