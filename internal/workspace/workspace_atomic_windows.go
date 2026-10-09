//go:build windows

package workspace

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync/atomic"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var workspaceTempSequence atomic.Uint64

func writeAtomic(root, path string, data []byte) error {
	return writeAtomicWithHook(root, path, data, 0, false, WriteHooks{})
}

func writeAtomicWithMode(root, path string, data []byte, requestedMode os.FileMode, setMode bool) error {
	return writeAtomicWithHook(root, path, data, requestedMode, setMode, WriteHooks{})
}

func writeAtomicWithHook(root, path string, data []byte, requestedMode os.FileMode, setMode bool, hooks WriteHooks) error {
	return writeAtomicWithDurability(root, path, data, requestedMode, setMode, hooks, false)
}

func writeAtomicDurableWithHook(root, path string, data []byte, requestedMode os.FileMode, setMode bool, hooks WriteHooks) error {
	return writeAtomicWithDurability(root, path, data, requestedMode, setMode, hooks, true)
}

func writeAtomicWithDurability(root, path string, data []byte, requestedMode os.FileMode, setMode bool, hooks WriteHooks, durable bool) error {
	return writeAtomicWithRenameDurability(root, path, data, requestedMode, setMode, hooks, renameWorkspaceHandle, durable)
}

// The per-operation rename parameter lets platform tests force retryable
// failures without depending on filesystem sharing/antivirus timing or a
// mutable process-wide syscall override.
func writeAtomicWithRename(root, path string, data []byte, requestedMode os.FileMode, setMode bool, hooks WriteHooks, rename func(windows.Handle, windows.Handle, string) error) error {
	return writeAtomicWithRenameDurability(root, path, data, requestedMode, setMode, hooks, rename, false)
}

