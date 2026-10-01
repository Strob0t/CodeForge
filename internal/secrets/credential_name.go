package secrets

import (
	"strings"
	"unicode"
)

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
// oci_key, subscription_key, client_secret, aws_secret_access_key,
// aws_session_token, vertex_credentials, Proxy-Authorization,
// X-Amz-Signature, ... Any *_key is treated as a credential: hiding a
// non-secret key name costs nothing, missing a secret one leaks it.
var credentialSuffixes = []string{
	"_key", "_apikey", "_secret", "_password", "_passwd", "_token", "_credential", "_credentials",
	"_authorization", "_signature", "_cookie",
}

// nonSecretNames end like a credential but are none: tokenizer special
// tokens and per-token prices (input_cost_per_token).
var nonSecretNames = map[string]bool{
	"eos_token": true, "bos_token": true, "pad_token": true, "unk_token": true,
	"sep_token": true, "cls_token": true, "mask_token": true,
}

// IsCredentialName reports whether a parameter, header or query parameter
// name holds a credential. Names are normalised to snake case (camelCase
// split, "-", "." and spaces read as "_", lower case) and compared exactly or
// by a defined suffix, never by substring, so max_tokens or custom_tokenizer
// are not credentials.
func IsCredentialName(name string) bool {
	n := normaliseName(name)
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

// normaliseName turns accessToken, X-Api-Key or client.secret into
// access_token, x_api_key and client_secret.
func normaliseName(name string) string {
	var b strings.Builder
	runes := []rune(name)
	for i, r := range runes {
		switch {
		case r == '-' || r == '.' || r == ' ':
			b.WriteByte('_')
		case unicode.IsUpper(r):
			if i > 0 && (unicode.IsLower(runes[i-1]) || unicode.IsDigit(runes[i-1])) {
				b.WriteByte('_')
			}
			b.WriteRune(unicode.ToLower(r))
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}
