package change_filter

import (
	"errors"
	"testing"

	"github.com/alextorq/dns-filter/clients/db"
)

type fakeRepo struct {
	client    *db.Client
	getErr    error
	updateErr error
}

func (r *fakeRepo) GetByID(uint) (*db.Client, error) {
	if r.getErr != nil {
		return nil, r.getErr
	}
	c := *r.client
	return &c, nil
}

func (r *fakeRepo) UpdateFields(_ uint, fields map[string]any) error {
	if r.updateErr != nil {
		return r.updateErr
	}
	r.client.Filtered = fields["filtered"].(bool)
	return nil
}

type fakeStore struct{ added, removed *db.Client }

func (s *fakeStore) AddClient(c *db.Client)    { s.added = c }
func (s *fakeStore) RemoveClient(c *db.Client) { s.removed = c }

func TestChangeFilter_UpdatesDBAndStore(t *testing.T) {
	t.Run("enable filtering removes exclusion", func(t *testing.T) {
		repo := &fakeRepo{client: &db.Client{ID: 1, IP: "10.0.0.1", Filtered: false}}
		store := &fakeStore{}
		got, err := ChangeFilter(repo, store, 1, true)
		if err != nil {
			t.Fatalf("ChangeFilter: %v", err)
		}
		if !got.Filtered || store.removed == nil || store.added != nil {
			t.Fatalf("unexpected result: client=%+v store=%+v", got, store)
		}
	})

	t.Run("disable filtering adds exclusion", func(t *testing.T) {
		repo := &fakeRepo{client: &db.Client{ID: 1, IP: "10.0.0.1", Filtered: true}}
		store := &fakeStore{}
		got, err := ChangeFilter(repo, store, 1, false)
		if err != nil {
			t.Fatalf("ChangeFilter: %v", err)
		}
		if got.Filtered || store.added == nil || store.removed != nil {
			t.Fatalf("unexpected result: client=%+v store=%+v", got, store)
		}
	})
}

func TestChangeFilter_ErrorDoesNotMutateStore(t *testing.T) {
	want := errors.New("update failed")
	repo := &fakeRepo{client: &db.Client{ID: 1}, updateErr: want}
	store := &fakeStore{}
	_, err := ChangeFilter(repo, store, 1, false)
	if !errors.Is(err, want) {
		t.Fatalf("expected %v, got %v", want, err)
	}
	if store.added != nil || store.removed != nil {
		t.Fatalf("store mutated after DB failure: %+v", store)
	}
}
