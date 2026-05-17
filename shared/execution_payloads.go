package shared

const (
	StdoutPayloadKey   string = "stdout"
	StderrPayloadKey   string = "stderr"
	ExitCodePayloadKey string = "exit_code"
	CommandPayloadKey  string = "command"
	TargetPayloadKey   string = "target"

	SignalExecOK   string = "EXEC_OK"
	SignalExecFail string = "EXEC_FAIL"
)
