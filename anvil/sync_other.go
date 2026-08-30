//go:build !linux

package anvil

import "os"

// fdatasync falls back to a full fsync on platforms without fdatasync
// (e.g. Windows FlushFileBuffers, macOS). fsync is a strict superset of
// fdatasync's durability guarantee, so this is correct, just slightly slower.
func fdatasync(f *os.File) error {
	return f.Sync()
}
