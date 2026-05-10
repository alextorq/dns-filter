package lru_cache

import "testing"

func TestClear_RemovesAllEntries(t *testing.T) {
	c := CreateCache[int](10)
	c.Add("a", 1)
	c.Add("b", 2)
	c.Add("c", 3)

	c.Clear()

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
