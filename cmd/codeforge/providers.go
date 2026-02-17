package main

// Provider blank imports — each import activates a self-registering adapter.
// Add new providers here as they are implemented.

import (
	_ "github.com/Strob0t/CodeForge/internal/adapter/gitlocal"
)
