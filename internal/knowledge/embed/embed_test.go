package embed

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/openai/openai-go/v2/option"

	"simon-go/internal/config"
)

func vectorNorm(v []float32) float64 {
	var sum float64
	for _, x := range v {
		sum += float64(x) * float64(x)
	}
	return math.Sqrt(sum)
}

func TestOpenAIEmbedNormalizesVectors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"object":"list","model":"text-embedding-3-small","data":[
			{"object":"embedding","index":0,"embedding":[3,4,0]}
		],"usage":{"prompt_tokens":1,"total_tokens":1}}`))
	}))
	defer srv.Close()

	o := NewOpenAIWithOptions("text-embedding-3-small", option.WithBaseURL(srv.URL), option.WithAPIKey("k"))
	vec, err := o.Embed(context.Background(), "hello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := vectorNorm(vec); math.Abs(got-1.0) > 1e-6 {
		t.Errorf("expected unit-normalized vector, norm = %v", got)
	}
	if math.Abs(float64(vec[0])-0.6) > 1e-6 || math.Abs(float64(vec[1])-0.8) > 1e-6 {
		t.Errorf("vec = %v, want [0.6 0.8 0]", vec)
	}
}

func TestOpenAIEmbedBatchPreservesOrder(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"object":"list","model":"m","data":[
			{"object":"embedding","index":0,"embedding":[1,0]},
			{"object":"embedding","index":1,"embedding":[0,1]}
		],"usage":{"prompt_tokens":1,"total_tokens":1}}`))
	}))
	defer srv.Close()

	o := NewOpenAIWithOptions("m", option.WithBaseURL(srv.URL), option.WithAPIKey("k"))
	vecs, err := o.EmbedBatch(context.Background(), []string{"a", "b"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(vecs) != 2 || vecs[0][0] != 1 || vecs[1][1] != 1 {
		t.Errorf("vecs = %v", vecs)
	}
}

func TestVoyageEmbedBatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req voyageRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		if len(req.Input) != 2 || req.Model != "voyage-2" {
			t.Errorf("unexpected request: %+v", req)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":[{"embedding":[1,0],"index":0},{"embedding":[0,2],"index":1}]}`))
	}))
	defer srv.Close()

	v := NewVoyage("key", "voyage-2")
	v.BaseURL = srv.URL
	vecs, err := v.EmbedBatch(context.Background(), []string{"a", "b"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(vecs) != 2 {
		t.Fatalf("expected 2 vectors, got %d", len(vecs))
	}
	if math.Abs(vectorNorm(vecs[0])-1.0) > 1e-6 || math.Abs(vectorNorm(vecs[1])-1.0) > 1e-6 {
		t.Errorf("expected normalized vectors, got %v", vecs)
	}
}

func TestDefaultSelectsProviderFromSettings(t *testing.T) {
	cases := []struct {
		provider string
		wantType string
	}{
		{"OPENAI", "*embed.OpenAI"},
		{"OLLAMA", "*embed.Ollama"},
		{"ANTHROPIC", "*embed.Voyage"},
	}
	for _, c := range cases {
		e, err := Default(config.Settings{EmbeddingProvider: c.provider, OllamaHost: "http://localhost:11434"})
		if err != nil {
			t.Fatalf("provider %s: unexpected error: %v", c.provider, err)
		}
		if got := typeName(e); got != c.wantType {
			t.Errorf("provider %s: got %s, want %s", c.provider, got, c.wantType)
		}
	}
}

func TestDefaultRejectsUnknownProvider(t *testing.T) {
	if _, err := Default(config.Settings{EmbeddingProvider: "bogus"}); err == nil {
		t.Error("expected an error for an unknown embedding provider")
	}
}

func typeName(v any) string {
	switch v.(type) {
	case *OpenAI:
		return "*embed.OpenAI"
	case *Ollama:
		return "*embed.Ollama"
	case *Voyage:
		return "*embed.Voyage"
	default:
		return "unknown"
	}
}
