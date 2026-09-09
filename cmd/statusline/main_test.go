package main

import "testing"

// Claude Code passes no arguments, so this exists purely so a binary someone
// downloaded can say what it is.
func TestVersionRequest(t *testing.T) {
	for _, args := range [][]string{
		{"statusline", "--version"},
		{"statusline", "-version"},
		{"statusline", "version"},
		{"statusline", "-V"},
	} {
		if !versionRequest(args) {
			t.Errorf("%v should ask for the version", args[1:])
		}
	}
	for _, args := range [][]string{
		{"statusline"},
		{"statusline", "--padding"},
		{"statusline", ""},
	} {
		if versionRequest(args) {
			t.Errorf("%v should render, not print a version", args[1:])
		}
	}
}

// An unstamped build must still answer, rather than printing an empty line.
func TestVersionDefaultIsNotEmpty(t *testing.T) {
	if version == "" {
		t.Error("version must never be empty")
	}
}
