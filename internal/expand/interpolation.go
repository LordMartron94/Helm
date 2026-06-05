package expand

import "strings"

// InterpolationContext carries scalar substitutions and structured artifact path lists.
// Path lists are joined with shell quoting only when a ${NAME} placeholder is expanded inside
// a string literal; argv and matrix expansion read PathLists directly.
type InterpolationContext struct {
	Scalars   map[string]string
	PathLists map[string][]string
}

func InterpolationContextExpandLiteral(ctx InterpolationContext, literal string) string {
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
			if paths, ok := ctx.PathLists[ident]; ok && len(paths) > 0 {
				out.WriteString(JoinPathsForShell(paths))
			} else if value, ok := ctx.Scalars[ident]; ok {
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

func InterpolationContextExpandRunCommand(ctx InterpolationContext, literal string) string {
	return expandFoldCommandLineWraps(InterpolationContextExpandLiteral(ctx, literal))
}

func InterpolationContextMergeScalars(into map[string]string, from map[string]string) map[string]string {
	if len(from) == 0 {
		return into
	}
	if into == nil {
		into = make(map[string]string, len(from))
	}
	for key, value := range from {
		into[key] = value
	}
	return into
}
