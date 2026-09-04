package benchmark

import (
	"context"
	"os"
	"os/exec"
	"strings"

	"github.com/saiaathish/picogent/internal/procenv"
)

func runOutcomeQualityWorkerCommand(ctx context.Context, command *exec.Cmd) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := command.Start(); err != nil {
		return err
	}
	terminate, cleanup, err := attachOutcomeQualityWorkerProcess(command)
	if err != nil {
		terminateOutcomeQualityWorkerCommand(command)
		_ = command.Wait()
		return err
	}
	defer cleanup()

	waitDone := make(chan error, 1)
	go func() {
		waitDone <- command.Wait()
	}()
	select {
	case err := <-waitDone:
		return err
	case <-ctx.Done():
		terminate()
		<-waitDone
		return ctx.Err()
	}
}

func outcomeQualityWorkerEnvironment() []string {
	allowed := map[string]struct{}{
		"PATH":                                  {},
		"HOME":                                  {},
		"USERPROFILE":                           {},
		"TMPDIR":                                {},
		"TMP":                                   {},
		"TEMP":                                  {},
		"SYSTEMROOT":                            {},
		"WINDIR":                                {},
		"LANG":                                  {},
		"LC_ALL":                                {},
		"LC_CTYPE":                              {},
		"TZ":                                    {},
		"PICOGENT_OUTCOME_QUALITY_WORKER_CHILD": {},
	}

	sanitized := procenv.Sanitized()
	out := make([]string, 0, len(sanitized))
	for _, entry := range sanitized {
		key, _, ok := strings.Cut(entry, "=")
		if !ok {
			continue
		}
		if _, ok := allowed[strings.ToUpper(key)]; ok {
			out = append(out, entry)
		}
	}
	if path, ok := os.LookupEnv("PATH"); ok && !hasOutcomeQualityWorkerEnvKey(out, "PATH") {
		out = append(out, "PATH="+path)
	}
	return out
}

func outcomeQualityWorkerEnvironmentWithCache(cacheDir string) []string {
	env := outcomeQualityWorkerEnvironment()
	cacheDir = strings.TrimSpace(cacheDir)
	if cacheDir == "" {
		return env
	}
	return append(env, "GOCACHE="+cacheDir)
}

func hasOutcomeQualityWorkerEnvKey(env []string, want string) bool {
	for _, entry := range env {
		key, _, ok := strings.Cut(entry, "=")
		if ok && strings.EqualFold(key, want) {
			return true
		}
	}
	return false
}
