package tools

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	neturl "net/url"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/saiaathish/picogent/internal/llm"
	"github.com/saiaathish/picogent/internal/perm"
)

const maxDirEntries = 200

type listDir struct{}

func (listDir) Spec() llm.ToolSpec {
	return llm.ToolSpec{
		Name:        "list_dir",
		Description: "List files and directories in a workspace path (like Claude Code's directory listing).",
		Parameters: schema(map[string]any{
			"path": map[string]any{"type": "string", "description": "Directory relative to workspace (default .)"},
		}, []string{}),
	}
}

func (listDir) Permission(args string, c Context) perm.Request {
	var in struct {
		Path string `json:"path"`
	}
	_ = parseJSON(args, &in)
	p := in.Path
	if p == "" {
		p = "."
	}
	return c.ClassifyPath("list_dir", p, c.Workspace, "list "+p)
}

func (listDir) Run(_ context.Context, args string, c Context) (string, error) {
	var in struct {
		Path string `json:"path"`
	}
	if err := parseJSON(args, &in); err != nil {
		return "", err
	}
	ws, err := mustWorkspace(c)
	if err != nil {
		return "", err
	}
	p := in.Path
	if p == "" {
		p = "."
	}
	abs, err := resolvePath(ws, p)
	if err != nil {
		return "", err
	}
	ents, err := os.ReadDir(abs)
	if err != nil {
		return "", err
	}
	sort.Slice(ents, func(i, j int) bool {
		if ents[i].IsDir() != ents[j].IsDir() {
			return ents[i].IsDir()
		}
		return ents[i].Name() < ents[j].Name()
	})
	var lines []string
	for i, e := range ents {
		if i >= maxDirEntries {
			lines = append(lines, fmt.Sprintf("… %d more …", len(ents)-maxDirEntries))
			break
		}
		tag := "file"
		if e.IsDir() {
			tag = "dir"
		}
		lines = append(lines, fmt.Sprintf("[%s] %s", tag, e.Name()))
	}
	return clip(strings.Join(lines, "\n")), nil
}

type webFetch struct{}

func (webFetch) Spec() llm.ToolSpec {
	return llm.ToolSpec{
		Name:        "web_fetch",
		Description: "Fetch a URL and return readable text (Claude Code WebFetch). HTML is stripped to plain text.",
		Parameters: schema(map[string]any{
			"url": map[string]any{"type": "string", "description": "HTTP or HTTPS URL"},
		}, []string{"url"}),
	}
}

func (webFetch) Permission(args string, _ Context) perm.Request {
	var in struct {
		URL string `json:"url"`
	}
	_ = parseJSON(args, &in)
	return perm.Request{Tool: "web_fetch", Summary: "fetch " + truncate(in.URL, 80)}
}

func (webFetch) Run(ctx context.Context, args string, _ Context) (string, error) {
	var in struct {
		URL string `json:"url"`
	}
	if err := parseJSON(args, &in); err != nil {
		return "", err
	}
	url := strings.TrimSpace(in.URL)
	if url == "" {
		return "", fmt.Errorf("url is required")
	}
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		return "", fmt.Errorf("url must be http or https")
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	parsed, err := parseWebFetchURL(url)
	if err != nil {
		return "", err
	}
	if err := validateWebFetchURL(ctx, parsed); err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Picogent/1.0.0")
	client := *http.DefaultClient
	client.CheckRedirect = func(next *http.Request, _ []*http.Request) error {
		if err := validateWebFetchURL(next.Context(), next.URL); err != nil {
			return err
		}
		return nil
	}
	res, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, 512<<10))
	if err != nil {
		return "", err
	}
	text := string(body)
	if strings.Contains(res.Header.Get("Content-Type"), "html") {
		text = stripHTML(text)
	}
	if res.StatusCode >= 400 {
		return "", fmt.Errorf("HTTP %d: %s", res.StatusCode, truncate(text, 200))
	}
	return clip(text), nil
}

// parseWebFetchURL is kept separate so the request path has one URL parser and the
// SSRF guard can be unit-tested without making a network request.
func parseWebFetchURL(raw string) (*neturl.URL, error) {
	u, err := neturl.ParseRequestURI(raw)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %w", err)
	}
	if u.Hostname() == "" {
		return nil, fmt.Errorf("URL host is required")
	}
	if u.User != nil {
		return nil, fmt.Errorf("URL credentials are not allowed")
	}
	return u, nil
}

func validateWebFetchURL(ctx context.Context, u *neturl.URL) error {
	if u == nil || (u.Scheme != "http" && u.Scheme != "https") {
		return fmt.Errorf("URL must use http or https")
	}
	host := strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
	if host == "" {
		return fmt.Errorf("URL host is required")
	}
	if host == "localhost" || strings.HasSuffix(host, ".localhost") || strings.HasSuffix(host, ".local") {
		return fmt.Errorf("refusing private or local URL host %q", host)
	}
	ips := net.ParseIP(host)
	if ips != nil {
		if blockedWebFetchIP(ips) {
			return fmt.Errorf("refusing private or local URL host %q", host)
		}
		return nil
	}
	resolved, err := net.DefaultResolver.LookupIP(ctx, "ip", host)
	if err != nil || len(resolved) == 0 {
		return fmt.Errorf("URL host %q could not be resolved", host)
	}
	for _, ip := range resolved {
		if blockedWebFetchIP(ip) {
			return fmt.Errorf("refusing URL host %q because it resolves to a private or local address", host)
		}
	}
	return nil
}

func blockedWebFetchIP(ip net.IP) bool {
	if ip == nil || ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() || ip.IsMulticast() {
		return true
	}
	// Go's IsPrivate intentionally excludes the shared carrier-grade NAT range.
	if v4 := ip.To4(); v4 != nil && v4[0] == 100 && v4[1] >= 64 && v4[1] <= 127 {
		return true
	}
	return false
}

func stripHTML(s string) string {
	var b strings.Builder
	inTag := false
	for _, r := range s {
		switch {
		case r == '<':
			inTag = true
		case r == '>':
			inTag = false
		case !inTag:
			b.WriteRune(r)
		}
	}
	out := strings.Join(strings.Fields(b.String()), " ")
	if len(out) > maxToolOut {
		return out[:maxToolOut] + " …"
	}
	return out
}
