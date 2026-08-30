//go:build !windows

package workspace

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"time"

	"golang.org/x/sys/unix"
)

type openKind uint8

const (
	openRead openKind = iota
	openWrite
	openEdit
)

var workspaceTempSequence atomic.Uint64

func isWorkspaceNotExist(err error) bool {
	return os.IsNotExist(err) || errors.Is(err, unix.ENOENT)
}

func open(root, path string, kind openKind) (*os.File, error) {
	rel, err := Relative(root, path)
	if err != nil {
		return nil, err
	}
	createParents := kind == openWrite
	parent, leaf, err := openParent(root, rel, createParents)
	if err != nil {
		return nil, err
	}
	defer unix.Close(parent)

	flags := unix.O_CLOEXEC | unix.O_NOFOLLOW
	switch kind {
	case openRead:
		// Nonblocking open prevents a FIFO or other special file from
		// suspending the freshness capture before the regular-file check.
		flags |= unix.O_RDONLY | unix.O_NONBLOCK
	case openWrite:
		flags |= unix.O_WRONLY | unix.O_CREAT
	case openEdit:
		flags |= unix.O_RDWR
	default:
		return nil, fmt.Errorf("unknown workspace operation")
	}

	fd, err := unix.Openat(parent, leaf, flags, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open workspace file %q: %w", rel, err)
	}
	f := os.NewFile(uintptr(fd), path)
	if f == nil {
		_ = unix.Close(fd)
		return nil, fmt.Errorf("open workspace file %q: could not wrap descriptor", rel)
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("stat workspace file %q: %w", rel, err)
	}
	if !info.Mode().IsRegular() {
		_ = f.Close()
		return nil, fmt.Errorf("workspace path %q is not a regular file", rel)
	}
	if err := rejectHardLinkFile(f); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("open workspace file %q: %w", rel, err)
	}
	return f, nil
}

