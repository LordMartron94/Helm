package expand

import (
	"strconv"
	"strings"
)

func JoinPathsForShell(paths []string) string {
	if len(paths) == 0 {
		return ""
	}

	var b strings.Builder
	for i, path := range paths {
		if i > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(strconv.Quote(path))
	}
	return b.String()
}
