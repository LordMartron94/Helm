package interpreter

import (
	"fmt"
	"path/filepath"
	"signal"

	"helm/internal/ir"
)

// HelmInterpreterLoadWorkspace parses the root helm file, auto-discovers *.helm files
// under the workspace root, and merges the workspace IR.
func HelmInterpreterLoadWorkspace(
	interpreter *HelmInterpreter,
	rootFile string,
	signalCtx *signal.SignalContext,
) (ir.HelmIR, error) {
	rootAbs, err := filepath.Abs(rootFile)
	if err != nil {
		return ir.HelmIR{}, err
	}
	rootDir := filepath.Dir(rootAbs)

	rootResult := HelmInterpreterInterpretFileWithOptions(
		interpreter,
		rootAbs,
		signalCtx,
		HelmInterpretOptions{},
	)
	if rootResult.Error != nil {
		return ir.HelmIR{}, fmt.Errorf("%s: %w", rootAbs, rootResult.Error)
	}
	if !rootResult.BuiltIR.Succeeded {
		return ir.HelmIR{}, fmt.Errorf("semantic errors in %s", rootAbs)
	}

	merged := ir.HelmIR{
		SourceDirectory:  rootDir,
		RootManifestPath: rootAbs,
		FileGlobals:      make(map[string]map[string]ir.HelmGlobalVariable),
		Targets:          make(map[string]ir.HelmTarget),
		Entities:         make(map[string]ir.HelmEntity),
		Interfaces:       make(map[string]ir.HelmInterfaceDecl),
		Adapters:         make(map[string]ir.HelmAdapterDecl),
		Configurations:   make(map[string]ir.HelmConfigurationDecl),
	}

	if err := workspaceMergePartIR(&merged, rootResult.BuiltIR, rootDir, rootAbs); err != nil {
		return ir.HelmIR{}, err
	}

	if merged.Workspace == nil {
		merged.Mode = ir.HelmModeLegacy
		if len(merged.Entities) > 0 {
			merged.Mode = ir.HelmModeWorkspace
		}
		merged.Succeeded = true
		return merged, nil
	}

	inheritedGlobals := merged.Workspace.Globals
	if inheritedGlobals == nil {
		inheritedGlobals = make(map[string]ir.HelmGlobalVariable)
	}

	discovered, discoverErr := WorkspaceDiscoverHelmFiles(
		rootDir,
		rootAbs,
		merged.Workspace.Excludes,
	)
	if discoverErr != nil {
		return ir.HelmIR{}, discoverErr
	}

	for _, filePath := range discovered {
		result := HelmInterpreterInterpretFileWithOptions(
			interpreter,
			filePath,
			signalCtx,
			HelmInterpretOptions{InheritedGlobals: inheritedGlobals},
		)
		if result.Error != nil {
			return ir.HelmIR{}, fmt.Errorf("%s: %w", filePath, result.Error)
		}
		if !result.BuiltIR.Succeeded {
			return ir.HelmIR{}, fmt.Errorf("semantic errors in %s", filePath)
		}
		if result.BuiltIR.Workspace != nil {
			return ir.HelmIR{}, fmt.Errorf("workspace block may only appear in root manifest %s", rootAbs)
		}
		if err := workspaceMergePartIR(&merged, result.BuiltIR, rootDir, filePath); err != nil {
			return ir.HelmIR{}, err
		}
	}

	merged.Mode = ir.HelmModeWorkspace
	merged.Succeeded = true

	if err := workspaceValidateSegregation(&merged); err != nil {
		return ir.HelmIR{}, err
	}

	return merged, nil
}

func workspaceMergePartIR(merged *ir.HelmIR, part ir.HelmIR, rootDir, filePath string) error {
	if len(part.GlobalVariables) > 0 {
		merged.FileGlobals[filePath] = part.GlobalVariables
	}

	for name, target := range part.Targets {
		if _, exists := merged.Targets[name]; exists {
			return fmt.Errorf("duplicate target %q across workspace files", name)
		}
		merged.Targets[name] = target
	}

	for key, entity := range part.Entities {
		entity = workspaceRelabelEntity(entity, rootDir, filePath)
		canonical := ir.HelmLabelCanonical(entity.Label)
		if _, exists := merged.Entities[canonical]; exists {
			return fmt.Errorf("duplicate entity %q across workspace files", canonical)
		}
		merged.Entities[canonical] = entity
		_ = key
	}

	for name, decl := range part.Interfaces {
		if _, exists := merged.Interfaces[name]; exists {
			return fmt.Errorf("duplicate interface %q across workspace files", name)
		}
		merged.Interfaces[name] = decl
	}

	for name, decl := range part.Adapters {
		if _, exists := merged.Adapters[name]; exists {
			return fmt.Errorf("duplicate adapter %q across workspace files", name)
		}
		merged.Adapters[name] = decl
	}

	for name, decl := range part.Configurations {
		if _, exists := merged.Configurations[name]; exists {
			return fmt.Errorf("duplicate configuration %q across workspace files", name)
		}
		merged.Configurations[name] = decl
	}

	if part.Workspace != nil {
		if merged.Workspace != nil {
			return fmt.Errorf("multiple workspace blocks across files")
		}
		merged.Workspace = part.Workspace
	}

	return nil
}

func workspaceRelabelEntity(entity ir.HelmEntity, rootDir, filePath string) ir.HelmEntity {
	rel, err := filepath.Rel(rootDir, filepath.Dir(filePath))
	entity.SourceFile = filePath
	if err != nil || rel == "." {
		return entity
	}
	entity.Label.Path = filepath.ToSlash(rel)
	entity.SourceFile = filePath
	return entity
}

func workspaceValidateSegregation(merged *ir.HelmIR) error {
	for name, target := range merged.Targets {
		if target.Artifacts != nil {
			return fmt.Errorf(
				"target '%s' declares artifacts; workspace mode requires entities for artifact production",
				name,
			)
		}
	}
	return nil
}
