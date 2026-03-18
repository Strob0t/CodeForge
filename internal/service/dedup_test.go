package service

import (
	"math"
	"strconv"
	"strings"
	"testing"

	cfcontext "github.com/Strob0t/CodeForge/internal/domain/context"
)

// --- simhash64 tests ---

func TestSimhash64_IdenticalTexts(t *testing.T) {
	text := "func handleAuth(user User) error { return nil }"
	h1 := simhash64(text)
	h2 := simhash64(text)
	if h1 != h2 {
		t.Errorf("identical texts produced different hashes: %x != %x", h1, h2)
	}
}

func TestSimhash64_SimilarTexts_SmallDistance(t *testing.T) {
	a := "func handleAuth(user User) error { return nil }"
	b := "func handleAuth(user User) error { return err }"
	ha := simhash64(a)
	hb := simhash64(b)
	dist := hammingDistance(ha, hb)
	// Similar texts should have a small hamming distance (< 10).
	if dist > 10 {
		t.Errorf("similar texts have hamming distance %d, expected <= 10", dist)
	}
}

func TestSimhash64_VeryDifferentTexts_LargeDistance(t *testing.T) {
	a := "func handleAuth(user User) error { return nil }"
	b := "import numpy as np\nclass DataProcessor:\n    def transform(self, data):\n        return np.array(data)"
	ha := simhash64(a)
	hb := simhash64(b)
	dist := hammingDistance(ha, hb)
	// Very different texts should have a large hamming distance (>= 10).
	if dist < 10 {
		t.Errorf("very different texts have hamming distance %d, expected >= 10", dist)
	}
}

func TestSimhash64_EmptyString(t *testing.T) {
	h := simhash64("")
	if h != 0 {
		t.Errorf("empty string should return 0, got %x", h)
	}
}

func TestSimhash64_ShortText_LessThan3Chars(t *testing.T) {
	tests := []string{"", "a", "ab"}
	for _, s := range tests {
		h := simhash64(s)
		if h != 0 {
			t.Errorf("simhash64(%q) = %x, want 0 (text too short for trigrams)", s, h)
		}
	}
}

func TestSimhash64_ExactlyThreeChars(t *testing.T) {
	h := simhash64("abc")
	if h == 0 {
		t.Error("simhash64(\"abc\") should produce non-zero hash for exactly 3 chars")
	}
}

func TestSimhash64_UnicodeText(t *testing.T) {
	// Unicode text should not panic and should produce a valid hash.
	h := simhash64("func greet() { fmt.Println(\"Hallo Welt\") }")
	if h == 0 {
		t.Error("unicode text should produce non-zero hash")
	}

	// Two similar unicode texts should have small distance.
	a := "// Kommentar: Diese Funktion berechnet den Wert"
	b := "// Kommentar: Diese Funktion berechnet das Ergebnis"
	ha := simhash64(a)
	hb := simhash64(b)
	dist := hammingDistance(ha, hb)
	if dist > 15 {
		t.Errorf("similar unicode texts have hamming distance %d, expected <= 15", dist)
	}
}

func TestSimhash64_Deterministic(t *testing.T) {
	// Multiple calls must always return the same value.
	text := "package main\n\nimport \"fmt\"\n\nfunc main() { fmt.Println(\"hello\") }"
	first := simhash64(text)
	for i := 0; i < 100; i++ {
		if got := simhash64(text); got != first {
			t.Fatalf("iteration %d: hash changed from %x to %x", i, first, got)
		}
	}
}

// --- hammingDistance tests ---

func TestHammingDistance_SameValue(t *testing.T) {
	if d := hammingDistance(42, 42); d != 0 {
		t.Errorf("same value: got %d, want 0", d)
	}
}

func TestHammingDistance_OneBitDifferent(t *testing.T) {
	if d := hammingDistance(0, 1); d != 1 {
		t.Errorf("one bit different: got %d, want 1", d)
	}
}

func TestHammingDistance_AllBitsDifferent(t *testing.T) {
	if d := hammingDistance(0, math.MaxUint64); d != 64 {
		t.Errorf("all bits different: got %d, want 64", d)
	}
}

