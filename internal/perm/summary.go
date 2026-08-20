package perm

import (
	"encoding/json"
	"path/filepath"
	"strings"
)

// FriendlyHint returns a short plain-language line explaining what the action does.
func FriendlyHint(req Request) string {
	switch req.Tool {
	case "bash":
		return hintBash(req.Command)
	case "write_file":
		return hintWrite(req.Path, true)
	case "edit_file":
		return hintWrite(req.Path, false)
	case "git":
		return hintGit(req.Command)
	case "web_fetch":
		return hintWebFetch(req.Summary)
	case "todo_write":
		return "Update the task checklist for this session"
	case "verify":
		return "Run the project's tests"
	case "mcp_manage":
		if req.Command == "add" {
			return "Add an MCP server (external tools). Approve only if you trust it."
		}
		if req.Command == "remove" {
			return "Remove an MCP server from Picogent"
		}
		return "Manage MCP servers"
	default:
		if strings.HasPrefix(req.Tool, "mcp_") {
			return hintMCP(req.Tool, req.Summary)
		}
		if req.Destructive {
			return "Run a potentially destructive action — review carefully before allowing"
		}
		if req.OutsideWorkspace {
			return "Access files outside the project folder"
		}
		if req.Path != "" {
			return "Modify " + filepath.Base(req.Path) + " in your project"
		}
		return "Allow the agent to continue this step"
	}
}

func hintBash(cmd string) string {
	c := strings.TrimSpace(cmd)
	lower := strings.ToLower(c)

	switch {
	case strings.HasPrefix(lower, "go test"), strings.Contains(lower, " go test"), strings.HasPrefix(lower, "make test"):
		return "Run tests to verify the code still works"
	case strings.HasPrefix(lower, "npm test"), strings.HasPrefix(lower, "pnpm test"), strings.HasPrefix(lower, "yarn test"):
		return "Run the JavaScript test suite"
	case strings.HasPrefix(lower, "pytest"), strings.Contains(lower, " pytest"):
		return "Run Python tests"
	case strings.HasPrefix(lower, "npm install"), strings.HasPrefix(lower, "pnpm install"), strings.HasPrefix(lower, "yarn install"):
		return "Install or update project dependencies"
	case strings.HasPrefix(lower, "go build"), strings.HasPrefix(lower, "npm run build"), strings.HasPrefix(lower, "make build"):
		return "Build the project to check it compiles"
	case strings.HasPrefix(lower, "go mod"), strings.HasPrefix(lower, "npm ci"):
		return "Sync module or package dependencies"
	case strings.HasPrefix(lower, "git status"):
		return "Check which files changed in git"
	case strings.HasPrefix(lower, "git diff"):
		return "Show the current code changes"
	case strings.HasPrefix(lower, "git log"):
		return "Inspect recent commit history"
	case strings.HasPrefix(lower, "git add"):
		return "Stage files for a commit"
	case strings.HasPrefix(lower, "git commit"):
		return "Create a git commit with the staged changes"
	case strings.HasPrefix(lower, "git checkout"), strings.HasPrefix(lower, "git restore"):
		return "Revert or switch files in git"
	case strings.HasPrefix(lower, "git push"):
		return "Push commits to the remote repository"
	case strings.HasPrefix(lower, "docker "), strings.HasPrefix(lower, "podman "):
		return "Run a container command"
	case strings.HasPrefix(lower, "curl "), strings.HasPrefix(lower, "wget "):
		return "Download something from the network"
	case strings.Contains(lower, "lint"), strings.Contains(lower, "fmt"), strings.Contains(lower, "format"):
		return "Run a formatter or linter on the codebase"
	case strings.HasPrefix(lower, "cd "):
		return "Change directory, then run: " + truncateHint(strings.TrimPrefix(c, "cd "), 60)
	case strings.HasPrefix(lower, "cat "), strings.HasPrefix(lower, "head "), strings.HasPrefix(lower, "tail "):
		return "Read file contents from the terminal"
	case strings.HasPrefix(lower, "ls"), strings.HasPrefix(lower, "find "), strings.HasPrefix(lower, "rg "), strings.HasPrefix(lower, "grep "):
		return "Search or list files from the terminal"
	case strings.HasPrefix(lower, "rm "):
		return "Delete files — review the paths before allowing"
	case strings.HasPrefix(lower, "mkdir"), strings.HasPrefix(lower, "touch "):
		return "Create files or folders via the shell"
	default:
		return "Run a shell command in your project folder"
	}
}

func hintWrite(path string, overwrite bool) string {
	base := filepath.Base(strings.TrimSpace(path))
	if base == "" || base == "." {
		base = path
	}
	if overwrite {
		return "Create or overwrite `" + base + "` with new content"
	}
	return "Apply a focused edit to `" + base + "`"
}

func hintGit(action string) string {
	switch strings.ToLower(strings.TrimSpace(action)) {
	case "status":
		return "Check git status for this repo"
	case "diff":
		return "Show uncommitted changes"
	case "commit":
		return "Stage and commit the current changes"
	default:
		return "Run git " + action + " on this repo"
	}
}

func hintWebFetch(summary string) string {
	s := strings.TrimPrefix(summary, "fetch ")
	if s != summary {
		return "Fetch a web page for context: " + truncateHint(s, 72)
	}
	return "Fetch a URL for reference material"
}

func hintMCP(tool, summary string) string {
	name := strings.TrimPrefix(tool, "mcp_")
	parts := strings.SplitN(name, "_", 2)
	service := name
	if len(parts) > 0 {
		service = parts[0]
	}
	if summary != "" && !strings.HasPrefix(summary, "mcp ") {
		return "Use " + service + ": " + truncateHint(summary, 72)
	}
	return "Call an external " + service + " integration"
}

func truncateHint(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// EnrichArgs parses tool JSON args to fill Path/Command when missing.
func EnrichHint(req Request, argsJSON string) string {
	if req.Path == "" || req.Command == "" {
		var m map[string]any
		if json.Unmarshal([]byte(argsJSON), &m) == nil {
			if req.Path == "" {
				if v, ok := m["path"].(string); ok {
					req.Path = v
				}
			}
			if req.Command == "" {
				if v, ok := m["command"].(string); ok {
					req.Command = v
				} else if v, ok := m["action"].(string); ok {
					req.Command = v
				}
			}
		}
	}
	return FriendlyHint(req)
}
