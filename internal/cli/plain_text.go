package cli

import "strings"

// StripTerminalEscapeSequences removes ANSI SGR escape sequences from rendered diagnostics.
func StripTerminalEscapeSequences(text string) string {
	if text == "" {
		return text
	}

	var builder strings.Builder
	builder.Grow(len(text))

	i := 0
	for i < len(text) {
		if text[i] == '\x1b' && i+1 < len(text) && text[i+1] == '[' {
			j := i + 2
			for j < len(text) && text[j] != 'm' {
				j++
			}
			if j < len(text) {
				i = j + 1
				continue
			}
		}
		builder.WriteByte(text[i])
		i++
	}

	return builder.String()
}
