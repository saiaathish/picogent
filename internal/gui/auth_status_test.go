package gui

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestGUIAuthStatusCacheSingleFlightAndExpiry(t *testing.T) {
	cache := guiAuthStatusCache{ttl: time.Hour}
	want := guiAuthStatus{codex: true, claude: true, opencode: false, antigravity: true}
	var calls atomic.Int32
	probeStarted := make(chan struct{})
	release := make(chan struct{})
	var startOnce sync.Once
	probe := func() guiAuthStatus {
		calls.Add(1)
		startOnce.Do(func() { close(probeStarted) })
		<-release
		return want
	}

	const readers = 6
	results := make(chan guiAuthStatus, readers)
	var wg sync.WaitGroup
	wg.Add(readers)
	for i := 0; i < readers; i++ {
		go func() {
			defer wg.Done()
			results <- cache.get(probe)
		}()
	}
	select {
	case <-probeStarted:
	case <-time.After(time.Second):
		t.Fatal("auth status probe did not start")
	}
	time.Sleep(20 * time.Millisecond)
	if got := calls.Load(); got != 1 {
		t.Fatalf("concurrent auth status probes=%d, want one", got)
	}
	close(release)
	wg.Wait()
	close(results)
	for got := range results {
		if got != want {
			t.Fatalf("cached auth status=%#v, want %#v", got, want)
		}
	}
	if got := cache.get(probe); got != want {
		t.Fatalf("fresh cached auth status=%#v, want %#v", got, want)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("fresh auth status probes=%d, want one", got)
	}

	cache.ttl = time.Millisecond
	time.Sleep(2 * time.Millisecond)
	if got := cache.get(probe); got != want {
		t.Fatalf("refreshed auth status=%#v, want %#v", got, want)
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("expired auth status probes=%d, want two", got)
	}
}
