package research

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultReviewerPerspectives(t *testing.T) {
	perspectives := DefaultReviewerPerspectives()
	assert.Len(t, perspectives, 3)
	assert.Equal(t, "completeness", perspectives[0].Name)
	assert.Equal(t, "citations", perspectives[1].Name)
	assert.Equal(t, "methodology", perspectives[2].Name)
}

func TestExtendedReviewerPerspectives(t *testing.T) {
	perspectives := ExtendedReviewerPerspectives()
	assert.Len(t, perspectives, 5)
	assert.Equal(t, "writing", perspectives[3].Name)
	assert.Equal(t, "novelty", perspectives[4].Name)
}

func TestParseReviewJSON_Direct(t *testing.T) {
	input := `{"soundness": 8, "completeness": 7, "citations": 6, "writing": 7, "overall": 7, "strengths": ["good"], "weaknesses": ["needs work"], "decision": "accept"}`
	review, err := ParseReviewJSON(input)
	require.NoError(t, err)
	assert.Equal(t, 8.0, review.Soundness)
	assert.Equal(t, 7.0, review.Overall)
	assert.Equal(t, "accept", review.Decision)
	assert.Len(t, review.Strengths, 1)
}

func TestParseReviewJSON_Embedded(t *testing.T) {
	input := `Here is my review:\n{"soundness": 6, "completeness": 5, "citations": 4, "writing": 5, "overall": 5, "strengths": [], "weaknesses": ["weak"], "decision": "reject"}\nThat's my review.`
	review, err := ParseReviewJSON(input)
	require.NoError(t, err)
	assert.Equal(t, 5.0, review.Overall)
	assert.Equal(t, "reject", review.Decision)
}

func TestParseReviewJSON_Invalid(t *testing.T) {
	_, err := ParseReviewJSON("no json here")
	assert.Error(t, err)
}

func TestAggregateReviews_Accept(t *testing.T) {
	reviews := []*SingleReview{
		{Overall: 8, Decision: "accept"},
		{Overall: 7, Decision: "accept"},
		{Overall: 7.5, Decision: "accept"},
	}
	result := AggregateReviews(reviews)
	assert.Equal(t, "accept", result.Decision)
	assert.InDelta(t, 7.5, result.Overall, 0.01)
	assert.Greater(t, result.Consensus, 0.8)
}

func TestAggregateReviews_Reject(t *testing.T) {
	reviews := []*SingleReview{
		{Overall: 5, Decision: "reject"},
		{Overall: 6, Decision: "accept"},
		{Overall: 5.5, Decision: "reject"},
	}
	result := AggregateReviews(reviews)
	assert.Equal(t, "reject", result.Decision)
	assert.Less(t, result.Overall, 7.0)
}

func TestAggregateReviews_Escalate(t *testing.T) {
	reviews := []*SingleReview{
		{Overall: 9, Decision: "accept"},
		{Overall: 3, Decision: "reject"},
		{Overall: 8, Decision: "accept"},
	}
	result := AggregateReviews(reviews)
	// std is high → consensus < 0.5 → escalate
	assert.Equal(t, "escalate", result.Decision)
}

func TestAggregateReviews_Empty(t *testing.T) {
	result := AggregateReviews(nil)
	assert.Equal(t, "reject", result.Decision)
}

func TestCalculateConsensus(t *testing.T) {
	t.Run("perfect agreement", func(t *testing.T) {
		reviews := []*SingleReview{{Overall: 7}, {Overall: 7}, {Overall: 7}}
		c := CalculateConsensus(reviews)
		assert.Equal(t, 1.0, c)
	})

	t.Run("single reviewer", func(t *testing.T) {
		reviews := []*SingleReview{{Overall: 5}}
		c := CalculateConsensus(reviews)
		assert.Equal(t, 1.0, c)
	})

	t.Run("moderate disagreement", func(t *testing.T) {
		reviews := []*SingleReview{{Overall: 6}, {Overall: 8}}
		c := CalculateConsensus(reviews)
		assert.Greater(t, c, 0.5)
		assert.Less(t, c, 1.0)
	})
}

func TestRunEnsembleReview(t *testing.T) {
	mockAgent := func(ctx context.Context, prompt string) (string, error) {
		review := SingleReview{
			Soundness: 7, Completeness: 8, Citations: 6, Writing: 7,
			Overall: 7, Strengths: []string{"good"}, Weaknesses: []string{"ok"},
			Decision: "accept",
		}
		data, _ := json.Marshal(review)
		return string(data), nil
	}

	perspectives := DefaultReviewerPerspectives()
	result, err := RunEnsembleReview(context.Background(), "test handoff content", perspectives, mockAgent)
	require.NoError(t, err)
	assert.Len(t, result.Reviews, 3)
	assert.Equal(t, "accept", result.Decision)
	assert.InDelta(t, 7.0, result.Overall, 0.01)
}

func TestRunEnsembleReview_NoPerspectives(t *testing.T) {
	_, err := RunEnsembleReview(context.Background(), "content", nil, nil)
	assert.Error(t, err)
}

func TestFormatReviewsForPrompt(t *testing.T) {
	reviews := []*SingleReview{
		{Perspective: "test", Overall: 7, Decision: "accept", Strengths: []string{"good"}},
	}
	output := FormatReviewsForPrompt(reviews)
	assert.Contains(t, output, "test")
	assert.Contains(t, output, "7.0")
	assert.Contains(t, output, "accept")
}
