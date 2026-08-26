package workspace

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	// MaxTrackedFiles bounds the amount of workspace state that one evidence
	// record can bind to. A truncated binding is never considered fresh.
	MaxTrackedFiles = 128
	// MaxPathInputs bounds caller-controlled path-list processing. Passing more
	// inputs is conservatively recorded as truncation even when they repeat.
	MaxPathInputs = MaxTrackedFiles * 4
	// MaxFingerprintBytes keeps capture work bounded. Files larger than this
	// limit remain explicitly unknown instead of being treated as unchanged.
	MaxFingerprintBytes = 1 << 20
	// MaxTrackedPathBytes bounds the serialized path carried by an observation.
	MaxTrackedPathBytes = 500
)

// Identity is an opaque filesystem identity. Volume and File are populated by
// the platform-specific implementation; callers must only compare the value
// and must never use it as a path or an authorization decision.
type Identity struct {
	Volume uint64 `json:"volume,omitempty"`
	File   uint64 `json:"file,omitempty"`
	Known  bool   `json:"known"`
}

// FileObservation is a bounded content observation for one workspace-relative
// regular file. Unknown observations are deliberately not evidence of
// equality.
type FileObservation struct {
	Path     string   `json:"path"`
	Exists   bool     `json:"exists"`
	Identity Identity `json:"identity"`
	Size     int64    `json:"size,omitempty"`
	Digest   string   `json:"digest,omitempty"`
	Known    bool     `json:"known"`
}

// Observation binds a bounded set of file bytes to one workspace identity.
// It contains no file contents and is safe to persist as compact evidence.
type Observation struct {
	Root           string            `json:"root"`
	RootIdentity   Identity          `json:"root_identity"`
	Files          []FileObservation `json:"files,omitempty"`
	FilesTruncated bool              `json:"files_truncated,omitempty"`
}

// Comparison describes whether two observations can be used as the same
// evidence boundary. Fresh is true only when every required check is known and
// all observed identities and digests agree.
type Comparison struct {
	Fresh   bool
	Changed bool
	Unknown bool
	Reason  string
}

// Capture takes a fresh, bounded observation of root and the requested files.
// Missing files are known observations; invalid paths, symlinks, and other
// unsafe filesystem failures return an error so callers fail closed.
func Capture(ctx context.Context, root string, paths []string) (Observation, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return Observation{}, err
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return Observation{}, fmt.Errorf("resolve workspace root: %w", err)
	}
	abs = filepath.Clean(abs)
	f, err := os.Open(abs)
	if err != nil {
		return Observation{}, fmt.Errorf("open workspace root: %w", err)
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return Observation{}, fmt.Errorf("stat workspace root: %w", err)
	}
	if !info.IsDir() {
		return Observation{}, errors.New("workspace root is not a directory")
	}
	rootState, err := describeFile(f)
	if err != nil {
		return Observation{}, fmt.Errorf("identify workspace root: %w", err)
	}
	canonical, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return Observation{}, fmt.Errorf("resolve workspace root symlinks: %w", err)
	}
	canonical = filepath.Clean(canonical)
	canonicalRoot, err := os.Open(canonical)
	if err != nil {
		return Observation{}, fmt.Errorf("open resolved workspace root: %w", err)
	}
	defer canonicalRoot.Close()
	canonicalState, err := describeFile(canonicalRoot)
	if err != nil {
		return Observation{}, fmt.Errorf("identify resolved workspace root: %w", err)
	}
	if !rootState.IsDir || !canonicalState.IsDir || !sameFileState(rootState, canonicalState) {
		return Observation{}, errors.New("workspace root changed while resolving")
	}
	rootState = canonicalState

	tracked, truncated, err := trackedPaths(abs, paths)
	if err != nil {
		return Observation{}, err
	}
	observation := Observation{
		Root:           canonical,
		RootIdentity:   rootState.Identity,
		FilesTruncated: truncated,
		Files:          make([]FileObservation, 0, len(tracked)),
	}
	for _, path := range tracked {
		if err := ctx.Err(); err != nil {
			return Observation{}, err
		}
		file, err := captureFile(ctx, canonical, path)
		if err != nil {
			return Observation{}, fmt.Errorf("capture %q: %w", path, err)
		}
		observation.Files = append(observation.Files, file)
	}
	// The root handle above remains attached to the original directory, so a
	// pathname replacement can otherwise mix old identity with new-root files.
	// Reopen both the caller's name and the resolved name after all file reads.
	currentRoot, rootErr := os.Open(abs)
	if rootErr != nil {
		observation.RootIdentity.Known = false
		return observation, nil
	}
	currentState, stateErr := describeFile(currentRoot)
	_ = currentRoot.Close()
	if stateErr != nil || !currentState.IsDir || !sameFileState(rootState, currentState) {
		observation.RootIdentity.Known = false
		return observation, nil
	}
	currentCanonical, canonicalErr := os.Open(canonical)
	if canonicalErr != nil {
		observation.RootIdentity.Known = false
		return observation, nil
	}
	canonicalState, canonicalStateErr := describeFile(currentCanonical)
	_ = currentCanonical.Close()
	if canonicalStateErr != nil || !canonicalState.IsDir || !sameFileState(rootState, canonicalState) {
		observation.RootIdentity.Known = false
	}
	return observation, nil
}

