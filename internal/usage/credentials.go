package usage

import (
	"encoding/json"
	"os"
	"strings"
	"time"

	"github.com/AbdallaM-Ibrahim/claude-code-statusline/internal/paths"
)

const (
	// oauthPrefix is what a claude.ai OAuth access token starts with. An API key
	// ("sk-ant-api…") authenticates with a different header and must never be
	// sent as a bearer token, so anything without this prefix is "no token".
	oauthPrefix = "sk-ant-oat"
	// expiryMargin: a token this close to expiry is not worth a round trip.
	// Claude Code refreshes it on its next request; this program never does.
	expiryMargin = 30 * time.Second
	// maxCredentialBytes bounds the read. The real file is a few kilobytes.
	maxCredentialBytes = 64 << 10
)

// token returns the current OAuth access token, or "" when none is usable. The
// token lives only in this call's stack: it is never logged, cached or echoed.
func token(now time.Time) string {
	data := readBounded(paths.Credentials(), maxCredentialBytes)
	if data == nil {
		data = keychainCredentials()
	}
	if data == nil {
		return ""
	}
	var c credentials
	if err := json.Unmarshal(data, &c); err != nil {
		return ""
	}
	return usableToken(c, now)
}

// usableToken applies the two checks that decide whether a request is worth
// making: the token is an OAuth access token, and it has not (nearly) expired.
func usableToken(c credentials, now time.Time) string {
	t := c.ClaudeAiOauth.AccessToken
	if !strings.HasPrefix(t, oauthPrefix) {
		return ""
	}
	if exp := c.ClaudeAiOauth.ExpiresAt; exp != 0 && !time.UnixMilli(exp).After(now.Add(expiryMargin)) {
		return ""
	}
	return t
}

// readBounded reads a regular file of at most max bytes, or returns nil. A
// symlink, a directory, an oversized or unreadable file all read as absent.
func readBounded(path string, max int64) []byte {
	fi, err := os.Lstat(path)
	if err != nil || !fi.Mode().IsRegular() || fi.Size() > max {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil || int64(len(data)) > max {
		return nil
	}
	return data
}
