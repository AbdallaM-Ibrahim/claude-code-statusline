package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

// Ahead/behind is by far the most expensive thing line 1 does: it walks both
// commit ancestries and reads every object along the way, which measured ~62ms
// against a repository with one packfile and 103 loose objects.
//
// The answer depends on exactly two inputs — the HEAD hash and the upstream ref
// hash — and both are cheap to resolve. So the counts are cached against that
// pair. Neither moving means the answer cannot have changed, which makes this a
// correctness-preserving cache rather than a staleness tradeoff: it is keyed on
// the full input, not on a timer.

type aheadBehindCache struct {
	Entries map[string]aheadBehindEntry `json:"entries"`
}

type aheadBehindEntry struct {
	Ahead  int   `json:"ahead"`
	Behind int   `json:"behind"`
	At     int64 `json:"at"`
}

func gitCachePath() string {
	return filepath.Join(claudeDir(), "statusline-git-cache.json")
}

func loadAheadBehindCache() *aheadBehindCache {
	c := &aheadBehindCache{Entries: map[string]aheadBehindEntry{}}
	data, err := os.ReadFile(gitCachePath())
	if err != nil {
		return c
	}
	if err := json.Unmarshal(data, c); err != nil || c.Entries == nil {
		return &aheadBehindCache{Entries: map[string]aheadBehindEntry{}}
	}
	return c
}

func (c *aheadBehindCache) save() {
	// Entries are keyed by commit hashes, so a branch that moves leaves its old
	// key behind forever. Drop anything untouched for a week.
	cutoff := time.Now().Add(-7 * 24 * time.Hour).Unix()
	for k, v := range c.Entries {
		if v.At < cutoff {
			delete(c.Entries, k)
		}
	}

	data, err := json.Marshal(c)
	if err != nil {
		return
	}
	path := gitCachePath()
	tmp := path + "." + strconv.Itoa(os.Getpid()) + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
	}
}

func aheadBehindKey(repoPath, head, upstream string) string {
	return repoPath + "|" + head + "|" + upstream
}
