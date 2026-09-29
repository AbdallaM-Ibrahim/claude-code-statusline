// Package atomicfile writes the status line's state files so a reader never sees
// a torn one: a per-process temp file, then a rename over the target.
//
// On Windows that rename is not unconditional. Go opens files without
// FILE_SHARE_DELETE, so while another session is reading the target — or an
// antivirus scanner or the search indexer has it open — replacing it fails with
// ERROR_ACCESS_DENIED or ERROR_SHARING_VIOLATION. Those holds last microseconds
// to milliseconds, so the Windows build retries briefly before giving up.
package atomicfile

import (
	"os"
	"strconv"
)

// Write replaces path with data, mode perm, and reports whether it did. On
// failure the target is untouched and the temp file is removed.
func Write(path string, data []byte, perm os.FileMode) bool {
	tmp := path + "." + strconv.Itoa(os.Getpid()) + ".tmp"
	if err := os.WriteFile(tmp, data, perm); err != nil {
		return false
	}
	if err := rename(tmp, path); err != nil {
		os.Remove(tmp)
		return false
	}
	return true
}
