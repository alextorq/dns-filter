package allow_domain_use_cases_clear_events

import (
	"errors"
	"testing"
)

type fakeRepo struct {
	calledWith int
	err        error
}

func (f *fakeRepo) DeleteOlderThan(days int) error {
	f.calledWith = days
	return f.err
}

func TestClearTask_DelegatesWithRetentionDays(t *testing.T) {
	repo := &fakeRepo{}
	if err := clearTask(repo); err != nil {
		t.Fatalf("err: %v", err)
	}
	if repo.calledWith != RetentionDays {
		t.Errorf("expected days=%d, got %d", RetentionDays, repo.calledWith)
	}
}

func TestClearTask_PropagatesError(t *testing.T) {
	boom := errors.New("db down")
	repo := &fakeRepo{err: boom}
	if err := clearTask(repo); !errors.Is(err, boom) {
		t.Errorf("expected %v, got %v", boom, err)
	}
}
