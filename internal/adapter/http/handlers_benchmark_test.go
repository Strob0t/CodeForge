package http_test

import (
	"strings"
	"testing"
)

func TestBenchmarkHandlers_UseChiURLParam(t *testing.T) {
	// FIX-069: Regression test — benchmark handlers MUST use chi.URLParam,
	// not the stdlib r.PathValue (which requires Go 1.22+ net/http patterns
	// and does not work with chi's routing).
	src := readHandlerSource(t, "handlers_benchmark.go")

	if strings.Contains(src, "r.PathValue") {
		t.Fatal("handlers_benchmark.go must use chi.URLParam, not r.PathValue")
	}

	if !strings.Contains(src, "chi.URLParam") {
		t.Fatal("handlers_benchmark.go must use chi.URLParam for path parameter extraction")
	}
}

func TestBenchmarkAnalyzeHandlers_UseChiURLParam(t *testing.T) {
	// AnalyzeBenchmarkRun was merged into handlers_benchmark.go.
	src := readHandlerSource(t, "handlers_benchmark.go")

	if strings.Contains(src, "r.PathValue") {
		t.Fatal("handlers_benchmark.go must use chi.URLParam, not r.PathValue")
	}
}
