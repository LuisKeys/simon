package router

import (
	"reflect"
	"testing"
)

func TestTokenizeLowercasesAndSplits(t *testing.T) {
	q := Tokenize("Claiming Jobs Safely")
	want := []string{"claiming", "jobs", "safely"}
	if !reflect.DeepEqual(q.Tokens, want) {
		t.Fatalf("Tokens = %v, want %v", q.Tokens, want)
	}
}

func TestTokenizeRemovesStopWords(t *testing.T) {
	q := Tokenize("How do workers claim the jobs")
	for _, tok := range q.Tokens {
		if tok == "how" || tok == "do" || tok == "the" {
			t.Fatalf("stop word %q leaked into tokens: %v", tok, q.Tokens)
		}
	}
}

func TestTokenizePreservesTechnicalIdentifiers(t *testing.T) {
	cases := []string{"PostgreSQL", "pgvector", "simon-go", "internal/knowledge", "KnowledgeSearcher"}
	for _, c := range cases {
		q := Tokenize(c)
		if len(q.Tokens) != 1 {
			t.Fatalf("Tokenize(%q) = %v, want a single preserved token", c, q.Tokens)
		}
	}
}

func TestTokenizeSkipLockedAndForUpdatePreserved(t *testing.T) {
	q := Tokenize("SKIP LOCKED and FOR UPDATE")
	want := map[string]bool{"skip": true, "locked": true, "update": true}
	got := map[string]bool{}
	for _, tok := range q.Tokens {
		got[tok] = true
	}
	for w := range want {
		if !got[w] {
			t.Fatalf("expected token %q in %v", w, q.Tokens)
		}
	}
}

func TestTokenizeDeduplicatesPreservingOrder(t *testing.T) {
	q := Tokenize("jobs jobs workers jobs")
	want := []string{"jobs", "workers"}
	if !reflect.DeepEqual(q.Tokens, want) {
		t.Fatalf("Tokens = %v, want %v", q.Tokens, want)
	}
}

func TestTokenizeEmptyQuery(t *testing.T) {
	q := Tokenize("   ")
	if len(q.Tokens) != 0 {
		t.Fatalf("expected no tokens for blank query, got %v", q.Tokens)
	}
}

func TestTokenizeUnicode(t *testing.T) {
	q := Tokenize("Café Database Ünïcöde")
	if len(q.Tokens) != 3 {
		t.Fatalf("expected 3 tokens, got %v", q.Tokens)
	}
}

func TestTokenizePunctuationBecomesSeparator(t *testing.T) {
	q := Tokenize("jobs, workers; retries!")
	want := []string{"jobs", "workers", "retries"}
	if !reflect.DeepEqual(q.Tokens, want) {
		t.Fatalf("Tokens = %v, want %v", q.Tokens, want)
	}
}
