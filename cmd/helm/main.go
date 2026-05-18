package main

import (
	"flag"
	"fmt"
	"helm/internal/cli"
	"os"
)

func main() {
	showVersion := flag.Bool("version", false, "print version and exit")
	colorModeFlag := flag.String("color-mode", "", "terminal color mode: none, ansi16, or truecolor (default: truecolor on TTY)")
	streamRunsFlag := flag.Bool("stream-runs", true, "stream target stdout/stderr while runs execute (use -stream-runs=false to buffer until completion)")
	flag.Parse()

	if *showVersion {
		if err := cli.PrintVersion(os.Stdout); err != nil {
			fmt.Fprintf(os.Stderr, "helm: %v\n", err)
			os.Exit(1)
		}
		return
	}

	inv, err := cli.ParseInvocation(flag.Args())
	if err != nil {
		fmt.Fprintf(os.Stderr, "helm: %v\n", err)
		os.Exit(2)
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

	streamRunOutput := *streamRunsFlag
	if err := cli.Run(cli.RunConfig{
		HelmFilePath:    inv.HelmFilePath,
		LSpecPath:       lSpecPath,
		ColorMode:       colorMode,
		StreamRunOutput: &streamRunOutput,
		CommandFields:   inv.CommandFields,
	}); err != nil {
		fmt.Fprintf(os.Stderr, "helm: %v\n", err)
		os.Exit(1)
	}
}
