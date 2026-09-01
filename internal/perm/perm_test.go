package perm_test

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"

	"github.com/saiaathish/picogent/internal/config"
	"github.com/saiaathish/picogent/internal/perm"
)

func TestFastAllowsInWorkspaceWrite(t *testing.T) {
	g := perm.New(config.ModeFast, "/tmp/ws", nil)
	d, err := g.Check(nil, perm.Request{Tool: "write_file", Path: "a.go"})
	if err != nil || d != perm.Allow {
		t.Fatalf("%v %v", d, err)
	}
}

func TestFastRequiresApprovalForWebFetch(t *testing.T) {
	g := perm.New(config.ModeFast, "/tmp/ws", nil)
	d, err := g.Check(context.Background(), perm.Request{Tool: "web_fetch", Summary: "fetch https://example.com"})
	if err != nil || d != perm.Deny {
		t.Fatalf("web fetch decision = %s, %v; want deny without explicit approval", d, err)
	}
}

func TestFastStillBlocksRM(t *testing.T) {
	g := perm.New(config.ModeFast, "/tmp/ws", nil)
	req := perm.ClassifyBash("rm -rf /", "/tmp/ws")
	d, _ := g.Check(nil, req)
	if d != perm.Deny {
		t.Fatalf("expected deny, got %s", d)
	}
}

func TestFastBashRequiresExplicitApprovalEvenForSimpleCommands(t *testing.T) {
	workspace := t.TempDir()
	for _, command := range []string{"go test ./...", "git status --short", "rg TODO internal", "cat README.md"} {
		req := perm.ClassifyBash(command, workspace)
		if req.OutsideWorkspace {
			t.Fatalf("simple command %q unexpectedly outside: %+v", command, req)
		}
		if got, err := perm.New(config.ModeFast, workspace, nil).Check(nil, req); err != nil || got != perm.Deny {
			t.Fatalf("simple command %q decision = %s, %v; want explicit approval", command, got, err)
		}
	}
}

func TestFastBashRunsAfterExplicitApproval(t *testing.T) {
	workspace := t.TempDir()
	g := perm.New(config.ModeFast, workspace, func(context.Context, perm.Request) (perm.Decision, error) {
		return perm.Allow, nil
	})
	if got, err := g.Check(context.Background(), perm.ClassifyBash("git status --short", workspace)); err != nil || got != perm.Allow {
		t.Fatalf("approved bash decision = %s, %v; want allow", got, err)
	}
}

func TestBashBoundaryRequiresApprovalForEscapeSyntax(t *testing.T) {
	workspace := t.TempDir()
	for _, command := range []string{
		"cat /etc/passwd",
		"cat ../secrets.txt",
		"cat $HOME/.ssh/id_rsa",
		"go test ./... && cat /etc/passwd",
		"printf secret > note.txt",
		"python -c 'open(\"/tmp/pwned\", \"w\").write(\"x\")'",
	} {
		req := perm.ClassifyBash(command, workspace)
		if req.OutsideWorkspace && req.Hint == "" {
			t.Fatalf("unsafe command %q did not explain approval boundary", command)
		}
		if got, err := perm.New(config.ModeFast, workspace, nil).Check(nil, req); err != nil || got != perm.Deny {
			t.Fatalf("unsafe command %q decision = %s, %v; want deny without prompt", command, got, err)
		}
	}
}

func TestBashBoundaryAllowsAbsolutePathInsideWorkspace(t *testing.T) {
	workspace := t.TempDir()
	req := perm.ClassifyBash("cat "+filepath.Join(workspace, "README.md"), workspace)
	if req.OutsideWorkspace {
		t.Fatalf("absolute in-workspace path was rejected: %+v", req)
	}
}

func TestBashBoundaryRejectsWindowsEscapingPaths(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("backslashes are shell escapes on POSIX")
	}
	workspace := t.TempDir()
	for _, command := range []string{
		`cat ..\secrets.txt`,
		`cat \Windows\System32\drivers\etc\hosts`,
	} {
		req := perm.ClassifyBash(command, workspace)
		if !req.OutsideWorkspace {
			t.Fatalf("Windows escaping path %q was auto-allowable: %+v", command, req)
		}
	}
}

