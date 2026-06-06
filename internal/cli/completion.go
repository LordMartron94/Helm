package cli

import (
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type CompletionRequest struct {
	Words []string
	CWord int
	Dir   string
}

var globalFlags = []string{"-version", "-color-mode", "-stream-runs"}

func CompleteWords(req CompletionRequest) []string {
	if len(req.Words) == 0 || req.CWord < 0 {
		return nil
	}
	if req.CWord >= len(req.Words) {
		req.Words = append(append([]string(nil), req.Words...), "")
	}

	dir := req.Dir
	if dir == "" {
		var err error
		dir, err = os.Getwd()
		if err != nil {
			return nil
		}
	}

	cur := req.Words[req.CWord]

	if req.CWord > 0 {
		switch req.Words[req.CWord-1] {
		case "-color-mode":
			return prefixFilter(cur, []string{"none", "ansi16", "truecolor"})
		case "-stream-runs":
			return prefixFilter(cur, []string{"true", "false", "on", "off"})
		}
	}

	posStart, positionals := splitCompletionPositionals(req.Words)
	if posStart < 0 {
		return nil
	}

	if req.CWord < posStart {
		if strings.HasPrefix(cur, "-") {
			return prefixFilter(cur, globalFlags)
		}
		return prefixFilter(cur, globalFlags)
	}

	relCWord := req.CWord - posStart
	if relCWord == 0 && (len(positionals) == 0 || (len(positionals) == 1 && positionals[0] == "")) {
		return completeFirstPositional(dir, cur)
	}

	helmFilePath, commandFields, cmdIndex := resolveCompletionCommandContext(positionals, relCWord)

	if cmdIndex < 0 {
		if relCWord == 0 {
			return completeFirstPositional(dir, cur)
		}
		return prefixFilter(cur, helmFileCandidates(dir, cur))
	}

	if len(commandFields) == 0 || cmdIndex >= len(commandFields) {
		if cmdIndex == 0 {
			return prefixFilter(cur, BuiltinCommands())
		}
		return nil
	}

	if cmdIndex == 0 {
		return prefixFilter(cur, BuiltinCommands())
	}

	verb := commandFields[0]
	args := commandFields[1:]

	switch verb {
	case "run":
		return completeRun(cur, helmFilePath, args, cmdIndex-1)
	case "export-graph":
		return completeExportGraph(cur, helmFilePath, args, cmdIndex-1)
	case "help":
		return completeHelp(helmFilePath, args, cmdIndex-1, cur)
	case "completion":
		return completeCompletion(args, cmdIndex-1, cur)
	case "set", "config":
		return completeSet(args, cmdIndex-1, cur)
	default:
		return nil
	}
}

func splitCompletionPositionals(words []string) (posStart int, positionals []string) {
	i := 1
	for i < len(words) {
		word := words[i]
		if !strings.HasPrefix(word, "-") {
			break
		}
		switch {
		case word == "-version":
			i++
		case word == "-color-mode", word == "-stream-runs":
			i++
			if i < len(words) && !strings.HasPrefix(words[i], "-") {
				i++
			}
		case strings.HasPrefix(word, "-color-mode="), strings.HasPrefix(word, "-stream-runs="):
			i++
		default:
			i++
		}
	}
	if i >= len(words) {
		return i, nil
	}
	return i, words[i:]
}

func resolveCompletionCommandContext(positionals []string, relCWord int) (helmFilePath string, commandFields []string, cmdIndex int) {
	if len(positionals) == 0 {
		return "", nil, -1
	}

	if IsBuiltinCommand(positionals[0]) {
		return "", positionals, relCWord
	}

	if isExplicitHelmFilePath(positionals[0]) {
		helmFilePath = positionals[0]
		if len(positionals) == 1 {
			if relCWord == 0 {
				return helmFilePath, nil, -1
			}
			return helmFilePath, nil, relCWord - 1
		}
		return helmFilePath, positionals[1:], relCWord - 1
	}

	if relCWord == 0 {
		return "", nil, -1
	}
	return "", positionals, relCWord
}

func completeFirstPositional(dir, cur string) []string {
	candidates := topLevelCommandsForCompletion()
	candidates = append(candidates, helmFileCandidates(dir, cur)...)
	return prefixFilter(cur, candidates)
}

func topLevelCommandsForCompletion() []string {
	commands := BuiltinCommands()
	commands = append(commands, "completion")
	sort.Strings(commands)
	return commands
}

func completeRun(cur, helmFilePath string, runArgs []string, runArgIndex int) []string {
	rest := runArgs
	for len(rest) > 0 && (rest[0] == "--bypass-cache" || rest[0] == "-q" || rest[0] == "--quiet") {
		rest = rest[1:]
		runArgIndex--
	}

	if runArgIndex <= 0 {
		candidates := []string{"--bypass-cache", "-q", "--quiet"}
		session := sessionCreateForCompletion(helmFilePath)
		if session != nil {
			candidates = append(candidates, session.Catalog.AllRunNames()...)
			SessionDestroy(session)
		}
		return prefixFilter(cur, candidates)
	}

	if runArgIndex == 1 {
		if strings.HasPrefix(cur, "-") {
			return prefixFilter(cur, []string{"--bypass-cache", "-q", "--quiet"})
		}
		session := sessionCreateForCompletion(helmFilePath)
		if session == nil {
			return prefixFilter(cur, []string{"--bypass-cache", "-q", "--quiet"})
		}
		defer SessionDestroy(session)
		return prefixFilter(cur, session.Catalog.AllRunNames())
	}

	if len(rest) == 0 {
		return nil
	}

	targetName := rest[0]
	session := sessionCreateForCompletion(helmFilePath)
	if session == nil {
		return nil
	}
	defer SessionDestroy(session)

	canonical, ok := session.Catalog.ResolveTargetName(targetName)
	if !ok {
		return nil
	}
	entry, ok := session.Catalog.Entry(canonical)
	if !ok {
		return nil
	}

	if strings.Contains(cur, "=") {
		key, _, found := strings.Cut(cur, "=")
		if !found {
			return nil
		}
		for _, param := range entry.Parameters {
			if param.Name == key {
				return prefixFilter(cur, []string{key + "="})
			}
		}
		return nil
	}

	names := make([]string, 0, len(entry.Parameters))
	for _, param := range entry.Parameters {
		names = append(names, param.Name+"=")
	}
	return prefixFilter(cur, names)
}

func completeExportGraph(cur, helmFilePath string, exportArgs []string, exportArgIndex int) []string {
	rest := exportArgs
	for len(rest) > 0 && strings.HasPrefix(rest[0], "-") {
		switch rest[0] {
		case "--output", "-o":
			if len(rest) < 2 {
				return nil
			}
			rest = rest[2:]
			exportArgIndex -= 2
		default:
			rest = rest[1:]
			exportArgIndex--
		}
	}

	if exportArgIndex <= 0 {
		candidates := []string{"--output", "-o"}
		session := sessionCreateForCompletion(helmFilePath)
		if session != nil {
			candidates = append(candidates, session.Catalog.AllRunNames()...)
			SessionDestroy(session)
		}
		return prefixFilter(cur, candidates)
	}

	if exportArgIndex == 1 {
		if strings.HasPrefix(cur, "-") {
			return prefixFilter(cur, []string{"--output", "-o"})
		}
		session := sessionCreateForCompletion(helmFilePath)
		if session == nil {
			return prefixFilter(cur, []string{"--output", "-o"})
		}
		defer SessionDestroy(session)
		return completeExportGraphTargets(cur, session)
	}

	if len(rest) == 0 {
		return nil
	}

	targetSpec := rest[0]
	if strings.Contains(targetSpec, ",") {
		return nil
	}

	session := sessionCreateForCompletion(helmFilePath)
	if session == nil {
		return nil
	}
	defer SessionDestroy(session)

	canonical, ok := session.Catalog.ResolveTargetName(targetSpec)
	if !ok {
		return nil
	}
	entry, ok := session.Catalog.Entry(canonical)
	if !ok {
		return nil
	}

	if strings.Contains(cur, "=") {
		key, _, found := strings.Cut(cur, "=")
		if !found {
			return nil
		}
		for _, param := range entry.Parameters {
			if param.Name == key {
				return prefixFilter(cur, []string{key + "="})
			}
		}
		return nil
	}

	names := make([]string, 0, len(entry.Parameters))
	for _, param := range entry.Parameters {
		names = append(names, param.Name+"=")
	}
	return prefixFilter(cur, names)
}

func completeExportGraphTargets(cur string, session *Session) []string {
	prefix, filter := exportGraphTargetCompletionPrefix(cur)
	names := session.Catalog.AllRunNames()
	matches := prefixFilter(filter, names)
	if prefix == "" {
		return matches
	}
	out := make([]string, 0, len(matches))
	for _, match := range matches {
		out = append(out, prefix+match)
	}
	return out
}

func completeHelp(helmFilePath string, args []string, argIndex int, cur string) []string {
	showHidden := false
	rest := args
	for len(rest) > 0 && rest[0] == "--show-hidden" {
		showHidden = true
		rest = rest[1:]
	}
	if argIndex > len(rest) {
		return nil
	}
	if argIndex > 0 && rest[argIndex-1] == "completion" {
		return nil
	}

	candidates := []string{"--show-hidden", "completion"}
	session := sessionCreateForCompletion(helmFilePath)
	if session != nil {
		candidates = append(candidates, session.Catalog.VisibleCanonicalNames()...)
		if showHidden {
			candidates = append(candidates, session.Catalog.HiddenCanonicalNames()...)
		}
		SessionDestroy(session)
	}
	return prefixFilter(cur, candidates)
}

func completeCompletion(args []string, argIndex int, cur string) []string {
	if argIndex == 0 {
		return prefixFilter(cur, []string{"bash"})
	}
	return nil
}

func completeSet(args []string, argIndex int, cur string) []string {
	switch argIndex {
	case 0:
		return prefixFilter(cur, []string{"stream-runs"})
	case 1:
		if len(args) > 0 && strings.EqualFold(args[0], "stream-runs") {
			return prefixFilter(cur, []string{"on", "off", "true", "false", "yes", "no", "1", "0"})
		}
	}
	return nil
}

func sessionCreateForCompletion(helmFilePath string) *Session {
	lSpecPath, releaseLSpec, err := ResolveHelmLSpecPath()
	if err != nil {
		return nil
	}
	helmFile, err := DiscoverHelmFile(helmFilePath)
	if err != nil {
		return nil
	}
	streamRuns := true
	session, err := SessionCreate(SessionConfig{
		HelmFile:         helmFile,
		LSpecPath:        lSpecPath,
		ColorMode:        ColorModeNone,
		DiagnosticOutput: io.Discard,
		StreamRunOutput:  &streamRuns,
	})
	if releaseLSpec != nil {
		releaseLSpec()
	}
	if err != nil {
		return nil
	}
	return session
}

func helmFileCandidates(dir, prefix string) []string {
	seen := make(map[string]struct{})
	var out []string

	add := func(name string) {
		if !strings.HasPrefix(name, prefix) {
			return
		}
		path := filepath.Join(dir, name)
		if info, err := os.Stat(path); err == nil && info.IsDir() {
			return
		}
		if _, ok := seen[name]; ok {
			return
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}

	add(helmfileDefaultName)

	matches, err := filepath.Glob(filepath.Join(dir, "*.helm"))
	if err == nil {
		for _, path := range matches {
			add(filepath.Base(path))
		}
	}

	if prefix != "" {
		entries, err := os.ReadDir(dir)
		if err == nil {
			for _, entry := range entries {
				if entry.IsDir() {
					continue
				}
				name := entry.Name()
				if isHelmFileName(name) {
					add(name)
				}
			}
		}
	}

	sort.Strings(out)
	return out
}

func prefixFilter(prefix string, candidates []string) []string {
	if prefix == "" {
		out := make([]string, len(candidates))
		copy(out, candidates)
		sort.Strings(out)
		return out
	}
	var out []string
	for _, candidate := range candidates {
		if strings.HasPrefix(candidate, prefix) {
			out = append(out, candidate)
		}
	}
	sort.Strings(out)
	return out
}
