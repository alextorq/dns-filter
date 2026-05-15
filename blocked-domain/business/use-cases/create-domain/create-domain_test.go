package create_domain

import (
	"errors"
	"testing"
)

type fakeRepo struct {
	notExist  bool
	created   []struct{ domain, source string }
	createErr error
}

func (f *fakeRepo) DomainNotExist(domain string) bool {
	return f.notExist
}

func (f *fakeRepo) CreateDomain(domain, source string) error {
	if f.createErr != nil {
		return f.createErr
	}
	f.created = append(f.created, struct{ domain, source string }{domain, source})
	return nil
}

type nopLog struct {
	errors []error
}

func (l *nopLog) Info(args ...any) {}
func (l *nopLog) Error(err error)  { l.errors = append(l.errors, err) }

func TestCreateDomain_HappyPath(t *testing.T) {
	repo := &fakeRepo{notExist: true}
	log := &nopLog{}

	err := CreateDomain(Deps{Repo: repo, Log: log},
		RequestBody{Domain: "ads.example", Source: "user"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(repo.created) != 1 || repo.created[0].domain != "ads.example" || repo.created[0].source != "user" {
		t.Errorf("expected create with (ads.example, user), got %+v", repo.created)
	}
}

func TestCreateDomain_RejectsEmpty(t *testing.T) {
	repo := &fakeRepo{notExist: true}
	err := CreateDomain(Deps{Repo: repo, Log: &nopLog{}}, RequestBody{Domain: ""})
	if !errors.Is(err, ErrEmptyDomain) {
		t.Fatalf("expected ErrEmptyDomain, got %v", err)
	}
	if len(repo.created) != 0 {
		t.Errorf("repo must not be called for empty domain, got %v", repo.created)
	}
}

func TestCreateDomain_RejectsExisting(t *testing.T) {
	repo := &fakeRepo{notExist: false}
	err := CreateDomain(Deps{Repo: repo, Log: &nopLog{}},
		RequestBody{Domain: "already.example"})
	if !errors.Is(err, ErrDomainAlreadyExists) {
		t.Errorf("expected ErrDomainAlreadyExists, got %v", err)
	}
	if len(repo.created) != 0 {
		t.Errorf("repo must not be called when domain exists, got %v", repo.created)
	}
}

func TestCreateDomain_WrapsRepoError(t *testing.T) {
	boom := errors.New("db down")
	repo := &fakeRepo{notExist: true, createErr: boom}
	log := &nopLog{}
	err := CreateDomain(Deps{Repo: repo, Log: log},
		RequestBody{Domain: "x.example"})
	if !errors.Is(err, boom) {
		t.Errorf("expected wrapped %v, got %v", boom, err)
	}
	if len(log.errors) != 1 {
		t.Errorf("expected one logged error, got %d", len(log.errors))
	}
}
