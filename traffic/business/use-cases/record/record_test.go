package traffic_use_cases_record

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	traffic_db "github.com/alextorq/dns-filter/traffic/db"
)

type fakeRepo struct {
	mu      sync.Mutex
	batches [][]traffic_db.DomainTraffic
	err     error
}

func (f *fakeRepo) UpsertBatch(rows []traffic_db.DomainTraffic) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	batch := make([]traffic_db.DomainTraffic, len(rows))
	copy(batch, rows)
	f.batches = append(f.batches, batch)
	return f.err
}

func (f *fakeRepo) snapshot() [][]traffic_db.DomainTraffic {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([][]traffic_db.DomainTraffic, len(f.batches))
	copy(out, f.batches)
	return out
}

// allRows flattens every flushed batch into a single slice for assertions.
func (f *fakeRepo) allRows() []traffic_db.DomainTraffic {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []traffic_db.DomainTraffic
	for _, b := range f.batches {
		out = append(out, b...)
	}
	return out
}

type recordingLog struct {
	warns atomic.Int32
	errs  atomic.Int32
}

func (l *recordingLog) Warn(args ...any) { l.warns.Add(1) }
func (l *recordingLog) Error(err error)  { l.errs.Add(1) }

func waitFor(t *testing.T, timeout time.Duration, msg string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", msg)
}

// TestAccumulatesDuplicateKeys: repeated events with the same aggregation key
// collapse into a single row whose Count is the sum.
func TestAccumulatesDuplicateKeys(t *testing.T) {
	repo := &fakeRepo{}
	at := time.Date(2026, 5, 25, 12, 0, 0, 0, time.Local)
	store := newWithChannelSize(repo, &recordingLog{}, 1000, 100)

	store.record(Event{Kind: "mac", Value: "aa:bb", IP: "10.0.0.5", Domain: "ads.example.", Blocked: true, At: at})
	store.record(Event{Kind: "mac", Value: "aa:bb", IP: "10.0.0.5", Domain: "ads.example.", Blocked: true, At: at})
	store.record(Event{Kind: "mac", Value: "aa:bb", IP: "10.0.0.5", Domain: "ads.example.", Blocked: true, At: at})

	store.flushNow()

	waitFor(t, time.Second, "flush", func() bool { return len(repo.snapshot()) >= 1 })

	rows := repo.allRows()
	if len(rows) != 1 {
		t.Fatalf("expected 1 aggregated row, got %d: %+v", len(rows), rows)
	}
	if rows[0].Count != 3 {
		t.Fatalf("expected Count=3, got %d", rows[0].Count)
	}
}

// TestDistinctKeysSeparateRows: events differing in any key dimension produce
// separate rows.
func TestDistinctKeysSeparateRows(t *testing.T) {
	repo := &fakeRepo{}
	at := time.Date(2026, 5, 25, 12, 0, 0, 0, time.Local)
	store := newWithChannelSize(repo, &recordingLog{}, 1000, 100)

	store.record(Event{Kind: "mac", Value: "aa:bb", IP: "10.0.0.5", Domain: "ads.example.", Blocked: true, At: at})
	store.record(Event{Kind: "mac", Value: "aa:bb", IP: "10.0.0.5", Domain: "ads.example.", Blocked: false, At: at}) // verdict differs
	store.record(Event{Kind: "mac", Value: "cc:dd", IP: "10.0.0.6", Domain: "ads.example.", Blocked: true, At: at})   // device differs
	store.record(Event{Kind: "mac", Value: "aa:bb", IP: "10.0.0.5", Domain: "cdn.example.", Blocked: true, At: at})   // domain differs

	store.flushNow()
	waitFor(t, time.Second, "flush", func() bool { return len(repo.snapshot()) >= 1 })

	rows := repo.allRows()
	if len(rows) != 4 {
		t.Fatalf("expected 4 distinct rows, got %d: %+v", len(rows), rows)
	}
}

// TestFlushOnTicker: with no capacity pressure, the worker flushes on its
// periodic tick. Uses a tiny ticker interval via the test seam.
func TestFlushOnTicker(t *testing.T) {
	repo := &fakeRepo{}
	at := time.Date(2026, 5, 25, 12, 0, 0, 0, time.Local)
	store := newWithChannelSizeAndInterval(repo, &recordingLog{}, 1000, 100, 10*time.Millisecond)

	store.record(Event{Kind: "ip", Value: "10.0.0.9", IP: "10.0.0.9", Domain: "example.", Blocked: false, At: at})

	waitFor(t, time.Second, "ticker flush", func() bool { return len(repo.snapshot()) >= 1 })

	if got := len(repo.allRows()); got != 1 {
		t.Fatalf("expected 1 row flushed by ticker, got %d", got)
	}
}

