package embed

import (
	"context"
	"net/http"
	"net/url"

	ollamasdk "github.com/ollama/ollama/api"
)

// Ollama embeds text via a local Ollama server's /api/embed endpoint.
type Ollama struct {
	Client *ollamasdk.Client
	Model  string
}

// NewOllama builds an Ollama-backed Embedder against host.
func NewOllama(host, model string) *Ollama {
	if model == "" {
		model = "text-embedding-3-small"
	}
	client, err := ollamaClientFor(host)
	if err != nil {
		// A malformed host string is a configuration error surfaced at
		// call time (Embed/EmbedBatch), consistent with Python's lazy
		// _get_client() raising on first use rather than construction.
		client = nil
	}
	return &Ollama{Client: client, Model: model}
}

func ollamaClientFor(host string) (*ollamasdk.Client, error) {
	if host == "" {
		return ollamasdk.ClientFromEnvironment()
	}
	base, err := url.Parse(host)
	if err != nil {
		return nil, err
	}
	return ollamasdk.NewClient(base, http.DefaultClient), nil
}

func (o *Ollama) Embed(ctx context.Context, text string) ([]float32, error) {
	vectors, err := o.EmbedBatch(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	return vectors[0], nil
}

func (o *Ollama) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	resp, err := o.Client.Embed(ctx, &ollamasdk.EmbedRequest{Model: o.Model, Input: texts})
	if err != nil {
		return nil, err
	}
	out := make([][]float32, len(resp.Embeddings))
	for i, v := range resp.Embeddings {
		out[i] = normalize(v)
	}
	return out, nil
}
