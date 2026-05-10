package pause_filter

import (
	"errors"
	"slices"
	"time"

	"github.com/alextorq/dns-filter/config"
	"github.com/alextorq/dns-filter/logger"
)

// AllowedMinutes is the whitelist of pause durations exposed to the UI.
// Server-side validation matches the frontend select options.
var AllowedMinutes = []int{5, 10, 15, 30}

var (
	ErrInvalidDuration = errors.New("pause duration must be one of 5, 10, 15, 30 minutes")
	ErrFilterDisabled  = errors.New("cannot pause: filter is already disabled")
)

func isAllowed(minutes int) bool {
	return slices.Contains(AllowedMinutes, minutes)
}

// PauseFilter pauses filtering for the given number of minutes by storing an
// absolute unix-second deadline. Returns the deadline, or ErrInvalidDuration
// if the duration is not whitelisted, or ErrFilterDisabled if the filter is
// already off (pause has no meaning then). Last writer wins under concurrent
// successful calls.
func PauseFilter(minutes int) (int64, error) {
	if !isAllowed(minutes) {
		return 0, ErrInvalidDuration
	}
	conf := config.GetConfig()
	if !conf.Enabled.Load() {
		return 0, ErrFilterDisabled
	}
	until := time.Now().Add(time.Duration(minutes) * time.Minute).Unix()
	conf.PausedUntilUnix.Store(until)
	logger.GetLogger().Info("Filter paused for", minutes, "minutes, until unix:", until)
	return until, nil
}

// ResumeFilter clears any active pause. Safe to call when not paused.
func ResumeFilter() {
	conf := config.GetConfig()
	if conf.PausedUntilUnix.Swap(0) != 0 {
		logger.GetLogger().Info("Filter pause cleared")
	}
}

// GetPausedUntil returns the active pause deadline (unix seconds), or 0 if no
// pause is active or the deadline has already passed.
func GetPausedUntil() int64 {
	conf := config.GetConfig()
	until := conf.PausedUntilUnix.Load()
	if until <= time.Now().Unix() {
		return 0
	}
	return until
}
