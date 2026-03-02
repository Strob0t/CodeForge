package quarantine

import (
	"math"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain/trust"
)

func TestScoreMessage_UntrustedSource(t *testing.T) {
	ann := &trust.Annotation{TrustLevel: trust.LevelUntrusted, Origin: "webhook"}
	score, factors := ScoreMessage(ann, []byte(`{"action":"read"}`))
	if score != 0.5 {
		t.Errorf("expected 0.5, got %f", score)
	}
	if len(factors) != 1 {
		t.Errorf("expected 1 factor, got %d", len(factors))
	}
}

func TestScoreMessage_PartialSource(t *testing.T) {
	ann := &trust.Annotation{TrustLevel: trust.LevelPartial, Origin: "mcp"}
	score, _ := ScoreMessage(ann, []byte(`{}`))
	if score != 0.2 {
		t.Errorf("expected 0.2, got %f", score)
	}
}

func TestScoreMessage_A2AOrigin(t *testing.T) {
	ann := &trust.Annotation{TrustLevel: trust.LevelPartial, Origin: "a2a"}
	score, factors := ScoreMessage(ann, []byte(`{}`))
	if score < 0.29 || score > 0.31 {
		t.Errorf("expected ~0.3 (partial 0.2 + a2a 0.1), got %f", score)
	}
	if len(factors) != 2 {
		t.Errorf("expected 2 factors, got %d", len(factors))
	}
}

func TestScoreMessage_FullTrust_NoTrustPenalty(t *testing.T) {
	ann := &trust.Annotation{TrustLevel: trust.LevelFull, Origin: "internal"}
	score, _ := ScoreMessage(ann, []byte(`{}`))
	if score != 0.0 {
		t.Errorf("expected 0.0, got %f", score)
	}
}

func TestScoreMessage_ShellInjection(t *testing.T) {
	ann := &trust.Annotation{TrustLevel: trust.LevelFull, Origin: "internal"}
	score, factors := ScoreMessage(ann, []byte(`{"cmd": "; rm -rf /"}`))
	if score != 0.3 {
		t.Errorf("expected 0.3, got %f", score)
	}
	if len(factors) != 1 || factors[0] != "shell injection pattern detected" {
		t.Errorf("unexpected factors: %v", factors)
	}
}

func TestScoreMessage_SQLInjection(t *testing.T) {
	ann := &trust.Annotation{TrustLevel: trust.LevelFull, Origin: "internal"}
	score, _ := ScoreMessage(ann, []byte(`{"query": "DROP TABLE users"}`))
	if score != 0.2 {
		t.Errorf("expected 0.2, got %f", score)
	}
}

func TestScoreMessage_PathTraversal(t *testing.T) {
	ann := &trust.Annotation{TrustLevel: trust.LevelFull, Origin: "internal"}
	score, _ := ScoreMessage(ann, []byte(`{"path": "../../etc/passwd"}`))
	if score != 0.2 {
		t.Errorf("expected 0.2, got %f", score)
	}
}

func TestScoreMessage_EnvVarAccess(t *testing.T) {
	ann := &trust.Annotation{TrustLevel: trust.LevelFull, Origin: "internal"}
	score, _ := ScoreMessage(ann, []byte(`{"code": "os.environ['SECRET']"}`))
	if score != 0.1 {
		t.Errorf("expected 0.1, got %f", score)
	}
}

func TestScoreMessage_CappedAtOne(t *testing.T) {
	ann := &trust.Annotation{TrustLevel: trust.LevelUntrusted, Origin: "a2a"}
	payload := `{"cmd": "; rm -rf /", "query": "DROP TABLE users", "path": "../../etc", "code": "os.environ['X']"}`
	score, _ := ScoreMessage(ann, []byte(payload))
	if math.Abs(score-1.0) > 0.001 {
		t.Errorf("expected 1.0 (capped), got %f", score)
	}
}

func TestScoreMessage_NilAnnotation(t *testing.T) {
	score, _ := ScoreMessage(nil, []byte(`{}`))
	if score != 0.0 {
		t.Errorf("expected 0.0 for nil annotation, got %f", score)
	}
}
