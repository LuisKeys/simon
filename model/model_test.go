package model_test

import (
	"context"
	"testing"

	internalmodel "github.com/LuisKeys/simon/internal/model"
	"github.com/LuisKeys/simon/model"
)

func TestEchoModelRepliesWithLastUserMessage(t *testing.T) {
	resp, err := model.EchoModel{}.Complete(context.Background(), model.Request{
		Messages: []model.Message{
			{Role: model.RoleSystem, Content: "system"},
			{Role: model.RoleUser, Content: "hello"},
		},
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	want := "Simon (echo): hello"
	if resp.Text != want {
		t.Errorf("Text = %q, want %q", resp.Text, want)
	}
}

type recordingModel struct {
	gotMessages []model.Message
}

func (m *recordingModel) Complete(_ context.Context, req model.Request) (model.Response, error) {
	m.gotMessages = req.Messages
	return model.Response{Text: "ok", Usage: model.Usage{InputTokens: 1, OutputTokens: 2, TotalTokens: 3}}, nil
}

func TestToInternalTranslatesRequestAndResponse(t *testing.T) {
	rm := &recordingModel{}
	internal := model.ToInternal(rm)

	resp, err := internal.Complete(context.Background(),
		[]internalmodel.Message{{Role: internalmodel.RoleUser, Content: "hi"}},
		nil,
	)
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if resp.Text != "ok" {
		t.Errorf("Text = %q, want %q", resp.Text, "ok")
	}
	if resp.Usage == nil || resp.Usage.TotalTokens != 3 {
		t.Errorf("Usage = %+v, want TotalTokens=3", resp.Usage)
	}
	if len(rm.gotMessages) != 1 || rm.gotMessages[0].Content != "hi" {
		t.Errorf("gotMessages = %+v, want one message with content %q", rm.gotMessages, "hi")
	}
}