// TestFlushOnCapacity: reaching the distinct-key capacity bound triggers an
// immediate flush without waiting for the ticker (which is set long here).
func TestFlushOnCapacity(t *testing.T) {
	repo := &fakeRepo{}
	at := time.Date(2026, 5, 25, 12, 0, 0, 0, time.Local)
	// capacity=2 distinct keys; long ticker so only capacity can trigger.
	store := newWithChannelSizeAndInterval(repo, &recordingLog{}, 2, 100, time.Hour)

	store.record(Event{Kind: "mac", Value: "aa:bb", IP: "10.0.0.5", Domain: "one.example.", Blocked: true, At: at})
	store.record(Event{Kind: "mac", Value: "aa:bb", IP: "10.0.0.5", Domain: "two.example.", Blocked: true, At: at})

	waitFor(t, time.Second, "capacity flush", func() bool { return len(repo.snapshot()) >= 1 })

	if got := len(repo.allRows()); got != 2 {
		t.Fatalf("expected 2 rows flushed at capacity, got %d", got)
	}
}

// TestLastSeenTracksMax: the row's LastSeen is the maximum At seen for the key,
// even if events arrive out of chronological order.
func TestLastSeenTracksMax(t *testing.T) {
	repo := &fakeRepo{}
	early := time.Date(2026, 5, 25, 8, 0, 0, 0, time.Local)
	late := time.Date(2026, 5, 25, 20, 0, 0, 0, time.Local)
	store := newWithChannelSize(repo, &recordingLog{}, 1000, 100)

	store.record(Event{Kind: "mac", Value: "aa:bb", IP: "10.0.0.5", Domain: "ads.example.", Blocked: true, At: late})
	store.record(Event{Kind: "mac", Value: "aa:bb", IP: "10.0.0.5", Domain: "ads.example.", Blocked: true, At: early})

	store.flushNow()
	waitFor(t, time.Second, "flush", func() bool { return len(repo.snapshot()) >= 1 })

	rows := repo.allRows()
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if !rows[0].LastSeen.Equal(late) {
		t.Fatalf("expected LastSeen=%v (max), got %v", late, rows[0].LastSeen)
	}
}

// TestLatestIPWins: the row's ClientIP reflects the most recently seen IP for
// the key (the device hopped DHCP IPs but the MAC key is stable).
func TestLatestIPWins(t *testing.T) {
	repo := &fakeRepo{}
	t1 := time.Date(2026, 5, 25, 8, 0, 0, 0, time.Local)
	t2 := time.Date(2026, 5, 25, 20, 0, 0, 0, time.Local)
	store := newWithChannelSize(repo, &recordingLog{}, 1000, 100)

	store.record(Event{Kind: "mac", Value: "aa:bb", IP: "10.0.0.5", Domain: "ads.example.", Blocked: true, At: t1})
	store.record(Event{Kind: "mac", Value: "aa:bb", IP: "10.0.0.99", Domain: "ads.example.", Blocked: true, At: t2})

	store.flushNow()
	waitFor(t, time.Second, "flush", func() bool { return len(repo.snapshot()) >= 1 })

	rows := repo.allRows()
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0].ClientIP != "10.0.0.99" {
		t.Fatalf("expected latest IP 10.0.0.99, got %q", rows[0].ClientIP)
	}
}

// TestDayBucketLocalMidnight: an event stamped just before/after local midnight
// must bucket into the correct LOCAL calendar day, never UTC. This is the
// footgun the spec calls out: time.Truncate(24h) truncates in UTC.
func TestDayBucketLocalMidnight(t *testing.T) {
	cases := []struct {
		name string
		at   time.Time
		want time.Time
	}{
		{
			name: "just after local midnight stays on that day",
			at:   time.Date(2026, 5, 25, 0, 5, 0, 0, time.Local),
			want: time.Date(2026, 5, 25, 0, 0, 0, 0, time.Local),
		},
		{
			name: "just before local midnight stays on the previous day",
			at:   time.Date(2026, 5, 25, 23, 55, 0, 0, time.Local),
			want: time.Date(2026, 5, 25, 0, 0, 0, 0, time.Local),
		},
		{
			name: "midday",
			at:   time.Date(2026, 5, 25, 13, 30, 0, 0, time.Local),
			want: time.Date(2026, 5, 25, 0, 0, 0, 0, time.Local),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := &fakeRepo{}
			store := newWithChannelSize(repo, &recordingLog{}, 1000, 100)
			store.record(Event{Kind: "ip", Value: "10.0.0.1", IP: "10.0.0.1", Domain: "x.", Blocked: false, At: tc.at})
			store.flushNow()
			waitFor(t, time.Second, "flush", func() bool { return len(repo.snapshot()) >= 1 })

			rows := repo.allRows()
			if len(rows) != 1 {
				t.Fatalf("expected 1 row, got %d", len(rows))
			}
			if !rows[0].Day.Equal(tc.want) {
				t.Fatalf("expected Day=%v, got %v", tc.want, rows[0].Day)
			}
		})
	}
}

