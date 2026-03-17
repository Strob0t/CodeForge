package service

import "embed"

//go:embed prompts
var promptsFS embed.FS

// PromptsFS returns the embedded prompt filesystem for use at application startup.
func PromptsFS() embed.FS {
	return promptsFS
}