func TestFastRequiresApprovalForVerify(t *testing.T) {
	g := perm.New(config.ModeFast, "/tmp/ws", nil)
	d, _ := g.Check(nil, perm.Request{Tool: "verify"})
	if d != perm.Deny {
		t.Fatalf("expected explicit approval, got %s", d)
	}
}

func TestSafeRequiresApprovalForVerify(t *testing.T) {
	g := perm.New(config.ModeSafe, "/tmp/ws", nil)
	d, _ := g.Check(nil, perm.Request{Tool: "verify"})
	if d != perm.Deny {
		t.Fatalf("expected explicit approval, got %s", d)
	}
}

func TestProjectHealthIsReadOnlyAndAutoAllowed(t *testing.T) {
	g := perm.New(config.ModeSafe, "/tmp/ws", nil)
	d, err := g.Check(nil, perm.Request{Tool: "project_health", Path: "/tmp/ws"})
	if err != nil || d != perm.Allow {
		t.Fatalf("project health decision = %s, %v; want allow", d, err)
	}
}

func TestFastMCPAsksForWriteTools(t *testing.T) {
	g := perm.New(config.ModeFast, "/tmp/ws", nil)
	d, _ := g.Check(nil, perm.Request{Tool: "mcp_github_create_issue"})
	if d != perm.Deny {
		t.Fatalf("write-like MCP should ask in Fast, got %s", d)
	}
	d, _ = g.Check(nil, perm.Request{Tool: "mcp_browseros_neo_tabs"})
	if d != perm.Deny {
		t.Fatalf("MCP tools should require approval in Fast, got %s", d)
	}
}

func TestMCPManageListAutoAllows(t *testing.T) {
	g := perm.New(config.ModeSafe, "/tmp/ws", nil)
	d, _ := g.Check(nil, perm.Request{Tool: "mcp_manage", Command: "list"})
	if d != perm.Allow {
		t.Fatalf("list should auto-allow, got %s", d)
	}
	d, _ = g.Check(nil, perm.Request{Tool: "mcp_manage", Command: "add", Summary: "MCP add mcp-github"})
	if d != perm.Deny {
		t.Fatalf("add should ask, got %s", d)
	}
}

func TestSafeBlocksWriteWithoutPrompter(t *testing.T) {
	g := perm.New(config.ModeSafe, "/tmp/ws", nil)
	d, _ := g.Check(nil, perm.Request{Tool: "write_file", Path: "a.go"})
	if d != perm.Deny {
		t.Fatalf("expected deny, got %s", d)
	}
}

func TestCheckWithProvenanceDistinguishesPromptedDecisions(t *testing.T) {
	workspace := t.TempDir()
	req := perm.Request{Tool: "write_file", Path: "note.txt"}

	decision, prompted, err := perm.New(config.ModeFast, workspace, nil).CheckWithProvenance(context.Background(), req)
	if err != nil || decision != perm.Allow || prompted {
		t.Fatalf("Fast decision = %s, prompted=%v, err=%v; want automatic allow", decision, prompted, err)
	}

	decision, prompted, err = perm.New(config.ModeSafe, workspace, func(context.Context, perm.Request) (perm.Decision, error) {
		return perm.Allow, nil
	}).CheckWithProvenance(context.Background(), req)
	if err != nil || decision != perm.Allow || !prompted {
		t.Fatalf("prompted decision = %s, prompted=%v, err=%v; want prompted allow", decision, prompted, err)
	}

	decision, prompted, err = perm.New(config.ModeSafe, workspace, func(context.Context, perm.Request) (perm.Decision, error) {
		return perm.Deny, nil
	}).CheckWithProvenance(context.Background(), req)
	if err != nil || decision != perm.Deny || !prompted {
		t.Fatalf("prompted denial = %s, prompted=%v, err=%v; want prompted deny", decision, prompted, err)
	}
}

