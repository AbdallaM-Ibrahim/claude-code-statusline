//go:build !windows

package atomicfile

import "os"

// rename is a plain rename: POSIX replaces the target atomically whoever has it
// open, so a failure here is never transient and not worth retrying.
func rename(from, to string) error { return os.Rename(from, to) }
