//go:build windows

package setup

import (
	"os"
	"path/filepath"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

// trustedManagedDirectory checks the directory's Windows security descriptor
// before a managed CLI path is accepted. The leaf argument is intentionally
// unused: Windows ACLs express the write boundary through the descriptor, not
// through a Unix-style permission bit split.
func trustedManagedDirectory(path string, _ bool) bool {
	return windowsACLPathProtected(path)
}

// executableAncestorsProtected verifies the canonical target and every
// directory from the trusted root to the filesystem root. Windows has no
// portable os.FileMode executable bit, so the ACL check also covers the target
// file itself.
func executableAncestorsProtected(root, target string) bool {
	root, ok := canonicalPath(root)
	if !ok {
		return false
	}
	target, ok = canonicalPath(target)
	if !ok {
		return false
	}
	rel, err := filepath.Rel(root, target)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return false
	}
	for current := root; ; current = filepath.Dir(current) {
		if !windowsACLPathProtected(current) {
			return false
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
	}
	current := root
	parts := strings.Split(rel, string(filepath.Separator))
	for i, part := range parts {
		current = filepath.Join(current, part)
		st, err := os.Stat(current)
		if err != nil {
			return false
		}
		if i < len(parts)-1 && !st.IsDir() {
			return false
		}
		if !windowsACLPathProtected(current) {
			return false
		}
	}
	return true
}

const windowsUntrustedWriteMask = uint32(
	windows.FILE_WRITE_DATA |
		windows.FILE_APPEND_DATA |
		windows.FILE_WRITE_EA |
		windows.FILE_WRITE_ATTRIBUTES |
		windows.DELETE |
		windows.WRITE_DAC |
		windows.WRITE_OWNER |
		0x00000040 | // FILE_DELETE_CHILD for directories.
		windows.GENERIC_WRITE |
		windows.GENERIC_ALL,
)

const trustedInstallerSIDText = "S-1-5-80-956008885-3418522649-1831038044-1853292631-2271478464"

// windowsACLPathProtected accepts ACLs whose write-capable grants are limited
// to a recognized trusted owner, the current user, LocalSystem, local
// administrators, or TrustedInstaller. This protects against another account
// or a broad user group modifying the path while preserving user-managed CLI
// directories. A same-user replacement race after this check remains outside
// this path-based proof.
func windowsACLPathProtected(path string) bool {
	sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	if err != nil || sd == nil {
		return false
	}
	owner, _, err := sd.Owner()
	if err != nil || owner == nil || !owner.IsValid() {
		return false
	}
	dacl, _, err := sd.DACL()
	if err != nil || dacl == nil {
		return false
	}
	currentUser, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil || currentUser == nil || currentUser.User.Sid == nil || !currentUser.User.Sid.IsValid() {
		return false
	}
	adminSID, err := windows.CreateWellKnownSid(windows.WinBuiltinAdministratorsSid)
	if err != nil {
		return false
	}
	systemSID, err := windows.CreateWellKnownSid(windows.WinLocalSystemSid)
	if err != nil {
		return false
	}
	trustedInstallerSID, err := windows.StringToSid(trustedInstallerSIDText)
	if err != nil {
		return false
	}

	for i := uint16(0); i < dacl.AceCount; i++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(dacl, uint32(i), &ace); err != nil || ace == nil {
			return false
		}
		header := (*windows.ACE_HEADER)(unsafe.Pointer(ace))
		switch header.AceType {
		case windows.ACCESS_DENIED_ACE_TYPE:
			continue
		case windows.ACCESS_ALLOWED_ACE_TYPE:
			if uint32(ace.Mask)&windowsUntrustedWriteMask == 0 {
				continue
			}
			sid := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
			if !sid.IsValid() || !windowsACLTrustedSID(sid, owner, currentUser.User.Sid, adminSID, systemSID, trustedInstallerSID) {
				return false
			}
		default:
			// Callback/object/compound ACE layouts need separate SID offsets
			// and evaluation semantics. Rejecting them avoids treating an
			// unrecognized write grant as safe.
			return false
		}
	}
	return true
}

func windowsACLTrustedSID(sid, owner, currentUser, adminSID, systemSID, trustedInstallerSID *windows.SID) bool {
	if sid.Equals(currentUser) || sid.Equals(adminSID) || sid.Equals(systemSID) || sid.Equals(trustedInstallerSID) {
		return true
	}
	if owner.Equals(currentUser) || owner.Equals(adminSID) || owner.Equals(systemSID) || owner.Equals(trustedInstallerSID) {
		return sid.Equals(owner) || sid.IsWellKnown(windows.WinCreatorOwnerSid)
	}
	return false
}
