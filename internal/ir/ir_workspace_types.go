package ir

// HelmExecutionMode distinguishes legacy v1.7 orchestration from Helm 2.0 workspace mode.
type HelmExecutionMode int

const (
	HelmModeLegacy HelmExecutionMode = iota
	HelmModeWorkspace
)

const (
	ERROR_DUPLICATE_WORKSPACE        string = "WS_001"
	ERROR_DUPLICATE_ENTITY           string = "ENTITY_002"
	ERROR_ENTITY_MISSING_USE         string = "ENTITY_003"
	ERROR_ENTITY_HAS_RUN             string = "ENTITY_001"
	ERROR_TARGET_HAS_ARTIFACTS_IN_WS string = "TARGET_024"
	ERROR_DUPLICATE_INTERFACE        string = "IFACE_001"
	ERROR_DUPLICATE_ADAPTER          string = "ADPT_001"
	ERROR_INVALID_LABEL              string = "LABEL_001"
	ERROR_UNKNOWN_ENTITY_LABEL       string = "LABEL_002"
	ERROR_ENTITY_CYCLE               string = "ENTITY_004"
	ERROR_ENTITY_BIN_HAS_INTERFACE   string = "ENTITY_005"
)

// HelmWorkspace is the root workspace manifest (Helm 2.0).
type HelmWorkspace struct {
	Globals  map[string]HelmGlobalVariable
	Excludes []string
}

// HelmLabel identifies an entity across the workspace (//path or //path:name).
type HelmLabel struct {
	Path string
	Name string
}

func HelmLabelCanonical(label HelmLabel) string {
	if label.Name == "" {
		return "//" + label.Path
	}
	return "//" + label.Path + ":" + label.Name
}

// HelmEntity is a buildable workspace component (artifacts via adapter only).
type HelmEntity struct {
	Name         string
	Label        HelmLabel
	Kind         string
	AdapterName  string
	Parameters   map[string]HelmParameterValue
	Deps         []HelmLabel
	InterfaceBag map[string]HelmStringListExpr
	SourceFile   string
}

// HelmInterfaceDecl names property-bag keys for an adapter family (no types).
type HelmInterfaceDecl struct {
	Name string
	Keys []string
}

// HelmAdapterPhase is one explicit step in an adapter sub-graph.
type HelmAdapterPhase struct {
	Name             string
	DependsOn        []string
	Matrix           *HelmMatrix
	MatrixLegOutputs []HelmArtifactInput
	MatrixRuns       []HelmRunCommand
	Outputs          []HelmArtifactInput
	Runs             []HelmRunCommand
	Env              map[string]HelmStringListExpr
}

// HelmAdapterDecl is a macro that expands into an ordered adapter sub-graph.
type HelmAdapterDecl struct {
	Name       string
	Parameters []HelmTargetParameter
	Phases     []HelmAdapterPhase
	// Legacy flat adapter fields (deprecated; used when Phases is empty).
	Outputs          []HelmArtifactInput
	Matrix           *HelmMatrix
	MatrixLegOutputs []HelmArtifactInput
	MatrixRuns       []HelmRunCommand
	Env              map[string]HelmStringListExpr
	Runs             []HelmRunCommand
}

func HelmAdapterDeclUsesPhases(decl HelmAdapterDecl) bool {
	return len(decl.Phases) > 0
}
