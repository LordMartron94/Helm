package cli

import (
	"helm/internal/ir"
	"sort"
)

type TargetCatalogEntry struct {
	CanonicalName string
	Aliases       []string
	HelpText      string
	Parameters    []ir.HelmTargetParameter
}

type TargetCatalog struct {
	entries   map[string]TargetCatalogEntry
	nameIndex map[string]string
}

func TargetCatalogBuild(builtIR ir.HelmIR) TargetCatalog {
	catalog := TargetCatalog{
		entries:   make(map[string]TargetCatalogEntry, len(builtIR.Targets)),
		nameIndex: make(map[string]string),
	}

	names := make([]string, 0, len(builtIR.Targets))
	for name := range builtIR.Targets {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, canonical := range names {
		target := builtIR.Targets[canonical]
		entry := TargetCatalogEntry{
			CanonicalName: canonical,
			Aliases:       append([]string(nil), target.Aliases...),
			HelpText:      target.HelpText,
			Parameters:    append([]ir.HelmTargetParameter(nil), target.Parameters...),
		}
		catalog.entries[canonical] = entry
		catalog.nameIndex[canonical] = canonical
		for _, alias := range target.Aliases {
			catalog.nameIndex[alias] = canonical
		}
	}

	return catalog
}

func (catalog TargetCatalog) ResolveTargetName(name string) (string, bool) {
	canonical, ok := catalog.nameIndex[name]
	return canonical, ok
}

func (catalog TargetCatalog) Entry(canonicalName string) (TargetCatalogEntry, bool) {
	entry, ok := catalog.entries[canonicalName]
	return entry, ok
}

func (catalog TargetCatalog) CanonicalNames() []string {
	names := make([]string, 0, len(catalog.entries))
	for name := range catalog.entries {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (catalog TargetCatalog) AllRunNames() []string {
	seen := make(map[string]struct{}, len(catalog.nameIndex))
	for name := range catalog.nameIndex {
		seen[name] = struct{}{}
	}
	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
