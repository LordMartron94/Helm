package targetexecutor

import (
	"helm/internal/expand"
	"strings"
)

// TargetResolvedParameters holds invocation parameters after resolution.
// Artifact path lists stay structured in PathLists; Scalars holds plain string parameters only.
type TargetResolvedParameters struct {
	Scalars     map[string]string
	PathLists   map[string][]string
	StringLists map[string][]string
}

func TargetResolvedParametersEmpty() TargetResolvedParameters {
	return TargetResolvedParameters{}
}

func (resolved TargetResolvedParameters) HasValues() bool {
	return len(resolved.Scalars) > 0 || len(resolved.PathLists) > 0 || len(resolved.StringLists) > 0
}

func (resolved TargetResolvedParameters) InterpolationContext(globalScalars map[string]string) expand.InterpolationContext {
	scalars := expand.InterpolationContextMergeScalars(nil, globalScalars)
	scalars = expand.InterpolationContextMergeScalars(scalars, resolved.Scalars)

	pathLists := map[string][]string{}
	for key, paths := range resolved.PathLists {
		pathLists[key] = append([]string(nil), paths...)
	}

	return expand.InterpolationContext{
		Scalars:   scalars,
		PathLists: pathLists,
	}
}

// TargetResolvedParametersExportScalars returns scalar parameters for display/export.
// Path-list parameters are rendered with JoinPathsForShell at the export boundary only.
func TargetResolvedParametersExportScalars(resolved TargetResolvedParameters) map[string]string {
	if !resolved.HasValues() {
		return nil
	}

	out := make(map[string]string, len(resolved.Scalars)+len(resolved.PathLists)+len(resolved.StringLists))
	for key, value := range resolved.Scalars {
		out[key] = value
	}
	for key, paths := range resolved.PathLists {
		out[key] = expand.JoinPathsForShell(paths)
	}
	for key, fragments := range resolved.StringLists {
		out[key] = strings.Join(fragments, " ")
	}
	return out
}
