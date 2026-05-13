package lru_cache

import "testing"

func TestClear_RemovesAllEntries(t *testing.T) {
	c := CreateCache[int](10)
	c.Add("a", 1)
	c.Add("b", 2)
	c.Add("c", 3)

	if n := c.Clear(); n != 3 {
		t.Fatalf("Clear must return entries removed, want 3 got %d", n)
	}

	for _, key := range []string{"a", "b", "c"} {
		if _, ok := c.Get(key); ok {
			t.Fatalf("key %q should be gone after Clear", key)
		}
	}
	// Cache must remain usable.
	c.Add("d", 4)
	if v, ok := c.Get("d"); !ok || v != 4 {
		t.Fatalf("cache unusable after Clear: got %v, %v", v, ok)
	}
}

// Clearing an empty cache must be a no-op and report 0 — guards against
// a manual-flush endpoint returning a misleading "cleared N entries"
// when there was nothing to clear.
func TestClear_EmptyReturnsZero(t *testing.T) {
	c := CreateCache[int](10)
	if n := c.Clear(); n != 0 {
		t.Fatalf("Clear on empty cache must return 0, got %d", n)
	}
	if l := c.Len(); l != 0 {
		t.Fatalf("Len on empty cache must be 0, got %d", l)
	}
}

func TestLen_TracksEntries(t *testing.T) {
	c := CreateCache[int](10)
	if l := c.Len(); l != 0 {
		t.Fatalf("expected len 0, got %d", l)
	}
	c.Add("a", 1)
	c.Add("b", 2)
	if l := c.Len(); l != 2 {
		t.Fatalf("expected len 2, got %d", l)
	}
}
