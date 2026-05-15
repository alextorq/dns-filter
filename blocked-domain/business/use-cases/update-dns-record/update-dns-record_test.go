package update_dns_record

import (
	"errors"
	"testing"

	"github.com/alextorq/dns-filter/blocked-domain/db"
)

type fakeRepo struct {
	rec       *db.BlockList
	getErr    error
	updateErr error
	saved     *db.BlockList
}

func (f *fakeRepo) GetByID(id uint) (*db.BlockList, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	return f.rec, nil
}

func (f *fakeRepo) UpdateBlockList(rec *db.BlockList) error {
	if f.updateErr != nil {
		return f.updateErr
	}
	f.saved = rec
	return nil
}

type nopLog struct {
	errors []error
}

func (l *nopLog) Info(args ...any) {}
func (l *nopLog) Error(err error)  { l.errors = append(l.errors, err) }

func TestUpdateDnsRecord_HappyPath(t *testing.T) {
	rec := &db.BlockList{ID: 42, Url: "x.example", Active: true}
	repo := &fakeRepo{rec: rec}

	got, err := UpdateDnsRecord(Deps{Repo: repo, Log: &nopLog{}},
		UpdateBlockList{ID: 42, Active: false})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Active {
		t.Error("expected Active=false after update")
	}
	if repo.saved == nil || repo.saved.ID != 42 || repo.saved.Active {
		t.Errorf("repo not saved correctly: %+v", repo.saved)
	}
}

func TestUpdateDnsRecord_NotFound(t *testing.T) {
	repo := &fakeRepo{getErr: errors.New("record not found")}
	log := &nopLog{}

	_, err := UpdateDnsRecord(Deps{Repo: repo, Log: log},
		UpdateBlockList{ID: 1, Active: false})
	if err == nil {
		t.Fatal("expected error when GetByID fails")
	}
	if len(log.errors) != 1 {
		t.Errorf("expected one logged error, got %d", len(log.errors))
	}
	if repo.saved != nil {
		t.Error("repo must not be saved when GetByID fails")
	}
}

func TestUpdateDnsRecord_SaveError(t *testing.T) {
	rec := &db.BlockList{ID: 1, Url: "x", Active: true}
	boom := errors.New("disk full")
	repo := &fakeRepo{rec: rec, updateErr: boom}
	log := &nopLog{}

	_, err := UpdateDnsRecord(Deps{Repo: repo, Log: log},
		UpdateBlockList{ID: 1, Active: false})
	if !errors.Is(err, boom) {
		t.Errorf("expected wrapped %v, got %v", boom, err)
	}
	if len(log.errors) != 1 {
		t.Errorf("expected one logged error, got %d", len(log.errors))
	}
}
