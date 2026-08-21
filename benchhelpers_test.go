package main

import (
	"context"
	"time"
)

func readGitForBench(repo string) (*GitState, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return readGit(ctx, repo)
}

func nowForBench() time.Time { return time.Now() }
