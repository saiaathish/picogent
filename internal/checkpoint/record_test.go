package checkpoint_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/saiaathish/picogent/internal/checkpoint"
)

func TestRecordRoundTripRestoresChangedFiles(t *testing.T) {
	workspace := t.TempDir()
	write(t, workspace, "existing.txt", "before", 0o640)
	cp, err := checkpoint.Capture(workspace, []string{"existing.txt", "created.txt"})
	if err != nil {
		t.Fatal(err)
	}
	write(t, workspace, "existing.txt", "after", 0o640)
	write(t, workspace, "created.txt", "new", 0o644)
	if err := cp.Seal(); err != nil {
		t.Fatal(err)
	}
	record, err := cp.Export()
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	var decoded checkpoint.Record
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	restored, err := checkpoint.Import(workspace, decoded)
	if err != nil {
		t.Fatal(err)
	}
	result, err := restored.Restore()
	if err != nil || !result.Complete {
		t.Fatalf("restored checkpoint = %+v, err=%v", result, err)
	}
	if got, err := os.ReadFile(filepath.Join(workspace, "existing.txt")); err != nil || string(got) != "before" {
		t.Fatalf("existing file = %q, err=%v", got, err)
	}
	if _, err := os.Stat(filepath.Join(workspace, "created.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("created file still exists: %v", err)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(filepath.Join(workspace, "existing.txt"))
		if err != nil {
			t.Fatal(err)
		}
		if got := info.Mode().Perm(); got != 0o640 {
			t.Fatalf("restored mode=%o, want 640", got)
		}
	}
}

func TestPublishedSubsetDropsUnpublishedPendingEntries(t *testing.T) {
	workspace := t.TempDir()
	write(t, workspace, "first.txt", "first before", 0o644)
	write(t, workspace, "second.txt", "second before", 0o644)
	cp, err := checkpoint.Capture(workspace, []string{"first.txt", "second.txt"})
	if err != nil {
		t.Fatal(err)
	}
	mode := os.FileMode(0o644)
	changed, err := cp.PrepareExpected("first.txt", []byte("first after"), mode)
	if err != nil || !changed {
		t.Fatalf("prepare first = changed:%v err:%v", changed, err)
	}
	write(t, workspace, "first.txt", "first after", mode)
	record, err := cp.Export()
	if err != nil {
		t.Fatal(err)
	}
	pending, err := checkpoint.Import(workspace, record)
	if err != nil {
		t.Fatal(err)
	}
	subset, found, err := pending.PublishedSubset()
	if err != nil || !found || subset == nil {
		t.Fatalf("published subset = %#v found:%v err:%v", subset, found, err)
	}
	if got := subset.Paths(); len(got) != 1 || got[0] != "first.txt" {
		t.Fatalf("published paths = %#v", got)
	}
	if _, err := subset.Restore(); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(filepath.Join(workspace, "first.txt")); err != nil || string(got) != "first before" {
		t.Fatalf("first file = %q, err=%v", got, err)
	}
	if got, err := os.ReadFile(filepath.Join(workspace, "second.txt")); err != nil || string(got) != "second before" {
		t.Fatalf("second file = %q, err=%v", got, err)
	}
}

func TestImportedRestoreResumesAfterEarlierPathWasAlreadyRestored(t *testing.T) {
	workspace := t.TempDir()
	write(t, workspace, "first.txt", "first before", 0o644)
	write(t, workspace, "second.txt", "second before", 0o644)
	cp, err := checkpoint.Capture(workspace, []string{"first.txt", "second.txt"})
	if err != nil {
		t.Fatal(err)
	}
	write(t, workspace, "first.txt", "first after", 0o644)
	write(t, workspace, "second.txt", "second after", 0o644)
	if err := cp.Seal(); err != nil {
		t.Fatal(err)
	}
	record, err := cp.Export()
	if err != nil {
		t.Fatal(err)
	}

	// Simulate a process dying after the first atomic restore and before the
	// durable undo journal can advance from sealed to restored.
	write(t, workspace, "first.txt", "first before", 0o644)
	restarted, err := checkpoint.Import(workspace, record)
	if err != nil {
		t.Fatal(err)
	}
	result, err := restarted.Restore()
	if err != nil || !result.Complete {
		t.Fatalf("resumed restore = %+v, err=%v", result, err)
	}
	if len(result.Unchanged) != 1 || result.Unchanged[0] != "first.txt" {
		t.Fatalf("already-restored paths = %#v", result.Unchanged)
	}
	if len(result.Restored) != 1 || result.Restored[0] != "second.txt" {
		t.Fatalf("resumed restored paths = %#v", result.Restored)
	}
	if got, err := os.ReadFile(filepath.Join(workspace, "first.txt")); err != nil || string(got) != "first before" {
		t.Fatalf("first file = %q, err=%v", got, err)
	}
	if got, err := os.ReadFile(filepath.Join(workspace, "second.txt")); err != nil || string(got) != "second before" {
		t.Fatalf("second file = %q, err=%v", got, err)
	}
}

func TestPublishedSubsetHandlesRepeatedWriteBeforeRename(t *testing.T) {
	workspace := t.TempDir()
	write(t, workspace, "state.txt", "before", 0o644)
	cp, err := checkpoint.Capture(workspace, []string{"state.txt"})
	if err != nil {
		t.Fatal(err)
	}
	mode := os.FileMode(0o644)
	if changed, err := cp.PrepareExpected("state.txt", []byte("first"), mode); err != nil || !changed {
		t.Fatalf("prepare first = changed:%v err:%v", changed, err)
	}
	write(t, workspace, "state.txt", "first", mode)
	if changed, err := cp.PrepareExpected("state.txt", []byte("second"), mode); err != nil || !changed {
		t.Fatalf("prepare second = changed:%v err:%v", changed, err)
	}
	record, err := cp.Export()
	if err != nil {
		t.Fatal(err)
	}
	if len(record.Entries) != 1 || record.Entries[0].Published == "" {
		t.Fatalf("pending record did not retain the prior published state: %#v", record)
	}
	pending, err := checkpoint.Import(workspace, record)
	if err != nil {
		t.Fatal(err)
	}
	subset, found, err := pending.PublishedSubset()
	if err != nil || !found || subset == nil {
		t.Fatalf("repeated-write published subset = %#v found:%v err:%v", subset, found, err)
	}
	if got := subset.Paths(); len(got) != 1 || got[0] != "state.txt" {
		t.Fatalf("repeated-write published paths = %#v", got)
	}
	if result, err := subset.Restore(); err != nil || !result.Complete {
		t.Fatalf("repeated-write restore = %+v, err=%v", result, err)
	}
	if got, err := os.ReadFile(filepath.Join(workspace, "state.txt")); err != nil || string(got) != "before" {
		t.Fatalf("repeated-write restored file = %q, err=%v", got, err)
	}
}

func TestPublishedSubsetRetainsPriorPublicationWhenNextWriteReturnsToBefore(t *testing.T) {
	workspace := t.TempDir()
	write(t, workspace, "state.txt", "before", 0o644)
	cp, err := checkpoint.Capture(workspace, []string{"state.txt"})
	if err != nil {
		t.Fatal(err)
	}
	mode := os.FileMode(0o644)
	if _, err := cp.PrepareExpected("state.txt", []byte("first"), mode); err != nil {
		t.Fatal(err)
	}
	write(t, workspace, "state.txt", "first", mode)
	if changed, err := cp.PrepareExpected("state.txt", []byte("before"), mode); err != nil || changed {
		t.Fatalf("prepare return-to-before = changed:%v err:%v", changed, err)
	}
	record, err := cp.Export()
	if err != nil {
		t.Fatal(err)
	}
	if len(record.Entries) != 1 || record.Entries[0].Published == "" {
		t.Fatalf("prior publication was dropped when expected returned to before: %#v", record)
	}
	pending, err := checkpoint.Import(workspace, record)
	if err != nil {
		t.Fatal(err)
	}
	subset, found, err := pending.PublishedSubset()
	if err != nil || !found || subset == nil {
		t.Fatalf("return-to-before published subset = %#v found:%v err:%v", subset, found, err)
	}
	if result, err := subset.Restore(); err != nil || !result.Complete {
		t.Fatalf("return-to-before restore = %+v, err=%v", result, err)
	}
	if got, err := os.ReadFile(filepath.Join(workspace, "state.txt")); err != nil || string(got) != "before" {
		t.Fatalf("return-to-before restored file = %q, err=%v", got, err)
	}
}

func TestPrepareExpectedRejectsUnexpectedSamePathState(t *testing.T) {
	workspace := t.TempDir()
	write(t, workspace, "state.txt", "before", 0o644)
	cp, err := checkpoint.Capture(workspace, []string{"state.txt"})
	if err != nil {
		t.Fatal(err)
	}
	mode := os.FileMode(0o644)
	if _, err := cp.PrepareExpected("state.txt", []byte("first"), mode); err != nil {
		t.Fatal(err)
	}
	write(t, workspace, "state.txt", "first", mode)
	write(t, workspace, "state.txt", "user edit", mode)
	if _, err := cp.PrepareExpected("state.txt", []byte("second"), mode); !errors.Is(err, checkpoint.ErrConflict) {
		t.Fatalf("unexpected same-path state error = %v", err)
	}
	if got, err := os.ReadFile(filepath.Join(workspace, "state.txt")); err != nil || string(got) != "user edit" {
		t.Fatalf("unexpected same-path state was changed = %q, err=%v", got, err)
	}
}

func TestImportRejectsInvalidRecord(t *testing.T) {
	workspace := t.TempDir()
	cases := []checkpoint.Record{
		{Version: checkpoint.RecordVersion + 1, Entries: []checkpoint.RecordEntry{{Path: "file.txt", Expected: "00"}}},
		{Version: checkpoint.RecordVersion, Entries: []checkpoint.RecordEntry{{Path: "../file.txt", Expected: "00"}}},
		{Version: checkpoint.RecordVersion, Entries: []checkpoint.RecordEntry{{Path: "file.txt", Expected: "00"}}},
	}
	for i, record := range cases {
		if _, err := checkpoint.Import(workspace, record); err == nil {
			t.Fatalf("case %d imported invalid record", i)
		}
	}
}
