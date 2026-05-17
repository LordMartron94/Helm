package main

import (
	"flag"
	"fmt"
	"helm/internal/cli"
	"os"
)

func main() {
	colorModeFlag := flag.String("color-mode", "", "terminal color mode: none, ansi16, or truecolor (default: truecolor on TTY)")
	flag.Parse()

	args := flag.Args()
	helmFile := ""
	if len(args) > 0 {
		helmFile = args[0]
	}

	colorMode := cli.ColorMode(*colorModeFlag)
	if *colorModeFlag == "" {
		colorMode = cli.ResolveDefaultColorMode()
	} else {
		switch colorMode {
		case cli.ColorModeNone, cli.ColorModeAnsi16, cli.ColorModeTrueColor:
		default:
			fmt.Fprintf(os.Stderr, "invalid -color-mode %q\n", *colorModeFlag)
			os.Exit(2)
		}
	}

	lSpecPath, err := cli.ResolveHelmLSpecPath()
	if err != nil {
		fmt.Fprintf(os.Stderr, "helm: %v\n", err)
		os.Exit(1)
	}

	if err := cli.Run(cli.RunConfig{
		HelmFilePath: helmFile,
		LSpecPath:    lSpecPath,
		ColorMode:    colorMode,
	}); err != nil {
		fmt.Fprintf(os.Stderr, "helm: %v\n", err)
		os.Exit(1)
	}
}
