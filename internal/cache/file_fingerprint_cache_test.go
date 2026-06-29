package cache

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFileFingerprintCacheReusesHashWhenMetadataUnchanged(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	filePath := filepath.Join(dir, "source.c")
	if err := os.WriteFile(filePath, []byte("int x;\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	fileCache, err := FileFingerprintCacheOpen(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer FileFingerprintCacheClose(fileCache)

	first, err := CacheFileFingerprint(dir, "source.c", fileCache)
	if err != nil {
		t.Fatal(err)
	}

	second, err := CacheFileFingerprint(dir, "source.c", fileCache)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("cached hash mismatch: %d != %d", first, second)
	}
}

func TestFileFingerprintCacheRehashesWhenMetadataChanges(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	filePath := filepath.Join(dir, "source.c")
	if err := os.WriteFile(filePath, []byte("int version_one;\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	fileCache, err := FileFingerprintCacheOpen(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer FileFingerprintCacheClose(fileCache)

	first, err := CacheFileFingerprint(dir, "source.c", fileCache)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(1100 * time.Millisecond)
	if err := os.WriteFile(filePath, []byte("int version_two;\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	second, err := CacheFileFingerprint(dir, "source.c", fileCache)
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatalf("fingerprint should change when same-size content changes: %d == %d", first, second)
	}
}

func TestFileFingerprintCachePersistsAcrossReopen(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	filePath := filepath.Join(dir, "source.c")
	if err := os.WriteFile(filePath, []byte("int x;\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	fileCache, err := FileFingerprintCacheOpen(dir)
	if err != nil {
		t.Fatal(err)
	}
	first, err := CacheFileFingerprint(dir, "source.c", fileCache)
	if err != nil {
		t.Fatal(err)
	}
	if err := FileFingerprintCacheClose(fileCache); err != nil {
		t.Fatal(err)
	}

	reopened, err := FileFingerprintCacheOpen(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer FileFingerprintCacheClose(reopened)

	second, err := CacheFileFingerprint(dir, "source.c", reopened)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("persisted hash mismatch: %d != %d", first, second)
	}
}
