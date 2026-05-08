package kb

import (
	"context"
	"encoding/binary"
	"fmt"
	"math"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/openai/openai-go/packages/param"
)

// EmbeddingProvider generates embeddings via OpenAI API.
type EmbeddingProvider struct {
	client *openai.Client
	model  string
	dims   int
}

// NewEmbeddingProvider creates an embedding provider using OpenAI API.
func NewEmbeddingProvider(apiKey string) *EmbeddingProvider {
	client := openai.NewClient(option.WithAPIKey(apiKey))
	return &EmbeddingProvider{
		client: &client,
		model:  "text-embedding-3-small",
		dims:   1536,
	}
}

// Embed generates an embedding vector for the given text.
func (e *EmbeddingProvider) Embed(ctx context.Context, text string) ([]float32, error) {
	resp, err := e.client.Embeddings.New(ctx, openai.EmbeddingNewParams{
		Input: openai.EmbeddingNewParamsInputUnion{
			OfString: param.NewOpt(text),
		},
		Model: openai.EmbeddingModel(e.model),
	})
	if err != nil {
		return nil, fmt.Errorf("embedding API: %w", err)
	}

	if len(resp.Data) == 0 {
		return nil, fmt.Errorf("embedding API returned no data")
	}

	vec := make([]float32, len(resp.Data[0].Embedding))
	for i, v := range resp.Data[0].Embedding {
		vec[i] = float32(v)
	}
	return vec, nil
}

// EmbedBatch generates embeddings for multiple texts in a single API call.
func (e *EmbeddingProvider) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}

	inputs := make([]string, len(texts))
	copy(inputs, texts)

	resp, err := e.client.Embeddings.New(ctx, openai.EmbeddingNewParams{
		Input: openai.EmbeddingNewParamsInputUnion{
			OfArrayOfStrings: inputs,
		},
		Model: openai.EmbeddingModel(e.model),
	})
	if err != nil {
		return nil, fmt.Errorf("embedding batch API: %w", err)
	}

	results := make([][]float32, len(resp.Data))
	for i, d := range resp.Data {
		vec := make([]float32, len(d.Embedding))
		for j, v := range d.Embedding {
			vec[j] = float32(v)
		}
		results[i] = vec
	}
	return results, nil
}

// EncodeEmbedding encodes a float32 vector to binary (little-endian).
func EncodeEmbedding(v []float32) []byte {
	buf := make([]byte, len(v)*4)
	for i, f := range v {
		binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(f))
	}
	return buf
}

// DecodeEmbedding decodes a binary blob to a float32 vector.
func DecodeEmbedding(b []byte) []float32 {
	v := make([]float32, len(b)/4)
	for i := range v {
		v[i] = math.Float32frombits(binary.LittleEndian.Uint32(b[i*4:]))
	}
	return v
}

// CosineSimilarity computes cosine similarity between two float32 vectors.
func CosineSimilarity(a, b []float32) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot, normA, normB float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}
