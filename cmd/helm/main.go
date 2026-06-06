package main

import (
	"flag"
	"fmt"
	"helm/internal/cli"
	"os"
	"strconv"
)

func main() {
	if handled := runEarlyCommand(os.Args); handled {
		return
	}

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

	streamRunOutput := *streamRunsFlag
	if err := cli.Run(cli.RunConfig{
		HelmFilePath:    inv.HelmFilePath,
		ColorMode:       colorMode,
		StreamRunOutput: &streamRunOutput,
		CommandFields:   inv.CommandFields,
	}); err != nil {
		if exitCode, ok := cli.CommandExitCode(err); ok {
			os.Exit(exitCode)
		}
		fmt.Fprintf(os.Stderr, "helm: %v\n", err)
		os.Exit(1)
	}
}

func runEarlyCommand(args []string) bool {
	if len(args) < 2 {
		return false
	}

	switch args[1] {
	case "completion":
		if len(args) < 3 || args[2] != "bash" {
			fmt.Fprintf(os.Stderr, "usage: helm completion bash\n")
			os.Exit(2)
		}
		fmt.Print(cli.BashCompletionScript())
		return true
	case "__complete":
		runShellComplete(args)
		return true
	default:
		return false
	}
}

func runShellComplete(args []string) {
	words := extractCompleteWords(args)
	if len(words) == 0 {
		return
	}
	cword := len(words) - 1
	if cword < 0 {
		cword = 0
	}
	if raw := os.Getenv("COMP_CWORD"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed >= 0 {
			cword = parsed
		}
	}
	if cword >= len(words) {
		words = append(words, "")
	}

	dir, err := os.Getwd()
	if err != nil {
		dir = ""
	}

	for _, suggestion := range cli.CompleteWords(cli.CompletionRequest{
		Words: words,
		CWord: cword,
		Dir:   dir,
	}) {
		fmt.Println(suggestion)
	}
}

func extractCompleteWords(args []string) []string {
	for i, arg := range args {
		if arg == "--" {
			return args[i+1:]
		}
	}
	if len(args) > 2 {
		return args[2:]
	}
	return nil
}