func trackedPaths(root string, paths []string) ([]string, bool, error) {
	inputLimit := len(paths)
	truncated := inputLimit > MaxPathInputs
	if inputLimit > MaxPathInputs {
		inputLimit = MaxPathInputs
	}
	seen := make(map[string]struct{}, inputLimit)
	tracked := make([]string, 0, inputLimit)
	for _, path := range paths[:inputLimit] {
		if strings.TrimSpace(path) == "" {
			continue
		}
		rel, err := Relative(root, path)
		if err != nil {
			return nil, false, fmt.Errorf("track %q: %w", path, err)
		}
		if _, ok := seen[rel]; ok {
			continue
		}
		seen[rel] = struct{}{}
		tracked = append(tracked, rel)
	}
	sort.Strings(tracked)
	if len(tracked) > MaxTrackedFiles {
		truncated = true
		tracked = tracked[:MaxTrackedFiles]
	}
	return tracked, truncated, nil
}

type fileState struct {
	Identity Identity
	Size     int64
	ModTime  int64
	IsDir    bool
}

func captureFile(ctx context.Context, root, rel string) (FileObservation, error) {
	f, err := OpenRead(root, rel)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return FileObservation{Path: rel, Known: true}, nil
		}
		return FileObservation{}, err
	}
	defer f.Close()

	before, err := describeFile(f)
	if err != nil {
		return FileObservation{}, err
	}
	observation := FileObservation{
		Path:     rel,
		Exists:   true,
		Identity: before.Identity,
		Size:     before.Size,
	}
	if before.Size < 0 || before.Size > MaxFingerprintBytes {
		return observation, nil
	}
	if err := ctx.Err(); err != nil {
		return FileObservation{}, err
	}

	digest, known, err := digestFile(ctx, f)
	if err != nil {
		return FileObservation{}, err
	}
	if !known {
		return observation, nil
	}
	if err := ctx.Err(); err != nil {
		return FileObservation{}, err
	}
	after, err := describeFile(f)
	if err != nil {
		return FileObservation{}, err
	}
	if !sameFileState(before, after) {
		return observation, nil
	}

	// A handle remains attached to the old inode when the final path is
	// replaced. Reopen the name once so replacement during capture is never
	// mistaken for stable content.
	current, err := OpenRead(root, rel)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return observation, nil
		}
		return FileObservation{}, err
	}
	currentState, stateErr := describeFile(current)
	if stateErr != nil {
		_ = current.Close()
		return FileObservation{}, stateErr
	}
	if !sameFileState(before, currentState) {
		_ = current.Close()
		return observation, nil
	}
	currentDigest, currentKnown, err := digestFile(ctx, current)
	currentAfter, currentAfterErr := describeFile(current)
	_ = current.Close()
	if err != nil {
		return FileObservation{}, err
	}
	if currentAfterErr != nil {
		return FileObservation{}, currentAfterErr
	}
	if !currentKnown || !sameFileState(currentState, currentAfter) || currentDigest != digest {
		return observation, nil
	}

	observation.Digest = digest
	observation.Known = true
	return observation, nil
}

func digestFile(ctx context.Context, f *os.File) (string, bool, error) {
	if err := ctx.Err(); err != nil {
		return "", false, err
	}
	hash := sha256.New()
	count, err := io.CopyN(hash, f, MaxFingerprintBytes+1)
	if err != nil && !errors.Is(err, io.EOF) {
		return "", false, err
	}
	if count > MaxFingerprintBytes {
		return "", false, nil
	}
	if err := ctx.Err(); err != nil {
		return "", false, err
	}
	return hex.EncodeToString(hash.Sum(nil)), true, nil
}

