package interpreter

import (
	"fmt"
	"foundation/location"
	"foundation/system"
	"helm/internal/entityexecutor"
	"helm/internal/ir"
	"helm/internal/targetexecutor"
	"helm/shared"
	"langspec"
	"langspec/bootstrap"
	"langspec/dsl"
	"langspec/dsl/semantics"
	"lexarch"
	"lingua/helm/artifacts"
	"memcore"
	"memforge"
	"os"
	"path/filepath"
	"signal"
	"structarch"
	"syntaxa"
	"syntaxa/lowering"
)

// ------------------------------------------------------------------ RESULT

type HelmInterpreterInterpretationResult struct {
	filePath   string
	sourceText string

	parseTrace *syntaxa.ParseTrace
	rootNode   *syntaxa.SyntaxaLSTNode[artifacts.Node]

	compiledSymbols *semantics.CompiledSymbolTable

	BuiltIR ir.HelmIR

	Error error
}

func HelmInterpreterInterpretationResultSourcePath(result HelmInterpreterInterpretationResult) string {
	return result.filePath
}

// HelmInterpreterInterpretationResultFromIR builds a result from merged workspace IR.
func HelmInterpreterInterpretationResultFromIR(
	interpreter *HelmInterpreter,
	filePath string,
	builtIR ir.HelmIR,
) HelmInterpreterInterpretationResult {
	return HelmInterpreterInterpretationResult{
		filePath:        filePath,
		compiledSymbols: interpreter.compiledSymbols,
		BuiltIR:         builtIR,
	}
}

// HelmInterpreterCompiledSymbols exposes compiled symbols for diagnostics.
func HelmInterpreterCompiledSymbols(interpreter *HelmInterpreter) *semantics.CompiledSymbolTable {
	return interpreter.compiledSymbols
}

func HelmInterpreterInterpretationResultDumpParseTrace(result HelmInterpreterInterpretationResult) {
	dsl.RenderParseTrace(os.Stdout, result.parseTrace, func(token lexarch.TokenKind) string {
		return result.compiledSymbols.TokenName(uint32(token))
	})
}

// ------------------------------------------------------------------ INTERPRETER

type HelmInterpreter struct {
	allocator       memcore.MarkRaw
	compiledSymbols *semantics.CompiledSymbolTable
	parser          *bootstrap.LangParser[artifacts.Node]
}

func HelmInterpreterTryCreate(helmSpecFile string) (*HelmInterpreter, error) {
	allocator := memforge.DynamicLinearAllocatorCreateFunction(uint64(10*memcore.KiloByte), memforge.DynamicLinearAllocatorGrowthTemplateDoubleOrNeededWithMaxPanic(uint64(1*memcore.GigaByte)))

	parser, compiledSymbols, err := bootstrap.CompileParserFromSpecWithCompiledSymbols[artifacts.Node](
		helmSpecFile,
		func(sizeBytes, alignment uint64) memcore.MarkRaw {
			return memforge.DynamicLinearAllocatorMallocUnsafe(allocator, sizeBytes, alignment)
		},
	)

	if err != nil {
		memforge.DynamicLinearAllocatorDestroy(allocator)
		return nil, fmt.Errorf("could not create helm parser: %w", err)
	}

	return &HelmInterpreter{
		allocator:       allocator,
		compiledSymbols: compiledSymbols,
		parser:          parser,
	}, nil
}

func HelmInterpreterCreate(helmSpecFile string) *HelmInterpreter {
	interpreter, err := HelmInterpreterTryCreate(helmSpecFile)
	if err != nil {
		panic(err)
	}
	return interpreter
}

func HelmInterpreterDestroy(interpreter *HelmInterpreter) {
	langspec.LangParserDestroy(interpreter.parser)
	memforge.DynamicLinearAllocatorDestroy(interpreter.allocator)
}

