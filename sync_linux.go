//go:build linux

package anvil

import (
	"os"
	"syscall"
)

// fdatasync flushes file data (and the size metadata needed to read it back)
// without the extra inode-metadata cost of a full fsync. This is the correct
// durability barrier for our append-style page writes (SRS v1.1 §C3).
func fdatasync(f *os.File) error {
	return syscall.Fdatasync(int(f.Fd()))
}
