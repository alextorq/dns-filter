package background

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestNewRunner_RejectsMissingJobs(t *testing.T) {
	if _, err := NewRunner(); err == nil || !strings.Contains(err.Error(), "at least one job") {
		t.Fatalf("NewRunner() error = %v, want missing-job error", err)
	}
}

func TestNewRunner_RejectsNilJob(t *testing.T) {
	if _, err := NewRunner(nil); err == nil || !strings.Contains(err.Error(), "job 0") {
		t.Fatalf("NewRunner(nil) error = %v, want indexed nil-job error", err)
	}
}

type pointerJob struct{}

func (*pointerJob) Run(context.Context) {}

func TestNewRunner_RejectsTypedNilJob(t *testing.T) {
	var job *pointerJob
	if _, err := NewRunner(job); err == nil || !strings.Contains(err.Error(), "job 0") {
		t.Fatalf("NewRunner(typed nil) error = %v, want indexed nil-job error", err)
	}
}

func TestRunner_RunStartsAllJobsAndWaitsForThem(t *testing.T) {
	started := make(chan int, 2)
	stopped := make(chan int, 2)
	job := func(id int) JobFunc {
		return func(ctx context.Context) {
			started <- id
			<-ctx.Done()
			stopped <- id
		}
	}
	runner, err := NewRunner(job(1), job(2))
	if err != nil {
		t.Fatalf("NewRunner failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		runner.Run(ctx)
		close(done)
	}()

	seen := map[int]bool{}
	for range 2 {
		select {
		case id := <-started:
			seen[id] = true
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for jobs to start")
		}
	}
	if !seen[1] || !seen[2] {
		t.Fatalf("started jobs = %v, want both jobs", seen)
	}
	select {
	case <-done:
		t.Fatal("Runner.Run returned while jobs were still running")
	default:
	}

	cancel()
	for range 2 {
		select {
		case <-stopped:
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for jobs to stop")
		}
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Runner.Run did not wait for stopped jobs")
	}
}

func TestRunner_PreCanceledContextSkipsJobs(t *testing.T) {
	called := false
	runner, err := NewRunner(JobFunc(func(context.Context) { called = true }))
	if err != nil {
		t.Fatalf("NewRunner failed: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	runner.Run(ctx)

	if called {
		t.Fatal("pre-canceled runner started a job")
	}
}

func TestNewRunner_CopiesInjectedJobSlice(t *testing.T) {
	firstCalls, replacementCalls := 0, 0
	jobs := []Job{JobFunc(func(context.Context) { firstCalls++ })}
	runner, err := NewRunner(jobs...)
	if err != nil {
		t.Fatalf("NewRunner failed: %v", err)
	}
	jobs[0] = JobFunc(func(context.Context) { replacementCalls++ })

	runner.Run(context.Background())

	if firstCalls != 1 || replacementCalls != 0 {
		t.Fatalf("calls after input mutation: first=%d replacement=%d", firstCalls, replacementCalls)
	}
}