func writeAtomicWithRenameDurability(root, path string, data []byte, requestedMode os.FileMode, setMode bool, hooks WriteHooks, rename func(windows.Handle, windows.Handle, string) error, durable bool) error {
	rel, err := Relative(root, path)
	if err != nil {
		return err
	}
	parts, err := pathParts(rel)
	if err != nil {
		return err
	}
	if err := hooks.check(); err != nil {
		return err
	}
	privateJournal := durable && setMode && requestedMode.Perm() == 0o600 && hooks.CreateParentMode.Perm() == 0o700 && hooks.PrivateParentDirectory
	if hooks.PrivateParentDirectory && !privateJournal {
		return errors.New("private parent directory requires a durable 0600 write and 0700 creation mode")
	}
	if privateJournal && len(parts) < 2 {
		return errors.New("private journal storage must be below the workspace root")
	}
	var directorySecurity, fileSecurity *windows.SECURITY_DESCRIPTOR
	if privateJournal {
		directorySecurity, err = privateWindowsSecurityDescriptor(true)
		if err != nil {
			return fmt.Errorf("prepare private workspace directory security: %w", err)
		}
		fileSecurity, err = privateWindowsSecurityDescriptor(false)
		if err != nil {
			return fmt.Errorf("prepare private workspace file security: %w", err)
		}
	}
	parent, err := openWindowsRoot(root)
	if err != nil {
		return fmt.Errorf("open workspace directory: %w", err)
	}
	if hooks.CheckRootIdentity != nil {
		identity, identityErr := workspaceRootIdentityForHandle(parent)
		if identityErr != nil {
			_ = windows.CloseHandle(parent)
			return fmt.Errorf("identify opened workspace root: %w", identityErr)
		}
		if checkErr := hooks.CheckRootIdentity(identity); checkErr != nil {
			_ = windows.CloseHandle(parent)
			return checkErr
		}
	}
	current := parent
	defer func() { _ = windows.CloseHandle(current) }()
	for index, part := range parts[:len(parts)-1] {
		if err := hooks.check(); err != nil {
			return err
		}
		var child windows.Handle
		var openErr error
		if durable {
			// Existing ancestors are only opened for traversal. Requesting write
			// access to their parents is necessary only when this operation must
			// create a directory entry there.
			privateParent := privateJournal && index == len(parts)-2
			directoryAccess := uint32(windows.FILE_GENERIC_READ)
			if privateParent {
				directoryAccess |= windows.READ_CONTROL | windows.WRITE_DAC
			}
			child, openErr = openWindowsDirectoryWithAccess(current, part, false, directoryAccess)
			if openErr == nil && privateParent {
				if securityErr := setPrivateWindowsDACL(child, directorySecurity, true); securityErr != nil {
					_ = windows.CloseHandle(child)
					child = 0
					openErr = fmt.Errorf("secure existing private journal directory %q: %w", part, securityErr)
				}
			}
			if errors.Is(openErr, os.ErrNotExist) {
				durableCreationParent, parentErr := openWorkspaceDurableParent(root, parts[:index])
				if parentErr != nil {
					return fmt.Errorf("open durable parent for workspace directory %q: %w", part, parentErr)
				}
				openedIdentity, identityErr := workspaceRootIdentityForHandle(current)
				if identityErr != nil {
					_ = windows.CloseHandle(durableCreationParent)
					return fmt.Errorf("identify workspace directory parent: %w", identityErr)
				}
				durableIdentity, identityErr := workspaceRootIdentityForHandle(durableCreationParent)
				if identityErr != nil {
					_ = windows.CloseHandle(durableCreationParent)
					return fmt.Errorf("identify durable workspace directory parent: %w", identityErr)
				}
				if !openedIdentity.Known || !durableIdentity.Known || openedIdentity != durableIdentity {
					_ = windows.CloseHandle(durableCreationParent)
					return errors.New("workspace directory parent identity changed during durable open")
				}
				if err := windows.FlushFileBuffers(durableCreationParent); err != nil {
					_ = windows.CloseHandle(durableCreationParent)
					return fmt.Errorf("flush workspace directory parent before creation: %w", err)
				}
				if privateJournal {
					createAccess := uint32(windows.FILE_GENERIC_READ | windows.READ_CONTROL | windows.WRITE_DAC)
					child, openErr = createPrivateWindowsDirectory(current, part, createAccess, directorySecurity)
				} else {
					child, openErr = openWindowsDirectoryWithAccessAndSecurity(current, part, true, windows.FILE_GENERIC_READ, nil)
				}
				if openErr == nil {
					if flushErr := windows.FlushFileBuffers(durableCreationParent); flushErr != nil {
						_ = windows.CloseHandle(child)
						child = 0
						openErr = fmt.Errorf("flush workspace directory parent after creation: %w", flushErr)
					}
				}
				_ = windows.CloseHandle(durableCreationParent)
			}
		} else {
			child, openErr = openWindowsDirectory(current, part, true)
		}
		if openErr != nil {
			return fmt.Errorf("open workspace directory %q: %w", part, openErr)
		}
		if child == 0 || child == windows.InvalidHandle {
			return fmt.Errorf("open workspace directory %q returned an invalid handle", part)
		}
		_ = windows.CloseHandle(current)
		current = child
	}
	var durableParent windows.Handle
	if durable {
		durableParent, err = openWorkspaceDurableParent(root, parts[:len(parts)-1])
		if err != nil {
			return fmt.Errorf("open durable workspace directory: %w", err)
		}
		defer windows.CloseHandle(durableParent)
		openedIdentity, identityErr := workspaceRootIdentityForHandle(current)
		if identityErr != nil {
			return fmt.Errorf("identify opened workspace directory: %w", identityErr)
		}
		durableIdentity, identityErr := workspaceRootIdentityForHandle(durableParent)
		if identityErr != nil {
			return fmt.Errorf("identify durable workspace directory: %w", identityErr)
		}
		if !openedIdentity.Known || !durableIdentity.Known || openedIdentity != durableIdentity {
			return errors.New("workspace directory identity changed during durable open")
		}
		if err := windows.FlushFileBuffers(durableParent); err != nil {
			return fmt.Errorf("flush workspace directory before durable write: %w", err)
		}
	}

	leaf := parts[len(parts)-1]
	if privateJournal {
		if err := securePrivateWindowsDirectoryEntries(current, fileSecurity); err != nil {
			return fmt.Errorf("secure existing workspace journal files: %w", err)
		}
	}
	if _, err := workspaceRegularEntry(current, leaf); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("inspect workspace file %q: %w", rel, err)
		}
	}

	tmpName := ""
	var file *os.File
	for attempt := uint64(0); attempt < 32; attempt++ {
		if err := hooks.check(); err != nil {
			return err
		}
		seq := workspaceTempSequence.Add(1)
		tmpName = fmt.Sprintf(".picogent-workspace-%d-%d-%d.tmp", os.Getpid(), time.Now().UnixNano(), seq)
		file, err = openWorkspaceExclusive(current, tmpName, path, fileSecurity)
		if err == nil {
			break
		}
		if !errors.Is(err, os.ErrExist) {
			return fmt.Errorf("create workspace temporary file %q: %w", rel, err)
		}
	}
	if file == nil {
		return errors.New("could not allocate a workspace temporary file")
	}
	if privateJournal {
		if err := verifyPrivateWindowsDACL(windows.Handle(file.Fd()), fileSecurity, false); err != nil {
			_ = file.Close()
			return fmt.Errorf("verify private workspace temporary file %q: %w", rel, err)
		}
	}
	removeTemp := true
	defer func() {
		if removeTemp {
			removeWorkspaceTemp(current, tmpName, file)
			_ = file.Close()
		}
	}()

	if setMode {
		if err := file.Chmod(requestedMode); err != nil {
			return fmt.Errorf("set workspace file mode %q: %w", rel, err)
		}
	}
	if err := writeWorkspaceAll(file, data); err != nil {
		return fmt.Errorf("write workspace file %q: %w", rel, err)
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("sync workspace file %q: %w", rel, err)
	}
	if matches, err := workspaceTempMatches(current, tmpName, file); err != nil {
		return fmt.Errorf("validate workspace temporary file %q: %w", rel, err)
	} else if !matches {
		return fmt.Errorf("workspace temporary file %q changed before commit", rel)
	}
	if _, err := workspaceRegularEntry(current, leaf); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("validate workspace target %q: %w", rel, err)
	}
	if hooks.PreparePublish != nil {
		info, err := file.Stat()
		if err != nil {
			return fmt.Errorf("inspect workspace publication %q: %w", rel, err)
		}
		if err := hooks.PreparePublish(info.Mode()); err != nil {
			return fmt.Errorf("prepare workspace publication %q: %w", rel, err)
		}
	}
	for attempt := 0; ; attempt++ {
		if err := hooks.check(); err != nil {
			return err
		}
		if err := rename(windows.Handle(file.Fd()), current, leaf); err == nil {
			break
		} else if !retryWorkspaceRename(err) || attempt >= 99 {
			return fmt.Errorf("publish workspace file %q: %w", rel, err)
		}
		time.Sleep(time.Millisecond)
	}
	removeTemp = false
	if err := file.Close(); err != nil {
		return fmt.Errorf("close workspace file %q: %w", rel, err)
	}
	if durable {
		if err := windows.FlushFileBuffers(durableParent); err != nil {
			return fmt.Errorf("flush workspace directory after durable write: %w", err)
		}
	}
	return nil
}

