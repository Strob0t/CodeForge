package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	cfcontext "github.com/Strob0t/CodeForge/internal/domain/context"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
)

// ---------------------------------------------------------------------------
// LLM context re-ranking (Phase 3 — Context Intelligence)
// ---------------------------------------------------------------------------

// contextEntriesToRerankPayload converts domain entries to NATS rerank payloads.
func contextEntriesToRerankPayload(entries []cfcontext.ContextEntry) []messagequeue.ContextRerankEntryPayload {
	out := make([]messagequeue.ContextRerankEntryPayload, len(entries))
	for i, e := range entries {
		out[i] = messagequeue.ContextRerankEntryPayload{
			Path: e.Path, Kind: string(e.Kind), Content: e.Content,
			Priority: e.Priority, Tokens: e.Tokens,
		}
	}
	return out
}

// rerankPayloadToContextEntries converts NATS rerank payloads back to domain entries.
func rerankPayloadToContextEntries(payloads []messagequeue.ContextRerankEntryPayload) []cfcontext.ContextEntry {
	out := make([]cfcontext.ContextEntry, len(payloads))
	for i, p := range payloads {
		out[i] = cfcontext.ContextEntry{
			Kind: cfcontext.EntryKind(p.Kind), Path: p.Path, Content: p.Content,
			Priority: p.Priority, Tokens: p.Tokens,
		}
	}
	return out
}

// RerankSync sends context entries to the Python worker for LLM re-ranking
// and blocks until the result arrives or the timeout expires.
func (s *ContextOptimizerService) RerankSync(ctx context.Context, projectID, query string, entries []cfcontext.ContextEntry) ([]cfcontext.ContextEntry, error) {
	requestID := uuid.New().String()
	ch := s.rerankWaiter.register(requestID)
	defer s.rerankWaiter.unregister(requestID)

	payload := messagequeue.ContextRerankRequestPayload{
		RequestID: requestID,
		ProjectID: projectID,
		Query:     query,
		Entries:   contextEntriesToRerankPayload(entries),
		Model:     s.orchCfg.ContextRerankModel,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return entries, fmt.Errorf("marshal rerank request: %w", err)
	}
	if err := s.queue.Publish(ctx, messagequeue.SubjectContextRerankRequest, data); err != nil {
		return entries, fmt.Errorf("publish rerank request: %w", err)
	}

	tctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	select {
	case result := <-ch:
		if result.Error != "" {
			return entries, fmt.Errorf("rerank worker: %s", result.Error)
		}
		return rerankPayloadToContextEntries(result.Entries), nil
	case <-tctx.Done():
		return entries, tctx.Err()
	}
}

// HandleRerankResult delivers a rerank result to the waiting caller.
func (s *ContextOptimizerService) HandleRerankResult(_ context.Context, payload *messagequeue.ContextRerankResultPayload) {
	s.rerankWaiter.deliver(payload.RequestID, payload)
}

// StartSubscribers subscribes to NATS subjects for context optimizer results.
func (s *ContextOptimizerService) StartSubscribers(ctx context.Context) ([]func(), error) {
	if s.queue == nil {
		return nil, nil
	}
	cancelRerank, err := s.queue.Subscribe(ctx, messagequeue.SubjectContextRerankResult, func(msgCtx context.Context, _ string, data []byte) error {
		var payload messagequeue.ContextRerankResultPayload
		if err := json.Unmarshal(data, &payload); err != nil {
			return fmt.Errorf("unmarshal context rerank result: %w", err)
		}
		s.HandleRerankResult(msgCtx, &payload)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("subscribe context rerank result: %w", err)
	}
	return []func(){cancelRerank}, nil
}
