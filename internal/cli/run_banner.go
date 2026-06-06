package cli

func shouldSuppressStartupBanner(session *Session, commandFields []string) bool {
	return resolveRunPresentation(session, commandFields) == DiagnosticPresentationSilent
}

func resolveRunPresentation(session *Session, commandFields []string) DiagnosticPresentation {
	if session == nil || len(commandFields) < 2 || commandFields[0] != "run" {
		return DiagnosticPresentationFull
	}

	quiet := false
	rest := commandFields[1:]
	for len(rest) > 0 && (rest[0] == "--bypass-cache" || rest[0] == "-q" || rest[0] == "--quiet") {
		if rest[0] == "-q" || rest[0] == "--quiet" {
			quiet = true
		}
		rest = rest[1:]
	}
	if len(rest) == 0 {
		return DiagnosticPresentationFull
	}

	canonical, ok := session.Catalog.ResolveTargetName(rest[0])
	if !ok {
		return DiagnosticPresentationFull
	}
	entryTarget, ok := session.Result.BuiltIR.Targets[canonical]
	if !ok {
		return DiagnosticPresentationFull
	}

	presentation := DiagnosticPresentationFull
	if quiet {
		presentation = DiagnosticPresentationQuiet
	}
	if entryTarget.Interactive {
		presentation = DiagnosticPresentationSilent
	}
	return presentation
}

// RunPresentationForCommandFields exposes presentation resolution for tests.
func RunPresentationForCommandFields(session *Session, commandFields []string) DiagnosticPresentation {
	return resolveRunPresentation(session, commandFields)
}