func openWorkspaceExclusive(parent windows.Handle, name, display string, security *windows.SECURITY_DESCRIPTOR) (*os.File, error) {
	objectName, err := windows.NewNTUnicodeString(name)
	if err != nil {
		return nil, err
	}
	oa := objectAttributes(objectName, parent)
	oa.SecurityDescriptor = security
	var iosb windows.IO_STATUS_BLOCK
	var allocation int64
	var handle windows.Handle
	err = windows.NtCreateFile(
		&handle,
		windows.FILE_GENERIC_READ|windows.FILE_GENERIC_WRITE|windows.DELETE,
		&oa,
		&iosb,
		&allocation,
		0,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		windows.FILE_CREATE,
		windows.FILE_NON_DIRECTORY_FILE|windows.FILE_OPEN_REPARSE_POINT,
		0,
		0,
	)
	if err != nil {
		var status windows.NTStatus
		if errors.As(err, &status) && status == windows.STATUS_OBJECT_NAME_COLLISION {
			return nil, fmt.Errorf("%w: %v", os.ErrExist, err)
		}
		return nil, translateNTError(err)
	}
	file := os.NewFile(uintptr(handle), display)
	if file == nil {
		_ = windows.CloseHandle(handle)
		return nil, fmt.Errorf("open workspace file %q: could not wrap handle", display)
	}
	return file, nil
}

