package secrets_test

import (
	"testing"

	"github.com/Strob0t/CodeForge/internal/secrets"
)

// S6-G review, item 8: credential names are matched exactly or by a defined
// suffix, not by substring, so max_tokens, *_cost_per_token or
// custom_tokenizer are kept.
func TestIsCredentialName(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{"api_key", true},
		{"API_KEY", true},
		{"openai_api_key", true},
		{"x-api-key", true},
		{"x-goog-api-key", true},
		{"key", true},
		{"secret", true},
		{"client_secret", true},
		{"password", true},
		{"db_password", true},
		{"token", true},
		{"azure_ad_token", true},
		{"aws_session_token", true},
		{"aws_access_key_id", true},
		{"aws_secret_access_key", true},
		{"vertex_credentials", true},
		{"Authorization", true},
		{"Proxy-Authorization", true},
		{"X-Amz-Signature", true},
		{"X-Amz-Credential", true},
		{"cookie", true},
		// Security review of the S6-G round: names the substring match used
		// to catch and the exact/suffix match missed.
		{"oci_key", true},
		{"subscription_key", true},
		{"client_key", true},
		{"secret_key", true},
		{"access_key", true},
		{"private_key", true},
		{"Ocp-Apim-Subscription-Key", true},
		{"x-rapidapi-key", true},
		{"x-functions-key", true},
		{"accessToken", true},
		{"authToken", true},
		{"clientSecret", true},
		{"refreshToken", true},
		{"client.secret", true},
		{"session_cookie", true},
		{"maxTokens", false},
		{"inputCostPerToken", false},
		{"max_tokens", false},
		{"max_completion_tokens", false},
		{"input_cost_per_token", false},
		{"output_cost_per_token", false},
		{"custom_tokenizer", false},
		{"eos_token", false},
		{"pad_token", false},
		{"model", false},
		{"api_base", false},
		{"aws_region_name", false},
		{"keywords", false},
		{"monkey", false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := secrets.IsCredentialName(tt.name); got != tt.want {
				t.Fatalf("IsCredentialName(%q) = %t, want %t", tt.name, got, tt.want)
			}
		})
	}
}

// RedactURL also hides the values of credential query parameters.
func TestRedactURL_QueryCredentials(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"key parameter", "https://generativelanguage.googleapis.com/v1?key=AIza123&alt=json", "https://generativelanguage.googleapis.com/v1?key=[REDACTED]&alt=json"},
		{"api_key last", "https://x.io/p?a=1&api_key=s3cret", "https://x.io/p?a=1&api_key=[REDACTED]"},
		{"before a fragment", "https://x.io/p?token=t0k#top", "https://x.io/p?token=[REDACTED]#top"},
		{"signed url", "https://b.s3.amazonaws.com/o?X-Amz-Credential=AKIA&X-Amz-Signature=abc&X-Amz-Expires=60", "https://b.s3.amazonaws.com/o?X-Amz-Credential=[REDACTED]&X-Amz-Signature=[REDACTED]&X-Amz-Expires=60"},
		{"with userinfo too", "https://u:p@x.io/p?secret=s", "https://[REDACTED]@x.io/p?secret=[REDACTED]"},
		{"in a sentence", "GET https://x.io/p?key=k failed", "GET https://x.io/p?key=[REDACTED] failed"},
		{"non-credential parameters stay", "postgresql://h:5432/db?sslmode=require&max_tokens=5", "postgresql://h:5432/db?sslmode=require&max_tokens=5"},
		{"empty value stays", "https://x.io/p?key=&a=1", "https://x.io/p?key=&a=1"},
		{"already redacted stays", "https://x.io/p?key=[REDACTED]", "https://x.io/p?key=[REDACTED]"},
		{"not in a url", "key=value", "key=value"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := secrets.RedactURL(tt.in); got != tt.want {
				t.Fatalf("RedactURL(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
