package usage

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/AbdallaM-Ibrahim/claude-code-statusline/internal/paths"
)

const testToken = "sk-ant-oat01-test-token-never-real"

// setup isolates every file this package touches under a temp CLAUDE_CONFIG_DIR
// and writes a usable credential there. It returns the fixed "now" the tests
// reason about.
func setup(t *testing.T) time.Time {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", dir)
	// The real clock, not a fixed date: lock staleness compares the caller's now
	// against a file mtime the filesystem stamps with the real clock.
	now := time.Now().Truncate(time.Millisecond)
	writeCredentials(t, testToken, now.Add(time.Hour).UnixMilli())
	return now
}

func writeCredentials(t *testing.T, token string, expiresAtMs int64) {
	t.Helper()
	var c credentials
	c.ClaudeAiOauth.AccessToken = token
	c.ClaudeAiOauth.ExpiresAt = expiresAtMs
	data, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(paths.Credentials(), data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func fixture(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "usage_response.json"))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// server stands in for the usage endpoint. It counts hits and records the last
// request's auth headers so a test can prove what was sent — and that the token
// went nowhere else.
type server struct {
	*httptest.Server
	hits    atomic.Int32
	auth    atomic.Pointer[string]
	beta    atomic.Pointer[string]
	handler func(w http.ResponseWriter, r *http.Request)
}

func newServer(t *testing.T, handler func(w http.ResponseWriter, r *http.Request)) *server {
	t.Helper()
	s := &server{handler: handler}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.hits.Add(1)
		a, b := r.Header.Get("Authorization"), r.Header.Get("anthropic-beta")
		s.auth.Store(&a)
		s.beta.Store(&b)
		s.handler(w, r)
	}))
	t.Cleanup(s.Close)

	prevEndpoint, prevClient := endpoint, client
	endpoint = s.URL + "/api/oauth/usage"
	client = &http.Client{Timeout: fetchTimeout, CheckRedirect: refuseRedirect}
	t.Cleanup(func() { endpoint, client = prevEndpoint, prevClient })
	return s
}

func serveFixture(t *testing.T) *server {
	body := fixture(t)
	return newServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(body)
	})
}

func on() Config { return Config{Enabled: true, Interval: DefaultInterval} }

func TestParseConfig(t *testing.T) {
	cases := []struct {
		in   string
		want Config
	}{
		{"", Config{}},
		{"0", Config{}},
		{"false", Config{}},
		{"OFF", Config{}},
		{"1", Config{Enabled: true, Interval: DefaultInterval}},
		{"true", Config{Enabled: true, Interval: DefaultInterval}},
		{" yes ", Config{Enabled: true, Interval: DefaultInterval}},
		{"10m", Config{Enabled: true, Interval: 10 * time.Minute}},
		{"1h", Config{Enabled: true, Interval: time.Hour}},
		{"30s", Config{Enabled: true, Interval: MinInterval}}, // floored
		{"-5m", Config{}},
		{"soon", Config{}},
		{"5", Config{}}, // a bare number is not a duration and not a boolean
	}
	for _, c := range cases {
		if got := ParseConfig(c.in); got != c.want {
			t.Errorf("ParseConfig(%q) = %+v, want %+v", c.in, got, c.want)
		}
	}
}

func TestUsableToken(t *testing.T) {
	now := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	mk := func(tok string, exp int64) credentials {
		var c credentials
		c.ClaudeAiOauth.AccessToken = tok
		c.ClaudeAiOauth.ExpiresAt = exp
		return c
	}
	if got := usableToken(mk(testToken, now.Add(time.Hour).UnixMilli()), now); got != testToken {
		t.Errorf("valid token rejected: %q", got)
	}
	if got := usableToken(mk(testToken, 0), now); got != testToken {
		t.Errorf("token without expiry should be usable, got %q", got)
	}
	if got := usableToken(mk(testToken, now.Add(10*time.Second).UnixMilli()), now); got != "" {
		t.Errorf("token inside the expiry margin should be unusable, got %q", got)
	}
	if got := usableToken(mk(testToken, now.Add(-time.Minute).UnixMilli()), now); got != "" {
		t.Errorf("expired token should be unusable, got %q", got)
	}
	if got := usableToken(mk("sk-ant-api03-not-oauth", 0), now); got != "" {
		t.Errorf("an API key must never be sent as a bearer token, got %q", got)
	}
	if got := usableToken(mk("", 0), now); got != "" {
		t.Errorf("empty token should be unusable, got %q", got)
	}
}

