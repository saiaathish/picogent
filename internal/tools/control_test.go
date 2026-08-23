package tools

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/saiaathish/picogent/internal/mcpbridge"
)

func TestAttachMCPKeepsManage(t *testing.T) {
	r := NewRegistry(Context{})
	if _, ok := r.Get("mcp_manage"); !ok {
		t.Fatal("mcp_manage missing before AttachMCP")
	}
	r.AttachMCP(&mcpbridge.Manager{})
	if _, ok := r.Get("mcp_manage"); !ok {
		t.Fatal("AttachMCP dropped builtin mcp_manage")
	}
	names := r.Specs()
	found := false
	for _, s := range names {
		if s.Name == "mcp_manage" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("mcp_manage missing from Specs after AttachMCP")
	}
}

func TestRegistryConcurrentAttachAndRead(t *testing.T) {
	r := NewRegistry(Context{})
	const iterations = 200
	var wg sync.WaitGroup
	errCh := make(chan string, 1)

	for writer := 0; writer < 4; writer++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				if i%2 == 0 {
					r.AttachMCP(&mcpbridge.Manager{})
				} else {
					r.AttachMCP(nil)
				}
			}
		}()
	}
	for reader := 0; reader < 4; reader++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				_ = r.Specs()
				if _, ok := r.Get("read_file"); !ok {
					select {
					case errCh <- "read_file disappeared":
					default:
					}
				}
				_ = r.HasMCP()
				_ = r.HasBrowserMCP()
			}
		}()
	}
	wg.Wait()
	select {
	case err := <-errCh:
		t.Fatal(err)
	default:
	}
}

func TestRegistryWithExclusiveSerializesMutations(t *testing.T) {
	r := NewRegistry(Context{})
	var active, maxActive int32
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r.WithExclusive(func() {
				current := atomic.AddInt32(&active, 1)
				for {
					old := atomic.LoadInt32(&maxActive)
					if current <= old || atomic.CompareAndSwapInt32(&maxActive, old, current) {
						break
					}
				}
				time.Sleep(time.Millisecond)
				atomic.AddInt32(&active, -1)
			})
		}()
	}
	wg.Wait()
	if got := atomic.LoadInt32(&maxActive); got != 1 {
		t.Fatalf("maximum concurrent exclusive mutations = %d, want 1", got)
	}
}

func TestMCPManageList(t *testing.T) {
	c := Context{
		MCPList: func() string { return "github" },
	}
	out, err := mcpManage{}.Run(context.Background(), `{"action":"list"}`, c)
	if err != nil || out != "github" {
		t.Fatalf("%q %v", out, err)
	}
}

func TestMCPManageAddRequiresID(t *testing.T) {
	c := Context{
		MCPAdd: func(context.Context, string) (string, error) { return "ok", nil },
	}
	_, err := mcpManage{}.Run(context.Background(), `{"action":"add"}`, c)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestMCPManageAdd(t *testing.T) {
	var got string
	c := Context{
		MCPAdd: func(_ context.Context, id string) (string, error) {
			got = id
			return "installed " + id, nil
		},
	}
	out, err := mcpManage{}.Run(context.Background(), `{"action":"add","id":"mcp-fetch"}`, c)
	if err != nil || got != "mcp-fetch" || out != "installed mcp-fetch" {
		t.Fatalf("%q %q %v", out, got, err)
	}
}

func TestMCPManageRemove(t *testing.T) {
	var got string
	c := Context{
		MCPRemove: func(_ context.Context, id string) (string, error) {
			got = id
			return "removed " + id, nil
		},
	}
	out, err := mcpManage{}.Run(context.Background(), `{"action":"remove","id":"mcp-fetch"}`, c)
	if err != nil || got != "mcp-fetch" || !strings.Contains(out, "removed") {
		t.Fatalf("%q %q %v", out, got, err)
	}
}

func TestVerifyTool(t *testing.T) {
	c := Context{
		Verify: func(context.Context) (string, error) { return "verify PASS", nil },
	}
	out, err := verifyTool{}.Run(context.Background(), "{}", c)
	if err != nil || out != "verify PASS" {
		t.Fatalf("%q %v", out, err)
	}
}

func TestVerifyToolPassesTargets(t *testing.T) {
	var got []string
	c := Context{VerifyTargets: func(_ context.Context, targets []string) (string, error) {
		got = append([]string(nil), targets...)
		return "verify PASS", nil
	}}
	out, err := verifyTool{}.Run(context.Background(), `{"targets":["internal/auth/auth.go"]}`, c)
	if err != nil {
		t.Fatal(err)
	}
	if out != "verify PASS" || len(got) != 1 || got[0] != "internal/auth/auth.go" {
		t.Fatalf("out=%q targets=%v", out, got)
	}
}
