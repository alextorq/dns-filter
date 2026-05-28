package main

import (
	"strings"
	"testing"

	"github.com/alextorq/dns-filter/config"
	"github.com/alextorq/dns-filter/domain-inspect/checks"
	"github.com/alextorq/dns-filter/logger"
	"github.com/alextorq/dns-filter/settings"
	suggest_inspect "github.com/alextorq/dns-filter/suggest-to-block/inspect"
	traffic_prune "github.com/alextorq/dns-filter/traffic/business/use-cases/prune"
)

// wiringDepsForRegister собирает минимальные зависимости для
// registerDynamicSettings: дескрипторы строятся без обращения к dns/cache-
// sink'ам (они нужны только в Apply), но `log_level.Default` дергает
// logger.GetLogLevel() в момент Register — поэтому реальный logger всё-таки
// нужен. Это безопасный синглтон, инициализированный пакетом.
func wiringDepsForRegister(c *config.Config) dynamicSettingsDeps {
	return dynamicSettingsDeps{
		conf: c,
		logr: logger.GetLogger(),
	}
}

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

// === suggest_inspect_enabled =================================================

// findDescriptor — линейная замена «дай дескриптор по ключу» для тестов: не
// тащим внутрь реестр сборки воркера, просто берём слайс. Если ключ не найден
// — это сильнее, чем «дефолт сравнить»: тест явно упадёт.
func findDescriptor(t *testing.T, set []settings.Setting, key string) settings.Setting {
	t.Helper()
	for _, s := range set {
		if s.Key == key {
			return s
		}
	}
	t.Fatalf("дескриптор %q не зарегистрирован", key)
	return settings.Setting{}
}

// collectAllDescriptors дергает registerDynamicSettings и возвращает все
// дескрипторы из реестра в порядке регистрации, чтобы из тестов было удобно
// проверять конкретные ключи.
func collectAllDescriptors(t *testing.T, c *config.Config) []settings.Setting {
	t.Helper()
	repo := newFakeSettingsRepo()
	m := settings.NewModule(repo)
	// Sink'и dns/cache не нужны на этапе Register — они вызываются только из
	// Apply, который тесты ниже не запускают для соответствующих ключей.
	registerDynamicSettings(m, wiringDepsForRegister(c))
	list, err := m.List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	// Возвращаем как []Setting, восстановив их обратно из эффективных значений
	// — но проще пересобрать: List возвращает Effective, не Setting. Заменим
	// через прямой Get? Module не отдаёт сами дескрипторы. Используем List как
	// доказательство регистрации, а Validate/Apply возьмём через Set.
	out := make([]settings.Setting, 0, len(list))
	for _, e := range list {
		// type/enum/default из Effective, validate/apply через Set m
		s := settings.Setting{Key: e.Key, Type: e.Type, Enum: e.Enum, Default: e.Default}
		out = append(out, s)
	}
	return out
}

// suggest_inspect_enabled: env=true сериализуется как "true" в Default, env=false
// → "false". Этого достаточно, чтобы доказать, что дескриптор смотрит на
// config.SuggestInspectEnabled.
func TestSuggestInspectEnabled_DefaultFromConfig(t *testing.T) {
	all := collectAllDescriptors(t, &config.Config{SuggestInspectEnabled: true})
	s := findDescriptor(t, all, "suggest_inspect_enabled")
	if s.Default != "true" {
		t.Errorf("default при env=true должен быть %q, got %q", "true", s.Default)
	}
	if s.Type != "bool" {
		t.Errorf("type должен быть bool, got %q", s.Type)
	}

	all = collectAllDescriptors(t, &config.Config{SuggestInspectEnabled: false})
	s = findDescriptor(t, all, "suggest_inspect_enabled")
	if s.Default != "false" {
		t.Errorf("default при env=false должен быть %q, got %q", "false", s.Default)
	}
}

// Set("true"/"false") валидно; всё прочее отклоняется — это страховка от
// случайного присвоения типа "enum" с строковыми значениями.
func TestSuggestInspectEnabled_ValidateBool(t *testing.T) {
	repo := newFakeSettingsRepo()
	m := settings.NewModule(repo)
	registerDynamicSettings(m, wiringDepsForRegister(&config.Config{SuggestInspectEnabled: false}))

	if err := m.Set("suggest_inspect_enabled", "true"); err != nil {
		t.Errorf("true должно проходить: %v", err)
	}
	if err := m.Set("suggest_inspect_enabled", "false"); err != nil {
		t.Errorf("false должно проходить: %v", err)
	}
	if err := m.Set("suggest_inspect_enabled", "maybe"); err == nil {
		t.Error("«maybe» должно отклоняться")
	}
}

