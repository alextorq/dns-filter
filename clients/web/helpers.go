package web

// boolOr resolves an optional bool request field: it returns *p when the client
// supplied the field, or def when it was omitted (p == nil). Centralises the
// "pointer means tri-state, nil takes the schema default" idiom shared by the
// optional-bool DTO fields (CreateClientRequest.Filtered, DiscoverRequest.FilterDocker).
func boolOr(p *bool, def bool) bool {
	if p != nil {
		return *p
	}
	return def
}
