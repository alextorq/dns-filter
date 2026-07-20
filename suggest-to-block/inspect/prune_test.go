package inspect

import (
	"context"
	"testing"
	"time"
)

type pruneTestRepo struct {
	calls  int
	cutoff time.Time
}

func (r *pruneTestRepo) DeleteOlderThan(cutoff time.Time) error {
	r.calls++
	r.cutoff = cutoff
	return nil
}

type pruneClock struct {
	now   time.Time
	after func()
}

func (c pruneClock) Now() time.Time {
	if c.after != nil {
		c.after()
	}
	return c.now
}

type pruneTestLogger struct{}

func (pruneTestLogger) Error(error) {}

func TestStartPrune_RejectsMissingClock(t *testing.T) {
	var typedNil *pruneClock
	for name, clock := range map[string]Clock{
		"nil":       nil,
		"typed nil": typedNil,
	} {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			defer func() {
				if recover() == nil {
					t.Fatal("missing clock must fail during construction")
				}
			}()
			StartPrune(ctx, &pruneTestRepo{}, time.Hour, clock, pruneTestLogger{})
		})
	}
}

func TestStartPrune_PreCanceledContextSkipsRepo(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	repo := &pruneTestRepo{}

	StartPrune(ctx, repo, 7*24*time.Hour, pruneClock{now: time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)}, pruneTestLogger{})

	if repo.calls != 0 {
		t.Fatalf("DeleteOlderThan calls = %d, want 0", repo.calls)
	}
}

func TestStartPrune_UsesInjectedClock(t *testing.T) {
	now := time.Date(2026, 7, 20, 12, 0, 0, 0, time.FixedZone("UTC+3", 3*60*60))
	repo := &pruneTestRepo{}
	ctx, cancel := context.WithCancel(context.Background())

	StartPrune(ctx, repo, 7*24*time.Hour, pruneClock{now: now, after: cancel}, pruneTestLogger{})

	if repo.calls != 1 {
		t.Fatalf("DeleteOlderThan calls = %d, want 1", repo.calls)
	}
	want := now.Add(-7 * 24 * time.Hour)
	if !repo.cutoff.Equal(want) {
		t.Fatalf("cutoff = %v, want %v", repo.cutoff, want)
	}
}
