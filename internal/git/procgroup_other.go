//go:build !unix

package git

import "os/exec"

// killGroupOnCancel kills only git itself where process groups are not
// available.
func killGroupOnCancel(*exec.Cmd) {}
