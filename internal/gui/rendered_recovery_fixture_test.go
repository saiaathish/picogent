//go:build rendered_fixture

package gui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRenderedRecoveryFixtureSeedRequiresFreshContainedState(t *testing.T) {
	t.Run("accepts empty disposable paths", func(t *testing.T) {
		home := filepath.Join(t.TempDir(), "home")
		workspace := filepath.Join(home, "workspace")
		t.Setenv("PICOGENT_RENDERED_FIXTURE_HOME", home)
		t.Setenv("PICOGENT_RENDERED_FIXTURE_WORKSPACE", workspace)

		gotHome, gotWorkspace, err := renderedRecoveryFixturePaths("seed")
		if err != nil {
			t.Fatal(err)
		}
		if gotHome != filepath.Clean(home) || gotWorkspace != filepath.Clean(workspace) {
			t.Fatalf("paths = %q, %q; want %q, %q", gotHome, gotWorkspace, home, workspace)
		}
	})

	t.Run("rejects nonempty home", func(t *testing.T) {
		home := t.TempDir()
		if err := os.WriteFile(filepath.Join(home, "marker"), []byte("not disposable"), 0o600); err != nil {
			t.Fatal(err)
		}
		t.Setenv("PICOGENT_RENDERED_FIXTURE_HOME", home)
		t.Setenv("PICOGENT_RENDERED_FIXTURE_WORKSPACE", "")

		if _, _, err := renderedRecoveryFixturePaths("seed"); err == nil || !strings.Contains(err.Error(), "must be empty") {
			t.Fatalf("seed error = %v, want an empty-home error", err)
		}
	})

	t.Run("rejects workspace outside home", func(t *testing.T) {
		home := filepath.Join(t.TempDir(), "home")
		workspace := filepath.Join(filepath.Dir(home), "outside-workspace")
		t.Setenv("PICOGENT_RENDERED_FIXTURE_HOME", home)
		t.Setenv("PICOGENT_RENDERED_FIXTURE_WORKSPACE", workspace)

		if _, _, err := renderedRecoveryFixturePaths("seed"); err == nil || !strings.Contains(err.Error(), "contained by fixture home") {
			t.Fatalf("seed error = %v, want a containment error", err)
		}
	})

	t.Run("rejects home outside temp", func(t *testing.T) {
		t.Setenv("PICOGENT_RENDERED_FIXTURE_HOME", filepath.Dir(os.TempDir()))
		t.Setenv("PICOGENT_RENDERED_FIXTURE_WORKSPACE", "")

		if _, _, err := renderedRecoveryFixturePaths("seed"); err == nil || !strings.Contains(err.Error(), "disposable path") {
			t.Fatalf("seed error = %v, want a disposable-path error", err)
		}
	})
}

func TestRenderedRecoveryFixtureReloadRequiresAbsentProbeAndTask(t *testing.T) {
	home := t.TempDir()
	workspace := filepath.Join(home, "workspace")
	if err := os.Mkdir(workspace, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PICOGENT_RENDERED_FIXTURE_HOME", home)
	t.Setenv("PICOGENT_RENDERED_FIXTURE_WORKSPACE", workspace)

	if _, _, err := renderedRecoveryFixturePaths("reload"); err == nil || !strings.Contains(err.Error(), "durable fixture task") {
		t.Fatalf("reload error = %v, want a durable-task error", err)
	}
}

func TestRenderedRecoveryFixtureManifestIsContainedAndExclusive(t *testing.T) {
	home := t.TempDir()
	inside := filepath.Join(home, "manifest.json")
	if got, err := renderedRecoveryFixtureManifestPath(home, inside); err != nil || got != inside {
		t.Fatalf("manifest path = %q, %v; want %q", got, err, inside)
	}
	if _, err := renderedRecoveryFixtureManifestPath(home, filepath.Join(filepath.Dir(home), "outside.json")); err == nil {
		t.Fatal("manifest path accepted a file outside fixture home")
	}

	manifest := renderedRecoveryFixtureManifest{Issue: "291", Source: "UNRECORDED"}
	if err := writeRenderedRecoveryFixtureManifest(inside, manifest); err != nil {
		t.Fatal(err)
	}
	if err := writeRenderedRecoveryFixtureManifest(inside, manifest); err == nil {
		t.Fatal("manifest write overwrote an existing file")
	}
	data, err := os.ReadFile(inside)
	if err != nil {
		t.Fatal(err)
	}
	var got renderedRecoveryFixtureManifest
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got.Issue != manifest.Issue || got.Source != manifest.Source {
		t.Fatalf("manifest = %#v, want %#v", got, manifest)
	}
}

func TestRenderedRecoveryFixtureSourceRequiresFullGitSHA(t *testing.T) {
	t.Setenv("PICOGENT_RENDERED_FIXTURE_SOURCE_SHA", "not-a-git-sha")
	if _, _, _, err := renderedRecoveryFixtureSource(); err == nil || !strings.Contains(err.Error(), "full 40-character Git SHA") {
		t.Fatalf("source error = %v, want a full-SHA error", err)
	}
}
