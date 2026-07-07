// Package ollama adapts the official Ollama Go client to Simon's
// model.Model interface, mirroring Python's simon/models/ollama.py.
// Like the Python provider, tool calls are not supported: tools passed to
// Complete are ignored, matching Python's `_ = tools`.
package ollama

import (
	"context"
	"net/http"
	"net/url"

	ollamasdk "github.com/ollama/ollama/api"

	"simon-go/internal/agent/response"
	"simon-go/internal/model"
)

// Model calls a local (or remote) Ollama server's /api/chat endpoint.
type Model struct {
	Client   *ollamasdk.Client
	ModelStr string
	Think    bool
}

// New builds an Ollama-backed Model against the given host (e.g.
// "http://localhost:11434"). An empty host defaults to
// ollamasdk.ClientFromEnvironment's OLLAMA_HOST-based resolution.
func New(host, modelName string) (*Model, error) {
	if modelName == "" {
		modelName = "llama3.1"
	}
	client, err := clientFor(host)
	if err != nil {
		return nil, err
	}
	return &Model{Client: client, ModelStr: modelName}, nil
}

func clientFor(host string) (*ollamasdk.Client, error) {
	if host == "" {
		return ollamasdk.ClientFromEnvironment()
	}
	base, err := url.Parse(host)
	if err != nil {
		return nil, err
	}
	return ollamasdk.NewClient(base, http.DefaultClient), nil
}

// Complete implements model.Model. tools is intentionally ignored (see
// package doc): the Python provider never wired tool support through to the
// Ollama client either.
func (m *Model) Complete(ctx context.Context, messages []model.Message, _ []model.ToolSpec) (response.AgentResponse, error) {
	stream := false
	req := &ollamasdk.ChatRequest{
		Model:    m.ModelStr,
		Messages: toOllamaMessages(messages),
		Stream:   &stream,
		Think:    &ollamasdk.ThinkValue{Value: m.Think},
	}

	var text string
	err := m.Client.Chat(ctx, req, func(resp ollamasdk.ChatResponse) error {
		text = resp.Message.Content
		return nil
	})
	if err != nil {
		return response.AgentResponse{}, err
	}
	return response.AgentResponse{Text: text}, nil
}

func toOllamaMessages(messages []model.Message) []ollamasdk.Message {
	out := make([]ollamasdk.Message, 0, len(messages))
	for _, msg := range messages {
		out = append(out, ollamasdk.Message{Role: string(msg.Role), Content: msg.Content})
	}
	return out
}
