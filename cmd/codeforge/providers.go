package main

// Provider blank imports — each import activates a self-registering adapter.
// Add new providers here as they are implemented.

import (
	_ "github.com/Strob0t/CodeForge/internal/adapter/autospec"
	_ "github.com/Strob0t/CodeForge/internal/adapter/gitea"
	_ "github.com/Strob0t/CodeForge/internal/adapter/githubpm"
	_ "github.com/Strob0t/CodeForge/internal/adapter/gitlocal"
	_ "github.com/Strob0t/CodeForge/internal/adapter/markdownspec"
	_ "github.com/Strob0t/CodeForge/internal/adapter/openspec"
	_ "github.com/Strob0t/CodeForge/internal/adapter/speckit"
	_ "github.com/Strob0t/CodeForge/internal/adapter/svn"
)