func privateWindowsSecurityDescriptor(directory bool) (*windows.SECURITY_DESCRIPTOR, error) {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return nil, fmt.Errorf("get current process user: %w", err)
	}
	if user == nil || user.User.Sid == nil {
		return nil, errors.New("current process has no user SID")
	}
	sid := user.User.Sid.String()
	if sid == "" {
		return nil, errors.New("could not format current process user SID")
	}
	inheritance := ""
	if directory {
		inheritance = "OICI"
	}
	descriptor, err := windows.SecurityDescriptorFromString(fmt.Sprintf("D:P(A;%s;FA;;;%s)", inheritance, sid))
	if err != nil {
		return nil, fmt.Errorf("build protected user-only DACL: %w", err)
	}
	control, _, err := descriptor.Control()
	if err != nil {
		return nil, fmt.Errorf("inspect private DACL protection: %w", err)
	}
	if control&windows.SE_DACL_PROTECTED == 0 {
		return nil, errors.New("private DACL is not protected from inheritance")
	}
	dacl, _, err := descriptor.DACL()
	if err != nil || dacl == nil || dacl.AceCount != 1 {
		if err != nil {
			return nil, fmt.Errorf("inspect private DACL entries: %w", err)
		}
		return nil, errors.New("private DACL must contain exactly one user entry")
	}
	var ace *windows.ACCESS_ALLOWED_ACE
	if err := windows.GetAce(dacl, 0, &ace); err != nil {
		return nil, fmt.Errorf("inspect private DACL user entry: %w", err)
	}
	wantFlags := uint8(0)
	if directory {
		wantFlags = windows.OBJECT_INHERIT_ACE | windows.CONTAINER_INHERIT_ACE
	}
	if ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE || ace.Header.AceFlags != wantFlags ||
		!windows.EqualSid((*windows.SID)(unsafe.Pointer(&ace.SidStart)), user.User.Sid) {
		return nil, errors.New("private DACL does not grant only the current user")
	}
	requiredAccess := windows.ACCESS_MASK(windows.FILE_GENERIC_READ | windows.FILE_GENERIC_WRITE)
	if ace.Mask&requiredAccess != requiredAccess {
		return nil, errors.New("private DACL does not grant the current user required journal access")
	}
	return descriptor, nil
}

func createPrivateWindowsDirectory(parent windows.Handle, name string, access uint32, descriptor *windows.SECURITY_DESCRIPTOR) (windows.Handle, error) {
	for attempt := 0; attempt < 8; attempt++ {
		handle, err := createWindowsDirectoryExclusiveWithSecurity(parent, name, access, descriptor)
		if err == nil {
			if err := verifyPrivateWindowsDACL(handle, descriptor, true); err != nil {
				_ = windows.CloseHandle(handle)
				return 0, fmt.Errorf("verify newly created directory ACL: %w", err)
			}
			return handle, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return 0, err
		}

		// A creator may have won after the caller observed the directory as
		// missing. Open that exact entry, replace its inherited ACL, and verify
		// the resulting descriptor before it can be used for journal storage.
		handle, err = openWindowsDirectoryWithAccess(parent, name, false, access)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return 0, fmt.Errorf("open directory created concurrently: %w", err)
		}
		if err := setPrivateWindowsDACL(handle, descriptor, true); err != nil {
			_ = windows.CloseHandle(handle)
			return 0, fmt.Errorf("secure directory created concurrently: %w", err)
		}
		return handle, nil
	}
	return 0, errors.New("workspace directory changed repeatedly during private creation")
}

