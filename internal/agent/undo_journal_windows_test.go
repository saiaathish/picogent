//go:build windows

package agent

import (
	"os"
	"path/filepath"
	"testing"
	"unsafe"

	"github.com/saiaathish/picogent/internal/taskstate"
	"golang.org/x/sys/windows"
)

func TestLoadUndoJournalHardensExistingWindowsStorageBeforeRead(t *testing.T) {
	a, _, task := newDurableUndoFixture(t, taskstate.StatusWorking)
	root := a.ConfigSnapshot().Workspace
	instance := testUndoWorkspaceInstance(t, root)
	identity, err := undoWorkspaceIdentity(root)
	if err != nil {
		t.Fatal(err)
	}
	record, err := a.latestUndo.checkpoint.Export()
	if err != nil {
		t.Fatal(err)
	}
	journal := undoJournal{
		Version: undoJournalVersion, State: undoJournalSealed, Workspace: identity,
		WorkspaceInstance: instance, SessionID: task.SessionID,
		TurnSequence: task.LastTurn().Sequence, TaskID: task.ID,
		IntentRevision: task.LastTurn().IntentRevision, Checkpoint: record,
	}
	if err := saveUndoJournal(root, task.SessionID, journal, false); err != nil {
		t.Fatal(err)
	}
	sealedPath, _, err := undoJournalPaths(root, task.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	undoDir := filepath.Dir(sealedPath)
	projectStateDir := filepath.Dir(undoDir)
	rulesPath := filepath.Join(projectStateDir, "rules.md")
	before, err := os.ReadFile(sealedPath)
	if err != nil {
		t.Fatal(err)
	}
	setAgentWindowsJournalDACL(t, projectStateDir, "D:P(A;OICI;FA;;;WD)")
	if err := os.WriteFile(rulesPath, []byte("shared project rules\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	setAgentWindowsJournalDACL(t, rulesPath, "D:P(A;;FA;;;WD)")
	for _, path := range []string{undoDir, sealedPath} {
		setAgentWindowsJournalDACL(t, path, "D:P(A;;FA;;;WD)")
	}

	loaded, err := loadUndoJournal(root, task.SessionID, false)
	if err != nil || loaded == nil || loaded.SessionID != task.SessionID {
		t.Fatalf("load after ACL migration = (%#v, %v)", loaded, err)
	}
	after, err := os.ReadFile(sealedPath)
	if err != nil || string(after) != string(before) {
		t.Fatalf("journal changed during ACL migration: err=%v unchanged=%t", err, string(after) == string(before))
	}
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil || user == nil || user.User.Sid == nil {
		t.Fatalf("get current process user SID: user=%v err=%v", user, err)
	}
	for _, check := range []struct {
		path      string
		directory bool
	}{
		{path: undoDir, directory: true},
		{path: sealedPath},
	} {
		assertAgentWindowsPrivateJournalDACL(t, check.path, check.directory, user.User.Sid)
	}
	assertAgentWindowsEveryoneDACL(t, projectStateDir)
	assertAgentWindowsEveryoneDACL(t, rulesPath)
	if rules, err := os.ReadFile(rulesPath); err != nil || string(rules) != "shared project rules\n" {
		t.Fatalf("shared project rules=%q err=%v", rules, err)
	}
}

func setAgentWindowsJournalDACL(t *testing.T, path, sddl string) {
	t.Helper()
	descriptor, err := windows.SecurityDescriptorFromString(sddl)
	if err != nil {
		t.Fatal(err)
	}
	dacl, _, err := descriptor.DACL()
	if err != nil {
		t.Fatal(err)
	}
	securityInformation := windows.SECURITY_INFORMATION(windows.DACL_SECURITY_INFORMATION | windows.PROTECTED_DACL_SECURITY_INFORMATION)
	if err := windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, securityInformation, nil, nil, dacl, nil); err != nil {
		t.Fatalf("set broad test DACL for %s: %v", path, err)
	}
}

func assertAgentWindowsPrivateJournalDACL(t *testing.T, path string, directory bool, user *windows.SID) {
	t.Helper()
	descriptor, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil || descriptor == nil {
		t.Fatalf("read DACL for %s: %v", path, err)
	}
	control, _, err := descriptor.Control()
	if err != nil || control&windows.SE_DACL_PROTECTED == 0 {
		t.Fatalf("DACL for %s is not protected: control=%#x err=%v", path, control, err)
	}
	dacl, _, err := descriptor.DACL()
	if err != nil || dacl == nil || dacl.AceCount != 1 {
		t.Fatalf("DACL for %s=%v err=%v; want one current-user ACE", path, dacl, err)
	}
	var ace *windows.ACCESS_ALLOWED_ACE
	if err := windows.GetAce(dacl, 0, &ace); err != nil {
		t.Fatal(err)
	}
	wantFlags := uint8(0)
	if directory {
		wantFlags = windows.OBJECT_INHERIT_ACE | windows.CONTAINER_INHERIT_ACE
	}
	requiredAccess := windows.ACCESS_MASK(windows.FILE_GENERIC_READ | windows.FILE_GENERIC_WRITE)
	if ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE || ace.Header.AceFlags != wantFlags ||
		!windows.EqualSid((*windows.SID)(unsafe.Pointer(&ace.SidStart)), user) || ace.Mask&requiredAccess != requiredAccess {
		t.Fatalf("DACL for %s does not grant only the current user required journal access", path)
	}
}

func assertAgentWindowsEveryoneDACL(t *testing.T, path string) {
	t.Helper()
	descriptor, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil || descriptor == nil {
		t.Fatalf("read shared DACL for %s: %v", path, err)
	}
	dacl, _, err := descriptor.DACL()
	if err != nil || dacl == nil {
		t.Fatalf("read shared DACL entries for %s: %v", path, err)
	}
	everyone, err := windows.StringToSid("S-1-1-0")
	if err != nil {
		t.Fatal(err)
	}
	for index := uint32(0); index < uint32(dacl.AceCount); index++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(dacl, index, &ace); err != nil || ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE {
			continue
		}
		if windows.EqualSid((*windows.SID)(unsafe.Pointer(&ace.SidStart)), everyone) {
			return
		}
	}
	t.Fatalf("shared DACL for %s lost its Everyone access entry", path)
}
