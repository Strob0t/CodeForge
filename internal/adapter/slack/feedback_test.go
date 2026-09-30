package slack

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	fb "github.com/Strob0t/CodeForge/internal/domain/feedback"
)

// KI-69 (c): the approval message shows the deciding profile and the
// arguments preview; agent-controlled text cannot inject Slack markup.
func TestFeedbackProvider_MessageShowsProfileAndPreview(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		var msg struct {
			Blocks []struct {
				Text struct {
					Text string `json:"text"`
				} `json:"text"`
			} `json:"blocks"`
		}
		_ = json.Unmarshal(data, &msg)
		got = msg.Blocks[0].Text.Text
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	_, err := NewFeedbackProvider(srv.URL).RequestFeedback(context.Background(), fb.FeedbackRequest{
		RunID: "run-1", CallID: "c-1", Tool: "Bash", Command: "echo `id` <!channel> & ls",
		Profile: "supervised-ask-all", ArgumentsPreview: `{"command": "echo <@U123>"}`,
	})
	if err != nil {
		t.Fatalf("RequestFeedback: %v", err)
	}
	for _, want := range []string{"supervised-ask-all", "Arguments", "&lt;@U123&gt;", "&lt;!channel&gt;", "&amp;"} {
		if !strings.Contains(got, want) {
			t.Errorf("message lacks %q:\n%s", want, got)
		}
	}
	for _, bad := range []string{"<!channel>", "<@U123>", "`id`"} {
		if strings.Contains(got, bad) {
			t.Errorf("message contains unescaped %q:\n%s", bad, got)
		}
	}
}
