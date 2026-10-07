package setup

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/saiaathish/picogent/internal/config"
	"gopkg.in/yaml.v3"
)

func TestPrepareDoesNotSave(t *testing.T) {
	for _, state := range []string{"missing", "existing", "unusable"} {
		t.Run(state, func(t *testing.T) {
			home := filepath.Join(t.TempDir(), "picogent")
			t.Setenv("PICOGENT_HOME", home)
			path, err := config.Path()
			if err != nil {
				t.Fatal(err)
			}
			original := []byte("existing config must remain unchanged\n")
			switch state {
			case "existing":
				if err := os.MkdirAll(home, 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, original, 0o600); err != nil {
					t.Fatal(err)
				}
			case "unusable":
				if err := os.WriteFile(home, original, 0o600); err != nil {
					t.Fatal(err)
				}
			}

			workspace := t.TempDir()
			cfg := config.Default()
			cfg.Provider = ""
			got, err := Prepare(cfg, "  "+workspace+"  ", "fast", "  custom-model  ")
			if err != nil {
				t.Fatal(err)
			}
			if got.Workspace != workspace || got.Mode != config.ModeFast || got.Provider != config.ProviderCodex || got.Model != "custom-model" || got.Router.Enabled || !got.SetupComplete {
				t.Fatalf("unexpected prepared config: %+v", got)
			}
			if state == "missing" {
				if _, err := os.Stat(home); !os.IsNotExist(err) {
					t.Fatalf("Prepare created the config home: %v", err)
				}
				return
			}
			if state == "unusable" {
				path = home
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(data, original) {
				t.Fatalf("Prepare changed existing bytes: %q", data)
			}
		})
	}
}

func TestApplyPersistsPreparedConfig(t *testing.T) {
	t.Setenv("PICOGENT_HOME", t.TempDir())
	workspace := t.TempDir()
	got, err := Apply(config.Default(), workspace, "fast", "custom-model")
	if err != nil {
		t.Fatal(err)
	}
	path, err := config.Path()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var saved config.Config
	if err := yaml.Unmarshal(data, &saved); err != nil {
		t.Fatal(err)
	}
	if saved.Workspace != got.Workspace || saved.Mode != got.Mode || saved.Provider != got.Provider || saved.Model != got.Model || saved.Router.Enabled != got.Router.Enabled || !saved.SetupComplete {
		t.Fatalf("saved config differs from Apply result: %+v; result: %+v", saved, got)
	}
}

// All installer tests use private homes, disable installs, and stub discovery.
func configTransactionInstaller(t *testing.T) (string, string, *int) {
	t.Helper()
	home := filepath.Join(t.TempDir(), "picogent")
	t.Setenv("PICOGENT_HOME", home)
	t.Setenv("PICOGENT_CODEX_HOME", t.TempDir())
	t.Setenv("PICOGENT_SETUP_SKIP_CLIS", "1")
	oldLookup := execLookPath
	t.Cleanup(func() { execLookPath = oldLookup })
	lookups := new(int)
	execLookPath = func(string) (string, error) {
		*lookups = *lookups + 1
		return "", os.ErrNotExist
	}
	path, err := config.Path()
	if err != nil {
		t.Fatal(err)
	}
	return home, path, lookups
}

func TestInstallCoresWithConfigUsesCustomInitializer(t *testing.T) {
	for _, writeConfig := range []bool{false, true} {
		name := "leave config missing"
		if writeConfig {
			name = "write custom config"
		}
		t.Run(name, func(t *testing.T) {
			home, path, lookups := configTransactionInstaller(t)
			original := []byte("custom initializer owns these bytes\n")
			calls := 0
			_, err := InstallCoresWithConfig(func() error {
				calls++
				if st, err := os.Stat(home); err != nil || !st.IsDir() {
					t.Fatalf("home not created before initializer: %v", err)
				}
				if *lookups != 0 {
					t.Fatal("CLI discovery ran before initializer")
				}
				if _, err := os.Stat(path); !os.IsNotExist(err) {
					t.Fatalf("installer initialized config before callback: %v", err)
				}
				if writeConfig {
					return os.WriteFile(path, original, 0o600)
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
			if calls != 1 || *lookups == 0 || Busy() {
				t.Fatalf("callback calls = %d, CLI lookups = %d, busy = %v", calls, *lookups, Busy())
			}
			if !writeConfig {
				if _, err := os.Stat(path); !os.IsNotExist(err) {
					t.Fatalf("installer saved config after no-op callback: %v", err)
				}
				return
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(data, original) {
				t.Fatalf("installer overwrote custom config: %q", data)
			}
		})
	}
}

func TestInstallCoresWithConfigInitializerErrorStopsSetup(t *testing.T) {
	for _, existing := range []bool{false, true} {
		name := "missing config"
		if existing {
			name = "existing config"
		}
		t.Run(name, func(t *testing.T) {
			home, path, lookups := configTransactionInstaller(t)
			original := []byte("preserve existing config on callback error\n")
			if existing {
				if err := os.MkdirAll(home, 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, original, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			wantErr := errors.New("config transaction failed")
			calls := 0
			_, err := InstallCoresWithConfig(func() error {
				calls++
				return wantErr
			})
			if !errors.Is(err, wantErr) || calls != 1 || *lookups != 0 || Busy() {
				t.Fatalf("error = %v, callback calls = %d, CLI lookups = %d, busy = %v", err, calls, *lookups, Busy())
			}
			if !existing {
				if _, err := os.Stat(path); !os.IsNotExist(err) {
					t.Fatalf("installer saved config after callback error: %v", err)
				}
				return
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(data, original) {
				t.Fatalf("installer overwrote config after callback error: %q", data)
			}
		})
	}
}

func TestInstallCoresWithConfigRejectsNilInitializer(t *testing.T) {
	home, _, lookups := configTransactionInstaller(t)
	if _, err := InstallCoresWithConfig(nil); err == nil {
		t.Fatal("expected error for nil initializer")
	}
	if *lookups != 0 || Busy() {
		t.Fatalf("CLI lookups = %d, busy = %v", *lookups, Busy())
	}
	if _, err := os.Stat(home); !os.IsNotExist(err) {
		t.Fatalf("nil initializer created config home: %v", err)
	}
}

func TestInstallCoresRetainsExistingConfig(t *testing.T) {
	home, path, _ := configTransactionInstaller(t)
	if err := os.MkdirAll(home, 0o700); err != nil {
		t.Fatal(err)
	}
	original := []byte("# retain standalone config exactly\nworkspace: custom\nsetup_complete: true\n")
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := InstallCores(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(data, original) {
		t.Fatalf("standalone installer overwrote config: %q", data)
	}
}

func TestInstallCoresReturnsNonMissingConfigStatError(t *testing.T) {
	home, path, lookups := configTransactionInstaller(t)
	if err := os.MkdirAll(home, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("config.yaml", path); err != nil {
		t.Skipf("cannot create symlink fixture: %v", err)
	}
	_, statErr := os.Stat(path)
	if statErr == nil || os.IsNotExist(statErr) {
		t.Skipf("platform does not expose a non-missing symlink-loop stat error: %v", statErr)
	}
	_, err := InstallCores()
	if err == nil || !errors.Is(err, errors.Unwrap(statErr)) || *lookups != 0 || Busy() {
		t.Fatalf("error = %v, want stat error %v; CLI lookups = %d, busy = %v", err, statErr, *lookups, Busy())
	}
	target, err := os.Readlink(path)
	if err != nil {
		t.Fatal(err)
	}
	if target != "config.yaml" {
		t.Fatalf("standalone installer changed config symlink: %q", target)
	}
}