func securePrivateDirectoryAndFiles(root string, parts []string) error {
	directorySecurity, err := privateWindowsSecurityDescriptor(true)
	if err != nil {
		return fmt.Errorf("prepare private workspace directory security: %w", err)
	}
	fileSecurity, err := privateWindowsSecurityDescriptor(false)
	if err != nil {
		return fmt.Errorf("prepare private workspace file security: %w", err)
	}
	current, err := openWindowsRoot(root)
	if err != nil {
		return fmt.Errorf("open workspace directory: %w", err)
	}
	defer func() { _ = windows.CloseHandle(current) }()
	for index, part := range parts {
		access := uint32(windows.FILE_GENERIC_READ)
		if index == len(parts)-1 {
			access |= windows.READ_CONTROL | windows.WRITE_DAC
		}
		child, err := openWindowsDirectoryWithAccess(current, part, false, access)
		if err != nil {
			return fmt.Errorf("open private workspace directory %q: %w", part, err)
		}
		if index == len(parts)-1 {
			if err := setPrivateWindowsDACL(child, directorySecurity, true); err != nil {
				_ = windows.CloseHandle(child)
				return fmt.Errorf("secure private workspace directory %q: %w", part, err)
			}
			_ = windows.CloseHandle(current)
			current = child
			return securePrivateWindowsDirectoryEntries(current, fileSecurity)
		}
		_ = windows.CloseHandle(current)
		current = child
	}
	return errors.New("private directory path is empty")
}

func setPrivateWindowsDACL(handle windows.Handle, expected *windows.SECURITY_DESCRIPTOR, directory bool) error {
	if err := verifyPrivateWindowsObjectOwner(handle); err != nil {
		return fmt.Errorf("refuse to migrate foreign-owned private storage: %w", err)
	}
	dacl, _, err := expected.DACL()
	if err != nil || dacl == nil {
		if err != nil {
			return fmt.Errorf("read expected private DACL: %w", err)
		}
		return errors.New("expected private DACL is missing")
	}
	securityInformation := windows.SECURITY_INFORMATION(windows.DACL_SECURITY_INFORMATION | windows.PROTECTED_DACL_SECURITY_INFORMATION)
	if err := windows.SetSecurityInfo(handle, windows.SE_FILE_OBJECT, securityInformation, nil, nil, dacl, nil); err != nil {
		return fmt.Errorf("apply protected current-user DACL: %w", err)
	}
	return verifyPrivateWindowsDACL(handle, expected, directory)
}

func verifyPrivateWindowsDACL(handle windows.Handle, expected *windows.SECURITY_DESCRIPTOR, directory bool) error {
	actual, err := windows.GetSecurityInfo(handle, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.OWNER_SECURITY_INFORMATION)
	if err != nil {
		return fmt.Errorf("read applied security descriptor: %w", err)
	}
	owner, _, err := actual.Owner()
	if err != nil {
		return fmt.Errorf("inspect applied object owner: %w", err)
	}
	if err := verifyPrivateWindowsTokenOwner(owner); err != nil {
		return fmt.Errorf("verify applied object owner: %w", err)
	}
	actualControl, _, err := actual.Control()
	if err != nil {
		return fmt.Errorf("inspect applied DACL protection: %w", err)
	}
	if actualControl&windows.SE_DACL_PROTECTED == 0 {
		return errors.New("applied DACL remains inheritable")
	}
	actualDACL, _, err := actual.DACL()
	if err != nil || actualDACL == nil || actualDACL.AceCount != 1 {
		if err != nil {
			return fmt.Errorf("inspect applied DACL entries: %w", err)
		}
		return errors.New("applied DACL must contain exactly one current-user entry")
	}
	var actualACE *windows.ACCESS_ALLOWED_ACE
	if err := windows.GetAce(actualDACL, 0, &actualACE); err != nil {
		return fmt.Errorf("inspect applied DACL entry: %w", err)
	}
	expectedDACL, _, err := expected.DACL()
	if err != nil || expectedDACL == nil || expectedDACL.AceCount != 1 {
		if err != nil {
			return fmt.Errorf("inspect expected DACL entries: %w", err)
		}
		return errors.New("expected private DACL must contain exactly one user entry")
	}
	var expectedACE *windows.ACCESS_ALLOWED_ACE
	if err := windows.GetAce(expectedDACL, 0, &expectedACE); err != nil {
		return fmt.Errorf("inspect expected DACL entry: %w", err)
	}
	wantFlags := uint8(0)
	if directory {
		wantFlags = windows.OBJECT_INHERIT_ACE | windows.CONTAINER_INHERIT_ACE
	}
	if actualACE.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE || actualACE.Header.AceFlags != wantFlags ||
		!windows.EqualSid((*windows.SID)(unsafe.Pointer(&actualACE.SidStart)), (*windows.SID)(unsafe.Pointer(&expectedACE.SidStart))) {
		return errors.New("applied DACL does not grant only the current user")
	}
	requiredAccess := windows.ACCESS_MASK(windows.FILE_GENERIC_READ | windows.FILE_GENERIC_WRITE)
	if actualACE.Mask&requiredAccess != requiredAccess {
		return errors.New("applied DACL does not grant the current user required journal access")
	}
	return nil
}

