package gui

import (
	"sync"
	"time"

	"github.com/saiaathish/picogent/internal/agyauth"
	"github.com/saiaathish/picogent/internal/claudeauth"
	"github.com/saiaathish/picogent/internal/codexauth"
	"github.com/saiaathish/picogent/internal/opencodeauth"
)

const guiAuthStatusCacheTTL = 500 * time.Millisecond

type guiAuthStatus struct {
	codex       bool
	claude      bool
	opencode    bool
	antigravity bool
}

type guiAuthStatusCache struct {
	mu        sync.Mutex
	value     guiAuthStatus
	fetchedAt time.Time
	inflight  chan struct{}
	ttl       time.Duration
}

func (c *guiAuthStatusCache) get(probe func() guiAuthStatus) guiAuthStatus {
	if probe == nil {
		probe = readGUIAuthStatus
	}
	for {
		now := time.Now()
		c.mu.Lock()
		ttl := c.ttl
		if ttl <= 0 {
			ttl = guiAuthStatusCacheTTL
		}
		if !c.fetchedAt.IsZero() && now.Sub(c.fetchedAt) < ttl {
			value := c.value
			c.mu.Unlock()
			return value
		}
		if wait := c.inflight; wait != nil {
			c.mu.Unlock()
			<-wait
			continue
		}
		wait := make(chan struct{})
		c.inflight = wait
		c.mu.Unlock()

		value := probe()

		c.mu.Lock()
		c.value = value
		c.fetchedAt = time.Now()
		c.inflight = nil
		close(wait)
		c.mu.Unlock()
		return value
	}
}

func (s *server) authStatusSnapshot() guiAuthStatus {
	return s.authStatusCache.get(readGUIAuthStatus)
}

func readGUIAuthStatus() guiAuthStatus {
	return guiAuthStatus{
		codex:       codexauth.LoggedIn(),
		claude:      claudeauth.LoggedIn(),
		opencode:    opencodeauth.LoggedIn(),
		antigravity: agyauth.LoggedIn(),
	}
}
