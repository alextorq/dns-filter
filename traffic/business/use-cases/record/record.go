// Package traffic_use_cases_record is the async write path for the unified
// per-device traffic counter. A buffered inbox channel feeds a single worker
// goroutine that aggregates
// events in RAM and flushes batches to the repo on a 20s ticker or when the
// distinct-key map reaches a capacity bound. The DNS hot path never blocks on a
// DB write — Record drops on a full channel rather than backpressuring queries.
package traffic_use_cases_record

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	traffic_db "github.com/alextorq/dns-filter/traffic/db"
	"github.com/alextorq/dns-filter/utils"
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

	stopping      atomic.Bool
	inFlightSends atomic.Int64
	stopOnce      sync.Once
	stopCh        chan struct{}
	done          chan struct{}
	stopErrMu     sync.Mutex
	stopErr       error
	warnStopped   sync.Once
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
		stopCh:   make(chan struct{}),
		done:     make(chan struct{}),
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
			s.handle(msg)
		case <-ticker.C:
			if len(s.buf) != 0 {
				_ = s.flush()
			}
		case <-s.stopCh:
			s.drainAndStop()
			return
		}
	}
}

func (s *TrafficEventStore) handle(msg inboxMsg) {
	if msg.done != nil {
		// Test-only synchronous flush request (FIFO after prior events).
		_ = s.flush()
		close(msg.done)
		return
	}
	s.accumulate(msg.event)
	if len(s.buf) >= s.capacity {
		_ = s.flush()
	}
}

// drainAndStop runs on the sole worker goroutine after producers have quiesced.
// It drains every event already accepted into the inbox, performs one final
// flush, stores its result for every Stop caller, then terminates the worker.
func (s *TrafficEventStore) drainAndStop() {
	for {
		select {
		case msg := <-s.ch:
			s.handle(msg)
		default:
			err := s.flush()
			s.stopErrMu.Lock()
			s.stopErr = err
			s.stopErrMu.Unlock()
			close(s.done)
			return
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
		if len(s.buf) >= s.capacity {
			// Buffer is full and a prior flush has not drained it — the DB write is
			// failing or stalling (flush retains the buffer on error, see flush()).
			// Shed NEW keys rather than grow the map without bound during an outage;
			// same drop-on-full policy as the inbox channel. Counts already buffered
			// keep accumulating and are retried by the next flush.
			s.log.Warn("Traffic aggregation buffer full, dropping event for: " + e.Domain)
			return
		}
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
// repo. On success the map is reset; on error it is KEPT so the next flush
// retries the accumulated counts. Errors are logged; the final flush error is
// also returned by Stop.
//
// Why retain on error: UpsertBatch is an additive upsert (count += excluded.count)
// applied as a single DB batch for our buffer sizes — capacity (2000 in prod)
// stays under db.upsertBatchSize (4000), so a flush is one atomic transaction
// that either fully commits or fully rolls back. A failed flush therefore
// committed nothing, and replaying the same rows is exactly-once. Resetting the
// map before checking the error (the previous behavior) silently dropped the
// whole batch on any DB error — e.g. SQLITE_BUSY under write-lock contention.
// Unbounded growth during a prolonged outage is prevented by accumulate, which
// sheds new keys once the buffer reaches capacity.
func (s *TrafficEventStore) flush() error {
	if len(s.buf) == 0 {
		return nil
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
	if err := s.repo.UpsertBatch(rows); err != nil {
		s.log.Error(fmt.Errorf("error processing batch traffic events: %w", err))
		return err // keep s.buf — the next flush (ticker or capacity) retries these counts
	}
	s.buf = make(map[aggKey]*aggVal)
	return nil
}

// record enqueues an already-stamped event, dropping on a full inbox. It is the
// unexported core shared by the public Record (which stamps At) and the tests
// (which supply a fixed At).
func (s *TrafficEventStore) record(e Event) {
	if s.stopping.Load() {
		s.warnRecordAfterStop(e.Domain)
		return
	}

	// Stop sets stopping before waiting for senders. The second check closes
	// the race where a sender observed false immediately before Stop began: it
	// joins the in-flight count, sees true, and exits without enqueueing.
	s.inFlightSends.Add(1)
	defer s.inFlightSends.Add(-1)
	if s.stopping.Load() {
		s.warnRecordAfterStop(e.Domain)
		return
	}

	select {
	case s.ch <- inboxMsg{event: e}:
	default:
		// Channel full — drop rather than block the DNS hot path.
		s.log.Warn("Traffic event channel full, dropping event for: " + e.Domain)
	}
}

func (s *TrafficEventStore) warnRecordAfterStop(domain string) {
	s.warnStopped.Do(func() {
		s.log.Warn("Traffic event store stopped, dropping event for: " + domain)
	})
}

// Record is the TrafficRecorder port the DNS server calls on the hot path. It
// stamps the query time internally and never blocks (drops on a full inbox).
func (s *TrafficEventStore) Record(kind, value, ip, domain string, blocked bool) {
	s.record(Event{
		Kind:    kind,
		Value:   value,
		IP:      ip,
		Domain:  utils.CanonicalDomain(domain),
		Blocked: blocked,
		At:      time.Now(),
	})
}

// Stop prevents new events from being accepted, drains all events already
// accepted into the inbox, performs one final flush, and terminates the worker.
// It is safe to call concurrently and repeatedly. ctx bounds how long the
// caller waits; stopping continues in the background after a timeout, so a
// later Stop call can observe the final result.
func (s *TrafficEventStore) Stop(ctx context.Context) error {
	s.stopOnce.Do(func() {
		s.stopping.Store(true)
		go func() {
			// A sender that raced with stopping either finishes its non-blocking
			// enqueue or observes stopping on its second check. Only then may the
			// worker drain the finite inbox and exit.
			poll := time.NewTicker(time.Millisecond)
			defer poll.Stop()
			for s.inFlightSends.Load() != 0 {
				<-poll.C
			}
			close(s.stopCh)
		}()
	})

	select {
	case <-s.done:
		return s.finalStopError()
	default:
	}

	select {
	case <-s.done:
		return s.finalStopError()
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *TrafficEventStore) finalStopError() error {
	s.stopErrMu.Lock()
	defer s.stopErrMu.Unlock()
	return s.stopErr
}

// flushNow is a test helper: it asks the worker to flush synchronously and
// blocks until the flush completes, so a test can assert on repo batches without
// waiting for the ticker. Production code never calls it.
func (s *TrafficEventStore) flushNow() {
	done := make(chan struct{})
	s.ch <- inboxMsg{done: done}
	<-done
}
