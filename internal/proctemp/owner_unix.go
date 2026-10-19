//go:build unix

package proctemp

import (
	"io/fs"
	"os"
	"syscall"
)

// ownedByUs reports whether the file belongs to the current user.
func ownedByUs(info fs.FileInfo) bool {
	st, ok := info.Sys().(*syscall.Stat_t)
	return ok && int(st.Uid) == os.Getuid()
}
