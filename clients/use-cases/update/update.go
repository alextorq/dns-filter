// Package update mutates user-editable metadata on an existing client. It
// deliberately does not touch the Filtered flag — that path goes through
// change_filter, which has the matching in-memory store side effect. Keeping
// the two responsibilities apart prevents accidental store divergence when a
// caller "just wants to rename".
package update

import (
	"github.com/alextorq/dns-filter/clients/db"
)

// Input is a sparse update: nil pointers leave the field untouched. We use
// pointers (rather than empty-string sentinels) so callers can explicitly
// clear a field by passing a pointer to "".
type Input struct {
	ID       uint
	Name     *string
	Hostname *string
	Vendor   *string
}

func Update(in Input) (*db.Client, error) {
	fields := map[string]any{}
	if in.Name != nil {
		fields["name"] = *in.Name
	}
	if in.Hostname != nil {
		fields["hostname"] = *in.Hostname
	}
	if in.Vendor != nil {
		fields["vendor"] = *in.Vendor
	}
	if len(fields) > 0 {
		if err := db.UpdateClientFields(in.ID, fields); err != nil {
			return nil, err
		}
	}
	return db.GetClientByID(in.ID)
}
