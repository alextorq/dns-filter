// Package sourcesync runs the application workflow that synchronizes remote
// block lists and refreshes the in-memory DNS filter afterwards.
package sourcesync

import (
	"context"
	"fmt"
	"reflect"
	"time"
)

const (
	retryBaseDelay = 30 * time.Second
	retryMaxDelay  = 30 * time.Minute
)

// Syncer is the source feature port used by Job.
type Syncer interface {
	Sync(context.Context) error
}

// FilterRefresher is the filter feature port used after a successful sync.
type FilterRefresher interface {
	UpdateFromDb() error
}

// Logger is the narrow logging port required by Job.
type Logger interface {
	Info(args ...any)
	Error(error)
}

type waitFunc func(context.Context, time.Duration) error

// Job coordinates source synchronization, retry, and filter refresh. It
// implements background.Job without depending on the background package.
type Job struct {
	syncer    Syncer
	refresher FilterRefresher
	log       Logger
	wait      waitFunc
}

// New validates the feature ports and creates a source synchronization job.
func New(syncer Syncer, refresher FilterRefresher, log Logger) (*Job, error) {
	if isNilDependency(syncer) {
		return nil, fmt.Errorf("source sync job: syncer is required")
	}
	if isNilDependency(refresher) {
		return nil, fmt.Errorf("source sync job: filter refresher is required")
	}
	if isNilDependency(log) {
		return nil, fmt.Errorf("source sync job: logger is required")
	}
	return &Job{
		syncer:    syncer,
		refresher: refresher,
		log:       log,
		wait:      waitForRetry,
	}, nil
}

func isNilDependency(dependency any) bool {
	if dependency == nil {
		return true
	}
	value := reflect.ValueOf(dependency)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}

// Run retries source synchronization with capped exponential backoff. After a
// successful sync it refreshes the in-memory filter exactly once. Cancellation
// is normal shutdown control flow and therefore is not logged as a failure.
func (j *Job) Run(ctx context.Context) {
	if ctx.Err() != nil {
		return
	}
	j.log.Info("Фоновая синхронизация источников запущена")

	delay := retryBaseDelay
	for attempt := 1; ; attempt++ {
		err := j.syncer.Sync(ctx)
		if err == nil {
			break
		}
		if ctx.Err() != nil {
			return
		}
		j.log.Error(fmt.Errorf("фоновая синхронизация источников не удалась (попытка %d), повтор через %s: %w", attempt, delay, err))
		if err := j.wait(ctx, delay); err != nil {
			return
		}
		delay *= 2
		if delay > retryMaxDelay {
			delay = retryMaxDelay
		}
	}

	if ctx.Err() != nil {
		return
	}
	if err := j.refresher.UpdateFromDb(); err != nil {
		j.log.Error(fmt.Errorf("обновление фильтра после фоновой синхронизации не удалось: %w", err))
		return
	}
	if ctx.Err() != nil {
		return
	}
	j.log.Info("Фоновая синхронизация источников завершена, фильтр обновлён")
}

func waitForRetry(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
