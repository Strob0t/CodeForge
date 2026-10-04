package postgres_test

import (
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain/conversation"
)

// TestStore_ClaimConversationTurnCompletion (S2-G fix 2, 2): a turn's worker
// completion is claimed once per conversation and turn, in the
// conversation's tenant; a redelivery finds the claim.
func TestStore_ClaimConversationTurnCompletion(t *testing.T) {
	f, other := newStatusFixture(t), newStatusFixture(t)
	c, err := f.store.CreateConversation(f.ctx, &conversation.Conversation{ProjectID: f.project.ID, Title: "turns"})
	if err != nil {
		t.Fatalf("CreateConversation: %v", err)
	}
	claim := func(fx *statusFixture, turn string) bool {
		t.Helper()
		first, err := fx.store.ClaimConversationTurnCompletion(fx.ctx, c.ID, turn)
		if err != nil {
			t.Fatalf("ClaimConversationTurnCompletion(%s): %v", turn, err)
		}
		return first
	}

	if !claim(f, "turn-1") {
		t.Fatal("the first completion of turn-1 was not claimed")
	}
	if claim(f, "turn-1") {
		t.Fatal("a repeated completion of turn-1 was claimed")
	}
	if !claim(f, "turn-2") {
		t.Fatal("the next turn's completion was taken for a repeat")
	}
	if claim(other, "turn-3") {
		t.Fatal("another tenant claimed a turn of the conversation")
	}
}
