// Package domain provides shared domain-level sentinel errors.
package domain

import "errors"

// ErrNotFound indicates the requested entity does not exist.
var ErrNotFound = errors.New("not found")

// ErrConflict indicates a concurrent modification conflict (optimistic locking).
var ErrConflict = errors.New("conflict: resource was modified by another request")
