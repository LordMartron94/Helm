package shared

const (
	MainSpanPhase             string = "Helm Interpretation"
	ParsingSpanPhase          string = "parsing"
	SemanticAnalysisSpanPhase string = "IR creation"
)

const (
	PhasePayloadKey   string = "phase"
	RulePayloadKey    string = "rule"
	MessagePayloadKey string = "message"

	SemanticAnalysisPhase string = "semantics"
)
