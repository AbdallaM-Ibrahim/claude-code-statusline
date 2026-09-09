// Package testutil holds fixtures shared by more than one package's tests: the
// repository the git tests read, synthetic transcripts for the cost scan, and
// the injection assertion both status lines are checked against.
//
// It is a normal (non-_test) package so it can be imported from _test files in
// several packages; nothing outside tests should import it.
package testutil

import (
	"os"
	"path/filepath"
	"testing"
)

// RepoPath resolves the repository the git tests and benchmarks read:
//
//	STATUSLINE_TEST_REPO   an explicit choice — point it at a large history to
//	                       make the ahead/behind benchmark say something
//	this module's own repo  every clone of this project is a git repository, so a
//	                       fresh checkout needs no setup at all
//	""                     nothing usable, and the caller skips
//
// This used to be a const naming a directory on the author's machine, which
// meant every git test silently skipped for everybody else.
func RepoPath() string {
	if p := os.Getenv("STATUSLINE_TEST_REPO"); p != "" {
		if isGitRepo(p) {
			return p
		}
		return ""
	}
	wd, err := os.Getwd()
	if err != nil {
		return ""
	}
	for dir := wd; ; {
		if isGitRepo(dir) {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

// isGitRepo accepts both a .git directory and the .git file a worktree gets.
func isGitRepo(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, ".git"))
	return err == nil
}

// Repo is RepoPath with the skip attached, for tests and benchmarks alike.
func Repo(tb testing.TB) string {
	tb.Helper()
	p := RepoPath()
	if p == "" {
		tb.Skip("no git repository found; set STATUSLINE_TEST_REPO to one")
	}
	return p
}
