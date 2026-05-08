package kb

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
)

// ParseDocument implements raw-first parsing without default semantic tree build.
func (p *pythonIndexer) ParseDocument(ctx context.Context, filePath string, opts ParseOptions) (*ParseResult, error) {
	ext := strings.ToLower(filepath.Ext(filePath))
	docType := opts.DocType
	if docType == "" {
		docType = strings.TrimPrefix(ext, ".")
	}

	switch ext {
	case ".pdf":
		result, err := p.buildPDFTreeSimple(ctx, filePath)
		if err != nil {
			return nil, err
		}
		return indexResultToParseResult(filePath, docType, result), nil
	case ".docx", ".dotx":
		result, err := buildDocTreeNative(filePath)
		if err != nil {
			if p.docparseScript == "" {
				return nil, err
			}
			result, err = p.buildDocTree(ctx, filePath)
			if err != nil {
				return nil, err
			}
		}
		return indexResultToParseResult(filePath, docType, result), nil
	case ".doc", ".pptx", ".xlsx":
		if p.docparseScript == "" {
			return nil, fmt.Errorf("document parsing unavailable for %s: scripts/run_docparse.py not found", ext)
		}
		result, err := p.buildDocTree(ctx, filePath)
		if err != nil {
			return nil, err
		}
		return indexResultToParseResult(filePath, docType, result), nil
	default:
		return nil, fmt.Errorf("unsupported document type: %s", ext)
	}
}

func indexResultToParseResult(filePath, docType string, result *IndexResult) *ParseResult {
	if result == nil {
		return &ParseResult{DocType: docType, PaperTitle: filepath.Base(filePath)}
	}
	chunks := make([]PaperChunk, 0, 64)
	appendTreeChunks(result.Tree.Structure, result.ModelUsed, &chunks)

	contentBytes := 0
	totalTokens := 0
	for _, c := range chunks {
		contentBytes += len(c.Content)
		totalTokens += c.TokenCount
	}

	return &ParseResult{
		PaperTitle:   result.Tree.DocName,
		DocType:      docType,
		Chunks:       chunks,
		TotalPages:   result.TotalPages,
		TotalTokens:  totalTokens,
		ContentBytes: contentBytes,
		Extractor:    result.Extractor,
	}
}

func appendTreeChunks(nodes []TreeNode, extractor string, chunks *[]PaperChunk) {
	for _, n := range nodes {
		content := strings.TrimSpace(n.Content)
		if content == "" {
			content = strings.TrimSpace(n.Summary)
		}
		chunk := PaperChunk{
			ChunkID:    n.NodeID,
			Kind:       chunkKindFromNode(n, extractor),
			PageStart:  n.StartIndex,
			PageEnd:    n.EndIndex,
			Title:      n.Title,
			Content:    content,
			TokenCount: estimateTokenCount(content),
			Source:     extractor,
		}
		*chunks = append(*chunks, chunk)
		if len(n.Nodes) > 0 {
			appendTreeChunks(n.Nodes, extractor, chunks)
		}
	}
}

func chunkKindFromNode(n TreeNode, extractor string) string {
	if strings.HasPrefix(strings.ToLower(n.NodeID), "page_") || extractor == "simple_extract" {
		return "page"
	}
	return "section"
}
