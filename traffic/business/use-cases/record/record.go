// Package traffic_use_cases_record is the async write path for the unified
// per-device traffic counter. It mirrors the BlockDomainEventStore lifecycle:
// a buffered inbox channel feeds a single worker goroutine that aggregates
// events in RAM and flushes batches to the repo on a 20s ticker or when the
// distinct-key map reaches a capacity bound. The DNS hot path never blocks on a
// DB write — Record drops on a full channel rather than backpressuring queries.
package traffic_use_cases_record

import (
	"fmt"
	"time"

	traffic_db "github.com/alextorq/dns-filter/traffic/db"
)

// Repo is the output port: additively upserting batched traffic counters.
type Repo interface {
	UpsertBatch(rows []traffic_db.DomainTraffic) error
}

type Logger interface {
	Warn(args ...any)
	Error(err error)
}

// Event is one observed DNS query, ready to be aggregated. At is stamped by the
// caller at query time (the public Record method stamps time.Now()).
type Event struct {
	Kind    string // identifier kind: "mac" | "ip"
	Value   string // the stable device key (MAC preferred, else IP)
	IP      string // last IP the device was seen using — informational
	Domain  string // canonical FQDN
	Blocked bool   // true = NXDOMAIN'd, false = forwarded upstream
	At      time.Time
}

// aggKey is the in-RAM aggregation key. It mirrors the DB unique index
// (client_kind, client_value, blocked, domain, day). Day is local-midnight (see
// dayBucket) so a query near midnight buckets into the correct local calendar
// day — NOT the UTC day that time.Truncate(24h) would give.
type aggKey struct {
	kind    string
	value   string
	blocked bool
	domain  string
	day     time.Time
}

// aggVal accumulates the counter and the informational max/latest fields for a
// single key within the current flush window.
type aggVal struct {
	count    int64
	lastSeen time.Time
	clientIP string
}

const (
	// defaultChannelSize is the inbox buffer for Record. Sized to absorb DNS
	// bursts without dropping events during a normal flush window — same as the
	// block-domain store's inbox.
	defaultChannelSize = 5000
	// defaultFlushInterval is the periodic flush cadence, matching the
	// block-domain store's 20s ticker.
	defaultFlushInterval = 20 * time.Second
)

// inboxMsg is what travels over the worker's inbox channel. Most messages are
// just an event; a flush message (done != nil) is a test-only synchronous-flush
// request. Routing both through the SAME channel keeps them FIFO-ordered, so a
// flushNow after N record calls is guaranteed to observe all N events.
type inboxMsg struct {
	event Event
	done  chan struct{} // non-nil ⇒ flush request, not an event
}

// TrafficEventStore is the async aggregator. Construct it with
// NewTrafficEventStore; the worker goroutine starts immediately.
type TrafficEventStore struct {
	repo     Repo
	log      Logger
	ch       chan inboxMsg
	buf      map[aggKey]*aggVal
	capacity int // flush when len(buf) reaches this many distinct keys
	interval time.Duration
}

// NewTrafficEventStore starts a background worker that aggregates traffic events
// and flushes them to the repo when capacity (distinct keys) is reached or on a
// 20s ticker. The hot DNS path must never block on a DB write — see Record.
func NewTrafficEventStore(repo Repo, log Logger, capacity int) *TrafficEventStore {
	return newWithChannelSizeAndInterval(repo, log, capacity, defaultChannelSize, defaultFlushInterval)
}

// newWithChannelSize is a test seam: exposes the inbox buffer so a unit test can
// force the "channel full → drop" branch deterministically. Uses the default
// flush interval.
func newWithChannelSize(repo Repo, log Logger, capacity, chanSize int) *TrafficEventStore {
	return newWithChannelSizeAndInterval(repo, log, capacity, chanSize, defaultFlushInterval)
}

// newWithChannelSizeAndInterval is the full test seam: exposes both the inbox
// buffer and the flush interval so a test can drive the ticker fast.
func newWithChannelSizeAndInterval(repo Repo, log Logger, capacity, chanSize int, interval time.Duration) *TrafficEventStore {
	s := &TrafficEventStore{
		repo:     repo,
		log:      log,
		ch:       make(chan inboxMsg, chanSize),
		buf:      make(map[aggKey]*aggVal),
		capacity: capacity,
		interval: interval,
	}
	go s.start()
	return s
}

