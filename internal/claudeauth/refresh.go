package claudeauth

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"time"
)

var refreshMu sync.Mutex

func CanConnect() bool {
	c, e := Load()
	return e == nil && c.AccessToken != "" && (!c.Expired() || c.RefreshToken != "")
}
func TokenContext(ctx context.Context) (string, error) {
	c, e := LoadContext(ctx)
	if e != nil {
		return "", fmt.Errorf("Claude login missing; reconnect in Setup")
	}
	if !c.Expired() {
		return c.AccessToken, nil
	}
	refreshMu.Lock()
	defer refreshMu.Unlock()
	c, e = LoadContext(ctx)
	if e == nil && !c.Expired() {
		return c.AccessToken, nil
	}
	if os.Getenv("CLAUDE_CONFIG_DIR") != "" {
		return "", fmt.Errorf("Claude session expired; reconnect the configured CLI account")
	}
	ctx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "claude", "auth", "status", "--json")
	cmd.Dir = os.TempDir()
	cmd.Stdin = nil
	// Status is a metadata operation. If CLI cannot refresh here, ask to reconnect.
	if cmd.Run() != nil {
		return "", fmt.Errorf("Claude session refresh unavailable; reconnect in Setup")
	}
	c, e = LoadContext(ctx)
	if e != nil || c.Expired() {
		return "", fmt.Errorf("Claude session expired; reconnect in Setup")
	}
	return c.AccessToken, nil
}
