package main

import (
	"log/slog"
	"slices"

	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/secrets"
)

// reloadOnSIGHUP handles SIGHUP.
//
// Services copy their settings when they are built, so the only value that
// changes at runtime is the LiteLLM master key: the LiteLLM client reads it
// from the secrets vault on every request, and the vault re-reads
// LITELLM_MASTER_KEY_FILE here. Every other setting that differs from the
// running configuration is named in a warning as needing a restart. The log
// carries setting names only, never values.
func reloadOnSIGHUP(running *config.Config, flags config.CLIFlags, vault *secrets.Vault) {
	if err := vault.Reload(); err != nil {
		slog.Error("SIGHUP: secrets reload failed, the previous secrets stay in use", "error", err)
	} else {
		slog.Info("SIGHUP: secrets reloaded", "keys", vault.Keys())
	}

	changed, err := config.ChangedSinceStart(running, flags)
	if err != nil {
		slog.Error("SIGHUP: the configuration cannot be loaded, the running configuration stays in use",
			"error", secrets.RedactURL(err.Error()))
		return
	}
	// The LiteLLM client prefers the vault's key over the configured one.
	if vault.Get("LITELLM_MASTER_KEY") != "" {
		changed = slices.DeleteFunc(changed, func(name string) bool { return name == "litellm.master_key" })
	}
	if len(changed) == 0 {
		slog.Info("SIGHUP: configuration unchanged since start")
		return
	}
	slog.Warn("SIGHUP: configuration changed, restart to apply", "settings", changed)
}
