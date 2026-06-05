package ir

import (
	"fmt"
	"strings"
)

// HelmLabelParse parses a label string (//path or //path:name).
func HelmLabelParse(text string) (HelmLabel, error) {
	text = strings.TrimSpace(text)
	if !strings.HasPrefix(text, "//") {
		return HelmLabel{}, fmt.Errorf("label must start with //")
	}
	rest := strings.TrimPrefix(text, "//")
	if rest == "" {
		return HelmLabel{}, fmt.Errorf("label path is empty")
	}

	colon := strings.LastIndex(rest, ":")
	if colon < 0 {
		parts := strings.Split(rest, "/")
		name := parts[len(parts)-1]
		return HelmLabel{Path: rest, Name: name}, nil
	}

	path := rest[:colon]
	name := rest[colon+1:]
	if path == "" || name == "" {
		return HelmLabel{}, fmt.Errorf("invalid label %q", text)
	}
	return HelmLabel{Path: path, Name: name}, nil
}

func helmLabelFromStringLiteral(builder *irBuilder, literal string) (HelmLabel, bool) {
	label, err := HelmLabelParse(literal)
	if err != nil {
		return HelmLabel{}, false
	}
	return label, true
}
