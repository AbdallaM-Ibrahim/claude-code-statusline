package main

import (
	"context"
	"time"
)

func readGitForBench() (*GitState, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return readGit(ctx, testRepo)
}

func nowForBench() time.Time { return time.Now() }
