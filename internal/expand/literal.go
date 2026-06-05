package expand

import "strings"

func ExpandInterpolateLiteral(
	literal string,
	globalVars map[string]string,
	parameters map[string]string,
) string {
	if literal == "" {
		return literal
	}

	var out strings.Builder
	out.Grow(len(literal))

	i := 0
	for i < len(literal) {
		if literal[i] == '$' && i+1 < len(literal) && literal[i+1] == '{' {
			close := strings.IndexByte(literal[i+2:], '}')
			if close < 0 {
				out.WriteByte(literal[i])
				i++
				continue
			}

			ident := literal[i+2 : i+2+close]
			if value, ok := globalVars[ident]; ok {
				out.WriteString(value)
			} else if value, ok := parameters[ident]; ok {
				out.WriteString(value)
			} else {
				out.WriteString("${")
				out.WriteString(ident)
				out.WriteByte('}')
			}

			i += 2 + close + 1
			continue
		}

		out.WriteByte(literal[i])
		i++
	}

	return out.String()
}

/*
ExpandInterpolateRunCommand resolves a run-command literal: it performs ${...} interpolation and then
folds the line-wrapping whitespace produced by triple-quoted strings (or by interpolated multiline
globals) into single spaces, yielding the one logical command line Helm shells out.

[Context]
The `run` keyword models a single command that is shlex-split and spawned directly. Triple-quoted
strings let authors wrap a long invocation across several indented lines for readability; those source
newlines and their surrounding indentation must collapse so the executed command, the cache
fingerprint, and the exported execution graph all present the same clean single-line command instead
of an embedded-newline blob.

[Side Effects]
Pure function. Does not mutate inputs.
*/
func ExpandInterpolateRunCommand(
	literal string,
	globalVars map[string]string,
	parameters map[string]string,
) string {
	return expandFoldCommandLineWraps(ExpandInterpolateLiteral(literal, globalVars, parameters))
}

/*
expandFoldCommandLineWraps replaces every whitespace run that contains a line break with a single
space and trims the ends. Whitespace runs without a line break (e.g. deliberate spacing between
arguments) are preserved verbatim, since only the line wrapping introduced by multiline strings needs
to be undone.
*/
func expandFoldCommandLineWraps(command string) string {
	if command == "" {
		return command
	}

	var out strings.Builder
	out.Grow(len(command))

	for i := 0; i < len(command); {
		if !expandIsCommandWhitespace(command[i]) {
			out.WriteByte(command[i])
			i++
			continue
		}

		runEnd := i
		hasLineBreak := false
		for runEnd < len(command) && expandIsCommandWhitespace(command[runEnd]) {
			if command[runEnd] == '\n' || command[runEnd] == '\r' {
				hasLineBreak = true
			}
			runEnd++
		}

		if hasLineBreak {
			out.WriteByte(' ')
		} else {
			out.WriteString(command[i:runEnd])
		}
		i = runEnd
	}

	return strings.TrimSpace(out.String())
}

func expandIsCommandWhitespace(c byte) bool {
	switch c {
	case ' ', '\t', '\n', '\r', '\f', '\v':
		return true
	default:
		return false
	}
}
