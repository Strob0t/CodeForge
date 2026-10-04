package litellm_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/Strob0t/CodeForge/internal/adapter/litellm"
	"github.com/Strob0t/CodeForge/internal/port/llm"
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

// modelInfoWithAnthropicKey is LiteLLM's /model/info with ANTHROPIC_API_KEY
// set and no other key (KI-125 review): check_provider_endpoint lists the
// anthropic/* route from Anthropic's model list, and those rows share the
// route's deployment id exactly like a keyless catalogue expansion (groq/*).
// openai/* without a key could not be listed and stays one row.
const modelInfoWithAnthropicKey = `{"data":[
	{"model_name":"anthropic/claude-opus-4-1-20250805","litellm_params":{"model":"anthropic/claude-opus-4-1-20250805"},"model_info":{"id":"dep-anthropic","output_cost_per_token":7.5e-05,"supports_function_calling":true}},
	{"model_name":"anthropic/claude-sonnet-4-5-20250929","litellm_params":{"model":"anthropic/claude-sonnet-4-5-20250929"},"model_info":{"id":"dep-anthropic","output_cost_per_token":1.5e-05}},
	{"model_name":"anthropic/claude-haiku-4-5-20251001","litellm_params":{"model":"anthropic/claude-haiku-4-5-20251001"},"model_info":{"id":"dep-anthropic","output_cost_per_token":5e-06}},
	{"model_name":"openai/*","litellm_params":{"model":"openai/*"},"model_info":{"id":"dep-openai"}},
	{"model_name":"groq/llama-3.3-70b-versatile","litellm_params":{"model":"groq/llama-3.3-70b-versatile"},"model_info":{"id":"dep-groq","output_cost_per_token":7.9e-07}},
	{"model_name":"groq/whisper-large-v3","litellm_params":{"model":"groq/whisper-large-v3"},"model_info":{"id":"dep-groq"}}
]}`

func clientWithKeys(t *testing.T, srvURL string, named []string, env map[string]string) *litellm.Client {
	t.Helper()
	keys, err := llm.NewProviderKeys(named, func(k string) string { return env[k] })
	if err != nil {
		t.Fatalf("NewProviderKeys: %v", err)
	}
	c := litellm.NewClient(srvURL, "k")
	c.SetProviderKeys(keys)
	return c
}

func modelNames(models []litellm.Model) []string {
	names := make([]string, 0, len(models))
	for i := range models {
		names = append(names, models[i].ModelName)
	}
	return names
}

// A provider with a key lists its real models; a keyless catalogue
// expansion stays one row.
func TestListModels_KeyedExpansionIsListed(t *testing.T) {
	srv := modelInfoServer(t, modelInfoWithAnthropicKey)

	models, err := clientWithKeys(t, srv.URL, []string{"anthropic"}, nil).ListModels(context.Background())
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	want := []string{
		"anthropic/claude-opus-4-1-20250805", "anthropic/claude-sonnet-4-5-20250929", "anthropic/claude-haiku-4-5-20251001",
		"openai/*", "groq/*",
	}
	if got := modelNames(models); !reflect.DeepEqual(got, want) {
		t.Fatalf("models:\n got %v\nwant %v", got, want)
	}
	if models[0].ModelInfo["supports_function_calling"] != true || models[0].ModelID != "dep-anthropic" {
		t.Errorf("a listed model keeps its metadata and deployment id: %+v", models[0])
	}
}

// Without the provider's key the same rows collapse (zero-config default).
func TestListModels_KeylessExpansionIsCollapsed(t *testing.T) {
	srv := modelInfoServer(t, modelInfoWithAnthropicKey)

	models, err := litellm.NewClient(srv.URL, "k").ListModels(context.Background())
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if got, want := modelNames(models), []string{"anthropic/*", "openai/*", "groq/*"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("models = %v, want %v", got, want)
	}
}

// The key variable in the Core's own environment counts like a named provider.
func TestListModels_KeyFromEnvironment(t *testing.T) {
	srv := modelInfoServer(t, modelInfoWithAnthropicKey)

	models, err := clientWithKeys(t, srv.URL, nil, map[string]string{"GROQ_API_KEY": "gsk-1"}).ListModels(context.Background())
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	want := []string{"anthropic/*", "openai/*", "groq/llama-3.3-70b-versatile", "groq/whisper-large-v3"}
	if got := modelNames(models); !reflect.DeepEqual(got, want) {
		t.Fatalf("models = %v, want %v", got, want)
	}
}

// A provider CodeForge has no key variable for is listed as LiteLLM reports
// it: the worker's routing also treats it as keyed.
func TestListModels_UnknownProviderExpansionIsListed(t *testing.T) {
	srv := modelInfoServer(t, `{"data":[
		{"model_name":"newprovider/a","litellm_params":{"model":"newprovider/a"},"model_info":{"id":"dep-new"}},
		{"model_name":"newprovider/b","litellm_params":{"model":"newprovider/b"},"model_info":{"id":"dep-new"}}
	]}`)

	models, err := litellm.NewClient(srv.URL, "k").ListModels(context.Background())
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if got, want := modelNames(models), []string{"newprovider/a", "newprovider/b"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("models = %v, want %v", got, want)
	}
}

// One deployment can expand to several providers (a "*" route): each keyless
// provider becomes one route row, a keyed one keeps its models.
func TestListModels_MixedDeploymentCollapsesPerProvider(t *testing.T) {
	srv := modelInfoServer(t, `{"data":[
		{"model_name":"anthropic/claude-a","litellm_params":{"model":"anthropic/claude-a"},"model_info":{"id":"dep-all"}},
		{"model_name":"groq/x","litellm_params":{"model":"groq/x"},"model_info":{"id":"dep-all"}},
		{"model_name":"anthropic/claude-b","litellm_params":{"model":"anthropic/claude-b"},"model_info":{"id":"dep-all"}},
		{"model_name":"groq/y","litellm_params":{"model":"groq/y"},"model_info":{"id":"dep-all"}},
		{"model_name":"mistral/z","litellm_params":{"model":"mistral/z"},"model_info":{"id":"dep-all"}}
	]}`)

	models, err := clientWithKeys(t, srv.URL, []string{"anthropic"}, nil).ListModels(context.Background())
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	want := []string{"anthropic/claude-a", "groq/*", "anthropic/claude-b", "mistral/*"}
	if got := modelNames(models); !reflect.DeepEqual(got, want) {
		t.Fatalf("models = %v, want %v", got, want)
	}
}

// On a cloud-only installation (no Ollama, no configured model) the default
// model is a concrete model of the keyed provider (KI-125 review): it was
// empty, and every chat message failed with "no LLM model configured".
func TestDiscoverModels_CloudOnlyBestModelIsConcrete(t *testing.T) {
	srv := modelInfoServer(t, modelInfoWithAnthropicKey)

	models, err := clientWithKeys(t, srv.URL, []string{"anthropic"}, nil).DiscoverModels(context.Background())
	if err != nil {
		t.Fatalf("DiscoverModels: %v", err)
	}
	if best := litellm.SelectStrongestModel(models); best != "anthropic/claude-opus-4-1-20250805" {
		t.Fatalf("best model = %q, want the strongest anthropic model", best)
	}

	// Without the key every cloud row is a route: no default model.
	keyless, err := litellm.NewClient(srv.URL, "k").DiscoverModels(context.Background())
	if err != nil {
		t.Fatalf("DiscoverModels: %v", err)
	}
	if best := litellm.SelectStrongestModel(keyless); best != "" {
		t.Fatalf("best model without keys = %q, want none", best)
	}
}
