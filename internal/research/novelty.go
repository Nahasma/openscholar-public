package research

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	"golang.org/x/sync/errgroup"

	"github.com/Nahasma/openscholar-public/internal/fileop"
)

// NoveltyResult 新颖性检查结果
type NoveltyResult struct {
	Risk            string         `json:"risk"`             // "low" | "medium" | "high"
	Report          string         `json:"report"`
	SimilarPapers   []SimilarPaper `json:"similar_papers"`
	Differentiation string         `json:"differentiation"`
}

// SimilarPaper 相似论文条目
type SimilarPaper struct {
	Title           string   `json:"title"`
	Source          string   `json:"source"`
	SimilarityScore float64  `json:"similarity_score"`
	OverlapAreas    []string `json:"overlap_areas"`
	Differentiators []string `json:"differentiators"`
}

// SearchResult 文献搜索结果（通用接口）
type SearchResult struct {
	Title   string `json:"title"`
	Source  string `json:"source"`
	Abstract string `json:"abstract,omitempty"`
}

// RunNoveltyCheck 执行新颖性检查
func RunNoveltyCheck(
	ctx context.Context,
	designContent string,
	keywords []string,
	searchFunc func(ctx context.Context, query string) ([]SearchResult, error),
	evaluateFunc func(ctx context.Context, design string, paper SearchResult) (*SimilarPaper, error),
	threshold float64,
	blockThreshold float64,
) (*NoveltyResult, error) {
	if len(keywords) == 0 {
		return &NoveltyResult{Risk: "low", Report: "无关键词可搜索，跳过新颖性检查"}, nil
	}

	// 并行搜索
	var mu sync.Mutex
	var allResults []SearchResult

	g, gctx := errgroup.WithContext(ctx)
	for _, kw := range keywords {
		keyword := kw
		g.Go(func() error {
			results, err := searchFunc(gctx, keyword)
			if err != nil {
				return nil // 搜索失败不阻塞
			}
			mu.Lock()
			allResults = append(allResults, results...)
			mu.Unlock()
			return nil
		})
	}
	g.Wait()

	// 去重
	allResults = deduplicateResults(allResults)

	// LLM 评估语义相似度
	var similarPapers []SimilarPaper
	for _, r := range allResults {
		sp, err := evaluateFunc(ctx, designContent, r)
		if err != nil {
			continue
		}
		if sp.SimilarityScore >= threshold {
			similarPapers = append(similarPapers, *sp)
		}
	}

	// 决策
	result := &NoveltyResult{SimilarPapers: similarPapers}
	maxSim := 0.0
	for _, sp := range similarPapers {
		if sp.SimilarityScore > maxSim {
			maxSim = sp.SimilarityScore
		}
	}

	switch {
	case maxSim >= blockThreshold:
		result.Risk = "high"
	case maxSim >= threshold:
		result.Risk = "medium"
	default:
		result.Risk = "low"
	}

	result.Report = generateNoveltyReport(result)
	return result, nil
}

// WriteNoveltyReport 将报告写入 .handoff/02-novelty-check.md
func WriteNoveltyReport(workDir string, result *NoveltyResult) error {
	path := filepath.Join(workDir, ".handoff", "02-novelty-check.md")
	return fileop.SafeWrite(path, []byte(result.Report), fileop.WithMkdir())
}

func generateNoveltyReport(result *NoveltyResult) string {
	var sb strings.Builder
	sb.WriteString("# 新颖性检查报告\n\n")
	sb.WriteString(fmt.Sprintf("**风险等级**: %s\n\n", result.Risk))

	if len(result.SimilarPapers) == 0 {
		sb.WriteString("未发现高度相似的已有工作。\n")
		return sb.String()
	}

	sb.WriteString(fmt.Sprintf("## 发现 %d 篇相似论文\n\n", len(result.SimilarPapers)))
	for i, sp := range result.SimilarPapers {
		sb.WriteString(fmt.Sprintf("### %d. %s\n", i+1, sp.Title))
		sb.WriteString(fmt.Sprintf("- 来源: %s\n", sp.Source))
		sb.WriteString(fmt.Sprintf("- 相似度: %.2f\n", sp.SimilarityScore))
		if len(sp.OverlapAreas) > 0 {
			sb.WriteString("- 重叠领域: " + strings.Join(sp.OverlapAreas, ", ") + "\n")
		}
		if len(sp.Differentiators) > 0 {
			sb.WriteString("- 差异点: " + strings.Join(sp.Differentiators, ", ") + "\n")
		}
		sb.WriteString("\n")
	}

	if result.Differentiation != "" {
		sb.WriteString("## 差异化声明\n\n")
		sb.WriteString(result.Differentiation + "\n")
	}

	return sb.String()
}

func deduplicateResults(results []SearchResult) []SearchResult {
	seen := make(map[string]bool)
	var unique []SearchResult
	for _, r := range results {
		key := strings.ToLower(r.Title)
		if !seen[key] {
			seen[key] = true
			unique = append(unique, r)
		}
	}
	return unique
}
