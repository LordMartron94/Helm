package ir

// IRGlobalsForFile returns globals visible when interpreting sourceFile.
// Workspace globals inject into discovered files only; file-local globals are per-file.
// Legacy single-file IR falls back to GlobalVariables when FileGlobals is unset.
func IRGlobalsForFile(builtIR HelmIR, sourceFile string) map[string]HelmGlobalVariable {
	return IRGlobalsForFileConfiguration(builtIR, sourceFile, "")
}

// IRGlobalsForFileConfiguration merges workspace globals, optional configuration globals,
// and file-local globals for entity adapter expansion.
func IRGlobalsForFileConfiguration(
	builtIR HelmIR,
	sourceFile string,
	configurationName string,
) map[string]HelmGlobalVariable {
	if builtIR.Workspace == nil && len(builtIR.FileGlobals) == 0 && len(builtIR.Configurations) == 0 {
		return builtIR.GlobalVariables
	}

	merged := make(map[string]HelmGlobalVariable)
	if builtIR.Workspace != nil && sourceFile != builtIR.RootManifestPath {
		for key, value := range builtIR.Workspace.Globals {
			merged[key] = value
		}
	}

	if configurationName != "" && configurationName != HelmConfigurationDefaultName {
		if decl, ok := builtIR.Configurations[configurationName]; ok {
			for key, value := range decl.Globals {
				merged[key] = value
			}
		}
	}

	if builtIR.FileGlobals != nil {
		if fileGlobals, ok := builtIR.FileGlobals[sourceFile]; ok {
			for key, value := range fileGlobals {
				merged[key] = value
			}
		}
	}
	return merged
}

// IRGlobalsForRootManifest returns file-local globals for the workspace root Helmfile.
func IRGlobalsForRootManifest(builtIR HelmIR) map[string]HelmGlobalVariable {
	if builtIR.RootManifestPath == "" {
		return builtIR.GlobalVariables
	}
	return IRGlobalsForFile(builtIR, builtIR.RootManifestPath)
}
