package atomicfile

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestWriteCreatesAndReplaces(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	for _, want := range []string{`{"v":1}`, `{"v":2}`} {
		if !Write(path, []byte(want), 0o600) {
			t.Fatalf("Write(%s) failed", want)
		}
		got, err := os.ReadFile(path)
		if err != nil || string(got) != want {
			t.Fatalf("content = %q, %v; want %q", got, err, want)
		}
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := fi.Mode().Perm(); perm&0o077 != 0 && runtime.GOOS != "windows" {
		t.Errorf("mode = %o, want 0600", perm)
	}
	assertNoTemp(t, filepath.Dir(path))
}

// A target that cannot be replaced leaves no temp file behind.
func TestWriteFailureCleansUp(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	if err := os.MkdirAll(filepath.Join(path, "blocker"), 0o700); err != nil {
		t.Fatal(err)
	}
	if Write(path, []byte("x"), 0o600) {
		t.Fatal("Write over a non-empty directory reported success")
	}
	assertNoTemp(t, dir)
}

func assertNoTemp(t *testing.T, dir string) {
	t.Helper()
	tmps, err := filepath.Glob(filepath.Join(dir, "*.tmp"))
	if err != nil {
		t.Fatal(err)
	}
	if len(tmps) != 0 {
		t.Errorf("temp files left behind: %v", tmps)
	}
}
