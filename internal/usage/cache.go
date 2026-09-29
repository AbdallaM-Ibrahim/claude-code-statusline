package usage

import (
	"encoding/json"

	"github.com/AbdallaM-Ibrahim/claude-code-statusline/internal/atomicfile"
	"github.com/AbdallaM-Ibrahim/claude-code-statusline/internal/paths"
)

// maxCacheBytes bounds the cache read. The real record is under 4 KB.
const maxCacheBytes = 1 << 20

// ReadCache returns the cache written by a previous Refresh, in the shape of
// ~/.claude.json's cachedUsageUtilization subtree, or nil.
func ReadCache() []byte { return readBounded(paths.UsageCache(), maxCacheBytes) }

// FetchedAtMs reports when a record was fetched, or 0 when the bytes carry no
// record — nil, unparsable, or a ~/.claude.json without the subtree.
func FetchedAtMs(data []byte) int64 {
	if len(data) == 0 {
		return 0
	}
	var r record
	if err := json.Unmarshal(data, &r); err != nil {
		return 0
	}
	return r.CachedUsageUtilization.FetchedAtMs
}

// writeCache writes atomically, 0600: the record names the account's plan and
// utilisation, nobody else's business.
func writeCache(path string, data []byte) bool {
	return atomicfile.Write(path, data, 0o600)
}
