package service

import (
	"testing"

	cfcontext "github.com/Strob0t/CodeForge/internal/domain/context"
	mq "github.com/Strob0t/CodeForge/internal/port/messagequeue"
)

func TestRerankEntriesToPayload(t *testing.T) {
	entries := []cfcontext.ContextEntry{
		{Kind: cfcontext.EntryFile, Path: "a.go", Content: "func A()", Tokens: 10, Priority: 80},
		{Kind: cfcontext.EntrySnippet, Path: "b.go", Content: "func B()", Tokens: 20, Priority: 70},
	}

	payloads := contextEntriesToRerankPayload(entries)

	if len(payloads) != 2 {
		t.Fatalf("expected 2 payloads, got %d", len(payloads))
	}
	if payloads[0].Path != "a.go" || payloads[0].Kind != string(cfcontext.EntryFile) {
		t.Errorf("unexpected first payload: %+v", payloads[0])
	}
	if payloads[1].Priority != 70 {
		t.Errorf("expected priority 70, got %d", payloads[1].Priority)
	}
}

func TestRerankPayloadToEntries(t *testing.T) {
	payloads := []mq.ContextRerankEntryPayload{
		{Path: "c.go", Kind: "file", Content: "func C()", Tokens: 15, Priority: 85},
		{Path: "d.go", Kind: "snippet", Content: "func D()", Tokens: 25, Priority: 72},
	}

	entries := rerankPayloadToContextEntries(payloads)

	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0].Path != "c.go" || entries[0].Priority != 85 {
		t.Errorf("unexpected first entry: %+v", entries[0])
	}
}