// === virustotal_key / safebrowsing_key =======================================

// Default обоих ключей берётся из env (через config.Config). Маски здесь
// неприменимы — Default не выходит через collectAllDescriptors маскированным
// (мы читаем через List, которая уже маскирует, поэтому проверяем по форме
// маски, а не по точному совпадению).
func TestVTSBSecrets_DefaultMaskedFromConfig(t *testing.T) {
	all := collectAllDescriptors(t, &config.Config{
		VirusTotalKey:   "abcdefghijklmnopqrstuvWXYZ123456", // последние 4: "3456"
		SafeBrowsingKey: "0000111122223333",                 // последние 4: "3333"
	})

	vt := findDescriptor(t, all, "virustotal_key")
	if vt.Type != settings.SecretType {
		t.Errorf("VT type должен быть secret, got %q", vt.Type)
	}
	if !strings.HasSuffix(vt.Default, "3456") {
		t.Errorf("VT default должен заканчиваться последними 4 символами оригинала, got %q", vt.Default)
	}
	if strings.Contains(vt.Default, "abcdef") {
		t.Errorf("VT default не должен содержать оригинальный префикс, got %q", vt.Default)
	}

	sb := findDescriptor(t, all, "safebrowsing_key")
	if !strings.HasSuffix(sb.Default, "3333") {
		t.Errorf("SB default должен заканчиваться 3333, got %q", sb.Default)
	}
	if strings.Contains(sb.Default, "0000") {
		t.Errorf("SB default не должен содержать префикс, got %q", sb.Default)
	}
}

// Set отвергает пустую строку и слишком короткие значения; принимает обычный
// ключ — и Apply записывает оригинальное (не маскированное) значение в атомик
// провайдер-чека. Без этой проверки маска протекала бы в hot-path.
func TestVTSBSecrets_SetRoundTrip(t *testing.T) {
	const realKey = "fresh-paste-of-vt-key-abcdefgh"
	repo := newFakeSettingsRepo()
	m := settings.NewModule(repo)
	registerDynamicSettings(m, wiringDepsForRegister(&config.Config{}))

	// Пустое значение: только Reset, не Set.
	if err := m.Set("virustotal_key", ""); err == nil {
		t.Error("пустое значение должно отклоняться (используйте Reset)")
	}
	// Слишком короткое.
	if err := m.Set("virustotal_key", "abc"); err == nil {
		t.Error("слишком короткое значение должно отклоняться")
	}
	// Норма.
	if err := m.Set("virustotal_key", "  "+realKey+"\n"); err != nil {
		t.Fatalf("set: %v", err)
	}
	if checks.GetVTKey() != realKey {
		t.Errorf("атомик VT должен содержать trim-нутый исходник, got %q", checks.GetVTKey())
	}

	if err := m.Set("safebrowsing_key", realKey); err != nil {
		t.Fatalf("set sb: %v", err)
	}
	if checks.GetSBKey() != realKey {
		t.Errorf("атомик SB должен содержать ключ, got %q", checks.GetSBKey())
	}

	// Reset — освобождает override; Apply отправляет env-default (пустую
	// строку в этом тесте) в атомик.
	if err := m.Reset("virustotal_key"); err != nil {
		t.Fatalf("reset: %v", err)
	}
	if checks.GetVTKey() != "" {
		t.Errorf("после Reset атомик должен совпадать с env-default (\"\"), got %q", checks.GetVTKey())
	}
}

// === suggest_inspect_enabled Apply пишет в атомик =============================

// Set прокачивает атомик пакета inspect: иначе UI-тогл изменит БД, но воркер
// продолжит работать (или, наоборот, останется выключенным).
func TestSuggestInspectEnabled_ApplyWritesAtomic(t *testing.T) {
	// Запоминаем исходное, чтобы не протекать состояние между тестами.
	prev := suggest_inspect.IsEnabled()
	t.Cleanup(func() { suggest_inspect.SetEnabled(prev) })

	repo := newFakeSettingsRepo()
	m := settings.NewModule(repo)
	registerDynamicSettings(m, wiringDepsForRegister(&config.Config{}))

	if err := m.Set("suggest_inspect_enabled", "true"); err != nil {
		t.Fatalf("set true: %v", err)
	}
	if !suggest_inspect.IsEnabled() {
		t.Error("после Set true атомик должен быть true")
	}

	if err := m.Set("suggest_inspect_enabled", "false"); err != nil {
		t.Fatalf("set false: %v", err)
	}
	if suggest_inspect.IsEnabled() {
		t.Error("после Set false атомик должен быть false")
	}
}
