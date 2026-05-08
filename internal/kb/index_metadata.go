package kb

import (
	"encoding/json"
	"strings"
)

// IndexMetadataFromResult converts an IndexResult into the stable metadata
// surfaced by KBAdd/KBQuery.
func IndexMetadataFromResult(result IndexResult) IndexMetadata {
	meta := IndexMetadata{
		IndexLevel:          result.IndexLevel,
		Extractor:           result.Extractor,
		FallbackReason:      result.FallbackReason,
		ContentAvailable:    result.ContentAvailable,
		TreeAvailable:       result.TreeAvailable,
		SummaryOnly:         result.SummaryOnly,
		MissingCapabilities: append([]string(nil), result.MissingCapabilities...),
		Health:              result.Health,
		ModelUsed:           result.ModelUsed,
		TotalPages:          result.TotalPages,
		TotalTokens:         result.TotalTokens,
		ContentBytes:        result.ContentBytes,
	}
	if meta.Extractor == "" {
		meta.Extractor = extractorFromModel(result.ModelUsed)
	}
	if meta.ContentBytes == 0 {
		meta.ContentBytes = contentBytesInTree(result.Tree)
	}
	if !meta.ContentAvailable {
		meta.ContentAvailable = meta.ContentBytes > 0
	}
	if meta.IndexLevel == "" {
		meta.IndexLevel = indexLevelFor(result.ModelUsed, meta.ContentAvailable)
	}
	if !meta.TreeAvailable {
		meta.TreeAvailable = meta.IndexLevel == IndexLevelFullTree
	}
	if !meta.SummaryOnly {
		meta.SummaryOnly = meta.IndexLevel == IndexLevelSummaryOnly
	}
	if len(meta.MissingCapabilities) == 0 {
		meta.MissingCapabilities = missingCapabilities(meta.IndexLevel, meta.ContentAvailable)
	}
	return meta
}

// IndexMetadataFromTree derives best-effort metadata when only a tree is available.
func IndexMetadataFromTree(tree PaperTree) IndexMetadata {
	contentBytes := contentBytesInTree(tree)
	meta := IndexMetadata{
		IndexLevel:       IndexLevelUnknown,
		ContentAvailable: contentBytes > 0,
		TreeAvailable:    len(tree.Structure) > 0 && !isFlatPageLegacyTree(tree, IndexMetadata{}),
		ContentBytes:     contentBytes,
	}
	if meta.ContentAvailable {
		meta.IndexLevel = IndexLevelSimpleFullText
	} else {
		meta.IndexLevel = IndexLevelSummaryOnly
		meta.SummaryOnly = true
	}
	meta.MissingCapabilities = missingCapabilities(meta.IndexLevel, meta.ContentAvailable)
	return meta
}

func IndexMetadataFromModelUsed(modelUsed string, totalPages, totalTokens int, contentAvailable bool) IndexMetadata {
	meta := IndexMetadata{
		IndexLevel:       indexLevelFor(modelUsed, contentAvailable),
		Extractor:        extractorFromModel(modelUsed),
		ContentAvailable: contentAvailable,
		ModelUsed:        modelUsed,
		TotalPages:       totalPages,
		TotalTokens:      totalTokens,
	}
	meta.TreeAvailable = meta.IndexLevel == IndexLevelFullTree
	meta.SummaryOnly = meta.IndexLevel == IndexLevelSummaryOnly
	meta.MissingCapabilities = missingCapabilities(meta.IndexLevel, meta.ContentAvailable)
	return meta
}

func indexLevelFor(modelUsed string, contentAvailable bool) string {
	switch strings.ToLower(strings.TrimSpace(modelUsed)) {
	case "simple_extract":
		if contentAvailable {
			return IndexLevelSimpleFullText
		}
		return IndexLevelSummaryOnly
	case "":
		if contentAvailable {
			return IndexLevelSimpleFullText
		}
		return IndexLevelSummaryOnly
	default:
		return IndexLevelFullTree
	}
}

func extractorFromModel(modelUsed string) string {
	switch strings.ToLower(strings.TrimSpace(modelUsed)) {
	case "simple_extract":
		return "simple_extract"
	case "go-native-docx":
		return "go-native-docx"
	case "":
		return "legacy"
	default:
		return modelUsed
	}
}

func missingCapabilities(indexLevel string, contentAvailable bool) []string {
	switch indexLevel {
	case IndexLevelFullTree:
		if contentAvailable {
			return nil
		}
		return []string{"full_content"}
	case IndexLevelSimpleFullText:
		if contentAvailable {
			return []string{"pageindex_tree"}
		}
		return []string{"full_content", "pageindex_tree"}
	case IndexLevelSummaryOnly:
		return []string{"full_content", "pageindex_tree"}
	default:
		if contentAvailable {
			return []string{"index_level"}
		}
		return []string{"full_content", "pageindex_tree", "index_level"}
	}
}

// StripTreeContent returns a copy of tree with large raw content removed.
func StripTreeContent(tree PaperTree) PaperTree {
	tree.Structure = stripNodeContent(tree.Structure)
	return tree
}

func stripNodeContent(nodes []TreeNode) []TreeNode {
	if len(nodes) == 0 {
		return nil
	}
	out := make([]TreeNode, len(nodes))
	for i, node := range nodes {
		node.Content = ""
		node.Nodes = stripNodeContent(node.Nodes)
		out[i] = node
	}
	return out
}

func contentBytesInTree(tree PaperTree) int {
	var total int
	var walk func([]TreeNode)
	walk = func(nodes []TreeNode) {
		for _, node := range nodes {
			total += len(node.Content)
			walk(node.Nodes)
		}
	}
	walk(tree.Structure)
	return total
}

func estimateTokenCount(content string) int {
	if content == "" {
		return 0
	}
	tokens := len([]rune(content)) / 4
	if tokens < 1 {
		return 1
	}
	return tokens
}

func marshalIndexMetadata(meta IndexMetadata) string {
	data, err := json.Marshal(meta)
	if err != nil {
		return "{}"
	}
	return string(data)
}
