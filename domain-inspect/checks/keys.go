package checks

import "sync/atomic"

// VT и SB ключи держатся в атомиках, а не читаются из config.GetConfig() на
// каждом запросе: тогда дескриптор настройки в settings_wiring.go может
// переписать значение в рантайме (Apply) — без рестарта и без гонок с
// обращающимися к ключу горутинами адаптера/воркера.
//
// Стартовое значение (env-default) кладётся в атомик хайдрейтом настроек
// (HydrateAll → Apply), поэтому до запуска воркер-горутины обе переменные
// уже соответствуют либо БД-override, либо env-default.
var (
	vtKey atomic.Pointer[string]
	sbKey atomic.Pointer[string]
)

// SetVTKey атомарно заменяет VT-ключ. Вызывается из Apply-хука настройки
// virustotal_key.
func SetVTKey(k string) {
	v := k
	vtKey.Store(&v)
}

// GetVTKey возвращает текущий VT-ключ или пустую строку, если не задан.
func GetVTKey() string {
	p := vtKey.Load()
	if p == nil {
		return ""
	}
	return *p
}

// SetSBKey — то же для Safe Browsing.
func SetSBKey(k string) {
	v := k
	sbKey.Store(&v)
}

func GetSBKey() string {
	p := sbKey.Load()
	if p == nil {
		return ""
	}
	return *p
}

// HasAnyKey — true, если задан хотя бы один из ключей. Воркер inspect использует
// это, чтобы не тратить тики и квоту, когда обе провайдерские проверки
// гарантированно вернут "skipped".
func HasAnyKey() bool { return GetVTKey() != "" || GetSBKey() != "" }
