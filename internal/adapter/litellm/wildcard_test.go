package litellm_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/Strob0t/CodeForge/internal/adapter/litellm"
)

// modelInfoWithoutKeys is the shape of LiteLLM's /model/info with no provider
// keys (recorded 2026-10-03, shortened): LiteLLM expands the groq/* route from
// its built-in catalogue (every row carries the route's deployment id), lists
// the ollama/* route from the Ollama server it points at (api_base), and
// keeps a route it could not expand (openai/* without a key) as one row.
const modelInfoWithoutKeys = `{"data":[
	{"model_name":"ollama/qwen3:4b-instruct","litellm_params":{"model":"ollama/qwen3:4b-instruct","api_base":"http://ollama:11434/v1"},"model_info":{"id":"dep-ollama","supports_function_calling":true}},
	{"model_name":"ollama/llama3.1:8b","litellm_params":{"model":"ollama/llama3.1:8b","api_base":"http://ollama:11434/v1"},"model_info":{"id":"dep-ollama"}},
	{"model_name":"openai/*","litellm_params":{"model":"openai/*"},"model_info":{"id":"dep-openai"}},
	{"model_name":"groq/llama-3.3-70b-versatile","litellm_params":{"model":"groq/llama-3.3-70b-versatile","tags":["default"]},"model_info":{"id":"dep-groq","output_cost_per_token":7.9e-07}},
	{"model_name":"groq/whisper-large-v3","litellm_params":{"model":"groq/whisper-large-v3","tags":["default"]},"model_info":{"id":"dep-groq"}},
	{"model_name":"groq/qwen/qwen3-32b","litellm_params":{"model":"groq/qwen/qwen3-32b","tags":["default"]},"model_info":{"id":"dep-groq"}},
	{"model_name":"my-model","litellm_params":{"model":"openai/gpt-4o-mini"},"model_info":{"id":"dep-mine"}}
]}`

func modelInfoServer(t *testing.T, body string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/model/info":
			_, _ = w.Write([]byte(body))
		case "/v1/models", "/health":
			_, _ = w.Write([]byte(`{}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// A cloud route's catalogue expansion is listed as one row (KI-125): without
// keys it would list hundreds of models nobody can call.
func TestListModels_CollapsesCatalogueExpansions(t *testing.T) {
	srv := modelInfoServer(t, modelInfoWithoutKeys)

	models, err := litellm.NewClient(srv.URL, "k").ListModels(context.Background())
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}

	type row struct{ name, id, paramsModel string }
	got := make([]row, 0, len(models))
	for _, m := range models {
		got = append(got, row{m.ModelName, m.ModelID, m.Params["model"].(string)})
	}
	want := []row{
		{"ollama/qwen3:4b-instruct", "dep-ollama", "ollama/qwen3:4b-instruct"},
		{"ollama/llama3.1:8b", "dep-ollama", "ollama/llama3.1:8b"},
		{"openai/*", "dep-openai", "openai/*"},
		{"groq/*", "dep-groq", "groq/*"},
		{"my-model", "dep-mine", "openai/gpt-4o-mini"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("models:\n got %v\nwant %v", got, want)
	}
	groq := models[3]
	if _, ok := groq.ModelInfo["output_cost_per_token"]; ok {
		t.Errorf("groq/* must not carry the first catalogue model's pricing: %v", groq.ModelInfo)
	}
	if tags, _ := groq.Params["tags"].([]any); len(tags) != 1 {
		t.Errorf("groq/* keeps the route's parameters (tags): %v", groq.Params)
	}
}

func TestDiscoverModels_CollapsesCatalogueExpansions(t *testing.T) {
	srv := modelInfoServer(t, modelInfoWithoutKeys)

	models, err := litellm.NewClient(srv.URL, "k").DiscoverModels(context.Background())
	if err != nil {
		t.Fatalf("DiscoverModels: %v", err)
	}

	names := make([]string, 0, len(models))
	for i := range models {
		names = append(names, models[i].ModelName)
	}
	want := []string{"ollama/qwen3:4b-instruct", "ollama/llama3.1:8b", "openai/*", "groq/*", "my-model"}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("names = %v, want %v", names, want)
	}
	if models[3].Provider != "groq" || models[3].ModelID != "dep-groq" || models[3].OutputCostPer != 0 {
		t.Errorf("groq/* = %+v", models[3])
	}
}

func TestListModels_SingleRowsAreKept(t *testing.T) {
	srv := modelInfoServer(t, `{"data":[
		{"model_name":"a","litellm_params":{"model":"openai/a"},"model_info":{"id":"1"}},
		{"model_name":"no-id","litellm_params":{"model":"groq/x"},"model_info":{}},
		{"model_name":"no-id-2","litellm_params":{"model":"groq/y"},"model_info":{}}
	]}`)

	models, err := litellm.NewClient(srv.URL, "k").ListModels(context.Background())
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if len(models) != 3 {
		t.Fatalf("rows without a shared deployment id are kept as they are: %+v", models)
	}
}