func TestRefreshOffMakesNoRequest(t *testing.T) {
	now := setup(t)
	s := serveFixture(t)
	if got := Refresh(context.Background(), Config{}, 0, now); got != nil {
		t.Fatalf("Refresh with the switch off returned %d bytes", len(got))
	}
	if s.hits.Load() != 0 {
		t.Fatal("switch off must not touch the network")
	}
	if ReadCache() != nil {
		t.Fatal("switch off must not write a cache")
	}
}

func TestRefreshSkipsFreshRecord(t *testing.T) {
	now := setup(t)
	s := serveFixture(t)
	fresh := now.Add(-time.Minute).UnixMilli()
	if got := Refresh(context.Background(), on(), fresh, now); got != nil {
		t.Fatal("a record one minute old is fresh at a five-minute interval")
	}
	if s.hits.Load() != 0 {
		t.Fatal("fresh record must not trigger a fetch")
	}
	stale := now.Add(-DefaultInterval).UnixMilli()
	if got := Refresh(context.Background(), on(), stale, now); got == nil {
		t.Fatal("a record exactly one interval old is stale")
	}
	if s.hits.Load() != 1 {
		t.Fatalf("hits = %d, want 1", s.hits.Load())
	}
}

func TestRefreshFetchesWritesCacheAndReturnsRecord(t *testing.T) {
	now := setup(t)
	s := serveFixture(t)

	got := Refresh(context.Background(), on(), 0, now)
	if got == nil {
		t.Fatal("Refresh returned nil")
	}
	if s.hits.Load() != 1 {
		t.Fatalf("hits = %d, want 1", s.hits.Load())
	}
	if a := *s.auth.Load(); a != "Bearer "+testToken {
		t.Errorf("Authorization = %q", a)
	}
	if b := *s.beta.Load(); b != betaHeader {
		t.Errorf("anthropic-beta = %q", b)
	}

	if FetchedAtMs(got) != now.UnixMilli() {
		t.Errorf("fetchedAtMs = %d, want %d", FetchedAtMs(got), now.UnixMilli())
	}
	cached := ReadCache()
	if string(cached) != string(got) {
		t.Fatal("the returned record and the cache file differ")
	}
	var rec record
	if err := json.Unmarshal(cached, &rec); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(rec.CachedUsageUtilization.Utilization), `"weekly_scoped"`) {
		t.Error("cache lost the per-model row the whole feature exists for")
	}
	if strings.Contains(string(cached), testToken) {
		t.Fatal("the token must never reach the cache")
	}
	if _, err := os.Stat(paths.UsageLock()); !os.IsNotExist(err) {
		t.Error("lock should be released after a successful fetch")
	}
	fi, err := os.Stat(paths.UsageCache())
	if err != nil {
		t.Fatal(err)
	}
	if perm := fi.Mode().Perm(); perm&0o077 != 0 && !isWindows() {
		t.Errorf("cache mode = %o, want 0600", perm)
	}
}

func TestRefreshRejectsNon200AndBacksOffForAnInterval(t *testing.T) {
	now := setup(t)
	s := newServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":{"type":"authentication_error"}}`))
	})
	if got := Refresh(context.Background(), on(), 0, now); got != nil {
		t.Fatal("a 401 must not produce a record")
	}
	if ReadCache() != nil {
		t.Fatal("a 401 must not write a cache")
	}
	// The lock is left behind on failure, so renders for the rest of the interval
	// do not hammer the endpoint with the same bad token — six sessions rendering
	// every ten seconds would otherwise be 36 requests a minute.
	for _, later := range []time.Duration{10 * time.Second, time.Minute, DefaultInterval - time.Second} {
		if got := Refresh(context.Background(), on(), 0, now.Add(later)); got != nil {
			t.Fatalf("attempt %s after a failure should not fetch", later)
		}
	}
	if s.hits.Load() != 1 {
		t.Fatalf("hits = %d, want 1 for the whole interval", s.hits.Load())
	}
	// One interval later the lock is stale and a fresh attempt is allowed.
	Refresh(context.Background(), on(), 0, now.Add(DefaultInterval+time.Second))
	if s.hits.Load() != 2 {
		t.Fatalf("hits = %d, want 2 after the interval elapsed", s.hits.Load())
	}
}

