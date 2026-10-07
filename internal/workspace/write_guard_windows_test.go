//go:build windows

package workspace

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/windows"
)

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
