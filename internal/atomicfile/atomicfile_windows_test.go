package atomicfile

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// holdOpen opens path the way a sibling session reading it does: os.Open, which
// on Windows does not grant FILE_SHARE_DELETE, so the target cannot be replaced.
func holdOpen(t *testing.T, path string) *os.File {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { f.Close() })
	return f
}

func seed(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "state.json")
	if err := os.WriteFile(path, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// The bug this package exists for: another session is mid-read when the rename
// lands. A plain os.Rename fails; Write waits the reader out.
func TestWriteRetriesPastBriefReader(t *testing.T) {
	path := seed(t)
	f := holdOpen(t, path)
	probe := path + ".probe"
	if err := os.WriteFile(probe, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(probe, path); err == nil {
		t.Skip("filesystem allows replacing an open file; nothing to retry")
	}
	os.Remove(probe)
	time.AfterFunc(10*time.Millisecond, func() { f.Close() })

	if !Write(path, []byte("new"), 0o600) {
		t.Fatal("Write gave up while the reader was still inside the retry window")
	}
	if got, _ := os.ReadFile(path); string(got) != "new" {
		t.Fatalf("content = %q, want new", got)
	}
	assertNoTemp(t, filepath.Dir(path))
}

// A handle held past every retry: Write fails, the target keeps its old content,
// and the temp file is gone.
func TestWriteGivesUpOnPersistentReader(t *testing.T) {
	path := seed(t)
	holdOpen(t, path)

	start := time.Now()
	if Write(path, []byte("new"), 0o600) {
		t.Fatal("Write replaced a file that stayed open throughout")
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("gave up after %s, want well under the render deadline", elapsed)
	}
	f, err := os.ReadFile(path)
	if err != nil || string(f) != "old" {
		t.Fatalf("target = %q, %v; want untouched", f, err)
	}
	assertNoTemp(t, filepath.Dir(path))
}
