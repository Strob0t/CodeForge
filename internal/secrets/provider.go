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
	value, err := readSecretFile(path)
	if err != nil {
		// Fallback to env var
		if v := os.Getenv(key); v != "" {
			return v, nil
		}
		return "", fmt.Errorf("secret %s: file %s not found and env var not set", key, path)
	}
	return value, nil
}

// FileEnvSuffix marks an environment variable that holds the path of a file
// containing the secret instead of the secret itself (Docker secrets), e.g.
// CODEFORGE_AUTH_JWT_SECRET_FILE=/run/secrets/codeforge-auth-jwt-secret.
const FileEnvSuffix = "_FILE"

// LookupFileEnv returns the secret for key from the file named by key+"_FILE".
// ok is false when key+"_FILE" is unset or empty. Setting both key and
// key+"_FILE" is an error: silently preferring one would hide a stale value.
// A missing, unreadable or empty file is an error too. Errors never contain
// the secret values.
func LookupFileEnv(key string) (value string, ok bool, err error) {
	fileKey := key + FileEnvSuffix
	path := os.Getenv(fileKey)
	if path == "" {
		return "", false, nil
	}
	if os.Getenv(key) != "" {
		return "", false, fmt.Errorf("both %s and %s are set, set only one", key, fileKey)
	}
	value, err = readSecretFile(path)
	if err != nil {
		return "", false, fmt.Errorf("%s: %w", fileKey, err)
	}
	if value == "" {
		return "", false, fmt.Errorf("%s: secret file %s is empty", fileKey, path)
	}
	return value, true, nil
}

// readSecretFile reads a secret file and trims surrounding whitespace, such as
// the trailing newline most editors and `echo` add.
func readSecretFile(path string) (string, error) {
	data, err := os.ReadFile(path) //nolint:gosec // G304: path comes from operator configuration
	if err != nil {
		return "", fmt.Errorf("read secret file: %w", err)
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
