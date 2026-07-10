// Package background coordinates process-owned, context-aware background jobs.
// Jobs keep their feature dependencies; the composition root adapts them to
// Job and starts the whole set through one Runner.
package background

import (
	"context"
	"fmt"
	"reflect"
	"sync"
)

// Job is the lifecycle port implemented by a long-running background task.
// Run must return after ctx is canceled.
type Job interface {
	Run(context.Context)
}

// JobFunc adapts a context-aware function or method to Job. It keeps feature
// packages independent from this orchestration package.
type JobFunc func(context.Context)

func (f JobFunc) Run(ctx context.Context) { f(ctx) }

// Runner owns a fixed application job set assembled at the composition root.
type Runner struct {
	jobs []Job
}

// NewRunner validates and copies the injected job set.
func NewRunner(jobs ...Job) (*Runner, error) {
	if len(jobs) == 0 {
		return nil, fmt.Errorf("background: at least one job is required")
	}
	for i, job := range jobs {
		if isNilJob(job) {
			return nil, fmt.Errorf("background: job %d is nil", i)
		}
	}
	return &Runner{jobs: append([]Job(nil), jobs...)}, nil
}

func isNilJob(job Job) bool {
	if job == nil {
		return true
	}
	v := reflect.ValueOf(job)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return v.IsNil()
	default:
		return false
	}
}

// Run starts every job concurrently and waits until all of them return. A
// pre-canceled context starts no work. Runner does not own ctx cancellation;
// process lifecycle remains the caller's responsibility.
func (r *Runner) Run(ctx context.Context) {
	select {
	case <-ctx.Done():
		return
	default:
	}

	var wg sync.WaitGroup
	wg.Add(len(r.jobs))
	for _, job := range r.jobs {
		go func() {
			defer wg.Done()
			job.Run(ctx)
		}()
	}
	wg.Wait()
}
