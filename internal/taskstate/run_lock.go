package taskstate

import "sync"

type runProcessLock struct {
	mu   sync.Mutex
	refs int
}

var runProcessLocks = struct {
	sync.Mutex
	byDir map[string]*runProcessLock
}{byDir: make(map[string]*runProcessLock)}

func acquireRunProcessLock(dir string) *runProcessLock {
	runProcessLocks.Lock()
	entry := runProcessLocks.byDir[dir]
	if entry == nil {
		entry = &runProcessLock{}
		runProcessLocks.byDir[dir] = entry
	}
	entry.refs++
	runProcessLocks.Unlock()

	entry.mu.Lock()
	return entry
}

func releaseRunProcessLock(dir string, entry *runProcessLock) {
	entry.mu.Unlock()

	runProcessLocks.Lock()
	entry.refs--
	if entry.refs == 0 && runProcessLocks.byDir[dir] == entry {
		delete(runProcessLocks.byDir, dir)
	}
	runProcessLocks.Unlock()
}
