package agyauth

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
)

var refreshMu sync.Mutex

// CanConnect allows the request path to try the CLI's own subscription refresh.
// LoggedIn remains a strict local status check for UI display.
func CanConnect() bool {
	c, e := Load()
	return UsesGeminiAPIKey() || e == nil && (!c.Expired() || c.RefreshToken != "")
}
func TokenContext(ctx context.Context) (string, error) {
	c, e := LoadContext(ctx)
	if e != nil {
		return "", fmt.Errorf("Antigravity login missing; reconnect in Setup")
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
	if os.Getenv("PICOGENT_AGY_HOME") != "" {
		return "", fmt.Errorf("Antigravity session expired; reconnect the configured CLI account")
	}
	bin, e := exec.LookPath("agy")
	if e != nil {
		home, _ := os.UserHomeDir()
		bin = filepath.Join(home, ".local", "bin", "agy")
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, "models")
	cmd.Dir = os.TempDir()
	cmd.Stdin = nil
	// CLI owns refresh tokens/storage. Never copy, rotate, or print credentials.
	if cmd.Run() != nil {
		return "", fmt.Errorf("Antigravity session refresh failed; reconnect in Setup")
	}
	c, e = LoadContext(ctx)
	if e != nil || c.Expired() {
		return "", fmt.Errorf("Antigravity session expired; reconnect in Setup")
	}
	return c.AccessToken, nil
}