// TestDayBucketInFixedZone pins a fixed non-UTC zone so the test discriminates
// the UTC-truncate footgun on any runner (CI sets no TZ). 01:00 at UTC+14 is the
// previous day in UTC; a time.Truncate(24h) regression would bucket it wrong.
func TestDayBucketInFixedZone(t *testing.T) {
	loc := time.FixedZone("UTC+14", 14*60*60)
	at := time.Date(2026, 5, 25, 1, 0, 0, 0, loc) // = 2026-05-24 11:00 UTC
	want := time.Date(2026, 5, 25, 0, 0, 0, 0, loc)
	if got := dayBucketIn(at, loc); !got.Equal(want) {
		t.Fatalf("dayBucketIn: expected %v, got %v", want, got)
	}
}

// TestDayRolloverDistinctKeys: the same device+domain+verdict on two different
// local days produces two rows (the Day is part of the key).
func TestDayRolloverDistinctKeys(t *testing.T) {
	repo := &fakeRepo{}
	day1 := time.Date(2026, 5, 25, 13, 0, 0, 0, time.Local)
	day2 := time.Date(2026, 5, 26, 13, 0, 0, 0, time.Local)
	store := newWithChannelSize(repo, &recordingLog{}, 1000, 100)

	store.record(Event{Kind: "mac", Value: "aa:bb", IP: "10.0.0.5", Domain: "ads.example.", Blocked: true, At: day1})
	store.record(Event{Kind: "mac", Value: "aa:bb", IP: "10.0.0.5", Domain: "ads.example.", Blocked: true, At: day2})

	store.flushNow()
	waitFor(t, time.Second, "flush", func() bool { return len(repo.snapshot()) >= 1 })

	if got := len(repo.allRows()); got != 2 {
		t.Fatalf("expected 2 rows across day rollover, got %d", got)
	}
}

// blockingRepo stalls the worker inside flush so the inbox channel saturates,
// exercising the drop-on-full branch of Record.
type blockingRepo struct {
	enter chan struct{}
	exit  chan struct{}
}

func (b *blockingRepo) UpsertBatch(_ []traffic_db.DomainTraffic) error {
	b.enter <- struct{}{}
	<-b.exit
	return nil
}

// TestRecordDropsWhenChannelFull: Record must DROP (and warn), never block, when
// the inbox is saturated — the DNS hot path can never be backpressured by a
// stuck DB write.
func TestRecordDropsWhenChannelFull(t *testing.T) {
	repo := &blockingRepo{enter: make(chan struct{}, 1), exit: make(chan struct{})}
	log := &recordingLog{}
	// capacity=1 forces a flush after the first distinct key; chanSize=1 means a
	// single extra send saturates the inbox while the worker is parked in flush.
	store := newWithChannelSize(repo, log, 1, 1)

	at := time.Date(2026, 5, 25, 12, 0, 0, 0, time.Local)
	store.record(Event{Kind: "mac", Value: "aa:bb", IP: "10.0.0.5", Domain: "first.", Blocked: true, At: at})
	// Worker reads "first" off the channel, aggregates it (hits capacity=1), and
	// parks inside UpsertBatch on <-b.exit. Once enter fires, the channel is empty.
	<-repo.enter
	// Fill the 1-slot buffer.
	store.record(Event{Kind: "mac", Value: "cc:dd", IP: "10.0.0.6", Domain: "second.", Blocked: true, At: at})
	// This send must hit the default branch → warn + drop, NOT block.
	store.record(Event{Kind: "mac", Value: "ee:ff", IP: "10.0.0.7", Domain: "dropped.", Blocked: true, At: at})

	waitFor(t, time.Second, "warn for drop", func() bool { return log.warns.Load() >= 1 })

	close(repo.exit)
}

// TestLogsRepoError: a failing UpsertBatch is logged, not swallowed.
func TestLogsRepoError(t *testing.T) {
	repo := &fakeRepo{err: errAlwaysFails}
	log := &recordingLog{}
	store := newWithChannelSize(repo, log, 1, 100)

	at := time.Date(2026, 5, 25, 12, 0, 0, 0, time.Local)
	store.record(Event{Kind: "mac", Value: "aa:bb", IP: "10.0.0.5", Domain: "x.", Blocked: true, At: at})

	waitFor(t, time.Second, "error logged", func() bool { return log.errs.Load() >= 1 })
}

var errAlwaysFails = &testErr{}

type testErr struct{}

func (*testErr) Error() string { return "db down" }
