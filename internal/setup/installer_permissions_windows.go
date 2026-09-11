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
	return windowsACLPathProtected(path, false)
}

// executableAncestorsProtected verifies the canonical target and every
// directory from the trusted root to the filesystem root. Windows has no
// portable os.FileMode executable bit, so the ACL check also covers the target
// file itself. Existing-directory ancestors may have standard create-only
// access, but the managed root and the target must not be writable by an
// untrusted principal.
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
		if !windowsACLPathProtected(current, true) {
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
		if !windowsACLPathProtected(current, i == len(parts)-1) {
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

const windowsCreateOnlyDirectoryMask = uint32(windows.FILE_WRITE_DATA | windows.FILE_APPEND_DATA)

const trustedInstallerSIDText = "S-1-5-80-956008885-3418522649-1831038044-1853292631-2271478464"

// windowsACLPathProtected accepts ACLs whose modification, deletion, or
// security-control grants are limited to a recognized trusted owner, the
// current user, LocalSystem, local administrators, or TrustedInstaller. When
// allowCreateOnlyDirectory is true, the create-file/create-directory rights
// that Windows grants on some existing system directories are ignored. A
// same-user replacement race after this check remains outside this path-based
// proof.
func windowsACLPathProtected(path string, allowCreateOnlyDirectory bool) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
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
	writeMask := windowsUntrustedWriteMask
	if info.IsDir() && allowCreateOnlyDirectory {
		writeMask &^= windowsCreateOnlyDirectoryMask
	}

	for i := uint16(0); i < dacl.AceCount; i++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(dacl, uint32(i), &ace); err != nil || ace == nil {
			return false
		}
		header := (*windows.ACE_HEADER)(unsafe.Pointer(ace))
		if header.AceSize < 8 {
			return false
		}
		if header.AceFlags&windows.INHERIT_ONLY_ACE != 0 {
			continue
		}
		switch header.AceType {
		case windows.ACCESS_DENIED_ACE_TYPE:
			continue
		case windows.ACCESS_ALLOWED_ACE_TYPE:
			if uint32(ace.Mask)&writeMask == 0 {
				continue
			}
			sid := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
			if !sid.IsValid() || !windowsACLTrustedSID(sid, owner, currentUser.User.Sid, adminSID, systemSID, trustedInstallerSID) {
				return false
			}
		default:
			// Callback/object/compound ACE layouts need separate SID offsets
			// and evaluation semantics. A read-only ACE cannot weaken this
			// write-protection check, but an unrecognized write-capable ACE
			// must fail closed.
			if windowsACEAccessMask(ace)&writeMask != 0 {
				return false
			}
		}
	}
	return true
}

func windowsACEAccessMask(ace *windows.ACCESS_ALLOWED_ACE) uint32 {
	return uint32(*(*windows.ACCESS_MASK)(unsafe.Add(unsafe.Pointer(ace), unsafe.Offsetof(ace.Mask))))
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
