//go:build windows

package workspace

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

func TestDurableWriteUsesProtectedCurrentUserACLs(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, ".picogent", "undo", "session.json")
	err := WriteAtomicDurableWithModeAndHooks(root, path, []byte("private journal\n"), 0o600, WriteHooks{
		CreateParentMode:       0o700,
		PrivateParentDirectory: true,
	})
	if err != nil {
		t.Fatalf("write durable journal: %v", err)
	}

	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil || user == nil || user.User.Sid == nil {
		t.Fatalf("get current process user SID: user=%v err=%v", user, err)
	}
	for _, check := range []struct {
		path      string
		directory bool
	}{
		{path: filepath.Join(root, ".picogent"), directory: true},
		{path: filepath.Join(root, ".picogent", "undo"), directory: true},
		{path: path},
	} {
		sd, err := windows.GetNamedSecurityInfo(check.path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
		if err != nil || sd == nil {
			t.Fatalf("read security descriptor for %s: %v", check.path, err)
		}
		control, _, err := sd.Control()
		if err != nil {
			t.Fatalf("read DACL control for %s: %v", check.path, err)
		}
		if control&windows.SE_DACL_PROTECTED == 0 {
			t.Errorf("%s DACL is inheritable; want protected", check.path)
		}
		dacl, _, err := sd.DACL()
		if err != nil || dacl == nil || dacl.AceCount != 1 {
			t.Errorf("%s DACL=%v err=%v; want exactly one current-user ACE", check.path, dacl, err)
			continue
		}
		var ace *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(dacl, 0, &ace); err != nil {
			t.Fatalf("read DACL entry for %s: %v", check.path, err)
		}
		if ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE {
			t.Errorf("%s DACL entry type=%d; want allowed ACE", check.path, ace.Header.AceType)
		}
		requiredAccess := windows.ACCESS_MASK(windows.FILE_GENERIC_READ | windows.FILE_GENERIC_WRITE)
		if ace.Mask&requiredAccess != requiredAccess {
			t.Errorf("%s DACL mask=%#x; lacks required journal access %#x", check.path, ace.Mask, requiredAccess)
		}
		wantFlags := uint8(0)
		if check.directory {
			wantFlags = windows.OBJECT_INHERIT_ACE | windows.CONTAINER_INHERIT_ACE
		}
		if ace.Header.AceFlags != wantFlags {
			t.Errorf("%s DACL inheritance flags=%#x; want %#x", check.path, ace.Header.AceFlags, wantFlags)
		}
		aceSID := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
		if !windows.EqualSid(aceSID, user.User.Sid) {
			t.Errorf("%s DACL grants access to a SID other than the current user", check.path)
		}
	}
}

func TestDurableWriteMigratesExistingWindowsJournalACLs(t *testing.T) {
	root, undoDir, targetPath, otherPath := newBroadInheritedWindowsJournalTree(t)
	if err := WriteAtomicDurableWithModeAndHooks(root, targetPath, []byte("updated journal\n"), 0o600, WriteHooks{
		CreateParentMode:       0o700,
		PrivateParentDirectory: true,
	}); err != nil {
		t.Fatalf("write into existing broad-ACL journal store: %v", err)
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
		{path: targetPath},
		{path: otherPath},
	} {
		assertPrivateWindowsDACL(t, check.path, check.directory, user.User.Sid)
	}
	assertBroadInheritedWindowsDACL(t, filepath.Join(root, ".picogent"))
	assertBroadInheritedWindowsDACL(t, filepath.Join(root, ".picogent", "rules.md"))
	if got, err := os.ReadFile(targetPath); err != nil || string(got) != "updated journal\n" {
		t.Fatalf("updated journal=%q err=%v", got, err)
	}
	if got, err := os.ReadFile(otherPath); err != nil || string(got) != "preserve this journal\n" {
		t.Fatalf("unrelated journal=%q err=%v", got, err)
	}
}

func TestSecurePrivateDirectoryMigratesExistingWindowsJournalACLs(t *testing.T) {
	root, undoDir, targetPath, otherPath := newBroadInheritedWindowsJournalTree(t)
	if err := SecurePrivateDirectoryAndFiles(root, undoDir); err != nil {
		t.Fatalf("secure existing journal directory: %v", err)
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
		{path: targetPath},
		{path: otherPath},
	} {
		assertPrivateWindowsDACL(t, check.path, check.directory, user.User.Sid)
	}
	assertBroadInheritedWindowsDACL(t, filepath.Join(root, ".picogent"))
	assertBroadInheritedWindowsDACL(t, filepath.Join(root, ".picogent", "rules.md"))
	if got, err := os.ReadFile(targetPath); err != nil || string(got) != "preserve this journal\n" {
		t.Fatalf("journal contents=%q err=%v", got, err)
	}
	if got, err := os.ReadFile(otherPath); err != nil || string(got) != "preserve this journal\n" {
		t.Fatalf("other journal contents=%q err=%v", got, err)
	}
}

