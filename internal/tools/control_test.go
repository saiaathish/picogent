package tools

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/saiaathish/picogent/internal/mcpbridge"
)

func TestAttachMCPKeepsManage(t *testing.T) {
	r := NewRegistry(Context{})
	defer r.Close()
	if _, ok := r.Get("mcp_manage"); !ok {
		t.Fatal("mcp_manage missing before AttachMCP")
	}
	if err := r.AttachMCP(&mcpbridge.Manager{}); err != nil {
		t.Fatal(err)
	}
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

func TestRegistryMCPReplacementRetiresOldRuntime(t *testing.T) {
	r := NewRegistry(Context{})
	old := &mcpbridge.Manager{}
	if err := r.AttachMCP(old); err != nil {
		t.Fatal(err)
	}
	oldView := r.MCPManagerSnapshot()
	if oldView == nil || oldView == old || !oldView.SameRuntime(old) {
		t.Fatal("registry did not attach a lease-backed view")
	}

	next := &mcpbridge.Manager{}
	if err := r.AttachMCP(next); err != nil {
		t.Fatal(err)
	}
	if _, err := old.Acquire(); !errors.Is(err, mcpbridge.ErrManagerClosed) {
		t.Fatalf("replaced manager acquire = %v, want ErrManagerClosed", err)
	}
	current := r.MCPManagerSnapshot()
	if current == nil || !current.SameRuntime(next) || current.SameRuntime(old) {
		t.Fatal("registry retained the replaced MCP runtime")
	}

	r.Close()
	if _, err := next.Acquire(); !errors.Is(err, mcpbridge.ErrManagerClosed) {
		t.Fatalf("closed registry left manager attachable: %v", err)
	}
	if err := r.AttachMCP(&mcpbridge.Manager{}); !errors.Is(err, ErrRegistryClosed) {
		t.Fatalf("late attach error = %v, want ErrRegistryClosed", err)
	}
	r.Close()
}

func TestRegistryMCPRefreshKeepsCurrentLease(t *testing.T) {
	r := NewRegistry(Context{})
	root := &mcpbridge.Manager{}
	if err := r.AttachMCP(root); err != nil {
		t.Fatal(err)
	}
	view := r.MCPManagerSnapshot()
	if err := r.AttachMCP(view); err != nil {
		t.Fatal(err)
	}
	if got := r.MCPManagerSnapshot(); got != view {
		t.Fatal("refresh replaced the current lease view")
	}
	r.Close()
}

func TestRegistryRejectsClosedMCPWithoutDroppingCurrent(t *testing.T) {
	r := NewRegistry(Context{})
	current := &mcpbridge.Manager{}
	if err := r.AttachMCP(current); err != nil {
		t.Fatal(err)
	}
	closed := &mcpbridge.Manager{}
	closed.Close()
	if err := r.AttachMCP(closed); !errors.Is(err, mcpbridge.ErrManagerClosed) {
		t.Fatalf("closed manager attach = %v, want ErrManagerClosed", err)
	}
	if got := r.MCPManagerSnapshot(); got == nil || !got.SameRuntime(current) {
		t.Fatal("failed MCP attach dropped the current runtime")
	}
	r.Close()
}

func TestRegistryConcurrentAttachAndRead(t *testing.T) {
	r := NewRegistry(Context{})
	defer r.Close()
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
