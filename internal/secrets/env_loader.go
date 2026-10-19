package secrets

import "os"

// EnvLoader returns a Loader that reads the specified environment variables.
// A key may also be given as key+"_FILE" (see LookupFileEnv); the file is
// re-read on every load, so a rotated secret file is picked up on reload.
// Missing variables are silently omitted from the result map.
func EnvLoader(keys ...string) Loader {
	return func() (map[string]string, error) {
		vals := make(map[string]string, len(keys))
		for _, k := range keys {
			v, ok, err := LookupFileEnv(k)
			if err != nil {
				return nil, err
			}
			if !ok {
				v = os.Getenv(k)
			}
			if v != "" {
				vals[k] = v
			}
		}
		return vals, nil
	}
}
