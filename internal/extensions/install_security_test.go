package extensions

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateSkillRepoRejectsProtocolAndArgumentInjection(t *testing.T) {
	for _, repo := range []string{"", "--upload-pack=evil", "ext::sh -c evil", "http://example.test/skill.git"} {
		if err := validateSkillRepo(repo); err == nil {
			t.Fatalf("accepted unsafe skill repository %q", repo)
		}
	}
	for _, repo := range []string{
		"https://github.com/cursor/skills-cursor",
		"ssh://git@example.test/skills.git",
		"git@example.test:skills.git",
		"file:///tmp/skills.git",
	} {
		if err := validateSkillRepo(repo); err != nil {
			t.Fatalf("rejected supported skill repository %q: %v", repo, err)
		}
	}
}

func TestCopyDirRejectsOversizedSkillBeforePublication(t *testing.T) {
	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "large.txt"), []byte(strings.Repeat("x", maxSkillInstallBytes+1)), 0o600); err != nil {
		t.Fatal(err)
	}
	dest := t.TempDir()
	root, err := os.OpenRoot(dest)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	sourceRoot, err := os.OpenRoot(src)
	if err != nil {
		t.Fatal(err)
	}
	defer sourceRoot.Close()
	if err := copyDir(root, sourceRoot, "skill"); err == nil {
		t.Fatal("oversized skill was copied")
	}
	if _, err := os.Stat(filepath.Join(dest, "skill", "large.txt")); !os.IsNotExist(err) {
		t.Fatalf("oversized skill left a published file: %v", err)
	}
}

func TestReadClaudeMarketplaceBodyRejectsOversizedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, strings.Repeat("x", maxClaudeMarketplaceSize+1))
	}))
	defer server.Close()
	resp, err := http.Get(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if _, err := readClaudeMarketplaceBody(resp); err == nil {
		t.Fatal("oversized marketplace body was accepted")
	}
}

func TestReadClaudeMarketplaceBodyRejectsNonOK(t *testing.T) {
	resp := &http.Response{StatusCode: http.StatusBadGateway, Status: "502 Bad Gateway", Body: io.NopCloser(strings.NewReader("bad"))}
	if _, err := readClaudeMarketplaceBody(resp); err == nil || !strings.Contains(fmt.Sprint(err), "502") {
		t.Fatalf("non-OK marketplace response = %v", err)
	}
}
