package main

import (
	"testing"

	"github.com/alextorq/dns-filter/config"
	"github.com/alextorq/dns-filter/settings"
	traffic_prune "github.com/alextorq/dns-filter/traffic/business/use-cases/prune"
)

// fakeSettingsRepo is a minimal in-memory settings.Repo for the wiring tests.
type fakeSettingsRepo struct{ data map[string]string }

func newFakeSettingsRepo() *fakeSettingsRepo { return &fakeSettingsRepo{data: map[string]string{}} }

func (f *fakeSettingsRepo) Get(key string) (string, bool, error) {
	v, ok := f.data[key]
	return v, ok, nil
}
func (f *fakeSettingsRepo) Set(key, value string) error { f.data[key] = value; return nil }
func (f *fakeSettingsRepo) Delete(key string) error     { delete(f.data, key); return nil }

// The descriptor's Validate must accept sane windows and reject 0, negatives,
// and the upper-bound + 1 — the same bounds as ValidateIntRange(1, 3650).
func TestTrafficRetentionSetting_ValidateBounds(t *testing.T) {
	s := trafficRetentionSetting(&config.Config{TrafficRetentionDays: 30})

	for _, ok := range []string{"30", "1", "3650"} {
		if err := s.Validate(ok); err != nil {
			t.Errorf("%q should be valid: %v", ok, err)
		}
	}
	for _, bad := range []string{"0", "-1", "3651"} {
		if err := s.Validate(bad); err == nil {
			t.Errorf("%q must be rejected", bad)
		}
	}
}

// The env/compiled default is sourced from config.TrafficRetentionDays.
func TestTrafficRetentionSetting_DefaultFromConfig(t *testing.T) {
	s := trafficRetentionSetting(&config.Config{TrafficRetentionDays: 45})
	if s.Default != "45" {
		t.Errorf("Default = %q, want 45 (sourced from config)", s.Default)
	}
}

// Persist+apply round-trip: Set validates, persists, then runs Apply, which
// must write the prune atomic so the next prune tick reads the new value.
func TestTrafficRetentionSetting_SetWritesAtomic(t *testing.T) {
	traffic_prune.SetRetentionDays(30)
	repo := newFakeSettingsRepo()
	m := settings.NewModule(repo)
	m.Register(trafficRetentionSetting(&config.Config{TrafficRetentionDays: 30}))

	if err := m.Set("traffic_retention_days", "7"); err != nil {
		t.Fatalf("set: %v", err)
	}
	if repo.data["traffic_retention_days"] != "7" {
		t.Errorf("expected 7 persisted, got %q", repo.data["traffic_retention_days"])
	}
	if got := traffic_prune.GetRetentionDays(); got != 7 {
		t.Errorf("Apply must write the prune atomic, got %d want 7", got)
	}
}

// An invalid value must be neither persisted nor applied to the atomic.
func TestTrafficRetentionSetting_SetInvalid_NotAppliedNotPersisted(t *testing.T) {
	traffic_prune.SetRetentionDays(30)
	repo := newFakeSettingsRepo()
	m := settings.NewModule(repo)
	m.Register(trafficRetentionSetting(&config.Config{TrafficRetentionDays: 30}))

	if err := m.Set("traffic_retention_days", "0"); err == nil {
		t.Fatal("expected 0 to be rejected")
	}
	if _, ok := repo.data["traffic_retention_days"]; ok {
		t.Error("invalid value must not be persisted")
	}
	if got := traffic_prune.GetRetentionDays(); got != 30 {
		t.Errorf("invalid value must not touch the atomic, got %d want 30", got)
	}
}

// HydrateAll at boot must apply the DB override over the env default.
func TestTrafficRetentionSetting_HydrateDBOverridesEnv(t *testing.T) {
	traffic_prune.SetRetentionDays(999) // poison so we can prove hydrate wrote it
	repo := newFakeSettingsRepo()
	repo.data["traffic_retention_days"] = "60" // operator override
	m := settings.NewModule(repo)
	m.Register(trafficRetentionSetting(&config.Config{TrafficRetentionDays: 30})) // env default 30

	if err := m.HydrateAll(); err != nil {
		t.Fatalf("hydrate: %v", err)
	}
	if got := traffic_prune.GetRetentionDays(); got != 60 {
		t.Errorf("hydrate must apply DB override 60 over env default 30, got %d", got)
	}
}

// HydrateAll with no override applies the env/compiled default.
func TestTrafficRetentionSetting_HydrateFallsBackToEnvDefault(t *testing.T) {
	traffic_prune.SetRetentionDays(999)
	repo := newFakeSettingsRepo() // empty: no override
	m := settings.NewModule(repo)
	m.Register(trafficRetentionSetting(&config.Config{TrafficRetentionDays: 30}))

	if err := m.HydrateAll(); err != nil {
		t.Fatalf("hydrate: %v", err)
	}
	if got := traffic_prune.GetRetentionDays(); got != 30 {
		t.Errorf("hydrate must apply env default 30 when no override, got %d", got)
	}
}
