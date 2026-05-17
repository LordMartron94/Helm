package shared

const (
	MainSpanPhase             string = "Helm Interpretation"
	ParsingSpanPhase          string = "Parsing"
	SemanticAnalysisSpanPhase string = "IR creation"
	ExecutionChainResolution  string = "Execution Chain Resolution"
	TargetExecutionSpanPhase  string = "Target Execution"
)

const (
	PhasePayloadKey   string = "phase"
	RulePayloadKey    string = "rule"
	MessagePayloadKey string = "message"

	SemanticAnalysisPhase string = "semantics"
	GraphResolutionPhase  string = "graph-resolution"
	TargetExecutionPhase  string = "target-execution"
)