func verifyPrivateWindowsObjectOwner(handle windows.Handle) error {
	descriptor, err := windows.GetSecurityInfo(handle, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION)
	if err != nil {
		return fmt.Errorf("read object owner: %w", err)
	}
	owner, _, err := descriptor.Owner()
	if err != nil {
		return fmt.Errorf("inspect object owner: %w", err)
	}
	return verifyPrivateWindowsTokenOwner(owner)
}

type windowsTokenOwnerInfo struct {
	Owner *windows.SID
}

func verifyPrivateWindowsTokenOwner(owner *windows.SID) error {
	if owner == nil {
		return errors.New("object owner is missing")
	}
	token := windows.GetCurrentProcessToken()
	var size uint32
	if err := windows.GetTokenInformation(token, windows.TokenOwner, nil, 0, &size); !errors.Is(err, windows.ERROR_INSUFFICIENT_BUFFER) {
		if err != nil {
			return fmt.Errorf("query process token owner size: %w", err)
		}
		return errors.New("query process token owner returned no buffer size")
	}
	if size == 0 {
		return errors.New("process token owner is empty")
	}
	buffer := make([]byte, size)
	if err := windows.GetTokenInformation(token, windows.TokenOwner, &buffer[0], uint32(len(buffer)), &size); err != nil {
		return fmt.Errorf("read process token owner: %w", err)
	}
	ownerInfo := (*windowsTokenOwnerInfo)(unsafe.Pointer(&buffer[0]))
	if ownerInfo.Owner == nil {
		return errors.New("process token owner SID is missing")
	}
	if !windows.EqualSid(owner, ownerInfo.Owner) {
		return errors.New("object is owned by a different Windows token owner")
	}
	return nil
}

func securePrivateWindowsDirectoryEntries(directory windows.Handle, fileSecurity *windows.SECURITY_DESCRIPTOR) error {
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(directory, &info); err != nil {
		return fmt.Errorf("inspect journal directory: %w", err)
	}
	if info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 || info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY == 0 {
		return errors.New("journal directory is not a regular directory")
	}
	var duplicate windows.Handle
	if err := windows.DuplicateHandle(windows.CurrentProcess(), directory, windows.CurrentProcess(), &duplicate, 0, false, windows.DUPLICATE_SAME_ACCESS); err != nil {
		return fmt.Errorf("duplicate journal directory handle: %w", err)
	}
	entries := os.NewFile(uintptr(duplicate), "undo journal directory")
	if entries == nil {
		_ = windows.CloseHandle(duplicate)
		return errors.New("wrap journal directory handle")
	}
	defer entries.Close()
	for {
		batch, readErr := entries.ReadDir(64)
		for _, entry := range batch {
			name := entry.Name()
			if name == "" || name == "." || name == ".." || strings.ContainsAny(name, `/\\:`) {
				return fmt.Errorf("unsafe journal directory entry %q", name)
			}
			if err := hardenPrivateWindowsFile(directory, name, fileSecurity); err != nil {
				return fmt.Errorf("secure journal entry %q: %w", name, err)
			}
		}
		if errors.Is(readErr, io.EOF) {
			return nil
		}
		if readErr != nil {
			return fmt.Errorf("read journal directory: %w", readErr)
		}
	}
}

