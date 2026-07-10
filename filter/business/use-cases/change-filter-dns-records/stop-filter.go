package change_filter_dns_records

type Logger interface {
	Info(args ...any)
}

type RuntimeState interface {
	ToggleAndResume() bool
}

// ChangeFilterDnsRecords atomically toggles the global filter flag and clears
// any in-flight pause (otherwise a switch off→on would show "Active" in the
// UI while the deadline still suppresses blocking until it expires). Returns
// the new state.
func ChangeFilterDnsRecords(state RuntimeState, log Logger) bool {
	enabled := state.ToggleAndResume()
	log.Info("Change filter status to", enabled)
	return enabled
}
