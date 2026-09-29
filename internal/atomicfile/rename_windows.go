//go:build windows

package atomicfile

import (
	"errors"
	"os"
	"syscall"
	"time"
)

// errSharingViolation is ERROR_SHARING_VIOLATION, which package syscall does not
// name.
const errSharingViolation syscall.Errno = 32

// renameBackoff is the wait before each retry: 62 ms in all, well inside the
// render's deadline, and far longer than a sibling session holds the file open
// to read it.
var renameBackoff = []time.Duration{
	2 * time.Millisecond,
	4 * time.Millisecond,
	8 * time.Millisecond,
	16 * time.Millisecond,
	32 * time.Millisecond,
}

// rename retries while the target is held open by someone else, and fails fast
// on anything else.
func rename(from, to string) error {
	err := os.Rename(from, to)
	for _, wait := range renameBackoff {
		if err == nil || !transient(err) {
			return err
		}
		time.Sleep(wait)
		err = os.Rename(from, to)
	}
	return err
}

// transient reports whether err is another handle on the file rather than a
// real failure.
func transient(err error) bool {
	return errors.Is(err, syscall.ERROR_ACCESS_DENIED) || errors.Is(err, errSharingViolation)
}
