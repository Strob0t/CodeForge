package context_test

import (
	"strings"
	"testing"

	cfctx "github.com/Strob0t/CodeForge/internal/domain/context"
)

func TestSharedContext_Validate_Valid(t *testing.T) {
	sc := &cfctx.SharedContext{TeamID: "team-1", ProjectID: "proj-1"}
	if err := sc.Validate(); err != nil {
		t.Fatalf("expected valid, got error: %v", err)
	}
}

func TestSharedContext_Validate_MissingTeamID(t *testing.T) {
	sc := &cfctx.SharedContext{ProjectID: "proj-1"}
	err := sc.Validate()
	if err == nil || !strings.Contains(err.Error(), "team_id") {
		t.Fatalf("expected team_id error, got: %v", err)
	}
}

func TestSharedContext_Validate_MissingProjectID(t *testing.T) {
	sc := &cfctx.SharedContext{TeamID: "team-1"}
	err := sc.Validate()
	if err == nil || !strings.Contains(err.Error(), "project_id") {
		t.Fatalf("expected project_id error, got: %v", err)
	}
}

func TestAddSharedItemRequest_Validate_Valid(t *testing.T) {
	r := &cfctx.AddSharedItemRequest{TeamID: "team-1", Key: "output", Value: "result data", Author: "550e8400-e29b-41d4-a716-446655440000"}
	if err := r.Validate(); err != nil {
		t.Fatalf("expected valid, got error: %v", err)
	}
}

func TestAddSharedItemRequest_Validate_MissingKey(t *testing.T) {
	r := &cfctx.AddSharedItemRequest{TeamID: "team-1", Value: "data"}
	err := r.Validate()
	if err == nil || !strings.Contains(err.Error(), "key") {
		t.Fatalf("expected key error, got: %v", err)
	}
}

func TestAddSharedItemRequest_Validate_MissingValue(t *testing.T) {
	r := &cfctx.AddSharedItemRequest{TeamID: "team-1", Key: "output"}
	err := r.Validate()
	if err == nil || !strings.Contains(err.Error(), "value") {
		t.Fatalf("expected value error, got: %v", err)
	}
}
