package inspect

import "sync/atomic"

// featureEnabled — рантайм-флаг включения reputation-обогащения. Запись только
// через Apply-хук дескриптора suggest_inspect_enabled (см. settings_wiring.go);
// чтение — на hot path воркера (см. Worker.RunOnce) и в suggest-to-block при
// маршрутизации в очередь inspect (см. Module.Collect).
//
// Атомик в package-level — самый легковесный способ связать BD-настройку с
// одной горутиной-воркером, не таща через NewWorker отдельный канал/мьютекс.
// Тесты воркера используют SetEnabled явно (или собственный gate-замок), так
// что zero-value=false не маскирует включённую фичу.
var featureEnabled atomic.Bool

// SetEnabled пишет новое значение флага. Вызывается:
//   - на старте: HydrateAll → Apply (effective = БД override → env default);
//   - в рантайме: PUT /api/settings/suggest_inspect_enabled → Apply.
func SetEnabled(v bool) { featureEnabled.Store(v) }

// IsEnabled читается воркером и сборщиком-suggest без блокировок.
func IsEnabled() bool { return featureEnabled.Load() }
