//go:build !unix

package workspacefs

// Platforms without FIFOs: opens need no extra flags. The regular-file
// checks after each open still apply.
const (
	openNonBlock  = 0
	openDirectory = 0
)

func notRegularOpen(error) bool {
	return false
}
