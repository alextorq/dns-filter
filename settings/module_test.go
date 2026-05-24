package settings

import (
	"errors"
	"strings"
	"testing"
)

type fakeRepo struct {
	data     map[string]string
	getErr   error
	setErr   error
	delErr   error
	setCalls int
}

func newFakeRepo() *fakeRepo { return &fakeRepo{data: map[string]string{}} }

func (f *fakeRepo) Get(key string) (string, bool, error) {
	if f.getErr != nil {
		return "", false, f.getErr
	}
	v, ok := f.data[key]
	return v, ok, nil
}

func (f *fakeRepo) Set(key, value string) error {
	if f.setErr != nil {
		return f.setErr
	}
	f.setCalls++
	f.data[key] = value
	return nil
}

func (f *fakeRepo) Delete(key string) error {
	if f.delErr != nil {
		return f.delErr
	}
	delete(f.data, key)
	return nil
}

// spyApply records every value handed to Apply so tests can assert what the
// runtime sink would have received.
func spyApply(dst *[]string) func(string) error {
	return func(v string) error {
		*dst = append(*dst, v)
		return nil
	}
}

func TestModule_Set_PersistsAndApplies(t *testing.T) {
	repo := newFakeRepo()
	m := NewModule(repo)
	var applied []string
	m.Register(Setting{
		Key:      "log_level",
		Type:     "enum",
		Enum:     []string{"DEBUG", "INFO"},
		Default:  "INFO",
		Validate: ValidateEnum("DEBUG", "INFO"),
		Apply:    spyApply(&applied),
	})

	if err := m.Set("log_level", "DEBUG"); err != nil {
		t.Fatalf("set: %v", err)
	}

	if repo.data["log_level"] != "DEBUG" {
		t.Errorf("expected DEBUG persisted, got %q", repo.data["log_level"])
	}
	if len(applied) != 1 || applied[0] != "DEBUG" {
		t.Errorf("expected Apply called once with DEBUG, got %v", applied)
	}
}

func TestModule_Set_UnknownKey(t *testing.T) {
	repo := newFakeRepo()
	m := NewModule(repo)

	err := m.Set("nope", "x")
	if !errors.Is(err, ErrUnknownKey) {
		t.Fatalf("expected ErrUnknownKey, got %v", err)
	}
	if repo.setCalls != 0 {
		t.Error("unknown key must not persist")
	}
}

// An invalid value must be rejected before it reaches the DB or the runtime
// sink — otherwise we'd apply a value we refused to store and lose it on
// restart, or store something the sink can't parse.
func TestModule_Set_InvalidValue_NotPersistedNotApplied(t *testing.T) {
	repo := newFakeRepo()
	m := NewModule(repo)
	var applied []string
	m.Register(Setting{
		Key:      "doh_upstream",
		Default:  "https://cloudflare-dns.com/dns-query",
		Validate: ValidateHTTPURL,
		Apply:    spyApply(&applied),
	})

	err := m.Set("doh_upstream", "not-a-url")
	if !errors.Is(err, ErrInvalidValue) {
		t.Fatalf("expected ErrInvalidValue, got %v", err)
	}
	if repo.setCalls != 0 {
		t.Error("invalid value must not be persisted")
	}
	if len(applied) != 0 {
		t.Errorf("invalid value must not be applied, got %v", applied)
	}
}

// If persistence fails, Apply must not run: the DB is the source of truth, and
// applying a value we couldn't store would silently diverge memory from disk.
func TestModule_Set_PersistError_DoesNotApply(t *testing.T) {
	repo := newFakeRepo()
	repo.setErr = errors.New("disk full")
	m := NewModule(repo)
	var applied []string
	m.Register(Setting{
		Key:      "log_level",
		Default:  "INFO",
		Validate: ValidateEnum("DEBUG", "INFO"),
		Apply:    spyApply(&applied),
	})

	if err := m.Set("log_level", "DEBUG"); err == nil {
		t.Fatal("expected persist error to surface")
	}
	if len(applied) != 0 {
		t.Errorf("apply must not run when persist fails, got %v", applied)
	}
}

func TestModule_Reset_DeletesAndAppliesDefault(t *testing.T) {
	repo := newFakeRepo()
	repo.data["log_level"] = "DEBUG"
	m := NewModule(repo)
	var applied []string
	m.Register(Setting{
		Key:      "log_level",
		Default:  "INFO",
		Validate: ValidateEnum("DEBUG", "INFO"),
		Apply:    spyApply(&applied),
	})

	if err := m.Reset("log_level"); err != nil {
		t.Fatalf("reset: %v", err)
	}
	if _, ok := repo.data["log_level"]; ok {
		t.Error("reset must delete the override row")
	}
	if len(applied) != 1 || applied[0] != "INFO" {
		t.Errorf("reset must apply the default, got %v", applied)
	}
}

func TestModule_HydrateAll_DBOverEnv(t *testing.T) {
	repo := newFakeRepo()
	repo.data["log_level"] = "WARN" // operator-set override
	m := NewModule(repo)
	var applied []string
	m.Register(Setting{
		Key:      "log_level",
		Default:  "INFO", // env default
		Validate: ValidateEnum("DEBUG", "INFO", "WARN", "ERROR"),
		Apply:    spyApply(&applied),
	})

	if err := m.HydrateAll(); err != nil {
		t.Fatalf("hydrate: %v", err)
	}
	if len(applied) != 1 || applied[0] != "WARN" {
		t.Errorf("hydrate must apply the DB override over env default, got %v", applied)
	}
}

