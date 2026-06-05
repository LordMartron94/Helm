package expand

import (
	"helm/internal/ir"
	"strings"
)

/*
ExpandFingerprintRunCommand returns a stable execution-fingerprint fragment for either string or argv
run commands. Argv templates fingerprint literal text and param names, not expanded path lists (input
artifact hashing covers path membership changes).
*/
func ExpandFingerprintRunCommand(
	command ir.HelmRunCommand,
	globalVars map[string]string,
	parameters map[string]string,
) string {
	if ir.HelmRunCommandIsArgv(command) {
		var buffer strings.Builder
		buffer.WriteString("argv")
		for _, element := range command.Argv {
			buffer.WriteByte(0)
			if element.ParamName != "" {
				buffer.WriteString("param:")
				buffer.WriteString(element.ParamName)
				continue
			}
			buffer.WriteString(ExpandInterpolateLiteral(element.Literal, globalVars, parameters))
		}
		return buffer.String()
	}

	return ExpandInterpolateRunCommand(command.String, globalVars, parameters)
}