func openDir(root, path string) (*os.File, error) {
	parts, err := directoryParts(root, path)
	if err != nil {
		return nil, err
	}
	fd, err := openUnixRoot(root)
	if err != nil {
		return nil, fmt.Errorf("open workspace directory: %w", err)
	}
	current := fd
	for _, part := range parts {
		child, openErr := unix.Openat(current, part, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
		if openErr != nil {
			_ = unix.Close(current)
			return nil, fmt.Errorf("open workspace directory %q: %w", part, openErr)
		}
		_ = unix.Close(current)
		current = child
	}
	f := os.NewFile(uintptr(current), path)
	if f == nil {
		_ = unix.Close(current)
		return nil, errors.New("open workspace directory: could not wrap descriptor")
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("stat workspace directory: %w", err)
	}
	if !info.IsDir() {
		_ = f.Close()
		return nil, fmt.Errorf("workspace path %q is not a directory", path)
	}
	return f, nil
}

func openParent(root, rel string, create bool) (int, string, error) {
	parts, err := pathParts(rel)
	if err != nil {
		return -1, "", err
	}
	fd, err := openUnixRoot(root)
	if err != nil {
		return -1, "", fmt.Errorf("open workspace directory: %w", err)
	}
	current := fd
	for _, part := range parts[:len(parts)-1] {
		child, openErr := unix.Openat(current, part, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
		if openErr != nil && create && errors.Is(openErr, unix.ENOENT) {
			if mkdirErr := unix.Mkdirat(current, part, 0o755); mkdirErr != nil && !errors.Is(mkdirErr, unix.EEXIST) {
				_ = unix.Close(current)
				return -1, "", fmt.Errorf("create workspace directory %q: %w", part, mkdirErr)
			}
			child, openErr = unix.Openat(current, part, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
		}
		if openErr != nil {
			_ = unix.Close(current)
			return -1, "", fmt.Errorf("open workspace directory %q: %w", part, openErr)
		}
		_ = unix.Close(current)
		current = child
	}
	return current, parts[len(parts)-1], nil
}

func openUnixRoot(root string) (int, error) {
	root, err := workspaceRootPath(root)
	if err != nil {
		return -1, err
	}
	if !filepath.IsAbs(root) {
		return -1, fmt.Errorf("workspace root is not absolute")
	}
	current, err := unix.Open("/", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return -1, err
	}
	parts := strings.Split(strings.TrimPrefix(filepath.Clean(root), string(filepath.Separator)), string(filepath.Separator))
	for _, part := range parts {
		if part == "" || part == "." {
			continue
		}
		child, openErr := unix.Openat(current, part, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
		if openErr != nil {
			_ = unix.Close(current)
			return -1, openErr
		}
		_ = unix.Close(current)
		current = child
	}
	return current, nil
}

func workspaceRootPath(root string) (string, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	abs = filepath.Clean(abs)
	if runtime.GOOS != "darwin" {
		return abs, nil
	}

	// macOS exposes these stable system directories through symlinks. Resolve
	// only those known aliases; application-created symlink components remain
	// rejected by O_NOFOLLOW during descriptor traversal.
	for alias, target := range map[string]string{
		"/var": "/private/var",
		"/tmp": "/private/tmp",
		"/etc": "/private/etc",
	} {
		if abs != alias && !strings.HasPrefix(abs, alias+string(filepath.Separator)) {
			continue
		}
		resolved, evalErr := filepath.EvalSymlinks(alias)
		if evalErr != nil || filepath.Clean(resolved) != target {
			return "", fmt.Errorf("trusted system alias %q is not stable", alias)
		}
		return target + strings.TrimPrefix(abs, alias), nil
	}
	return abs, nil
}

func remove(root, path string) error {
	rel, err := Relative(root, path)
	if err != nil {
		return err
	}
	parent, leaf, err := openParent(root, rel, false)
	if err != nil {
		return err
	}
	defer unix.Close(parent)
	// Keep the Unix behavior aligned with Windows: a caller asking to remove
	// a workspace file must not silently remove an attacker-injected symlink
	// or another non-regular entry. The final unlink remains name-based, so a
	// replacement after this check can at worst remove that entry inside the
	// already descriptor-anchored parent; it cannot follow the link outside.
	if _, exists, err := workspaceTargetMode(parent, leaf); err != nil {
		return fmt.Errorf("remove workspace file %q: %w", rel, err)
	} else if !exists {
		// Preserve the usual not-exist error and its os.IsNotExist behavior.
		if err := unix.Unlinkat(parent, leaf, 0); err != nil {
			return fmt.Errorf("remove workspace file %q: %w", rel, err)
		}
		return nil
	}
	if err := unix.Unlinkat(parent, leaf, 0); err != nil {
		return fmt.Errorf("remove workspace file %q: %w", rel, err)
	}
	return nil
}

func writeAtomic(root, path string, data []byte) error {
	return writeAtomicWithMode(root, path, data, 0, false)
}

func writeAtomicWithMode(root, path string, data []byte, requestedMode os.FileMode, setMode bool) error {
	rel, err := Relative(root, path)
	if err != nil {
		return err
	}
	parent, leaf, err := openParent(root, rel, true)
	if err != nil {
		return err
	}
	defer unix.Close(parent)

	targetMode, targetExists, err := workspaceTargetMode(parent, leaf)
	if err != nil {
		return fmt.Errorf("stat workspace file %q: %w", rel, err)
	}
	if targetExists && targetMode&0o222 == 0 {
		return fmt.Errorf("workspace file %q is not writable", rel)
	}
	createMode := targetMode
	if setMode {
		createMode = uint32(requestedMode.Perm())
	}

	tmpName := ""
	var file *os.File
	for attempt := uint64(0); attempt < 32; attempt++ {
		seq := workspaceTempSequence.Add(1)
		tmpName = fmt.Sprintf(".picogent-workspace-%d-%d-%d.tmp", os.Getpid(), time.Now().UnixNano(), seq)
		fd, openErr := unix.Openat(parent, tmpName, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_CLOEXEC|unix.O_NOFOLLOW, createMode)
		if openErr == nil {
			file, err = unixFile(fd, path)
			if err != nil {
				return err
			}
			break
		}
		if !errors.Is(openErr, unix.EEXIST) {
			return fmt.Errorf("create workspace temporary file %q: %w", rel, openErr)
		}
	}
	if file == nil {
		return errors.New("could not allocate a workspace temporary file")
	}
	removeTemp := true
	defer func() {
		if removeTemp {
			removeWorkspaceTemp(parent, tmpName, file)
			_ = file.Close()
		}
	}()

	if setMode {
		if err := file.Chmod(requestedMode); err != nil {
			return fmt.Errorf("set workspace file mode %q: %w", rel, err)
		}
	} else if targetExists {
		if err := unix.Fchmod(int(file.Fd()), targetMode); err != nil {
			return fmt.Errorf("preserve workspace file mode %q: %w", rel, err)
		}
	}
	if err := writeWorkspaceAll(file, data); err != nil {
		return fmt.Errorf("write workspace file %q: %w", rel, err)
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("sync workspace file %q: %w", rel, err)
	}
	if matches, err := workspaceTempMatches(parent, tmpName, file); err != nil {
		return fmt.Errorf("validate workspace temporary file %q: %w", rel, err)
	} else if !matches {
		return fmt.Errorf("workspace temporary file %q changed before commit", rel)
	}
	if _, _, err := workspaceTargetMode(parent, leaf); err != nil {
		return fmt.Errorf("validate workspace target %q: %w", rel, err)
	}
	// Renameat publishes a complete inode, so ordinary path readers never see
	// the temporary file being filled. POSIX has no compare-and-rename-by-inode
	// primitive; an uncooperative same-UID writer can still race this pathname
	// after the identity check, which is outside this helper's guarantee.
	if err := unix.Renameat(parent, tmpName, parent, leaf); err != nil {
		return fmt.Errorf("publish workspace file %q: %w", rel, err)
	}
	removeTemp = false
	if err := file.Close(); err != nil {
		return fmt.Errorf("close workspace file %q: %w", rel, err)
	}
	if err := unix.Fsync(parent); err != nil && !errors.Is(err, unix.EINVAL) && !errors.Is(err, unix.ENOTSUP) && !errors.Is(err, unix.EOPNOTSUPP) {
		return fmt.Errorf("sync workspace directory for %q: %w", rel, err)
	}
	return nil
}

func workspaceTargetMode(parent int, leaf string) (uint32, bool, error) {
	var target unix.Stat_t
	if err := unix.Fstatat(parent, leaf, &target, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		if errors.Is(err, unix.ENOENT) {
			return 0o644, false, nil
		}
		return 0, false, err
	}
	targetMode := uint32(target.Mode)
	switch targetMode & uint32(unix.S_IFMT) {
	case uint32(unix.S_IFREG):
		if err := rejectHardLinkCount(uint64(target.Nlink)); err != nil {
			return 0, false, err
		}
		return targetMode & 0o7777, true, nil
	case uint32(unix.S_IFLNK):
		return 0, false, fmt.Errorf("workspace path %q is a symbolic link", leaf)
	default:
		return 0, false, fmt.Errorf("workspace path %q is not a regular file", leaf)
	}
}

func unixFile(fd int, name string) (*os.File, error) {
	if fd < 0 {
		return nil, errors.New("invalid workspace file descriptor")
	}
	f := os.NewFile(uintptr(fd), name)
	if f == nil {
		_ = unix.Close(fd)
		return nil, fmt.Errorf("open workspace file %q: could not wrap descriptor", name)
	}
	return f, nil
}

func workspaceTempMatches(parent int, name string, file *os.File) (bool, error) {
	var expected unix.Stat_t
	if err := unix.Fstat(int(file.Fd()), &expected); err != nil {
		return false, err
	}
	var named unix.Stat_t
	if err := unix.Fstatat(parent, name, &named, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		if errors.Is(err, unix.ENOENT) {
			return false, nil
		}
		return false, err
	}
	return expected.Dev == named.Dev && expected.Ino == named.Ino, nil
}

func removeWorkspaceTemp(parent int, name string, file *os.File) {
	matches, err := workspaceTempMatches(parent, name, file)
	if err == nil && matches {
		_ = unix.Unlinkat(parent, name, 0)
	}
}
