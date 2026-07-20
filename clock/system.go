// Package clock contains production adapters for consumer-owned wall-clock
// ports. Feature packages declare the narrow Clock interface they need; main
// injects one System instance into the complete application graph.
package clock

import "time"

// System is the production wall clock.
type System struct{}

// Now returns the current local wall time.
func (System) Now() time.Time { return time.Now() }

// NewSystem constructs the production wall clock adapter.
func NewSystem() System { return System{} }
