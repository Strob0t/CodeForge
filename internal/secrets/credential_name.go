package secrets

import "strings"

// credentialNames are names that hold a credential as a whole.
var credentialNames = map[string]bool{
	"key":               true,
	"api_key":           true,
	"apikey":            true,
	"secret":            true,
	"password":          true,
	"passwd":            true,
	"token":             true,
	"authorization":     true,
	"credential":        true,
	"credentials":       true,
	"cookie":            true,
	"sig":               true,
	"signature":         true,
	"aws_access_key_id": true,
}

// credentialSuffixes mark names that hold a credential: openai_api_key,
// client_secret, aws_secret_access_key, aws_session_token,
// vertex_credentials, Proxy-Authorization, X-Amz-Signature, ...
var credentialSuffixes = []string{
	"_api_key", "_apikey", "_secret", "_password", "_token", "_credential", "_credentials",
	"_access_key", "_secret_key", "_private_key", "_authorization", "_signature",
}

// nonSecretNames end like a credential but are none: tokenizer special
// tokens and per-token prices (input_cost_per_token).
var nonSecretNames = map[string]bool{
	"eos_token": true, "bos_token": true, "pad_token": true, "unk_token": true,
	"sep_token": true, "cls_token": true, "mask_token": true,
}

// IsCredentialName reports whether a parameter, header or query parameter
// name holds a credential. Names are compared case-insensitively with "-"
// read as "_", exactly or by a defined suffix (never by substring, so
// max_tokens or custom_tokenizer are not credentials).
func IsCredentialName(name string) bool {
	n := strings.ReplaceAll(strings.ToLower(name), "-", "_")
	if n == "" || nonSecretNames[n] || strings.HasSuffix(n, "_per_token") {
		return false
	}
	if credentialNames[n] {
		return true
	}
	for _, suffix := range credentialSuffixes {
		if strings.HasSuffix(n, suffix) {
			return true
		}
	}
	return false
}