func hardenPrivateWindowsFile(parent windows.Handle, name string, fileSecurity *windows.SECURITY_DESCRIPTOR) error {
	file, err := openWindowsFileForSecurity(parent, name)
	if err != nil {
		return err
	}
	defer file.Close()
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(windows.Handle(file.Fd()), &info); err != nil {
		return fmt.Errorf("inspect file: %w", err)
	}
	if info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 || info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY != 0 {
		return errors.New("journal entry is not a regular file")
	}
	if err := rejectHardLinkFile(file); err != nil {
		return fmt.Errorf("journal entry has an unsafe link count: %w", err)
	}
	identity, err := identityForFile(file)
	if err != nil {
		return fmt.Errorf("identify journal entry: %w", err)
	}
	if err := setPrivateWindowsDACL(windows.Handle(file.Fd()), fileSecurity, false); err != nil {
		return err
	}
	byName, err := openWindowsFileForSecurity(parent, name)
	if err != nil {
		return fmt.Errorf("reopen secured journal entry: %w", err)
	}
	defer byName.Close()
	actual, err := identityForFile(byName)
	if err != nil {
		return fmt.Errorf("reidentify secured journal entry: %w", err)
	}
	if actual != identity {
		return errors.New("journal entry changed while securing its DACL")
	}
	return nil
}

func openWindowsFileForSecurity(parent windows.Handle, name string) (*os.File, error) {
	objectName, err := windows.NewNTUnicodeString(name)
	if err != nil {
		return nil, err
	}
	oa := objectAttributes(objectName, parent)
	var iosb windows.IO_STATUS_BLOCK
	var allocation int64
	var handle windows.Handle
	err = windows.NtCreateFile(
		&handle,
		windows.FILE_READ_ATTRIBUTES|windows.READ_CONTROL|windows.WRITE_DAC,
		&oa,
		&iosb,
		&allocation,
		0,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		windows.FILE_OPEN,
		windows.FILE_NON_DIRECTORY_FILE|windows.FILE_OPEN_REPARSE_POINT,
		0,
		0,
	)
	if err != nil {
		return nil, translateNTError(err)
	}
	file := os.NewFile(uintptr(handle), name)
	if file == nil {
		_ = windows.CloseHandle(handle)
		return nil, errors.New("wrap journal file handle")
	}
	return file, nil
}

func workspaceRegularEntry(parent windows.Handle, name string) (Identity, error) {
	h, err := openWindowsFile(parent, name, openEdit)
	if err != nil {
		return Identity{}, err
	}
	f := os.NewFile(uintptr(h), name)
	if f == nil {
		_ = windows.CloseHandle(h)
		return Identity{}, errors.New("could not wrap workspace file handle")
	}
	defer f.Close()
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(h, &info); err != nil {
		return Identity{}, err
	}
	if info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		return Identity{}, fmt.Errorf("workspace path %q is a reparse point", name)
	}
	if info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY != 0 {
		return Identity{}, fmt.Errorf("workspace path %q is not a regular file", name)
	}
	stat, err := f.Stat()
	if err != nil {
		return Identity{}, err
	}
	if !stat.Mode().IsRegular() {
		return Identity{}, fmt.Errorf("workspace path %q is not a regular file", name)
	}
	if err := rejectHardLinkFile(f); err != nil {
		return Identity{}, fmt.Errorf("workspace path %q has an unsafe link count: %w", name, err)
	}
	if info.FileAttributes&windows.FILE_ATTRIBUTE_READONLY != 0 {
		return Identity{}, fmt.Errorf("workspace path %q is read-only", name)
	}
	return Identity{Volume: uint64(info.VolumeSerialNumber), File: uint64(info.FileIndexHigh)<<32 | uint64(info.FileIndexLow), Known: true}, nil
}

