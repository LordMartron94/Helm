package entityexecutor

import (
	"encoding/json"
	"path/filepath"

	"helm/internal/ir"
)

// EntityGraphExportEntry describes one resolved entity for export-graph JSON.
type EntityGraphExportEntry struct {
	Label       string              `json:"label"`
	Adapter     string              `json:"adapter"`
	Directory   string              `json:"directory"`
	RunArgvs    [][]string          `json:"run_argvs,omitempty"`
	DependsOn   []string            `json:"depends_on,omitempty"`
	PropertyBag map[string][]string `json:"property_bag,omitempty"`
	OutputPath  string              `json:"output_path,omitempty"`
}

// EntityExecutionGraphExport is the entity section of export-graph output.
type EntityExecutionGraphExport struct {
	Entities map[string]EntityGraphExportEntry `json:"entities"`
}

// EntityExportExecutionGraph resolves entity build steps without running them.
func EntityExportExecutionGraph(builtIR ir.HelmIR, roots []ir.HelmLabel) (*EntityExecutionGraphExport, error) {
	plan, err := EntityBuildExecutionPlan(builtIR, roots)
	if err != nil {
		return nil, err
	}

	usagePropagation := EntityBuildUsagePropagation(builtIR, roots)

	export := &EntityExecutionGraphExport{
		Entities: make(map[string]EntityGraphExportEntry),
	}

	keys := EntitySortedKeys(plan)
	for _, entityKey := range keys {
		entry, exportErr := entityExportGraphEntry(builtIR, entityKey, usagePropagation)
		if exportErr != nil {
			return nil, exportErr
		}
		export.Entities[entityKey] = entry
	}
	return export, nil
}

func entityExportGraphEntry(
	builtIR ir.HelmIR,
	entityKey string,
	usagePropagation EntityUsagePropagation,
) (EntityGraphExportEntry, error) {
	entity := builtIR.Entities[entityKey]
	absDir, err := filepath.Abs(builtIR.SourceDirectory)
	if err != nil {
		return EntityGraphExportEntry{}, err
	}

	entry := EntityGraphExportEntry{
		Label:       entityKey,
		Adapter:     entity.AdapterName,
		Directory:   absDir,
		PropertyBag: map[string][]string{},
	}

	for key := range entity.InterfaceBag {
		entry.PropertyBag[key] = entityBagFragments(entity, key)
	}

	for _, dep := range entity.Deps {
		entry.DependsOn = append(entry.DependsOn, ir.HelmLabelCanonical(dep))
	}

	var usage EntityPropertyBag
	if usagePropagation != nil {
		usage = usagePropagation[entityKey]
	}

	plan, expandErr := EntityExpandAdapter(builtIR.SourceDirectory, builtIR, entityKey, usage)
	if expandErr != nil {
		return EntityGraphExportEntry{}, expandErr
	}

	entry.OutputPath = entityAdapterPrimaryOutputPath(plan)
	for _, step := range plan.Steps {
		entry.RunArgvs = append(entry.RunArgvs, step.Argv)
	}

	return entry, nil
}

// EntityExportExecutionGraphJSON marshals entity export with stable formatting.
func EntityExportExecutionGraphJSON(builtIR ir.HelmIR, roots []ir.HelmLabel) ([]byte, error) {
	export, err := EntityExportExecutionGraph(builtIR, roots)
	if err != nil {
		return nil, err
	}
	return json.MarshalIndent(export, "", "  ")
}
