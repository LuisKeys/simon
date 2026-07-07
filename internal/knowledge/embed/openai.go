package embed

import (
	"context"

	openaisdk "github.com/openai/openai-go/v2"
	"github.com/openai/openai-go/v2/option"
)

// OpenAI embeds text via the OpenAI embeddings API.
type OpenAI struct {
	Client openaisdk.Client
	Model  string
}

// NewOpenAI builds an OpenAI-backed Embedder. An empty model defaults to
// "text-embedding-3-small", matching Settings.EmbeddingModel's default.
func NewOpenAI(apiKey, model string) *OpenAI {
	return NewOpenAIWithOptions(model, option.WithAPIKey(apiKey))
}

// NewOpenAIWithOptions builds an OpenAI Embedder from raw SDK request
// options, letting callers (notably tests) override the base URL.
func NewOpenAIWithOptions(model string, opts ...option.RequestOption) *OpenAI {
	if model == "" {
		model = "text-embedding-3-small"
	}
	return &OpenAI{Client: openaisdk.NewClient(opts...), Model: model}
}

func (o *OpenAI) Embed(ctx context.Context, text string) ([]float32, error) {
	vectors, err := o.EmbedBatch(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	return vectors[0], nil
}

func (o *OpenAI) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	resp, err := o.Client.Embeddings.New(ctx, openaisdk.EmbeddingNewParams{
		Model: openaisdk.EmbeddingModel(o.Model),
		Input: openaisdk.EmbeddingNewParamsInputUnion{OfArrayOfStrings: texts},
	})
	if err != nil {
		return nil, err
	}
	out := make([][]float32, len(resp.Data))
	for i, item := range resp.Data {
		out[i] = normalize(toFloat32(item.Embedding))
	}
	return out, nil
}

func toFloat32(vec []float64) []float32 {
	out := make([]float32, len(vec))
	for i, v := range vec {
		out[i] = float32(v)
	}
	return out
}
