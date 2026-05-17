package cli

import (
	"fmt"
	"helm/internal/version"
	"io"
)

func PrintVersion(w io.Writer) error {
	_, err := fmt.Fprintln(w, version.String())
	return err
}