func TestPrivateDirectoryCreationHardensConcurrentCollision(t *testing.T) {
	root := t.TempDir()
	setWindowsTestDACL(t, root, "D:P(A;OICI;FA;;;WD)")
	path := filepath.Join(root, "undo")
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatal(err)
	}
	assertBroadInheritedWindowsDACL(t, path)
	parent, err := openWindowsRoot(root)
	if err != nil {
		t.Fatalf("open root directory: %v", err)
	}
	defer windows.CloseHandle(parent)
	descriptor, err := privateWindowsSecurityDescriptor(true)
	if err != nil {
		t.Fatal(err)
	}
	access := uint32(windows.FILE_GENERIC_READ | windows.READ_CONTROL | windows.WRITE_DAC)
	if _, err := createWindowsDirectoryExclusiveWithSecurity(parent, "undo", access, descriptor); !errors.Is(err, os.ErrExist) {
		t.Fatalf("exclusive create collision = %v, want os.ErrExist", err)
	}
	secured, err := createPrivateWindowsDirectory(parent, "undo", access, descriptor)
	if err != nil {
		t.Fatalf("secure concurrently-created directory: %v", err)
	}
	defer windows.CloseHandle(secured)
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil || user == nil || user.User.Sid == nil {
		t.Fatalf("get current process user SID: user=%v err=%v", user, err)
	}
	assertPrivateWindowsDACL(t, path, true, user.User.Sid)
}

func TestDurableWriteDoesNotRequireWritableExistingWorkspaceRoot(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, ".picogent", "undo", "session.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil || user == nil || user.User.Sid == nil {
		t.Fatalf("get current process user SID: user=%v err=%v", user, err)
	}
	sid := user.User.Sid.String()
	if sid == "" {
		t.Fatal("format current process user SID")
	}
	setWindowsTestDACL(t, root, fmt.Sprintf("D:P(A;;GRGX;;;%s)", sid))
	t.Cleanup(func() {
		if err := setWindowsPathDACL(root, fmt.Sprintf("D:P(A;OICI;FA;;;%s)", sid)); err != nil {
			t.Errorf("restore workspace root DACL: %v", err)
		}
	})
	if handle, err := openWindowsRootWithAccess(root, windows.FILE_GENERIC_READ|windows.FILE_GENERIC_WRITE); err == nil {
		_ = windows.CloseHandle(handle)
		t.Fatal("test setup left the workspace root writable")
	}

	if err := WriteAtomicDurableWithModeAndHooks(root, path, []byte("durable\n"), 0o600, WriteHooks{
		CreateParentMode:       0o700,
		PrivateParentDirectory: true,
	}); err != nil {
		t.Fatalf("durable write below existing writable journal directory: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != "durable\n" {
		t.Fatalf("journal contents=%q err=%v", got, err)
	}
}

func setWindowsTestDACL(t *testing.T, path, sddl string) {
	t.Helper()
	if err := setWindowsPathDACL(path, sddl); err != nil {
		t.Fatalf("set test DACL for %s: %v", path, err)
	}
}

