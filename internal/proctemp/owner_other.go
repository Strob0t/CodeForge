//go:build !unix

package proctemp

import "io/fs"

// ownedByUs cannot tell the owner here: nothing is removed as stale.
func ownedByUs(fs.FileInfo) bool { return false }
