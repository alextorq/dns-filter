package dns

import (
	"testing"
	"time"

	dnsLib "github.com/miekg/dns"
)

// SetConcurrency must swap in a semaphore of the requested capacity so the
// refresh pool can be resized at runtime via the settings module.
func TestRefreshWorker_SetConcurrency_Resizes(t *testing.T) {
	w := newRefreshWorker(newMemoryCache(), &staticResolver{}, &upstreamCoordinator{}, noopLogger{}, 2)

	if got := cap(w.limiter.Load().tokens); got != 2 {
		t.Fatalf("initial limiter cap = %d, want 2", got)
	}

	w.SetConcurrency(5)
	if got := cap(w.limiter.Load().tokens); got != 5 {
		t.Errorf("after SetConcurrency(5) cap = %d, want 5", got)
	}
}

// A non-positive concurrency must clamp to 1 rather than create an unusable
// zero-capacity semaphore (which would drop every refresh).
func TestRefreshWorker_SetConcurrency_ClampsNonPositive(t *testing.T) {
	w := newRefreshWorker(newMemoryCache(), &staticResolver{}, &upstreamCoordinator{}, noopLogger{}, 4)

	w.SetConcurrency(0)
	if got := cap(w.limiter.Load().tokens); got != 1 {
		t.Errorf("SetConcurrency(0) cap = %d, want clamp to 1", got)
	}

	w.SetConcurrency(-3)
	if got := cap(w.limiter.Load().tokens); got != 1 {
		t.Errorf("SetConcurrency(-3) cap = %d, want clamp to 1", got)
	}
}

// The subtle invariant: a refresh started under the OLD semaphore must release
// its token back to that old semaphore after a SetConcurrency swap, and a new
// refresh must be admitted by the NEW semaphore. This exercises the resize
// mid-flight path end-to-end (run under `go test -race`): a deadlock, a
// dropped-when-it-shouldn't-be, or a token imbalance would surface here.
func TestRefreshWorker_ResizeWhileRefreshInFlight(t *testing.T) {
	release := make(chan struct{})
	resolver := &blockingResolver{release: release, rcode: dnsLib.RcodeSuccess}
	cache := newMemoryCache()
	w := newRefreshWorker(cache, resolver, &upstreamCoordinator{}, noopLogger{}, 1)

	q := func(name string) dnsLib.Question {
		return dnsLib.Question{Name: name, Qtype: dnsLib.TypeA, Qclass: dnsLib.ClassINET}
	}

	// First refresh grabs the only slot of the size-1 semaphore and blocks
	// inside Exchange until we release it.
	w.Refresh("a.example.com.:A", q("a.example.com."))
	eventuallyTrue(t, time.Second, func() bool {
		return resolver.calls.Load() >= 1
	}, "first refresh reaches upstream and blocks")

	// Grow the pool. The in-flight refresh still holds a token on the OLD
	// semaphore; the new one has free capacity.
	w.SetConcurrency(2)

	// A new distinct key must now be admitted by the new semaphore (it would
	// have been dropped under the old size-1 semaphore that is still occupied).
	w.Refresh("b.example.com.:A", q("b.example.com."))
	eventuallyTrue(t, time.Second, func() bool {
		return resolver.calls.Load() >= 2
	}, "second refresh admitted by the resized semaphore")

	// Unblock both; they each release their token to the semaphore they
	// captured. A double-release or wrong-semaphore release would desync the
	// counts and the follow-up refresh below would hang.
	close(release)

	// The pool is healthy after the resize: a third refresh still goes through.
	eventuallyTrue(t, 2*time.Second, func() bool {
		w.Refresh("c.example.com.:A", q("c.example.com."))
		return resolver.calls.Load() >= 3
	}, "pool keeps admitting refreshes after the in-flight ones drain")
}
