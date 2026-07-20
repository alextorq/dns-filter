package pause_filter

import (
	"errors"
	"slices"
	"time"
)

// AllowedMinutes is the whitelist of pause durations exposed to the UI.
// Server-side validation matches the frontend select options.
var AllowedMinutes = []int{5, 10, 15, 30}

var (
	ErrInvalidDuration = errors.New("pause duration must be one of 5, 10, 15, 30 minutes")
	ErrFilterDisabled  = errors.New("cannot pause: filter is already disabled")
)

type Logger interface {
	Info(args ...any)
}

type RuntimeState interface {
	Enabled() bool
	PausedUntil() int64
	SetPausedUntil(int64)
	ClearPause() bool
}

func isAllowed(minutes int) bool {
	return slices.Contains(AllowedMinutes, minutes)
}

// PauseFilter pauses filtering for the given number of minutes by storing an
// absolute unix-second deadline. Returns the deadline, or ErrInvalidDuration
// if the duration is not whitelisted, or ErrFilterDisabled if the filter is
// already off (pause has no meaning then). Last writer wins under concurrent
// successful calls.
func PauseFilter(state RuntimeState, log Logger, minutes int, now time.Time) (int64, error) {
	if !isAllowed(minutes) {
		return 0, ErrInvalidDuration
	}
	if !state.Enabled() {
		return 0, ErrFilterDisabled
	}
	until := now.Add(time.Duration(minutes) * time.Minute).Unix()
	state.SetPausedUntil(until)
	log.Info("Filter paused for", minutes, "minutes, until unix:", until)
	return until, nil
}

// ResumeFilter clears any active pause. Safe to call when not paused.
func ResumeFilter(state RuntimeState, log Logger) {
	if state.ClearPause() {
		log.Info("Filter pause cleared")
	}
}

// GetPausedUntil returns the active pause deadline (unix seconds), or 0 if no
// pause is active or the deadline has already passed.
func GetPausedUntil(state RuntimeState, now time.Time) int64 {
	until := state.PausedUntil()
	if until <= now.Unix() {
		return 0
	}
	return until
}
