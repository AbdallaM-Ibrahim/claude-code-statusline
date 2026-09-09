package usage

import (
	"os"
	"time"
)

// lockTTL bounds both how long a lock is honoured and how long a failed attempt
// backs off. The lock file is removed on success and left behind on failure, so
// a revoked token or an unreachable API costs one request per lockTTL rather
// than one per render, and several sessions rendering at once make one request.
const lockTTL = 60 * time.Second

// acquire creates the lock file exclusively. A lock older than lockTTL belongs
// to a process that died or an attempt that failed, and is taken over.
func acquire(path string, now time.Time) bool {
	if create(path) {
		return true
	}
	fi, err := os.Stat(path)
	if err != nil || now.Sub(fi.ModTime()) < lockTTL {
		return false
	}
	if os.Remove(path) != nil {
		return false
	}
	return create(path)
}

func create(path string) bool {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return false
	}
	f.Close()
	return true
}

// release removes the lock after a successful fetch.
func release(path string) { os.Remove(path) }