func TestClassifyPathResolvesOutsideSymlinkBeforeFastApproval(t *testing.T) {
	workspace := t.TempDir()
	outside := t.TempDir()
	target := filepath.Join(outside, "secret.txt")
	if err := os.WriteFile(target, []byte("private"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(workspace, "escape")); err != nil {
		if runtime.GOOS == "windows" {
			t.Skip("symlink creation requires privileges on Windows")
		}
		t.Fatal(err)
	}

	req := perm.ClassifyPath("write_file", "escape/secret.txt", workspace, "write escape/secret.txt")
	if !req.OutsideWorkspace {
		t.Fatalf("symlink target was classified in-workspace: %+v", req)
	}
	want, err := filepath.EvalSymlinks(target)
	if err != nil {
		t.Fatal(err)
	}
	if req.Path != want {
		t.Fatalf("resolved path = %q, want %q", req.Path, want)
	}

	gate := perm.New(config.ModeFast, workspace, nil)
	if got, err := gate.Check(context.Background(), req); err != nil || got != perm.Deny {
		t.Fatalf("Fast decision = %s, %v; want deny", got, err)
	}
	gate.AddAlwaysAllowed("write_file")
	if got, err := gate.Check(context.Background(), req); err != nil || got != perm.Deny {
		t.Fatalf("always-allowed decision = %s, %v; want deny", got, err)
	}
}

func TestOutsidePathStillRequiresAndHonorsExplicitApproval(t *testing.T) {
	workspace := t.TempDir()
	outside := t.TempDir()
	req := perm.ClassifyPath("write_file", filepath.Join(outside, "note.txt"), workspace, "write note.txt")
	if !req.OutsideWorkspace {
		t.Fatalf("outside path was classified in-workspace: %+v", req)
	}

	prompts := 0
	gate := perm.New(config.ModeFast, workspace, func(_ context.Context, got perm.Request) (perm.Decision, error) {
		prompts++
		if !got.OutsideWorkspace {
			t.Fatalf("prompt request = %+v, want outside workspace", got)
		}
		return perm.Allow, nil
	})
	gate.AddAlwaysAllowed("write_file")
	for i := 0; i < 2; i++ {
		if got, err := gate.Check(context.Background(), req); err != nil || got != perm.Allow {
			t.Fatalf("approval %d = %s, %v; want allow", i, got, err)
		}
	}
	if prompts != 2 {
		t.Fatalf("outside path prompts = %d, want 2", prompts)
	}
}

func TestGateConcurrentStateUpdates(t *testing.T) {
	gate := perm.New(config.ModeSafe, "/tmp/ws", func(context.Context, perm.Request) (perm.Decision, error) {
		return perm.AllowAlways, nil
	})

	const workers = 8
	const iterations = 200
	var wg sync.WaitGroup
	for worker := 0; worker < workers; worker++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				switch (worker + i) % 5 {
				case 0:
					gate.SetAlwaysAllowed([]string{"write_file", "edit_file"})
				case 1:
					gate.AddAlwaysAllowed("mcp_manage")
				case 2:
					gate.SetMode(config.ModeFast)
				case 3:
					gate.SetMode(config.ModeSafe)
					gate.ResetTurn()
				default:
					gate.SetPrompt(func(context.Context, perm.Request) (perm.Decision, error) {
						return perm.AllowAlways, nil
					})
				}
				_, _ = gate.Check(context.Background(), perm.Request{Tool: "mcp_manage", Command: "add"})
			}
		}(worker)
	}
	wg.Wait()
}

func TestCloneForTurnFreezesPermissionMode(t *testing.T) {
	live := perm.New(config.ModeSafe, "/tmp/ws", nil)
	turn := live.CloneForTurn()
	live.SetMode(config.ModeFast)
	req := perm.Request{Tool: "write_file", Path: "a.go"}
	if got, err := turn.Check(context.Background(), req); err != nil || got != perm.Deny {
		t.Fatalf("cloned turn decision = %s, %v; want safe deny", got, err)
	}
	if got, err := live.Check(context.Background(), req); err != nil || got != perm.Allow {
		t.Fatalf("live gate decision = %s, %v; want fast allow", got, err)
	}
}
