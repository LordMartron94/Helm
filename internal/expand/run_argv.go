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
	ctx InterpolationContext,
) string {
	if ir.HelmRunCommandIsArgv(command) {
		var buffer strings.Builder
		buffer.WriteString("argv")
		for _, element := range command.Argv {
			buffer.WriteByte(0)
			if element.Collect != nil {
				buffer.WriteString("collect:")
				buffer.WriteString(element.Collect.DependenciesParam)
				buffer.WriteByte(0)
				buffer.WriteString(element.Collect.ExportKey)
				continue
			}
			if element.ParamName != "" {
				buffer.WriteString("param:")
				buffer.WriteString(element.ParamName)
				continue
			}
			if element.AbsPath != "" {
				buffer.WriteString("abs_path:")
				buffer.WriteString(InterpolationContextExpandLiteral(ctx, element.AbsPath))
				continue
			}
			buffer.WriteString(InterpolationContextExpandLiteral(ctx, element.Literal))
		}
		return buffer.String()
	}

	return InterpolationContextExpandRunCommand(ctx, command.String)
}
