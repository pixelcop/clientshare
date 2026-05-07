package cache

import (
	"sync"
	"testing"
)

func TestCacheSetGetDelete(t *testing.T) {
	c := New[string, int]()

	if _, ok := c.Get("missing"); ok {
		t.Fatalf("expected missing key to return ok=false")
	}

	c.Set("a", 10)
	value, ok := c.Get("a")
	if !ok {
		t.Fatalf("expected key to exist")
	}
	if value != 10 {
		t.Fatalf("expected value 10, got %d", value)
	}

	c.Delete("a")
	if _, ok := c.Get("a"); ok {
		t.Fatalf("expected deleted key to return ok=false")
	}
}

func TestCacheConcurrentAccess(t *testing.T) {
	c := New[int, int]()

	const workers = 32
	const iterations = 100

	var wg sync.WaitGroup
	wg.Add(workers)

	for worker := 0; worker < workers; worker++ {
		go func(workerID int) {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				key := workerID*iterations + i
				c.Set(key, key)
				if value, ok := c.Get(key); !ok || value != key {
					t.Fatalf("expected key %d to be retrievable", key)
				}
			}
		}(worker)
	}

	wg.Wait()
}
