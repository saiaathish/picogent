package checkpoint_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"

	"github.com/saiaathish/picogent/internal/checkpoint"
)

func TestSealPreservesPreparedFingerprintBeforeSeal(t *testing.T) {
	for _, tc := range []struct {
		name    string
		created bool
		removed bool
		content string
		mode    os.FileMode
	}{
		{name: "user edit", content: "user", mode: 0o644},
		{name: "user edit to created file", created: true, content: "user", mode: 0o644},
		{name: "user deletion", removed: true},
		{name: "partial write", content: "ag", mode: 0o644},
		{name: "mode change", content: "agent", mode: 0o600},
	} {
		for _, fresh := range []bool{false, true} {
			name := tc.name + "/in-process"
			if fresh {
				name = tc.name + "/imported"
			}
			t.Run(name, func(t *testing.T) {
				if tc.name == "mode change" && runtime.GOOS == "windows" {
					t.Skip("Windows projects writable permission modes to the same fingerprint")
				}
				root := t.TempDir()
				if !tc.created {
					write(t, root, "note.txt", "before", 0o644)
				}
				write(t, root, "other.txt", "other before", 0o644)
				cp, err := checkpoint.Capture(root, []string{"note.txt", "other.txt"})
				if err != nil {
					t.Fatal(err)
				}
				if _, err := cp.PrepareExpected("note.txt", []byte("agent"), 0o644); err != nil {
					t.Fatal(err)
				}
				write(t, root, "note.txt", "agent", 0o644)
				prepared, err := cp.Export()
				if err != nil || len(prepared.Entries) != 1 {
					t.Fatalf("prepared record = %+v, err=%v", prepared, err)
				}
				// Unprepared captures must retain their existing snapshot-at-seal behavior.
				write(t, root, "other.txt", "other agent", 0o644)
				if tc.removed {
					if err := os.Remove(filepath.Join(root, "note.txt")); err != nil {
						t.Fatal(err)
					}
				} else {
					write(t, root, "note.txt", tc.content, tc.mode)
				}
				if err := cp.Seal(); err != nil {
					t.Fatal(err)
				}
				record, err := cp.Export()
				if err != nil || len(record.Entries) != 2 {
					t.Fatalf("sealed record = %+v, err=%v", record, err)
				}
				if record.Entries[0].Expected != prepared.Entries[0].Expected || record.Entries[0].Published != "" {
					t.Fatalf("seal replaced the prepared publication: prepared=%+v sealed=%+v", prepared.Entries[0], record.Entries[0])
				}
				if fresh {
					cp = importSealedRecord(t, root, record)
				}
				result, err := cp.Restore()
				if !errors.Is(err, checkpoint.ErrConflict) || result.Complete || len(result.Conflicts) != 1 || result.Conflicts[0].Path != "note.txt" {
					t.Fatalf("restore accepted an unprepared state: result=%+v err=%v", result, err)
				}
				assertContents(t, root, "other.txt", "other agent")
				if tc.removed {
					if _, err := os.Stat(filepath.Join(root, "note.txt")); !errors.Is(err, os.ErrNotExist) {
						t.Fatalf("undo recreated a user-deleted file: %v", err)
					}
				} else {
					assertContents(t, root, "note.txt", tc.content)
					info, err := os.Stat(filepath.Join(root, "note.txt"))
					if err != nil {
						t.Fatal(err)
					}
					if runtime.GOOS != "windows" && info.Mode().Perm() != tc.mode {
						t.Fatalf("undo changed the user mode: got=%o want=%o", info.Mode().Perm(), tc.mode)
					}
				}
			})
		}
	}
}

func TestSealResolvesPreparedWriteFailure(t *testing.T) {
	for _, tc := range []struct {
		name         string
		publishFirst bool
		next         string
		current      string
		conflict     bool
	}{
		{name: "first rename failed", current: "before"},
		{name: "error after publication", publishFirst: true, current: "agent"},
		{name: "later rename failed", publishFirst: true, next: "second", current: "agent"},
		{name: "return to before rename failed", publishFirst: true, next: "before", current: "agent"},
		{name: "return to before published", publishFirst: true, next: "before", current: "before"},
		{name: "user edit after preparing return to before", publishFirst: true, next: "before", current: "user", conflict: true},
	} {
		for _, fresh := range []bool{false, true} {
			name := tc.name + "/in-process"
			if fresh {
				name = tc.name + "/imported"
			}
			t.Run(name, func(t *testing.T) {
				root := t.TempDir()
				write(t, root, "note.txt", "before", 0o644)
				cp, err := checkpoint.Capture(root, []string{"note.txt"})
				if err != nil {
					t.Fatal(err)
				}
				if _, err := cp.PrepareExpected("note.txt", []byte("agent"), 0o644); err != nil {
					t.Fatal(err)
				}
				first, err := cp.Export()
				if err != nil || len(first.Entries) != 1 {
					t.Fatalf("first prepared record = %+v, err=%v", first, err)
				}
				if tc.publishFirst {
					write(t, root, "note.txt", "agent", 0o644)
				}
				if tc.next != "" {
					if _, err := cp.PrepareExpected("note.txt", []byte(tc.next), 0o644); err != nil {
						t.Fatal(err)
					}
				}
				write(t, root, "note.txt", tc.current, 0o644)
				if err := cp.Seal(); err != nil {
					t.Fatal(err)
				}
				record, err := cp.Export()
				if err != nil {
					t.Fatal(err)
				}
				paths, err := cp.ChangedPaths()
				if err != nil {
					t.Fatal(err)
				}
				if tc.current == "before" {
					if len(paths) != 0 || len(record.Entries) != 0 {
						t.Fatalf("unpublished or reverted write retained undo: paths=%v record=%+v", paths, record)
					}
					// Empty records are deliberately not importable: no durable
					// undo should be offered for an unpublished or reverted write.
					result, err := cp.Restore()
					if err != nil || !result.Complete || !slices.Equal(result.Unchanged, []string{"note.txt"}) {
						t.Fatalf("no-op restore = %+v, err=%v", result, err)
					}
					assertContents(t, root, "note.txt", "before")
					return
				}
				if !slices.Equal(paths, []string{"note.txt"}) || len(record.Entries) != 1 || record.Entries[0].Expected != first.Entries[0].Expected || record.Entries[0].Published != "" {
					t.Fatalf("seal lost the actual publication: paths=%v record=%+v first=%+v", paths, record, first)
				}
				if fresh {
					cp = importSealedRecord(t, root, record)
				}
				result, err := cp.Restore()
				if tc.conflict {
					if !errors.Is(err, checkpoint.ErrConflict) || result.Complete {
						t.Fatalf("undo accepted a newer edit after a failed write: result=%+v err=%v", result, err)
					}
					assertContents(t, root, "note.txt", tc.current)
				} else {
					if err != nil || !result.Complete {
						t.Fatalf("undo lost a published write after an error: result=%+v err=%v", result, err)
					}
					assertContents(t, root, "note.txt", "before")
				}
			})
		}
	}
}

func importSealedRecord(t *testing.T, root string, record checkpoint.Record) *checkpoint.Checkpoint {
	t.Helper()
	encoded, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	var decoded checkpoint.Record
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	cp, err := checkpoint.Import(root, decoded)
	if err != nil {
		t.Fatal(err)
	}
	return cp
}