// Several sessions render at once with the same stale record. One request.
func TestRefreshConcurrentSessionsFetchOnce(t *testing.T) {
	now := setup(t)
	body := fixture(t)
	s := newServer(t, func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(50 * time.Millisecond) // widen the window the lock has to cover
		w.Write(body)
	})
	const sessions = 16
	var wg sync.WaitGroup
	var fetched atomic.Int32
	for i := 0; i < sessions; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if Refresh(context.Background(), on(), 0, now) != nil {
				fetched.Add(1)
			}
		}()
	}
	wg.Wait()
	if s.hits.Load() != 1 {
		t.Fatalf("hits = %d, want exactly 1 across %d concurrent sessions", s.hits.Load(), sessions)
	}
	if fetched.Load() != 1 {
		t.Fatalf("%d sessions believe they fetched, want 1", fetched.Load())
	}
	if ReadCache() == nil {
		t.Fatal("the one fetch should have left a cache")
	}
	if _, err := os.Stat(paths.UsageLock()); !os.IsNotExist(err) {
		t.Error("lock should be released after the successful fetch")
	}
}

// The caller decided the record was stale from a read made before the lock was
// taken. If another session fetched in between, the cache on disk is fresh and
// the re-check under the lock must skip the request.
func TestRefreshDoubleChecksCacheUnderLock(t *testing.T) {
	now := setup(t)
	s := serveFixture(t)
	var rec record
	rec.CachedUsageUtilization.FetchedAtMs = now.Add(-time.Minute).UnixMilli()
	rec.CachedUsageUtilization.Utilization = json.RawMessage(`{"limits":[]}`)
	data, err := json.Marshal(rec)
	if err != nil {
		t.Fatal(err)
	}
	if !writeCache(paths.UsageCache(), data) {
		t.Fatal("could not seed the cache")
	}
	if got := Refresh(context.Background(), on(), 0 /* caller saw no record */, now); got != nil {
		t.Fatal("fetched although the cache on disk was fresh")
	}
	if s.hits.Load() != 0 {
		t.Fatalf("hits = %d, want 0", s.hits.Load())
	}
	if _, err := os.Stat(paths.UsageLock()); !os.IsNotExist(err) {
		t.Error("lock should be released when the re-check finds nothing to do")
	}
}

// Many sessions find the same stale lock at the same instant. Exactly one may
// take it over; "stat, remove, create" without the takeover marker lets several
// through, each deleting the other's fresh lock.
func TestAcquireStaleTakeoverIsExclusive(t *testing.T) {
	setup(t)
	lock := paths.UsageLock()
	ttl := 2 * time.Minute
	if !create(lock) {
		t.Fatal("could not create lock")
	}
	old := time.Now().Add(-ttl - time.Minute)
	if err := os.Chtimes(lock, old, old); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	const contenders = 32
	var wg sync.WaitGroup
	var winners atomic.Int32
	start := make(chan struct{})
	for i := 0; i < contenders; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			if acquire(lock, now, ttl) {
				winners.Add(1)
			}
		}()
	}
	close(start)
	wg.Wait()
	if winners.Load() != 1 {
		t.Fatalf("%d contenders took over the stale lock, want exactly 1", winners.Load())
	}
	if _, err := os.Stat(lock + ".takeover"); !os.IsNotExist(err) {
		t.Error("takeover marker left behind")
	}
}

