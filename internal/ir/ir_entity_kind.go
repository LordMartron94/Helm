package ir

const (
	HelmEntityKindInterface  = "interface"
	HelmEntityKindHeaderOnly = "header_only"
)

// HelmEntityKindIsMetadataOnly reports whether kind denotes a property-bag node
// with no adapter-driven build steps.
func HelmEntityKindIsMetadataOnly(kind string) bool {
	return kind == HelmEntityKindInterface || kind == HelmEntityKindHeaderOnly
}

// HelmEntityIsMetadataOnly reports whether entity exists in the graph for
// dependency and property-bag propagation only.
func HelmEntityIsMetadataOnly(entity HelmEntity) bool {
	return HelmEntityKindIsMetadataOnly(entity.Kind)
}
