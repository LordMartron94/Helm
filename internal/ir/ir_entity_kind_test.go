package ir

import "testing"

func TestHelmEntityKindIsMetadataOnly(t *testing.T) {
	if !HelmEntityKindIsMetadataOnly(HelmEntityKindInterface) {
		t.Fatal("interface kind should be metadata-only")
	}
	if !HelmEntityKindIsMetadataOnly(HelmEntityKindHeaderOnly) {
		t.Fatal("header_only kind should be metadata-only")
	}
	if HelmEntityKindIsMetadataOnly("lib") {
		t.Fatal("lib kind should not be metadata-only")
	}
	if HelmEntityKindIsMetadataOnly("") {
		t.Fatal("empty kind should not be metadata-only")
	}
}

func TestHelmEntityIsMetadataOnly(t *testing.T) {
	entity := HelmEntity{Kind: HelmEntityKindInterface}
	if !HelmEntityIsMetadataOnly(entity) {
		t.Fatal("expected metadata-only entity")
	}
	entity.Kind = "lib"
	if HelmEntityIsMetadataOnly(entity) {
		t.Fatal("compiled entity should not be metadata-only")
	}
}