func TestRefreshRejectsBodyWithoutLimits(t *testing.T) {
	now := setup(t)
	s := newServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"error":{"type":"rate_limit_error"}}`))
	})
	if got := Refresh(context.Background(), on(), 0, now); got != nil {
		t.Fatal("an in-band error envelope must not become a record")
	}
	if s.hits.Load() != 1 || ReadCache() != nil {
		t.Fatal("no cache for a body without limits")
	}
}

func TestRefreshRefusesRedirect(t *testing.T) {
	now := setup(t)
	var followed atomic.Int32
	body := fixture(t)
	s := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/elsewhere" {
			followed.Add(1)
			w.Write(body)
			return
		}
		http.Redirect(w, r, "/elsewhere", http.StatusFound)
	})
	if got := Refresh(context.Background(), on(), 0, now); got != nil {
		t.Fatal("a redirect must not produce a record")
	}
	if followed.Load() != 0 {
		t.Fatal("the client followed a redirect; the token may only go to the configured host")
	}
	if s.hits.Load() != 1 {
		t.Fatalf("hits = %d, want 1", s.hits.Load())
	}
}

func TestRefreshWithoutUsableTokenMakesNoRequest(t *testing.T) {
	now := setup(t)
	s := serveFixture(t)

	os.Remove(paths.Credentials())
	if got := Refresh(context.Background(), on(), 0, now); got != nil {
		t.Fatal("no credentials file, yet a record")
	}
	writeCredentials(t, testToken, now.Add(-time.Minute).UnixMilli())
	if got := Refresh(context.Background(), on(), 0, now); got != nil {
		t.Fatal("expired token, yet a record")
	}
	writeCredentials(t, "sk-ant-api03-key", 0)
	if got := Refresh(context.Background(), on(), 0, now); got != nil {
		t.Fatal("API key, yet a record")
	}
	if err := os.WriteFile(paths.Credentials(), []byte("not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := Refresh(context.Background(), on(), 0, now); got != nil {
		t.Fatal("garbage credentials, yet a record")
	}
	if s.hits.Load() != 0 {
		t.Fatalf("hits = %d, want 0: no usable token means no request", s.hits.Load())
	}
}

func TestRefreshHonoursLockHeldByAnotherSession(t *testing.T) {
	now := setup(t)
	s := serveFixture(t)
	if !create(paths.UsageLock()) {
		t.Fatal("could not create lock")
	}
	if got := Refresh(context.Background(), on(), 0, now); got != nil {
		t.Fatal("fetched while another session held the lock")
	}
	if s.hits.Load() != 0 {
		t.Fatal("lock held must mean no request")
	}
}

func TestAcquireTakesOverStaleLock(t *testing.T) {
	setup(t)
	now := time.Now()
	ttl := 2 * time.Minute
	lock := paths.UsageLock()
	if !acquire(lock, now, ttl) {
		t.Fatal("first acquire failed")
	}
	if acquire(lock, now.Add(ttl/2), ttl) {
		t.Fatal("a lock younger than ttl was taken over")
	}
	// The file's real mtime is "now"; only a later clock makes it stale.
	if !acquire(lock, now.Add(ttl+time.Second), ttl) {
		t.Fatal("a lock older than ttl was not taken over")
	}
	release(lock)
	if _, err := os.Stat(lock); !os.IsNotExist(err) {
		t.Fatal("release left the lock behind")
	}
}

func TestFetchedAtMs(t *testing.T) {
	if got := FetchedAtMs([]byte(`{"cachedUsageUtilization":{"fetchedAtMs":2000,"utilization":{}}}`)); got != 2000 {
		t.Errorf("FetchedAtMs = %d, want 2000", got)
	}
	for _, none := range [][]byte{nil, []byte(``), []byte(`{"unrelated":true}`), []byte(`garbage`)} {
		if got := FetchedAtMs(none); got != 0 {
			t.Errorf("FetchedAtMs(%q) = %d, want 0", none, got)
		}
	}
}

func TestValidateRequiresLimitsList(t *testing.T) {
	if _, ok := validate(fixture(t)); !ok {
		t.Error("the real response should validate")
	}
	for _, bad := range []string{`{}`, `{"limits":null}`, `[]`, `"text"`, `not json`, ``} {
		if _, ok := validate([]byte(bad)); ok {
			t.Errorf("validate(%q) accepted", bad)
		}
	}
}

func TestReadBoundedRejectsOversizeAndMissing(t *testing.T) {
	dir := t.TempDir()
	big := filepath.Join(dir, "big")
	if err := os.WriteFile(big, make([]byte, 100), 0o600); err != nil {
		t.Fatal(err)
	}
	if readBounded(big, 99) != nil {
		t.Error("oversize file should read as absent")
	}
	if readBounded(big, 100) == nil {
		t.Error("file at the cap should be read")
	}
	if readBounded(filepath.Join(dir, "missing"), 100) != nil {
		t.Error("missing file should read as absent")
	}
	if readBounded(dir, 1<<20) != nil {
		t.Error("a directory should read as absent")
	}
}

func isWindows() bool { return os.PathSeparator == '\\' }
