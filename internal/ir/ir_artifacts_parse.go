package ir

import (
	"fmt"
	"lingua/helm/artifacts"
	"syntaxa"
)

func extractArtifactsFromBlock(
	builder *irBuilder,
	node *syntaxa.SyntaxaLSTNode[artifacts.Node],
	scope resolveScope,
) *HelmArtifacts {
	artifactsIR := &HelmArtifacts{}

	if inputsNode := node.FindFirstKind(artifacts.NodeCacheInputs); inputsNode != nil {
		artifactsIR.Inputs = extractInputArtifactSequence(builder, inputsNode, scope)
	}

	if outputsNode := node.FindFirstKind(artifacts.NodeCacheOutputDirectory); outputsNode != nil {
		artifactsIR.Outputs = extractOutputArtifactSequence(builder, outputsNode, scope)
	}

	if dynamicNode := node.FindFirstKind(artifacts.NodeDynamic); dynamicNode != nil {
		artifactsIR.Dynamic = extractDynamicArtifactSequence(builder, dynamicNode, scope)
	}

	artifactsIR.Volatile = extractArtifactsVolatile(builder, node)
	return artifactsIR
}

func parseAdapterPhaseArtifacts(
	builder *irBuilder,
	node *syntaxa.SyntaxaLSTNode[artifacts.Node],
	scope resolveScope,
	adapterName string,
	phaseName string,
	artifactsDeclared *bool,
) *HelmArtifacts {
	if *artifactsDeclared {
		emitSemanticError(
			builder,
			node,
			ERROR_DUPLICATE_ARTIFACTS,
			fmt.Sprintf("artifacts block for adapter '%s' phase '%s' already declared", adapterName, phaseName),
		)
		return nil
	}
	*artifactsDeclared = true
	return extractArtifactsFromBlock(builder, node, scope)
}

func parseLegacyAdapterArtifacts(
	builder *irBuilder,
	node *syntaxa.SyntaxaLSTNode[artifacts.Node],
	scope resolveScope,
	adapterName string,
	artifactsDeclared *bool,
) *HelmArtifacts {
	if *artifactsDeclared {
		emitSemanticError(
			builder,
			node,
			ERROR_DUPLICATE_ARTIFACTS,
			fmt.Sprintf("artifacts block for adapter '%s' already declared", adapterName),
		)
		return nil
	}
	*artifactsDeclared = true
	return extractArtifactsFromBlock(builder, node, scope)
}
