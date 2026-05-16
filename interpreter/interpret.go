package interpreter

import (
	"fmt"
	"foundation/system"
	"langspec"
	"langspec/bootstrap"
	"langspec/dsl"
	"langspec/dsl/semantics"
	"lexarch"
	"lingua/helm/artifacts"
	"memcore"
	"memforge"
	"os"
	"syntaxa"
	"syntaxa/lowering"
)

// ------------------------------------------------------------------ RESULT

type HelmInterpreterInterpretationResult struct {
	parseTrace *syntaxa.ParseTrace
	rootNode   *syntaxa.SyntaxaLSTNode[artifacts.Node]

	compiledSymbols *semantics.CompiledSymbolTable

	Error error
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

func HelmInterpreterInterpretFile(interpreter *HelmInterpreter, file string) HelmInterpreterInterpretationResult {
	result := HelmInterpreterInterpretationResult{
		compiledSymbols: interpreter.compiledSymbols,
	}

	if !system.PathHasExt(file, ".helm") {
		result.Error = fmt.Errorf("file '%s' is not a .helm file", file)
		return result
	}

	sourceContent, err := system.FileReadAllRunes(file)
	if err != nil {
		result.Error = fmt.Errorf("file reading failed with error: %w", err)
		return result
	}

	session := langspec.LangParserSessionCreate[rune](file, nil)
	trace, rootNode, syntaxErrors, err := langspec.LangParserParseFile(interpreter.parser, session, nil)

	result.parseTrace = trace
	result.rootNode = rootNode

	if syntaxErrors.HasErrors() {
		dsl.RenderSyntaxErrorsWithContext(os.Stdout, sourceContent, syntaxErrors, func(r rune, col int) int {
			if r == '\t' {
				return col + 4
			}
			return col + 1
		})

		result.Error = fmt.Errorf("parsing failed with %d syntax errors", len(syntaxErrors.Errors))
		return result
	}

	if err != nil {
		result.Error = fmt.Errorf("error while parsing file: %w", err)
		return result
	}

	return result
}

func HelmInterpreterDumpLexemes(interpreter *HelmInterpreter, file string) error {
	if !system.PathHasExt(file, ".helm") {
		return fmt.Errorf("file '%s' is not a .helm file", file)
	}

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