func HelmInterpreterDumpGrammar(
	interpreter *HelmInterpreter,
) {
	grammarPackage := interpreter.parser.GetGrammarPackage()

	grammarDump := grammarPackage.Root.DebugDump(syntaxa.GrammarDebugFormatter[lexarch.TokenKind, artifacts.Node]{
		FormatKind: syntaxa.GrammarKind.String,
		FormatToken: func(tk lexarch.TokenKind) string {
			return interpreter.compiledSymbols.TokenName(uint32(tk))
		},
		FormatOutputNodeKind: func(u artifacts.Node) string {
			return interpreter.compiledSymbols.NodeKindName(uint32(u))
		},
	})

	fmt.Fprint(os.Stdout, grammarDump)

	pkgDump := grammarPackage.DebugDump(syntaxa.GrammarPackageDebugFormatter[artifacts.Node]{
		FormatToken: func(tk lexarch.TokenKind) string {
			return interpreter.compiledSymbols.TokenName(uint32(tk))
		},
	}, func() *syntaxa.GrammarAnalysis {
		return lowering.GetAnalysis(grammarPackage)
	})

	fmt.Fprint(os.Stdout, pkgDump)
}

// HelmInterpretOptions configures workspace-aware interpretation of a single file.
type HelmInterpretOptions struct {
	InheritedGlobals map[string]ir.HelmGlobalVariable
}

func HelmInterpreterInterpretFile(
	interpreter *HelmInterpreter,
	file string,
	ctx *signal.SignalContext,
) HelmInterpreterInterpretationResult {
	return HelmInterpreterInterpretFileWithOptions(interpreter, file, ctx, HelmInterpretOptions{})
}

func HelmInterpreterInterpretFileWithOptions(
	interpreter *HelmInterpreter,
	file string,
	ctx *signal.SignalContext,
	opts HelmInterpretOptions,
) HelmInterpreterInterpretationResult {
	signal.SignalContextPushSpan(ctx, shared.MainSpanPhase)

	result := HelmInterpreterInterpretationResult{
		compiledSymbols: interpreter.compiledSymbols,
		filePath:        file,
	}

	sourceContent, err := system.FileReadAllRunes(file)
	if err != nil {
		result.Error = fmt.Errorf("file reading failed with error: %w", err)
		return result
	}
	sourceText := string(sourceContent)
	result.sourceText = sourceText

	session := langspec.LangParserSessionCreate[rune](file, nil)
	trace, rootNode, syntaxErrors, err := langspec.LangParserParseFile(interpreter.parser, session, nil)

	result.parseTrace = trace
	result.rootNode = rootNode

	if syntaxErrors.HasErrors() {
		signal.SignalContextPushSpan(ctx, shared.ParsingSpanPhase)
		defer signal.SignalContextPopSpan(ctx)

		for i, syntaxErr := range syntaxErrors.Errors {
			startL, startC, endL, endC := resolveLineSpan(syntaxErr, sourceText)

			loc := location.LocationCreate("file", "", file, "", "", map[string]any{
				"start_line":   startL,
				"start_column": startC,
				"end_line":     endL,
				"end_column":   endC,
			})

			typeStr := "syntax"
			if syntaxErr.ProducedByLexer {
				typeStr = "lexer"
			}

			signalID := fmt.Sprintf("ERR_PARSE_%03d", i)

			signal.SignalContextBuild(ctx, signalID, "ERROR").
				Location(&loc).
				Payload(shared.PhasePayloadKey, typeStr).
				Payload(shared.RulePayloadKey, syntaxErr.Rule).
				Payload(shared.MessagePayloadKey, syntaxErr.Message).
				Emit()
		}

		result.Error = fmt.Errorf("parsing failed with %d syntax errors", len(syntaxErrors.Errors))
		return result
	}

	if err != nil {
		result.Error = fmt.Errorf("error while parsing file: %w", err)
		return result
	}

	compiledIR := ir.IRFromSyntax(file, sourceText, rootNode, ctx, opts.InheritedGlobals)
	result.BuiltIR = compiledIR

	if !compiledIR.Succeeded {
		result.Error = fmt.Errorf("interpretation aborted due to semantic errors")
		return result
	}

	return result
}

