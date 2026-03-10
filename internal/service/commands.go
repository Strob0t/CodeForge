package service

import (
	"context"

	"github.com/Strob0t/CodeForge/internal/domain/command"
)

// builtinCommands is the static list of chat slash commands.
var builtinCommands = []command.Command{
	{ID: "/compact", Label: "compact", Category: "chat", Icon: "compress", Description: "Summarize conversation history"},
	{ID: "/rewind", Label: "rewind", Category: "chat", Icon: "rewind", Description: "Rewind to a previous step"},
	{ID: "/clear", Label: "clear", Category: "chat", Icon: "clear", Description: "Clear conversation history"},
	{ID: "/diff", Label: "diff", Category: "chat", Icon: "diff", Description: "Show all file changes in session"},
	{ID: "/cost", Label: "cost", Category: "chat", Icon: "cost", Description: "Show session cost summary"},
	{ID: "/help", Label: "help", Category: "chat", Icon: "help", Description: "Show available commands"},
	{ID: "/mode", Label: "mode", Category: "agent", Icon: "mode", Description: "Switch agent mode", Args: "mode_name"},
	{ID: "/model", Label: "model", Category: "agent", Icon: "model", Description: "Switch LLM model", Args: "model_name"},
}

// CommandService aggregates available commands from built-in sources.
type CommandService struct{}

// NewCommandService creates a new CommandService.
func NewCommandService() *CommandService {
	return &CommandService{}
}

// ListCommands returns all available slash commands.
func (s *CommandService) ListCommands(_ context.Context) []command.Command {
	result := make([]command.Command, len(builtinCommands))
	copy(result, builtinCommands)
	return result
}
