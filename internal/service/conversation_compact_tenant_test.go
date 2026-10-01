package service_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain/conversation"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
	"github.com/Strob0t/CodeForge/internal/service"
	"github.com/Strob0t/CodeForge/internal/tenantctx"
)

const compactTenant = "00000000-0000-0000-0000-0000000c0a7c"

// compactStore holds one conversation of compactTenant.
type compactStore struct {
	runtimeMockStore
}

func (s *compactStore) GetConversation(_ context.Context, id string) (*conversation.Conversation, error) {
	return &conversation.Conversation{ID: id, TenantID: compactTenant, ProjectID: "proj-1"}, nil
}

// S6-G review, item 9: CompactConversation took the tenant of
// conversation.compact.request from tenantctx.FromContext, which silently
// falls back to the default tenant. It now names the conversation's tenant.
func TestCompactConversation_NamesTheConversationsTenant(t *testing.T) {
	for name, ctx := range map[string]context.Context{
		"request tenant": tenantctx.WithTenant(context.Background(), compactTenant),
		"no tenant":      context.Background(),
	} {
		t.Run(name, func(t *testing.T) {
			queue := &runtimeMockQueue{}
			svc := service.NewConversationMessageService(&compactStore{}, queue, &runtimeMockBroadcaster{})
			if err := svc.CompactConversation(ctx, "conv-1"); err != nil {
				t.Fatalf("CompactConversation: %v", err)
			}
			msg, ok := queue.lastMessage(messagequeue.SubjectConversationCompactRequest)
			if !ok {
				t.Fatal("no conversation.compact.request published")
			}
			var payload map[string]string
			if err := json.Unmarshal(msg.Data, &payload); err != nil {
				t.Fatal(err)
			}
			if payload["tenant_id"] != compactTenant || payload["conversation_id"] != "conv-1" {
				t.Fatalf("payload = %v, want conv-1 of %s", payload, compactTenant)
			}
		})
	}
}
