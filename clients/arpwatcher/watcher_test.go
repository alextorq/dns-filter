package arpwatcher

import (
	"errors"
	"fmt"
	"testing"

	"github.com/alextorq/dns-filter/clients/db"
	"github.com/alextorq/dns-filter/clients/discovery"
)

type watcherRepo struct {
	rows    []db.Client
	listErr error
	updates int
}

func (r *watcherRepo) GetAll() ([]db.Client, error) {
	if r.listErr != nil {
		return nil, r.listErr
	}
	return append([]db.Client(nil), r.rows...), nil
}

func (r *watcherRepo) UpdateFields(id uint, fields map[string]any) error {
	for i := range r.rows {
		if r.rows[i].ID == id {
			r.rows[i].MAC = fields["mac"].(string)
			r.updates++
			return nil
		}
	}
	return errors.New("not found")
}

type watcherLog struct {
	warns  []string
	errors []error
}

func (*watcherLog) Info(...any)  {}
func (*watcherLog) Debug(...any) {}
func (l *watcherLog) Warn(args ...any) {
	l.warns = append(l.warns, fmt.Sprint(args...))
}
func (l *watcherLog) Error(err error) { l.errors = append(l.errors, err) }

func TestWatcherTickBackfillsMACAndSyncsExclusions(t *testing.T) {
	repo := &watcherRepo{rows: []db.Client{{ID: 7, IP: "192.168.1.10"}}}
	syncCalls := 0
	w := NewWatcher(NewCache(), repo, func() error {
		syncCalls++
		return nil
	})
	w.ReadARP = func() ([]discovery.ARPEntry, error) {
		return []discovery.ARPEntry{entry("192.168.1.10", "aa:bb:cc:dd:ee:01")}, nil
	}
	w.FilterARP = func(in []discovery.ARPEntry) []discovery.ARPEntry { return in }

	if !w.tick(&watcherLog{}) {
		t.Fatal("successful tick stopped watcher")
	}
	if repo.updates != 1 || repo.rows[0].MAC != "aa:bb:cc:dd:ee:01" {
		t.Fatalf("MAC not backfilled: rows=%+v updates=%d", repo.rows, repo.updates)
	}
	if syncCalls != 1 {
		t.Fatalf("expected one exclusion sync, got %d", syncCalls)
	}
	if got, ok := w.Cache.MAC("192.168.1.10"); !ok || got != "aa:bb:cc:dd:ee:01" {
		t.Fatalf("cache not updated: got=%q ok=%v", got, ok)
	}
}

func TestWatcherTickUnsupportedStopsLoop(t *testing.T) {
	w := NewWatcher(NewCache(), &watcherRepo{}, func() error { return nil })
	w.ReadARP = func() ([]discovery.ARPEntry, error) {
		return nil, discovery.ErrUnsupported
	}
	log := &watcherLog{}
	if w.tick(log) {
		t.Fatal("unsupported platform must stop watcher")
	}
	if len(log.warns) != 1 {
		t.Fatalf("expected one warning, got %v", log.warns)
	}
}