func HelmInterpreterExecuteTarget(
	result HelmInterpreterInterpretationResult,
	ctx *signal.SignalContext,
	entryTarget string,
	invocations map[string]targetexecutor.TargetInvocation,
	opts targetexecutor.TargetExecutorOptions,
) error {
	if _, err := getTargetExecutionChain(result, ctx, entryTarget); err != nil {
		return err
	}

	signal.SignalContextPushSpan(ctx, shared.TargetExecutionSpanPhase)
	defer signal.SignalContextPopSpan(ctx)

	execOpts := opts
	execOpts.SignalContext = ctx
	if !execOpts.DisableArtifactCache && execOpts.CacheRoot == "" && result.filePath != "" {
		execOpts.CacheRoot = filepath.Join(filepath.Dir(result.filePath), ".helm", "cache")
	}

	entityOpts := entityexecutor.EntityExecutorOptions{
		CacheRoot: execOpts.CacheRoot,
		TargetHookRunner: func(targetName string) error {
			return targetexecutor.TargetExecutorRunGraph(
				result.BuiltIR,
				targetName,
				nil,
				execOpts,
			)
		},
	}
	if entityErr := entityexecutor.EntityExecutorEnsureForTarget(
		result.BuiltIR,
		entryTarget,
		entityOpts,
	); entityErr != nil {
		return entityErr
	}

	return targetexecutor.TargetExecutorRunGraph(result.BuiltIR, entryTarget, invocations, execOpts)
}

func HelmInterpreterDebugExecutionChainForTarget(result HelmInterpreterInterpretationResult, ctx *signal.SignalContext, target string) ([][]string, error) {
	return getTargetExecutionChain(result, ctx, target)
}

func HelmInterpreterDumpLexemes(interpreter *HelmInterpreter, file string) error {
	session := langspec.LangParserSessionCreate[rune](file, nil)

	lexemes, _ := langspec.LangParserLexFile(interpreter.parser, session)
	for i, lexeme := range lexemes {
		fmt.Fprintf(
			os.Stdout,
			"%03d) tok=%s role=%s span=[%d:%d] raw=%q\n",
			i,
			interpreter.compiledSymbols.TokenName(uint32(lexeme.Token)),
			interpreter.compiledSymbols.RoleName(uint32(lexeme.Role)),
			lexeme.Start,
			lexeme.End,
			string(lexeme.Raw),
		)
	}

	return nil
}

// ------------------------------------------------------------- PRIVATE HELPERS

func getTargetExecutionChain(result HelmInterpreterInterpretationResult, ctx *signal.SignalContext, target string) ([][]string, error) {
	signal.SignalContextPushSpan(ctx, shared.ExecutionChainResolution)
	defer signal.SignalContextPopSpan(ctx)

	chain, err := executionChainForTarget(result.BuiltIR, target)

	if err != nil {
		if errCasted, ok := err.(structarch.CycleError[string]); ok {
			loc := location.LocationCreate("file", "", result.filePath, "", "", map[string]any{})

			signal.SignalContextBuild(ctx, ERROR_CYCLIC_TARGET_CHAIN, "ERROR").
				Location(&loc).
				Payload(shared.PhasePayloadKey, shared.GraphResolutionPhase).
				Payload(shared.MessagePayloadKey, err.Error()).
				Emit()

			return nil, errCasted
		} else {
			loc := location.LocationCreate("file", "", result.filePath, "", "", map[string]any{})

			signal.SignalContextBuild(ctx, ERROR_GENERIC_GRAPH_ERROR, "ERROR").
				Location(&loc).
				Payload(shared.PhasePayloadKey, shared.GraphResolutionPhase).
				Payload(shared.MessagePayloadKey, err.Error()).
				Emit()

			return chain, err
		}
	}

	return chain, err
}

func resolveLineSpan(e syntaxa.SyntaxError, source string) (int, int, int, int) {
	if e.StartLine > 0 && e.EndLine > 0 {
		return e.StartLine, e.StartColumn, e.EndLine, e.EndColumn
	}

	if source == "" {
		return 0, 0, 0, 0
	}

	start := e.AbsolutePosition
	if start < 0 {
		start = 0
	}

	end := e.AbsoluteEnd
	if end < start {
		end = start
	}

	sl, sc, el, ec, ok := syntaxa.LineSpanFromByteOffsets(start, end, source, 4)
	if !ok {
		return 0, 0, 0, 0
	}
	return sl, sc, el, ec
}