func TestHammingDistance_KnownValues(t *testing.T) {
	tests := []struct {
		a, b uint64
		want int
	}{
		{0b1010, 0b1001, 2},                 // two bits differ
		{0xFF, 0x00, 8},                     // 8 bits differ
		{0xFFFF, 0xFF00, 8},                 // 8 bits differ
		{0, 0, 0},                           // zeros
		{math.MaxUint64, math.MaxUint64, 0}, // both max
	}
	for _, tc := range tests {
		got := hammingDistance(tc.a, tc.b)
		if got != tc.want {
			t.Errorf("hammingDistance(%x, %x) = %d, want %d", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestHammingDistance_Symmetric(t *testing.T) {
	a, b := uint64(0xDEADBEEF), uint64(0xCAFEBABE)
	if hammingDistance(a, b) != hammingDistance(b, a) {
		t.Error("hamming distance should be symmetric")
	}
}

// --- deduplicateCandidates tests ---

func TestDeduplicateCandidates_OverlappingLines_KeepsHigherScored(t *testing.T) {
	// Two entries with nearly identical content but different priorities.
	// Use threshold=6 to catch near-duplicates with minor trailing differences.
	candidates := []cfcontext.ContextEntry{
		{Kind: cfcontext.EntryHybrid, Path: "auth.go", Content: "func handleAuth(user User) error { return nil }", Priority: 80, Tokens: 10},
		{Kind: cfcontext.EntryFile, Path: "auth.go", Content: "func handleAuth(user User) error { return nil } // verified", Priority: 60, Tokens: 12},
	}

	// Verify they are near-duplicates at threshold=6.
	ha := simhash64(candidates[0].Content)
	hb := simhash64(candidates[1].Content)
	dist := hammingDistance(ha, hb)
	if dist > 6 {
		t.Fatalf("test precondition failed: expected distance <= 6, got %d", dist)
	}

	result := deduplicateCandidates(candidates, 6)
	if len(result) != 1 {
		t.Fatalf("expected 1 candidate after dedup, got %d", len(result))
	}
	if result[0].Priority != 80 {
		t.Errorf("expected higher-scored candidate (priority=80) to be kept, got priority=%d", result[0].Priority)
	}
}

func TestDeduplicateCandidates_CrossFileDuplicates(t *testing.T) {
	// Same content in different files — should be deduplicated.
	content := "func processRequest(ctx context.Context, req *Request) (*Response, error) {\n\treturn &Response{}, nil\n}"
	candidates := []cfcontext.ContextEntry{
		{Kind: cfcontext.EntryHybrid, Path: "handler_v1.go", Content: content, Priority: 70, Tokens: 20},
		{Kind: cfcontext.EntryFile, Path: "handler_v2.go", Content: content, Priority: 65, Tokens: 20},
	}

	result := deduplicateCandidates(candidates, 3)
	if len(result) != 1 {
		t.Fatalf("expected 1 candidate after cross-file dedup, got %d", len(result))
	}
	if result[0].Path != "handler_v1.go" {
		t.Errorf("expected higher-scored handler_v1.go to be kept, got %s", result[0].Path)
	}
}

func TestDeduplicateCandidates_NoDuplicates_AllKept(t *testing.T) {
	candidates := []cfcontext.ContextEntry{
		{Kind: cfcontext.EntryFile, Path: "auth.go", Content: "func handleAuth(user User) error { return nil }", Priority: 80, Tokens: 10},
		{Kind: cfcontext.EntryFile, Path: "db.go", Content: "func queryUsers(ctx context.Context) ([]User, error) { return nil, nil }", Priority: 70, Tokens: 15},
		{Kind: cfcontext.EntryFile, Path: "server.go", Content: "func main() { http.ListenAndServe(\":8080\", nil) }", Priority: 60, Tokens: 12},
	}

	result := deduplicateCandidates(candidates, 3)
	if len(result) != 3 {
		t.Fatalf("expected all 3 candidates to survive dedup, got %d", len(result))
	}
}

func TestDeduplicateCandidates_EmptyInput(t *testing.T) {
	result := deduplicateCandidates(nil, 3)
	if len(result) != 0 {
		t.Errorf("expected 0 candidates for nil input, got %d", len(result))
	}

	result = deduplicateCandidates([]cfcontext.ContextEntry{}, 3)
	if len(result) != 0 {
		t.Errorf("expected 0 candidates for empty input, got %d", len(result))
	}
}

func TestDeduplicateCandidates_SingleCandidate(t *testing.T) {
	candidates := []cfcontext.ContextEntry{
		{Kind: cfcontext.EntryFile, Path: "main.go", Content: "package main\nfunc main() {}", Priority: 50, Tokens: 5},
	}

	result := deduplicateCandidates(candidates, 3)
	if len(result) != 1 {
		t.Fatalf("expected 1 candidate for single input, got %d", len(result))
	}
	if result[0].Path != "main.go" {
		t.Errorf("expected main.go, got %s", result[0].Path)
	}
}

func TestDeduplicateCandidates_AllDuplicates_KeepsHighestScored(t *testing.T) {
	base := "func processData(input []byte) ([]byte, error) { return input, nil }"
	candidates := []cfcontext.ContextEntry{
		{Kind: cfcontext.EntryHybrid, Path: "a.go", Content: base, Priority: 50, Tokens: 15},
		{Kind: cfcontext.EntryFile, Path: "b.go", Content: base, Priority: 90, Tokens: 15},
		{Kind: cfcontext.EntryGraph, Path: "c.go", Content: base, Priority: 70, Tokens: 15},
	}

	result := deduplicateCandidates(candidates, 3)
	if len(result) != 1 {
		t.Fatalf("expected 1 candidate when all are duplicates, got %d", len(result))
	}
	if result[0].Priority != 90 {
		t.Errorf("expected highest-scored candidate (priority=90), got priority=%d", result[0].Priority)
	}
}

func TestDeduplicateCandidates_ThresholdEdge_ExactlyAtThreshold(t *testing.T) {
	// Create two texts that are similar but not identical.
	// We test that when hamming distance == threshold, it IS considered a duplicate.
	a := "func handleRequest(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }"
	b := "func handleRequest(w http.ResponseWriter, r *http.Request) { w.WriteHeader(201) }"

	ha := simhash64(a)
	hb := simhash64(b)
	dist := hammingDistance(ha, hb)

	// Use the actual distance as threshold — should deduplicate.
	candidates := []cfcontext.ContextEntry{
		{Kind: cfcontext.EntryFile, Path: "a.go", Content: a, Priority: 80, Tokens: 20},
		{Kind: cfcontext.EntryFile, Path: "b.go", Content: b, Priority: 70, Tokens: 20},
	}
	result := deduplicateCandidates(candidates, dist)
	if len(result) != 1 {
		t.Errorf("at threshold=%d (exact distance), expected dedup to 1, got %d", dist, len(result))
	}
}

func TestDeduplicateCandidates_ThresholdEdge_AboveThreshold(t *testing.T) {
	// When threshold is distance-1, entries should NOT be deduplicated.
	a := "func handleRequest(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }"
	b := "func handleRequest(w http.ResponseWriter, r *http.Request) { w.WriteHeader(201) }"

	ha := simhash64(a)
	hb := simhash64(b)
	dist := hammingDistance(ha, hb)

	if dist == 0 {
		t.Skip("texts produce identical hashes, cannot test above-threshold case")
	}

	candidates := []cfcontext.ContextEntry{
		{Kind: cfcontext.EntryFile, Path: "a.go", Content: a, Priority: 80, Tokens: 20},
		{Kind: cfcontext.EntryFile, Path: "b.go", Content: b, Priority: 70, Tokens: 20},
	}
	result := deduplicateCandidates(candidates, dist-1)
	if len(result) != 2 {
		t.Errorf("at threshold=%d (below actual distance %d), expected both kept, got %d", dist-1, dist, len(result))
	}
}

func TestDeduplicateCandidates_ZeroThreshold_ExactMatchOnly(t *testing.T) {
	// Threshold=0 means exact-match only (hamming distance 0).
	base := "func identical() { return }"
	candidates := []cfcontext.ContextEntry{
		{Kind: cfcontext.EntryFile, Path: "a.go", Content: base, Priority: 80, Tokens: 10},
		{Kind: cfcontext.EntryFile, Path: "b.go", Content: base, Priority: 60, Tokens: 10},
	}
	// Identical content → hamming distance 0, which is <= 0.
	result := deduplicateCandidates(candidates, 0)
	if len(result) != 1 {
		t.Errorf("expected dedup of identical content at threshold=0, got %d candidates", len(result))
	}
}

func TestDeduplicateCandidates_NegativeThreshold_UsesDefault(t *testing.T) {
	// Threshold < 0 should fall back to default of 3.
	base := "func identical() { return }"
	candidates := []cfcontext.ContextEntry{
		{Kind: cfcontext.EntryFile, Path: "a.go", Content: base, Priority: 80, Tokens: 10},
		{Kind: cfcontext.EntryFile, Path: "b.go", Content: base, Priority: 60, Tokens: 10},
	}
	result := deduplicateCandidates(candidates, -1)
	if len(result) != 1 {
		t.Errorf("expected dedup with default threshold, got %d candidates", len(result))
	}
}

func TestDeduplicateCandidates_ShortContent(t *testing.T) {
	// Short content (< 3 chars) all hash to 0, so they are "duplicates".
	// Only the highest-scored should survive.
	candidates := []cfcontext.ContextEntry{
		{Kind: cfcontext.EntryFile, Path: "a.go", Content: "ab", Priority: 50, Tokens: 1},
		{Kind: cfcontext.EntryFile, Path: "b.go", Content: "xy", Priority: 70, Tokens: 1},
	}
	result := deduplicateCandidates(candidates, 3)
	if len(result) != 1 {
		t.Fatalf("expected 1 (short texts hash to 0), got %d", len(result))
	}
	if result[0].Priority != 70 {
		t.Errorf("expected highest-priority (70) kept, got %d", result[0].Priority)
	}
}

func TestDeduplicateCandidates_MixedDuplicatesAndUnique(t *testing.T) {
	dupContent := "func processRequest(ctx context.Context) error { return nil }"
	candidates := []cfcontext.ContextEntry{
		{Kind: cfcontext.EntryHybrid, Path: "handler.go", Content: dupContent, Priority: 85, Tokens: 15},
		{Kind: cfcontext.EntryFile, Path: "unique1.go", Content: "package database\nfunc Connect(dsn string) (*DB, error) { return nil, nil }", Priority: 75, Tokens: 20},
		{Kind: cfcontext.EntryFile, Path: "handler_copy.go", Content: dupContent, Priority: 60, Tokens: 15},
		{Kind: cfcontext.EntryFile, Path: "unique2.go", Content: "package config\nfunc Load(path string) (*Config, error) { return nil, nil }", Priority: 55, Tokens: 18},
	}

	result := deduplicateCandidates(candidates, 3)
	if len(result) != 3 {
		t.Fatalf("expected 3 candidates (2 unique + 1 from dup pair), got %d", len(result))
	}

	// Verify the dup pair kept the higher-scored one.
	paths := make(map[string]bool)
	for _, c := range result {
		paths[c.Path] = true
	}
	if !paths["handler.go"] {
		t.Error("expected handler.go (higher priority dup) to be kept")
	}
	if paths["handler_copy.go"] {
		t.Error("expected handler_copy.go (lower priority dup) to be removed")
	}
	if !paths["unique1.go"] || !paths["unique2.go"] {
		t.Error("expected both unique files to be kept")
	}
}

func TestDeduplicateCandidates_PreservesOrder_ByPriorityDescending(t *testing.T) {
	candidates := []cfcontext.ContextEntry{
		{Kind: cfcontext.EntryFile, Path: "low.go", Content: "package low\nfunc Low() {}", Priority: 30, Tokens: 5},
		{Kind: cfcontext.EntryFile, Path: "high.go", Content: "package high\nfunc High() { return }", Priority: 90, Tokens: 5},
		{Kind: cfcontext.EntryFile, Path: "mid.go", Content: "package mid\nfunc Mid() { x := 1; _ = x }", Priority: 60, Tokens: 5},
	}

	result := deduplicateCandidates(candidates, 3)
	if len(result) != 3 {
		t.Fatalf("expected 3 (no duplicates), got %d", len(result))
	}
	// Result should be sorted by priority descending.
	for i := 1; i < len(result); i++ {
		if result[i].Priority > result[i-1].Priority {
			t.Errorf("result not sorted by priority desc: [%d].Priority=%d > [%d].Priority=%d",
				i, result[i].Priority, i-1, result[i-1].Priority)
		}
	}
}

func TestDeduplicateCandidates_LargeSet(t *testing.T) {
	// Build a set with 50 near-duplicates (trailing spaces vary) and 20 unique entries.
	base := "func processData(input []byte) ([]byte, error) { return input, nil }"
	var candidates []cfcontext.ContextEntry
	for i := 0; i < 50; i++ {
		// Near-duplicates: append a small variation (trailing spaces).
		content := base + strings.Repeat(" ", i)
		candidates = append(candidates, cfcontext.ContextEntry{
			Kind:     cfcontext.EntryFile,
			Path:     "dup_" + strconv.Itoa(i) + ".go",
			Content:  content,
			Priority: 50 + i,
			Tokens:   15,
		})
	}
	// 20 genuinely unique entries with structurally different content.
	uniqueContents := []string{
		"package database\nimport \"database/sql\"\nfunc Connect(dsn string) (*sql.DB, error) { return sql.Open(\"postgres\", dsn) }",
		"package config\nimport \"os\"\nfunc Load() string { return os.Getenv(\"APP_CONFIG\") }",
		"package auth\nimport \"crypto/sha256\"\nfunc HashPassword(pw string) []byte { h := sha256.Sum256([]byte(pw)); return h[:] }",
		"package logger\nimport \"log/slog\"\nfunc Setup() { slog.SetDefault(slog.Default()) }",
		"package router\nimport \"net/http\"\nfunc NewMux() *http.ServeMux { return http.NewServeMux() }",
		"package middleware\nfunc CORS(next http.Handler) http.Handler { return next }",
		"package validator\nimport \"regexp\"\nvar emailRe = regexp.MustCompile(`^[a-z]+@[a-z]+\\.[a-z]+$`)",
		"package cache\nimport \"sync\"\ntype LRU struct { mu sync.Mutex; items map[string]string }",
		"package queue\ntype Message struct { ID string; Body []byte; Timestamp int64 }",
		"package metrics\nimport \"sync/atomic\"\nvar requestCount atomic.Int64",
		"package search\nfunc BM25Score(tf, df, dl, avgdl float64) float64 { return tf / (tf + 1.2) }",
		"package template\nimport \"text/template\"\nfunc Render(name string) *template.Template { return template.Must(template.New(name).Parse(\"\")) }",
		"package websocket\ntype Conn struct { ID string; Send chan []byte }",
		"package migration\nfunc Up(version int) error { return nil }",
		"package scheduler\nimport \"time\"\nfunc Every(d time.Duration, fn func()) { go func() { for { fn(); time.Sleep(d) } }() }",
		"package notification\ntype Alert struct { UserID string; Message string; Read bool }",
		"package billing\ntype Invoice struct { Amount float64; Currency string; Items []LineItem }",
		"package testing\nfunc AssertEqual(t interface{}, got, want interface{}) {}",
		"package deploy\nimport \"os/exec\"\nfunc RunScript(path string) error { return exec.Command(\"bash\", path).Run() }",
		"package monitoring\nimport \"runtime\"\nfunc MemStats() runtime.MemStats { var m runtime.MemStats; runtime.ReadMemStats(&m); return m }",
	}
	for i, content := range uniqueContents {
		candidates = append(candidates, cfcontext.ContextEntry{
			Kind:     cfcontext.EntryFile,
			Path:     "unique_" + strconv.Itoa(i) + ".go",
			Content:  content,
			Priority: 10 + i,
			Tokens:   10,
		})
	}

	result := deduplicateCandidates(candidates, 3)
	// The 50 near-duplicates should collapse to a handful.
	// Plus 20 unique entries should mostly survive.
	if len(result) >= 70 {
		t.Errorf("expected dedup to reduce count significantly below 70, got %d", len(result))
	}
	if len(result) < 15 {
		t.Errorf("expected at least 15 unique candidates to survive, got %d", len(result))
	}
	t.Logf("dedup reduced %d candidates to %d", len(candidates), len(result))
}
