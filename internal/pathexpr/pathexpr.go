package pathexpr

import (
	"fmt"
	"helm/internal/expand"
	"helm/internal/ir"
	"helm/internal/workspacepath"
	"strings"
)

// PathExprScope supplies resolved path lists and scalars for path expression evaluation.
type PathExprScope struct {
	PathLists map[string][]string
	Scalars   map[string]string
}

func PathExprScopeFromInterpolation(ctx expand.InterpolationContext) PathExprScope {
	pathLists := map[string][]string{}
	for key, paths := range ctx.PathLists {
		pathLists[key] = append([]string(nil), paths...)
	}
	return PathExprScope{
		PathLists: pathLists,
		Scalars:   ctx.Scalars,
	}
}

func PathExprEvaluate(
	helmRoot string,
	expr ir.HelmPathExpr,
	scope PathExprScope,
) ([]string, error) {
	switch expr.Kind {
	case ir.PathExprLiteral:
		text := expand.InterpolationContextExpandLiteral(
			expand.InterpolationContext{Scalars: scope.Scalars},
			expr.Literal,
		)
		if text == "" {
			return nil, nil
		}
		if paths, ok := expand.PathsFromShellParameterList(text); ok {
			return pathExprNormalizePaths(helmRoot, paths)
		}
		return pathExprNormalizePaths(helmRoot, []string{text})
	case ir.PathExprParamRef, ir.PathExprGlobalRef, ir.PathExprLetRef:
		paths, ok := scope.PathLists[expr.Name]
		if !ok {
			return nil, fmt.Errorf("path reference '%s' is not bound", expr.Name)
		}
		return pathExprNormalizePaths(helmRoot, paths)
	case ir.PathExprCall:
		return pathExprEvaluateCall(helmRoot, expr, scope)
	default:
		return nil, fmt.Errorf("unknown path expression kind")
	}
}

func pathExprEvaluateCall(
	helmRoot string,
	expr ir.HelmPathExpr,
	scope PathExprScope,
) ([]string, error) {
	switch expr.CallName {
	case "map_ext":
		return pathExprMapExt(helmRoot, expr.CallArgs, scope)
	case "rebase_dir":
		return pathExprRebaseDir(helmRoot, expr.CallArgs, scope)
	case "join_prefix":
		return pathExprJoinPrefix(helmRoot, expr.CallArgs, scope)
	default:
		return nil, fmt.Errorf("unknown path function '%s'", expr.CallName)
	}
}

func pathExprMapExt(
	helmRoot string,
	args []ir.HelmPathExpr,
	scope PathExprScope,
) ([]string, error) {
	if len(args) != 3 {
		return nil, fmt.Errorf("map_ext requires 3 arguments")
	}

	paths, err := PathExprEvaluate(helmRoot, args[0], scope)
	if err != nil {
		return nil, err
	}
	oldExt, err := pathExprEvaluateScalarArg(args[1], scope)
	if err != nil {
		return nil, err
	}
	newExt, err := pathExprEvaluateScalarArg(args[2], scope)
	if err != nil {
		return nil, err
	}

	out := make([]string, 0, len(paths))
	for _, path := range paths {
		if strings.HasSuffix(path, oldExt) {
			path = path + newExt
		}
		out = append(out, workspacepath.WorkspaceNormalize(path))
	}
	return out, nil
}

func pathExprRebaseDir(
	helmRoot string,
	args []ir.HelmPathExpr,
	scope PathExprScope,
) ([]string, error) {
	if len(args) != 3 {
		return nil, fmt.Errorf("rebase_dir requires 3 arguments")
	}

	paths, err := PathExprEvaluate(helmRoot, args[0], scope)
	if err != nil {
		return nil, err
	}
	oldBase, err := pathExprEvaluateScalarArg(args[1], scope)
	if err != nil {
		return nil, err
	}
	newBase, err := pathExprEvaluateScalarArg(args[2], scope)
	if err != nil {
		return nil, err
	}

	oldPrefix := strings.TrimSuffix(workspacepath.WorkspaceNormalize(oldBase), "/") + "/"
	newPrefix := strings.TrimSuffix(workspacepath.WorkspaceNormalize(newBase), "/") + "/"

	out := make([]string, 0, len(paths))
	for _, path := range paths {
		normalized := workspacepath.WorkspaceNormalize(path)
		if strings.HasPrefix(normalized, oldPrefix) {
			normalized = newPrefix + strings.TrimPrefix(normalized, oldPrefix)
		}
		out = append(out, workspacepath.WorkspaceNormalize(normalized))
	}
	return out, nil
}

func pathExprJoinPrefix(
	helmRoot string,
	args []ir.HelmPathExpr,
	scope PathExprScope,
) ([]string, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("join_prefix requires 2 arguments")
	}

	paths, err := PathExprEvaluate(helmRoot, args[0], scope)
	if err != nil {
		return nil, err
	}
	prefix, err := pathExprEvaluateScalarArg(args[1], scope)
	if err != nil {
		return nil, err
	}
	prefix = strings.TrimSuffix(workspacepath.WorkspaceNormalize(prefix), "/")

	out := make([]string, 0, len(paths))
	for _, path := range paths {
		joined := prefix + "/" + workspacepath.WorkspaceNormalize(path)
		out = append(out, workspacepath.WorkspaceNormalize(joined))
	}
	return out, nil
}

func pathExprEvaluateScalarArg(expr ir.HelmPathExpr, scope PathExprScope) (string, error) {
	if expr.Kind != ir.PathExprLiteral {
		return "", fmt.Errorf("expected string literal argument")
	}
	text := expand.InterpolationContextExpandLiteral(
		expand.InterpolationContext{Scalars: scope.Scalars},
		expr.Literal,
	)
	if text == "" {
		return "", fmt.Errorf("path function argument resolved to empty string")
	}
	return text, nil
}

func pathExprNormalizePaths(helmRoot string, paths []string) ([]string, error) {
	out := make([]string, 0, len(paths))
	for _, path := range paths {
		rel, ok := workspacepath.WorkspaceRelative(helmRoot, path)
		if !ok {
			return nil, fmt.Errorf("path '%s' is outside workspace", path)
		}
		out = append(out, rel)
	}
	return out, nil
}
