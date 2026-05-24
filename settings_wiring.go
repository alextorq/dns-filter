package main

import (
	"strconv"
	"strings"

	"github.com/alextorq/dns-filter/config"
	"github.com/alextorq/dns-filter/dns"
	dns_cache "github.com/alextorq/dns-filter/dns-cache"
	"github.com/alextorq/dns-filter/logger"
	"github.com/alextorq/dns-filter/settings"
)

// dynamicSettingsDeps bundles the runtime sinks that the DB-backed settings
// push their values into. It is assembled at the composition root where every
// sink already exists.
type dynamicSettingsDeps struct {
	conf      *config.Config
	logr      *logger.ChanLogger
	resolver  *dns.ReloadableResolver
	cache     *dns_cache.CacheWithMetrics
	dnsServer *dns.DnsServer
}

// registerDynamicSettings declares the canonical set of DB-backed runtime
// settings, binding each to its env/compiled default (the fallback layer when
// no DB override exists) and an Apply hook that pushes the value into the
// running process. Both startup hydration and runtime changes go through the
// same Apply, so adding a new dynamic setting is a single descriptor here plus
// a setter on the relevant sink.
//
// Secrets (admin password, API keys) and boot-time-only knobs (DB path, mode,
// listen ports) are deliberately absent — they stay env-only. See
// ARCHITECTURE.md for the static/dynamic/secret classification.
func registerDynamicSettings(m *settings.Module, d dynamicSettingsDeps) {
	c := d.conf
	m.Register(
		settings.Setting{
			Key:      "log_level",
			Type:     "enum",
			Enum:     logger.Levels[:],
			Default:  d.logr.GetLogLevel(),
			Validate: func(raw string) error { _, err := logger.LogLevelFromStringOrError(raw); return err },
			Apply:    func(raw string) error { d.logr.UpdateLogLevel(raw); return nil },
		},
		settings.Setting{
			Key:      "doh_upstream",
			Type:     "url",
			Default:  c.DoHUpstream,
			Validate: settings.ValidateHTTPURL,
			Apply: func(raw string) error {
				d.resolver.SetEndpoint(strings.TrimSpace(raw))
				// Answers cached from the previous upstream may differ; flush so
				// the new resolver's view takes over immediately.
				d.cache.Clear()
				return nil
			},
		},
		settings.Setting{
			Key:  "doh_bootstrap_ips",
			Type: "ip-list",
			// Show the IPs the resolver will actually bootstrap with for the
			// configured upstream (Cloudflare → built-in defaults), not the raw
			// (often empty) env value.
			Default:  strings.Join(dns.EffectiveBootstrapIPs(c.DoHUpstream, c.DoHBootstrapIPs), ","),
			Validate: settings.ValidateIPList,
			Apply: func(raw string) error {
				d.resolver.SetBootstrapIPs(settings.ParseIPList(raw))
				d.cache.Clear()
				return nil
			},
		},
		settings.Setting{
			Key:      "cache_swr",
			Type:     "bool",
			Default:  strconv.FormatBool(c.CacheSWR),
			Validate: settings.ValidateBool,
			Apply:    func(raw string) error { d.dnsServer.SetSWR(settings.ParseBool(raw)); return nil },
		},
		settings.Setting{
			Key:      "cache_stale_grace",
			Type:     "duration",
			Default:  c.CacheStaleGrace.String(),
			Validate: settings.ValidateDuration,
			Apply:    func(raw string) error { d.cache.SetStaleGrace(settings.ParseDuration(raw)); return nil },
		},
		settings.Setting{
			Key:      "cache_stale_ttl",
			Type:     "duration",
			Default:  c.CacheStaleTTL.String(),
			Validate: settings.ValidateDuration,
			Apply:    func(raw string) error { d.cache.SetStaleTTL(settings.ParseDuration(raw)); return nil },
		},
		settings.Setting{
			Key:      "cache_refresh_concurrency",
			Type:     "int",
			Default:  strconv.Itoa(c.CacheRefreshConcurrency),
			Validate: settings.ValidatePositiveInt,
			Apply:    func(raw string) error { d.dnsServer.SetRefreshConcurrency(settings.ParseInt(raw)); return nil },
		},
	)
}
