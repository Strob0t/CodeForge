package secrets

import "os"

// EnvLoader returns a Loader that reads the specified environment variables.
// Missing variables are silently omitted from the result map.
func EnvLoader(keys ...string) Loader {
	return func() (map[string]string, error) {
		vals := make(map[string]string, len(keys))
		for _, k := range keys {
			if v := os.Getenv(k); v != "" {
				vals[k] = v
			}
		}
		return vals, nil
	}
}