// dayBucket truncates t to local midnight. It MUST use the calendar Date in the
// local zone — time.Truncate(24*time.Hour) truncates relative to the UTC epoch
// and would put a query just after local midnight onto the wrong day in any
// non-UTC zone.
func dayBucket(t time.Time) time.Time {
	return dayBucketIn(t, time.Local)
}

// dayBucketIn is dayBucket with an explicit zone, split out so a test can pin a
// fixed non-UTC location and catch a regression to time.Truncate regardless of
// the runner's TZ.
func dayBucketIn(t time.Time, loc *time.Location) time.Time {
	y, m, d := t.In(loc).Date()
	return time.Date(y, m, d, 0, 0, 0, 0, loc)
}

func (s *TrafficEventStore) start() {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case msg := <-s.ch:
			if msg.done != nil {
				// Test-only synchronous flush request (FIFO after prior events).
				s.flush()
				close(msg.done)
				continue
			}
			s.accumulate(msg.event)
			if len(s.buf) >= s.capacity {
				s.flush()
			}
		case <-ticker.C:
			if len(s.buf) != 0 {
				s.flush()
			}
		}
	}
}

// accumulate folds one event into the in-RAM map: bumps Count, tracks the max
// LastSeen and the latest IP (by event time).
func (s *TrafficEventStore) accumulate(e Event) {
	k := aggKey{
		kind:    e.Kind,
		value:   e.Value,
		blocked: e.Blocked,
		domain:  e.Domain,
		day:     dayBucket(e.At),
	}
	v, ok := s.buf[k]
	if !ok {
		v = &aggVal{}
		s.buf[k] = v
	}
	v.count++
	if e.At.After(v.lastSeen) {
		v.lastSeen = e.At
		v.clientIP = e.IP
	}
}

// flush converts the current map into a []DomainTraffic and hands it to the
// repo, then resets the map. Errors are logged, not propagated (this is a
// background worker).
func (s *TrafficEventStore) flush() {
	if len(s.buf) == 0 {
		return
	}
	rows := make([]traffic_db.DomainTraffic, 0, len(s.buf))
	for k, v := range s.buf {
		rows = append(rows, traffic_db.DomainTraffic{
			ClientKind:  k.kind,
			ClientValue: k.value,
			ClientIP:    v.clientIP,
			Domain:      k.domain,
			Blocked:     k.blocked,
			Day:         k.day,
			Count:       v.count,
			LastSeen:    v.lastSeen,
		})
	}
	s.buf = make(map[aggKey]*aggVal)
	if err := s.repo.UpsertBatch(rows); err != nil {
		s.log.Error(fmt.Errorf("error processing batch traffic events: %w", err))
	}
}

// record enqueues an already-stamped event, dropping on a full inbox. It is the
// unexported core shared by the public Record (which stamps At) and the tests
// (which supply a fixed At).
func (s *TrafficEventStore) record(e Event) {
	select {
	case s.ch <- inboxMsg{event: e}:
	default:
		// Channel full — drop rather than block the DNS hot path.
		s.log.Warn("Traffic event channel full, dropping event for: " + e.Domain)
	}
}

// Record is the TrafficRecorder port the DNS server calls on the hot path. It
// stamps the query time internally and never blocks (drops on a full inbox).
func (s *TrafficEventStore) Record(kind, value, ip, domain string, blocked bool) {
	s.record(Event{
		Kind:    kind,
		Value:   value,
		IP:      ip,
		Domain:  domain,
		Blocked: blocked,
		At:      time.Now(),
	})
}

// flushNow is a test helper: it asks the worker to flush synchronously and
// blocks until the flush completes, so a test can assert on repo batches without
// waiting for the ticker. Production code never calls it.
func (s *TrafficEventStore) flushNow() {
	done := make(chan struct{})
	s.ch <- inboxMsg{done: done}
	<-done
}
