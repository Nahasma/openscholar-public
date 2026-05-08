package kb

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// AgenticRAGConfig controls the self-assessment retry loop.
type AgenticRAGConfig struct {
	MaxRounds     int     // maximum retry rounds (default 3)
	MinConfidence float64 // minimum confidence threshold (default 0.7)
}

// DefaultAgenticRAGConfig returns sensible defaults.
func DefaultAgenticRAGConfig() AgenticRAGConfig {
	return AgenticRAGConfig{MaxRounds: 3, MinConfidence: 0.7}
}

// AgenticRAG wraps CrossPaperSearch with a self-assessment + retry loop.
func (s *service) AgenticRAG(ctx context.Context, callLLM LLMCaller, question string, cfg AgenticRAGConfig) (*CrossSearchResult, error) {
	if cfg.MaxRounds <= 0 {
		cfg.MaxRounds = 3
	}
	if cfg.MinConfidence <= 0 {
		cfg.MinConfidence = 0.7
	}

	currentQuery := question
	var bestResult *CrossSearchResult

	for round := 0; round < cfg.MaxRounds; round++ {
		result, err := s.CrossPaperSearch(ctx, callLLM, currentQuery, 5)
		if err != nil {
			if bestResult != nil {
				return bestResult, nil // return previous best
			}
			return nil, fmt.Errorf("search round %d: %w", round+1, err)
		}
		bestResult = result

		// Self-assessment
		assessment, err := s.assessAnswer(ctx, callLLM, question, result.Answer)
		if err != nil || assessment.Confidence >= cfg.MinConfidence {
			break // good enough or assessment failed
		}

		// Refine query based on missing information
		if assessment.RefinedQuery != "" && assessment.RefinedQuery != currentQuery {
			currentQuery = assessment.RefinedQuery
		} else {
			break // no refinement possible
		}
	}

	return bestResult, nil
}

type assessmentResult struct {
	Confidence   float64 `json:"confidence"`
	Missing      string  `json:"missing"`
	RefinedQuery string  `json:"refined_query"`
}

func (s *service) assessAnswer(ctx context.Context, callLLM LLMCaller, question, answer string) (*assessmentResult, error) {
	prompt := fmt.Sprintf(assessmentPrompt, question, answer)

	response, err := callLLM(ctx, prompt)
	if err != nil {
		return &assessmentResult{Confidence: 1.0}, nil // assume good if assessment fails
	}

	var result assessmentResult
	if err := json.Unmarshal([]byte(response), &result); err != nil {
		// Try extracting JSON
		start := strings.Index(response, "{")
		end := strings.LastIndex(response, "}")
		if start >= 0 && end > start {
			_ = json.Unmarshal([]byte(response[start:end+1]), &result)
		}
	}

	return &result, nil
}

const assessmentPrompt = `Assess whether this answer adequately addresses the research question.

**Question:** %s

**Answer:** %s

Return a JSON object with:
- "confidence": float 0.0-1.0 (how well the answer addresses the question)
- "missing": string (what information is missing, empty if nothing)
- "refined_query": string (a better search query to find missing information, empty if not needed)

JSON:`
