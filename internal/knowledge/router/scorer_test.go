package router

import (
	"math"
	"testing"
)

func TestScoreFieldsExactPhraseRanksHighest(t *testing.T) {
	q := Tokenize("worker queue")
	exact := scoreFields(q, Fields{Title: "Worker Queue Guide"})
	partial := scoreFields(q, Fields{Description: "mentions worker somewhere, queue elsewhere"})
	if exact.Value <= partial.Value {
		t.Fatalf("exact phrase score %v should exceed partial token score %v", exact.Value, partial.Value)
	}
}

func TestScoreFieldsTitleWeighting(t *testing.T) {
	q := Tokenize("retry")
	titleHit := scoreFields(q, Fields{Title: "Retry Guide"})
	generalHit := scoreFields(q, Fields{General: []string{"retry.md"}})
	if titleHit.Value <= generalHit.Value {
		t.Fatalf("title match %v should outweigh general match %v", titleHit.Value, generalHit.Value)
	}
}

func TestScoreFieldsKeywordWeighting(t *testing.T) {
	q := Tokenize("SKIP LOCKED")
	s := scoreFields(q, Fields{Keywords: []string{"SKIP LOCKED"}})
	if s.Value == 0 {
		t.Fatalf("expected nonzero score for keyword match")
	}
	foundReason := false
	for _, r := range s.Reasons {
		if r == `keyword matched "SKIP LOCKED"` {
			foundReason = true
		}
	}
	if !foundReason {
		t.Fatalf("expected keyword reason, got %v", s.Reasons)
	}
}

func TestScoreFieldsAliasWeighting(t *testing.T) {
	q := Tokenize("postgres queue")
	s := scoreFields(q, Fields{Aliases: []string{"postgres queue"}})
	if s.Value == 0 {
		t.Fatalf("expected nonzero score for alias match")
	}
}

func TestScoreFieldsSummaryWeighting(t *testing.T) {
	q := Tokenize("lease timeout")
	s := scoreFields(q, Fields{Summary: "Describes retries, lease, and timeout handling."})
	if s.Value == 0 {
		t.Fatalf("expected nonzero score for summary token match")
	}
}

func TestScoreFieldsStableOrderingForTies(t *testing.T) {
	q := Tokenize("database")
	a := scoreFields(q, Fields{Title: "Database Guide"})
	b := scoreFields(q, Fields{Title: "Database Guide"})
	if a.Value != b.Value {
		t.Fatalf("identical fields should score identically, got %v and %v", a.Value, b.Value)
	}
}

func TestScoreFieldsNoNaNOrInfinity(t *testing.T) {
	q := Tokenize("")
	s := scoreFields(q, Fields{Title: "Anything"})
	if math.IsNaN(s.Value) || math.IsInf(s.Value, 0) {
		t.Fatalf("score must be finite, got %v", s.Value)
	}

	q2 := Tokenize("database")
	s2 := scoreFields(q2, Fields{})
	if math.IsNaN(s2.Value) || math.IsInf(s2.Value, 0) {
		t.Fatalf("score must be finite for empty fields, got %v", s2.Value)
	}
}

func TestScoreFieldsHasReasons(t *testing.T) {
	q := Tokenize("worker retry")
	s := scoreFields(q, Fields{Title: "Worker Retry Guide"})
	if len(s.Reasons) == 0 {
		t.Fatalf("expected score reasons for a match")
	}
}