func TestModule_HydrateAll_FallsBackToEnvDefault(t *testing.T) {
	repo := newFakeRepo() // empty: no override
	m := NewModule(repo)
	var applied []string
	m.Register(Setting{
		Key:      "log_level",
		Default:  "INFO",
		Validate: ValidateEnum("DEBUG", "INFO"),
		Apply:    spyApply(&applied),
	})

	if err := m.HydrateAll(); err != nil {
		t.Fatalf("hydrate: %v", err)
	}
	if len(applied) != 1 || applied[0] != "INFO" {
		t.Errorf("hydrate must apply env default when no override, got %v", applied)
	}
}

// A stored value that no longer validates (e.g. an enum that shrank across a
// version bump) must not brick startup: hydrate falls back to the default,
// still applies it, and reports the problem via the returned error.
func TestModule_HydrateAll_InvalidStoredValue_FallsBackAndReports(t *testing.T) {
	repo := newFakeRepo()
	repo.data["log_level"] = "TRACE" // no longer a valid level
	m := NewModule(repo)
	var applied []string
	m.Register(Setting{
		Key:      "log_level",
		Default:  "INFO",
		Validate: ValidateEnum("DEBUG", "INFO", "WARN", "ERROR"),
		Apply:    spyApply(&applied),
	})

	err := m.HydrateAll()
	if err == nil {
		t.Fatal("expected hydrate to report the invalid stored value")
	}
	if len(applied) != 1 || applied[0] != "INFO" {
		t.Errorf("hydrate must fall back to default on invalid stored value, got %v", applied)
	}
}

func TestModule_List_ReportsOverrideAndDefault(t *testing.T) {
	repo := newFakeRepo()
	repo.data["log_level"] = "DEBUG"
	m := NewModule(repo)
	m.Register(
		Setting{Key: "log_level", Type: "enum", Enum: []string{"DEBUG", "INFO"}, Default: "INFO", Validate: ValidateEnum("DEBUG", "INFO")},
		Setting{Key: "doh_upstream", Type: "url", Default: "https://cloudflare-dns.com/dns-query", Validate: ValidateHTTPURL},
	)

	list, err := m.List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 settings, got %d", len(list))
	}

	// Registration order preserved.
	if list[0].Key != "log_level" || list[1].Key != "doh_upstream" {
		t.Errorf("expected registration order, got %q then %q", list[0].Key, list[1].Key)
	}
	if list[0].Value != "DEBUG" || !list[0].Overridden {
		t.Errorf("log_level should report DB value DEBUG and Overridden=true, got %+v", list[0])
	}
	if list[1].Value != list[1].Default || list[1].Overridden {
		t.Errorf("doh_upstream should report default and Overridden=false, got %+v", list[1])
	}
}

func TestModule_Get_UnknownKey(t *testing.T) {
	m := NewModule(newFakeRepo())
	if _, err := m.Get("nope"); !errors.Is(err, ErrUnknownKey) {
		t.Fatalf("expected ErrUnknownKey, got %v", err)
	}
}

// A repo read failure during hydration must be reported (not swallowed) and
// must not leave the process applying a value it never read.
func TestModule_HydrateAll_ReadErrorReported(t *testing.T) {
	repo := newFakeRepo()
	repo.getErr = errors.New("db down")
	m := NewModule(repo)
	var applied []string
	m.Register(Setting{Key: "log_level", Default: "INFO", Apply: spyApply(&applied)})

	if err := m.HydrateAll(); err == nil {
		t.Fatal("expected hydrate to report the read error")
	}
	if len(applied) != 0 {
		t.Errorf("apply must not run when the read failed, got %v", applied)
	}
}

// An Apply hook that fails during hydration must surface through the returned
// error rather than panic or be silently dropped.
func TestModule_HydrateAll_ApplyErrorReported(t *testing.T) {
	m := NewModule(newFakeRepo())
	m.Register(Setting{
		Key:     "doh_upstream",
		Default: "https://cloudflare-dns.com/dns-query",
		Apply:   func(string) error { return errors.New("cannot build resolver") },
	})

	err := m.HydrateAll()
	if err == nil {
		t.Fatal("expected hydrate to report the apply error")
	}
	if !strings.Contains(err.Error(), "cannot build resolver") {
		t.Errorf("error should wrap the apply failure, got %v", err)
	}
}

// If the delete fails, Reset must not apply the default — the override is still
// in the DB, so claiming a revert by applying the default would diverge memory
// from disk.
func TestModule_Reset_DeleteErrorDoesNotApply(t *testing.T) {
	repo := newFakeRepo()
	repo.data["log_level"] = "DEBUG"
	repo.delErr = errors.New("disk full")
	m := NewModule(repo)
	var applied []string
	m.Register(Setting{Key: "log_level", Default: "INFO", Apply: spyApply(&applied)})

	if err := m.Reset("log_level"); err == nil {
		t.Fatal("expected delete error to surface")
	}
	if len(applied) != 0 {
		t.Errorf("apply must not run when delete fails, got %v", applied)
	}
}

func TestModule_List_ReadErrorSurfaces(t *testing.T) {
	repo := newFakeRepo()
	repo.getErr = errors.New("db down")
	m := NewModule(repo)
	m.Register(Setting{Key: "log_level", Default: "INFO"})

	if _, err := m.List(); err == nil {
		t.Error("expected List to surface the read error")
	}
}