func describeFile(f *os.File) (fileState, error) {
	info, err := f.Stat()
	if err != nil {
		return fileState{}, err
	}
	identity, err := identityForFile(f)
	if err != nil {
		return fileState{}, err
	}
	return fileState{Identity: identity, Size: info.Size(), ModTime: info.ModTime().UnixNano(), IsDir: info.IsDir()}, nil
}

func sameFileState(left, right fileState) bool {
	return left.Identity == right.Identity && left.Size == right.Size && left.ModTime == right.ModTime
}

// Compare checks whether after is a valid continuation of before. A changed
// identity or digest invalidates the old evidence; an unknown or truncated
// capture remains unverified rather than being called fresh.
func Compare(before, after Observation) Comparison {
	if reason := invalidObservation(before); reason != "" {
		return Comparison{Unknown: true, Reason: reason}
	}
	if reason := invalidObservation(after); reason != "" {
		return Comparison{Unknown: true, Reason: reason}
	}
	if !before.RootIdentity.Known || !after.RootIdentity.Known {
		return Comparison{Unknown: true, Reason: "workspace identity is unknown"}
	}
	if filepath.Clean(before.Root) != filepath.Clean(after.Root) || before.RootIdentity != after.RootIdentity {
		return Comparison{Changed: true, Reason: "workspace identity changed"}
	}
	if before.FilesTruncated || after.FilesTruncated {
		return Comparison{Unknown: true, Reason: "tracked file set is truncated"}
	}
	if len(before.Files) == 0 || len(after.Files) == 0 {
		return Comparison{Unknown: true, Reason: "no tracked file evidence"}
	}
	if len(before.Files) != len(after.Files) {
		return Comparison{Unknown: true, Reason: "tracked file set changed"}
	}
	for i := range before.Files {
		left, right := before.Files[i], after.Files[i]
		if left.Path != right.Path {
			return Comparison{Unknown: true, Reason: "tracked file set changed"}
		}
		if !left.Known || !right.Known {
			return Comparison{Unknown: true, Reason: "file observation is unknown"}
		}
		if left.Exists != right.Exists {
			return Comparison{Changed: true, Reason: "tracked file existence changed"}
		}
		if !left.Exists {
			continue
		}
		if left.Identity != right.Identity || left.Size != right.Size || left.Digest == "" || right.Digest == "" {
			if left.Identity != right.Identity || left.Size != right.Size {
				return Comparison{Changed: true, Reason: "tracked file identity changed"}
			}
			return Comparison{Unknown: true, Reason: "tracked file digest is missing"}
		}
		if left.Digest != right.Digest {
			return Comparison{Changed: true, Reason: "tracked file content changed"}
		}
	}
	return Comparison{Fresh: true}
}

func invalidObservation(observation Observation) string {
	if strings.TrimSpace(observation.Root) == "" {
		return "workspace root is missing"
	}
	if len(observation.Root) > MaxTrackedPathBytes*2 {
		return "workspace root is too long"
	}
	if observation.RootIdentity.Known && observation.RootIdentity.Volume == 0 && observation.RootIdentity.File == 0 {
		return "workspace identity is malformed"
	}
	if len(observation.Files) > MaxTrackedFiles {
		return "tracked file set exceeds bound"
	}
	seen := make(map[string]struct{}, len(observation.Files))
	for _, file := range observation.Files {
		if strings.TrimSpace(file.Path) == "" || len(file.Path) > MaxTrackedPathBytes || filepath.IsAbs(file.Path) {
			return "tracked file path is malformed"
		}
		rel, err := Relative(observation.Root, file.Path)
		if err != nil || filepath.Clean(rel) != filepath.Clean(file.Path) {
			return "tracked file path is outside workspace"
		}
		if _, ok := seen[file.Path]; ok {
			return "tracked file set contains duplicates"
		}
		seen[file.Path] = struct{}{}
		if file.Size < 0 || file.Size > MaxFingerprintBytes {
			return "tracked file size is invalid"
		}
		if file.Known && file.Exists {
			if !file.Identity.Known || file.Identity.Volume == 0 && file.Identity.File == 0 {
				return "tracked file identity is malformed"
			}
			if len(file.Digest) != sha256.Size*2 {
				return "tracked file digest is malformed"
			}
			if _, err := hex.DecodeString(file.Digest); err != nil {
				return "tracked file digest is malformed"
			}
		}
	}
	return ""
}
