package shared

const (
	StdoutPayloadKey   string = "stdout"
	StderrPayloadKey   string = "stderr"
	ExitCodePayloadKey string = "exit_code"
	CommandPayloadKey  string = "command"
	TargetPayloadKey   string = "target"

	ReasonPayloadKey             string = "reason"
	StateFingerprintPayloadKey   string = "state_fingerprint"
	OutputFingerprintPayloadKey  string = "output_fingerprint"
	DurationNSPayloadKey         string = "duration_ns"
	CacheHitReason               string = "cache_hit"

	SignalExecOK       string = "EXEC_OK"
	SignalExecFail     string = "EXEC_FAIL"
	SignalExecSkipped  string = "EXEC_SKIPPED"
	SignalCacheUpdated string = "CACHE_UPDATED"
)
