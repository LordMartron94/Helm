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