func workspaceTempMatches(parent windows.Handle, name string, source *os.File) (bool, error) {
	if source == nil {
		return false, errors.New("workspace temporary source is nil")
	}
	expected, err := identityForFile(source)
	if err != nil {
		return false, err
	}
	namedHandle, err := openWindowsFile(parent, name, openRead)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	named := os.NewFile(uintptr(namedHandle), name)
	if named == nil {
		_ = windows.CloseHandle(namedHandle)
		return false, errors.New("could not wrap workspace temporary handle")
	}
	defer named.Close()
	actual, err := identityForFile(named)
	if err != nil {
		return false, err
	}
	return expected == actual, nil
}

func removeWorkspaceTemp(parent windows.Handle, name string, source *os.File) {
	if source == nil {
		return
	}
	expected, err := identityForFile(source)
	if err != nil {
		return
	}
	file, err := openWorkspaceDelete(parent, name)
	if err != nil {
		return
	}
	defer file.Close()
	actual, err := identityForFile(file)
	if err != nil || expected != actual {
		return
	}
	_ = deleteWorkspaceHandle(windows.Handle(file.Fd()))
}

func openWorkspaceDelete(parent windows.Handle, name string) (*os.File, error) {
	objectName, err := windows.NewNTUnicodeString(name)
	if err != nil {
		return nil, err
	}
	oa := objectAttributes(objectName, parent)
	var iosb windows.IO_STATUS_BLOCK
	var allocation int64
	var handle windows.Handle
	err = windows.NtCreateFile(
		&handle,
		windows.DELETE|windows.FILE_GENERIC_READ,
		&oa,
		&iosb,
		&allocation,
		0,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		windows.FILE_OPEN,
		windows.FILE_NON_DIRECTORY_FILE|windows.FILE_OPEN_REPARSE_POINT,
		0,
		0,
	)
	if err != nil {
		return nil, translateNTError(err)
	}
	file := os.NewFile(uintptr(handle), name)
	if file == nil {
		_ = windows.CloseHandle(handle)
		return nil, errors.New("could not wrap workspace delete handle")
	}
	return file, nil
}

func deleteWorkspaceHandle(handle windows.Handle) error {
	var iosb windows.IO_STATUS_BLOCK
	disposition := uint32(windows.FILE_DISPOSITION_DELETE | windows.FILE_DISPOSITION_IGNORE_READONLY_ATTRIBUTE)
	return translateNTError(windows.NtSetInformationFile(
		handle,
		&iosb,
		(*byte)(unsafe.Pointer(&disposition)),
		uint32(unsafe.Sizeof(disposition)),
		windows.FileDispositionInformationEx,
	))
}

type workspaceFileRenameInformation struct {
	ReplaceIfExists uint32
	RootDirectory   windows.Handle
	FileNameLength  uint32
	FileName        [1]uint16
}

func renameWorkspaceHandle(source, parent windows.Handle, name string) error {
	utf16Name, err := windows.UTF16FromString(name)
	if err != nil {
		return err
	}
	fileNameLength := (len(utf16Name) - 1) * 2
	var header workspaceFileRenameInformation
	bufferSize := int(unsafe.Offsetof(header.FileName)) + fileNameLength
	buffer := make([]byte, bufferSize)
	info := (*workspaceFileRenameInformation)(unsafe.Pointer(&buffer[0]))
	info.ReplaceIfExists = windows.FILE_RENAME_REPLACE_IF_EXISTS | windows.FILE_RENAME_POSIX_SEMANTICS
	info.RootDirectory = parent
	info.FileNameLength = uint32(fileNameLength)
	copy((*[windows.MAX_LONG_PATH]uint16)(unsafe.Pointer(&info.FileName[0]))[:fileNameLength/2:fileNameLength/2], utf16Name[:len(utf16Name)-1])
	var iosb windows.IO_STATUS_BLOCK
	return translateNTError(windows.NtSetInformationFile(
		source,
		&iosb,
		&buffer[0],
		uint32(bufferSize),
		windows.FileRenameInformation,
	))
}

func retryWorkspaceRename(err error) bool {
	var status windows.NTStatus
	if errors.As(err, &status) {
		return status == windows.STATUS_ACCESS_DENIED || status == windows.STATUS_SHARING_VIOLATION
	}
	return errors.Is(err, windows.ERROR_ACCESS_DENIED) || errors.Is(err, windows.ERROR_SHARING_VIOLATION)
}
