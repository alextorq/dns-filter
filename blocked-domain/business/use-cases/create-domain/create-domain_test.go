package create_domain

import (
	"errors"
	"testing"
)

type createdRow struct {
	domain, source string
	reasons        []Reason
}

type fakeRepo struct {
	notExist  bool
	created   []createdRow
	createErr error
}

func (f *fakeRepo) DomainNotExist(domain string) bool {
	return f.notExist
}

func (f *fakeRepo) CreateDomain(domain, source string) error {
	if f.createErr != nil {
		return f.createErr
	}
	f.created = append(f.created, createdRow{domain: domain, source: source})
	return nil
}

func (f *fakeRepo) CreateDomainWithReasons(domain, source string, reasons []Reason) error {
	if f.createErr != nil {
		return f.createErr
	}
	f.created = append(f.created, createdRow{domain: domain, source: source, reasons: reasons})
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
	// Домен попадает в БД в канонической FQDN-форме (#30).
	if len(repo.created) != 1 || repo.created[0].domain != "ads.example." || repo.created[0].source != "user" {
		t.Errorf("expected create with (ads.example., user), got %+v", repo.created)
	}
}

// TestCreateDomain_NormalizesInput — ручной ввод в любой форме (регистр,
// пробелы, без точки) должен лечь в БД в одной канонической форме, иначе
// горячий путь DNS его не найдёт (#30).
func TestCreateDomain_NormalizesInput(t *testing.T) {
	for _, in := range []string{"Example.com", "  example.com  ", "example.com.", "EXAMPLE.COM."} {
		repo := &fakeRepo{notExist: true}
		err := CreateDomain(Deps{Repo: repo, Log: &nopLog{}}, RequestBody{Domain: in})
		if err != nil {
			t.Fatalf("input %q: unexpected error: %v", in, err)
		}
		if len(repo.created) != 1 || repo.created[0].domain != "example.com." {
			t.Errorf("input %q: expected stored domain %q, got %+v", in, "example.com.", repo.created)
		}
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

// TestCreateDomain_RejectsWhitespaceOnly — вход из одних пробелов после
// нормализации пуст и должен отклоняться так же, как пустая строка.
func TestCreateDomain_RejectsWhitespaceOnly(t *testing.T) {
	repo := &fakeRepo{notExist: true}
	err := CreateDomain(Deps{Repo: repo, Log: &nopLog{}}, RequestBody{Domain: "   "})
	if !errors.Is(err, ErrEmptyDomain) {
		t.Fatalf("expected ErrEmptyDomain, got %v", err)
	}
	if len(repo.created) != 0 {
		t.Errorf("repo must not be called for blank domain, got %v", repo.created)
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

// TestCreateDomain_WithReasons_PersistsReasons — авто-блок передаёт reasons,
// и use-case обязан уйти в CreateDomainWithReasons, донеся их до репозитория
// в канонической форме домена (#95).
func TestCreateDomain_WithReasons_PersistsReasons(t *testing.T) {
	repo := &fakeRepo{notExist: true}
	reasons := []Reason{
		{Code: "subdomain_of_blocked", Match: "example.com"},
		{Code: "suspicious_entropy"},
	}

	err := CreateDomain(Deps{Repo: repo, Log: &nopLog{}},
		RequestBody{Domain: "ads.example", Source: "AutoBlocked", Reasons: reasons})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(repo.created) != 1 {
		t.Fatalf("expected one created row, got %+v", repo.created)
	}
	got := repo.created[0]
	if got.domain != "ads.example." || got.source != "AutoBlocked" {
		t.Errorf("expected (ads.example., AutoBlocked), got (%s, %s)", got.domain, got.source)
	}
	if len(got.reasons) != 2 || got.reasons[0].Code != "subdomain_of_blocked" ||
		got.reasons[0].Match != "example.com" || got.reasons[1].Code != "suspicious_entropy" {
		t.Errorf("reasons not forwarded to repo, got %+v", got.reasons)
	}
}

// TestCreateDomain_WithReasons_RejectsExisting — негатив: домен уже в
// blocklist → запись (и его reasons) не происходит, дублей нет (#95 AC:
// идемпотентность повторного Collect).
func TestCreateDomain_WithReasons_RejectsExisting(t *testing.T) {
	repo := &fakeRepo{notExist: false}
	err := CreateDomain(Deps{Repo: repo, Log: &nopLog{}},
		RequestBody{Domain: "already.example", Source: "AutoBlocked",
			Reasons: []Reason{{Code: "subdomain_of_blocked"}}})
	if !errors.Is(err, ErrDomainAlreadyExists) {
		t.Fatalf("expected ErrDomainAlreadyExists, got %v", err)
	}
	if len(repo.created) != 0 {
		t.Errorf("repo must not be called when domain exists, got %+v", repo.created)
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
