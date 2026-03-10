// Package command defines the Command domain entity for chat slash commands.
package command

// Command represents a slash command available in the chat interface.
type Command struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Category    string `json:"category"`
	Icon        string `json:"icon,omitempty"`
	Description string `json:"description"`
	Args        string `json:"args,omitempty"`
}
