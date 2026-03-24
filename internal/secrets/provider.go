package secrets

import (
	"fmt"
	"os"
	"strings"
)

// Provider abstracts secret retrieval. Implementations:
//   - EnvProvider: reads from environment variables (development)
//   - FileProvider: reads from Docker Secrets files (production)
type Provider interface {
	Get(key string) (string, error)
}

// EnvProvider reads secrets from environment variables.
type EnvProvider struct{}

// Get returns the value of the environment variable key, or an error if unset.
func (EnvProvider) Get(key string) (string, error) {
	v := os.Getenv(key)
	if v == "" {
		return "", fmt.Errorf("env var %s not set", key)
	}
	return v, nil
}

// FileProvider reads secrets from Docker Secrets files (/run/secrets/).
// Falls back to environment variables if the file does not exist.
type FileProvider struct {
	Dir string // default: /run/secrets
}

// NewFileProvider creates a FileProvider using DOCKER_SECRETS_DIR or /run/secrets.
func NewFileProvider() *FileProvider {
	dir := os.Getenv("DOCKER_SECRETS_DIR")
	if dir == "" {
		dir = "/run/secrets"
	}
	return &FileProvider{Dir: dir}
}

// Get reads the secret from a file named after the key (lowercased, underscores
// replaced with hyphens). Falls back to the environment variable if the file
// does not exist.
func (fp *FileProvider) Get(key string) (string, error) {
	fileName := strings.ToLower(strings.ReplaceAll(key, "_", "-"))
	path := fp.Dir + "/" + fileName
	data, err := os.ReadFile(path) //nolint:gosec // path derived from trusted key name
	if err != nil {
		// Fallback to env var
		if v := os.Getenv(key); v != "" {
			return v, nil
		}
		return "", fmt.Errorf("secret %s: file %s not found and env var not set", key, path)
	}
	return strings.TrimSpace(string(data)), nil
}

// Auto selects FileProvider if /run/secrets exists, else EnvProvider.
func Auto() Provider {
	if info, err := os.Stat("/run/secrets"); err == nil && info.IsDir() {
		return NewFileProvider()
	}
	return EnvProvider{}
}
