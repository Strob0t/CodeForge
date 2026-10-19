//go:build unix

package workspacefs

import (
	"errors"
	"syscall"
)

const (
	// openNonBlock keeps an open of a FIFO from waiting for a peer.
	openNonBlock = syscall.O_NONBLOCK
	// openDirectory fails an open of anything but a directory at once.
	openDirectory = syscall.O_DIRECTORY
)

// notRegularOpen reports an open refused because the name is not a regular
// file: a socket or a FIFO without a reader (ENXIO), a directory opened for
// writing (EISDIR).
func notRegularOpen(err error) bool {
	return errors.Is(err, syscall.ENXIO) || errors.Is(err, syscall.EISDIR)
}
