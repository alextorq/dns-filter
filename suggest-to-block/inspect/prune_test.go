package inspect

import (
	"context"
	"testing"
	"time"
)

type pruneTestRepo struct {
	calls int
}

func (r *pruneTestRepo) DeleteOlderThan(time.Time) error {
	r.calls++
	return nil
}

type pruneTestLogger struct{}

func (pruneTestLogger) Error(error) {}

func TestStartPrune_PreCanceledContextSkipsRepo(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	repo := &pruneTestRepo{}

	StartPrune(ctx, repo, 7*24*time.Hour, pruneTestLogger{})

	if repo.calls != 0 {
		t.Fatalf("DeleteOlderThan calls = %d, want 0", repo.calls)
	}
}
