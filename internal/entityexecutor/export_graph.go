package entityexecutor

import (
	"encoding/json"
	"path/filepath"

	"helm/internal/ir"
)

// EntityGraphExportEntry describes one resolved entity for export-graph JSON.
type EntityGraphExportEntry struct {
	Label         string              `json:"label"`
	Configuration string              `json:"configuration,omitempty"`
	Adapter       string              `json:"adapter"`
	Directory     string              `json:"directory"`
	RunArgvs      [][]string          `json:"run_argvs,omitempty"`
	DependsOn     []string            `json:"depends_on,omitempty"`
	PropertyBag   map[string][]string `json:"property_bag,omitempty"`
	OutputPath    string              `json:"output_path,omitempty"`
}

// EntityExecutionGraphExport is the entity section of export-graph output.
type EntityExecutionGraphExport struct {
	Entities map[string]EntityGraphExportEntry `json:"entities"`
}

// EntityExportExecutionGraph resolves entity build steps without running them.
func EntityExportExecutionGraph(builtIR ir.HelmIR, roots []EntityInstance) (*EntityExecutionGraphExport, error) {
	plan, err := EntityBuildExecutionPlan(builtIR, roots)
	if err != nil {
		return nil, err
	}

	usagePropagation := EntityBuildUsagePropagation(builtIR, roots)

	export := &EntityExecutionGraphExport{
		Entities: make(map[string]EntityGraphExportEntry),
	}

	keys := EntitySortedKeys(plan)
	for _, instanceKey := range keys {
		entry, exportErr := entityExportGraphEntry(builtIR, instanceKey, usagePropagation)
		if exportErr != nil {
			return nil, exportErr
		}
		export.Entities[instanceKey] = entry
	}
	return export, nil
}

func entityExportGraphEntry(
	builtIR ir.HelmIR,
	instanceKey string,
	usagePropagation EntityUsagePropagation,
) (EntityGraphExportEntry, error) {
	entity, inst, err := entityLookupForInstanceKey(builtIR, instanceKey)
	if err != nil {
		return EntityGraphExportEntry{}, err
	}

	adapterName, adapterErr := ir.HelmEntityAdapterNameFor(entity, inst.Configuration)
	if adapterErr != nil {
		return EntityGraphExportEntry{}, adapterErr
	}

	deps, depErr := ir.HelmEntityDepsForConfiguration(entity, inst.Configuration)
	if depErr != nil {
		return EntityGraphExportEntry{}, depErr
	}

	absDir, err := filepath.Abs(builtIR.SourceDirectory)
	if err != nil {
		return EntityGraphExportEntry{}, err
	}

	entry := EntityGraphExportEntry{
		Label:         ir.HelmLabelCanonical(inst.Label),
		Configuration: inst.Configuration,
		Adapter:       adapterName,
		Directory:     absDir,
		PropertyBag:   map[string][]string{},
	}

	for key := range entity.InterfaceBag {
		entry.PropertyBag[key] = entityBagFragments(entity, key)
	}

	for _, dep := range deps {
		depInst := ir.HelmEntityDepResolveInstance(dep, inst.Configuration)
		entry.DependsOn = append(entry.DependsOn, entityInstanceKey(depInst))
	}

	var usage EntityPropertyBag
	if usagePropagation != nil {
		usage = usagePropagation[instanceKey]
	}

	plan, expandErr := EntityExpandAdapter(builtIR.SourceDirectory, builtIR, instanceKey, usage)
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
func EntityExportExecutionGraphJSON(builtIR ir.HelmIR, roots []EntityInstance) ([]byte, error) {
	export, err := EntityExportExecutionGraph(builtIR, roots)
	if err != nil {
		return nil, err
	}
	return json.MarshalIndent(export, "", "  ")
}

// EntityExportExecutionGraphFromLabels resolves default-configuration instances from labels.
func EntityExportExecutionGraphFromLabels(builtIR ir.HelmIR, roots []ir.HelmLabel) (*EntityExecutionGraphExport, error) {
	return EntityExportExecutionGraph(builtIR, EntityLegacyRootsFromLabels(roots))
}

// EntityExportExecutionGraphJSONFromLabels marshals export for label-only roots.
func EntityExportExecutionGraphJSONFromLabels(builtIR ir.HelmIR, roots []ir.HelmLabel) ([]byte, error) {
	export, err := EntityExportExecutionGraphFromLabels(builtIR, roots)
	if err != nil {
		return nil, err
	}
	return json.MarshalIndent(export, "", "  ")
}
