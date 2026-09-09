package usage

import (
	"os"
	"time"
)

// acquire creates the lock file exclusively and reports whether this process now
// owns it.
//
// A lock younger than ttl is honoured: either another session is fetching right
// now, or an attempt failed within the last interval and is backing off — the
// lock is removed on success and left behind on failure, so a revoked token or
// an unreachable API costs one request per interval, never one per render.
//
// A lock older than ttl belongs to a process that died mid-fetch or to an
// attempt that failed more than an interval ago, and is taken over — by exactly
// one contender. "Stat, remove, create" is not atomic, and two sessions racing
// for the same stale lock would otherwise both come away owning one, so the
// takeover itself is serialised through a second exclusive file.
func acquire(path string, now time.Time, ttl time.Duration) bool {
	if create(path) {
		return true
	}
	fi, err := os.Stat(path)
	if err != nil || now.Sub(fi.ModTime()) < ttl {
		return false
	}

	takeover := path + ".takeover"
	if !create(takeover) {
		// Another session is mid-takeover; leave the lock to it. Unless that
		// session died inside this microsecond window: a takeover marker that
		// never clears would silence the feature for good, so a stale one is
		// removed and the next render retries cleanly.
		if tfi, err := os.Stat(takeover); err == nil && now.Sub(tfi.ModTime()) >= ttl {
			os.Remove(takeover)
		}
		return false
	}
	defer os.Remove(takeover)

	// Holding the takeover marker. If a live owner replaced the lock while this
	// process queued for the marker, it wins.
	if cur, err := os.Stat(path); err == nil && now.Sub(cur.ModTime()) < ttl {
		return false
	}
	os.Remove(path)
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

// release removes the lock after a successful fetch, or after the re-check under
// the lock found nothing left to fetch.
func release(path string) { os.Remove(path) }