func newBroadInheritedWindowsJournalTree(t *testing.T) (root, undoDir, targetPath, otherPath string) {
	t.Helper()
	root = t.TempDir()
	setWindowsTestDACL(t, root, "D:P(A;OICI;FA;;;WD)")
	undoDir = filepath.Join(root, ".picogent", "undo")
	if err := os.MkdirAll(undoDir, 0o700); err != nil {
		t.Fatal(err)
	}
	targetPath = filepath.Join(undoDir, "current-session.json")
	otherPath = filepath.Join(undoDir, "older-session.pending.json")
	rulesPath := filepath.Join(root, ".picogent", "rules.md")
	if err := os.WriteFile(rulesPath, []byte("project rules\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	assertBroadInheritedWindowsDACL(t, rulesPath)
	for _, path := range []string{targetPath, otherPath} {
		if err := os.WriteFile(path, []byte("preserve this journal\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		assertBroadInheritedWindowsDACL(t, path)
	}
	assertBroadInheritedWindowsDACL(t, filepath.Join(root, ".picogent"))
	assertBroadInheritedWindowsDACL(t, undoDir)
	return root, undoDir, targetPath, otherPath
}

func assertBroadInheritedWindowsDACL(t *testing.T, path string) {
	t.Helper()
	descriptor, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil || descriptor == nil {
		t.Fatalf("read broad DACL for %s: %v", path, err)
	}
	control, _, err := descriptor.Control()
	if err != nil {
		t.Fatalf("read broad DACL control for %s: %v", path, err)
	}
	if control&windows.SE_DACL_PROTECTED != 0 {
		t.Fatalf("test setup for %s unexpectedly protected the DACL", path)
	}
	dacl, _, err := descriptor.DACL()
	if err != nil || dacl == nil {
		t.Fatalf("read broad DACL entries for %s: %v", path, err)
	}
	everyone, err := windows.StringToSid("S-1-1-0")
	if err != nil {
		t.Fatal(err)
	}
	for index := uint16(0); index < dacl.AceCount; index++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(dacl, uint32(index), &ace); err != nil || ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE {
			continue
		}
		sid := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
		if ace.Header.AceFlags&windows.INHERITED_ACE != 0 && windows.EqualSid(sid, everyone) {
			return
		}
	}
	t.Fatalf("test setup for %s lacks an inherited Everyone ACE", path)
}

func assertPrivateWindowsDACL(t *testing.T, path string, directory bool, user *windows.SID) {
	t.Helper()
	descriptor, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil || descriptor == nil {
		t.Fatalf("read private DACL for %s: %v", path, err)
	}
	control, _, err := descriptor.Control()
	if err != nil || control&windows.SE_DACL_PROTECTED == 0 {
		t.Fatalf("DACL for %s is not protected: control=%#x err=%v", path, control, err)
	}
	dacl, _, err := descriptor.DACL()
	if err != nil || dacl == nil || dacl.AceCount != 1 {
		t.Fatalf("DACL for %s=%v err=%v; want one current-user entry", path, dacl, err)
	}
	var ace *windows.ACCESS_ALLOWED_ACE
	if err := windows.GetAce(dacl, 0, &ace); err != nil {
		t.Fatalf("read private DACL entry for %s: %v", path, err)
	}
	wantFlags := uint8(0)
	if directory {
		wantFlags = windows.OBJECT_INHERIT_ACE | windows.CONTAINER_INHERIT_ACE
	}
	requiredAccess := windows.ACCESS_MASK(windows.FILE_GENERIC_READ | windows.FILE_GENERIC_WRITE)
	if ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE || ace.Header.AceFlags != wantFlags ||
		!windows.EqualSid((*windows.SID)(unsafe.Pointer(&ace.SidStart)), user) || ace.Mask&requiredAccess != requiredAccess {
		t.Errorf("DACL for %s is not private to the current user with required access", path)
	}
}

func setWindowsPathDACL(path, sddl string) error {
	sd, err := windows.SecurityDescriptorFromString(sddl)
	if err != nil {
		return fmt.Errorf("parse security descriptor: %w", err)
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		return fmt.Errorf("read DACL: %w", err)
	}
	securityInformation := windows.SECURITY_INFORMATION(windows.DACL_SECURITY_INFORMATION | windows.PROTECTED_DACL_SECURITY_INFORMATION)
	if err := windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, securityInformation, nil, nil, dacl, nil); err != nil {
		return fmt.Errorf("write DACL: %w", err)
	}
	return nil
}

func TestWriteGuardRechecksWindowsRenameRetry(t *testing.T) {
	for _, revoked := range []bool{true, false} {
		name := "same authority"
		if revoked {
			name = "revoked authority"
		}
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, "note.txt")
			if err := os.WriteFile(path, []byte("before"), 0o644); err != nil {
				t.Fatal(err)
			}
			refused := errors.New("authority revoked during rename retry")
			publicationChecks, renames := 0, 0
			prepared := false
			hooks := WriteHooks{
				Check: func() error {
					if prepared {
						publicationChecks++
						if revoked && renames > 0 {
							return refused
						}
					}
					return nil
				},
				PreparePublish: func(os.FileMode) error { prepared = true; return nil },
			}
			// Denying delete sharing does not deterministically block a rename
			// with FILE_RENAME_POSIX_SEMANTICS. Inject the first syscall error
			// into this operation, then use the real handle rename if allowed.
			err := writeAtomicWithRename(root, path, []byte("after"), 0, false, hooks, func(source, parent windows.Handle, leaf string) error {
				renames++
				if renames == 1 {
					return windows.ERROR_SHARING_VIOLATION
				}
				return renameWorkspaceHandle(source, parent, leaf)
			})
			want, wantRenames := "after", 2
			if revoked {
				want, wantRenames = "before", 1
				if !errors.Is(err, refused) {
					t.Fatalf("retry did not refuse revoked authority: %v", err)
				}
			} else if err != nil {
				t.Fatalf("retry refused unchanged authority: %v", err)
			}
			if publicationChecks != 2 || renames != wantRenames {
				t.Fatalf("retry checks=%d renames=%d, want 2/%d", publicationChecks, renames, wantRenames)
			}
			got, err := os.ReadFile(path)
			if err != nil || string(got) != want {
				t.Fatalf("retry target=%q err=%v, want %q", got, err, want)
			}
			entries, err := os.ReadDir(root)
			if err != nil {
				t.Fatal(err)
			}
			for _, entry := range entries {
				if strings.HasPrefix(entry.Name(), ".picogent-workspace-") {
					t.Fatal("Windows retry retained staged file")
				}
			}
		})
	}
}
