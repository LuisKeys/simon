package embed

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"simon-go/pkg/simonerr"
)

// Voyage embeds text via Voyage AI's REST API — Anthropic's recommended
// embeddings provider (there is no official Go SDK, unlike Python's
// `voyageai` package, so this is a minimal hand-rolled HTTP client).
type Voyage struct {
	APIKey  string
	Model   string
	BaseURL string
	Client  *http.Client
}

// NewVoyage builds a Voyage-backed Embedder. An empty model defaults to
// "voyage-2".
func NewVoyage(apiKey, model string) *Voyage {
	if model == "" {
		model = "voyage-2"
	}
	return &Voyage{
		APIKey:  apiKey,
		Model:   model,
		BaseURL: "https://api.voyageai.com/v1",
		Client:  http.DefaultClient,
	}
}

type voyageRequest struct {
	Input []string `json:"input"`
	Model string   `json:"model"`
}

type voyageResponse struct {
	Data []struct {
		Embedding []float32 `json:"embedding"`
		Index     int       `json:"index"`
	} `json:"data"`
}

func (v *Voyage) Embed(ctx context.Context, text string) ([]float32, error) {
	vectors, err := v.EmbedBatch(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	return vectors[0], nil
}

func (v *Voyage) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	body, err := json.Marshal(voyageRequest{Input: texts, Model: v.Model})
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, v.BaseURL+"/embeddings", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+v.APIKey)

	resp, err := v.Client.Do(req)
	if err != nil {
		return nil, simonerr.NewProviderError("voyage embeddings request failed", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, simonerr.NewProviderError(fmt.Sprintf("voyage embeddings: status %d: %s", resp.StatusCode, string(data)), nil)
	}

	var parsed voyageResponse
	if err := json.Unmarshal(data, &parsed); err != nil {
		return nil, err
	}

	out := make([][]float32, len(parsed.Data))
	for _, item := range parsed.Data {
		out[item.Index] = normalize(item.Embedding)
	}
	return out, nil
}
