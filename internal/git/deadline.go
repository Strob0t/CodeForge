package git

import (
	"errors"
	"slices"
	"sync/atomic"
	"time"
)

// Deadlines of the Go Core's git processes (KI-187): git opens files of the
// agent-writable workspace by name with a blocking open(), so a FIFO there
// (an ignore or attributes file, .git/info/exclude, a loose ref) would block
// git - and the Go Core goroutine and git pool slot waiting for it - until
// something writes to it. Every git process ends at its deadline or the
// caller's, whichever comes first.
const (
	// DefaultCommandTimeout bounds local commands (git.command_timeout).
	DefaultCommandTimeout = 2 * time.Minute
	// DefaultNetworkTimeout bounds commands that reach a remote: clone,
	// fetch, pull, push, ls-remote (git.network_timeout).
	DefaultNetworkTimeout = 10 * time.Minute
)

// ErrGitTimeout: a git command ran into its deadline or the caller's.
var ErrGitTimeout = errors.New("git command deadline exceeded")

var localTimeout, remoteTimeout atomic.Int64

// SetTimeouts sets the deadlines of local and network git commands; a value
// <= 0 keeps the default.
func SetTimeouts(command, network time.Duration) {
	localTimeout.Store(int64(command))
	remoteTimeout.Store(int64(network))
}

// commandTimeout is the deadline of the git command cmd.
func commandTimeout(cmd string) time.Duration {
	d, fallback := localTimeout.Load(), DefaultCommandTimeout
	if cmd == "clone" || slices.Contains(networkCommands, cmd) {
		d, fallback = remoteTimeout.Load(), DefaultNetworkTimeout
	}
	if d <= 0 {
		return fallback
	}
	return time.Duration(d)
}
