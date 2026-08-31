package benchmark_test

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/saiaathish/picogent/internal/config"
	"gopkg.in/yaml.v3"
)

const processEnvelopePrompt = "respond with a benchmark answer"

// BenchmarkProcessEnvelope measures the real CLI process boundary around one
// deterministic first turn. The provider is loopback and scripted so the
// result isolates Picogent startup/config/agent/HTTP control flow from live
// model latency and quality.
func BenchmarkProcessEnvelope(b *testing.B) {
	binary, workspace, server, requests := processEnvelopeFixture(b)
	defer server.Close()

	b.Run("fresh-state", func(b *testing.B) {
		var outputBytes int
		var childMaxRSS int64
		for i := 0; i < b.N; i++ {
			b.StopTimer()
			home := b.TempDir()
			writeProcessConfig(b, home, workspace, server.URL)
			b.StartTimer()
			stdout, stderr, rss, err := runBenchmarkProcess(binary, workspace, home)
			b.StopTimer()
			assertProcessEnvelope(b, stdout, stderr, err)
			outputBytes = len(stdout) + len(stderr)
			if rss > childMaxRSS {
				childMaxRSS = rss
			}
		}
		b.ReportMetric(float64(outputBytes), "output-bytes/op")
		if childMaxRSS > 0 {
			b.ReportMetric(float64(childMaxRSS), "child-max-rss-B/op")
		}
	})

	b.Run("warm-state", func(b *testing.B) {
		home := b.TempDir()
		writeProcessConfig(b, home, workspace, server.URL)
		stdout, stderr, _, err := runBenchmarkProcess(binary, workspace, home)
		assertProcessEnvelope(b, stdout, stderr, err)

		var outputBytes int
		var childMaxRSS int64
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			stdout, stderr, rss, runErr := runBenchmarkProcess(binary, workspace, home)
			err = runErr
			assertProcessEnvelope(b, stdout, stderr, err)
			outputBytes = len(stdout) + len(stderr)
			if rss > childMaxRSS {
				childMaxRSS = rss
			}
		}
		b.ReportMetric(float64(outputBytes), "output-bytes/op")
		if childMaxRSS > 0 {
			b.ReportMetric(float64(childMaxRSS), "child-max-rss-B/op")
		}
	})

	b.Logf("binary=%s provider_requests=%d", binary, requests.Load())
}

func processEnvelopeFixture(b *testing.B) (string, string, *httptest.Server, *atomic.Int64) {
	b.Helper()
	workspace := b.TempDir()
	requests := new(atomic.Int64)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			http.NotFound(w, r)
			return
		}
		requests.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"choices":[{"message":{"role":"assistant","content":"benchmark response"}}]}`)
	}))

	root := benchmarkModuleRoot(b)
	binary := filepath.Join(b.TempDir(), "picogent")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	build := exec.Command("go", "build", "-o", binary, "./cmd/picogent")
	build.Dir = root
	if output, err := build.CombinedOutput(); err != nil {
		server.Close()
		b.Fatalf("build process-envelope binary: %v\n%s", err, output)
	}
	return binary, workspace, server, requests
}

func writeProcessConfig(b *testing.B, home, workspace, baseURL string) {
	b.Helper()
	cfg := config.Default()
	cfg.Workspace = workspace
	cfg.Mode = config.ModeFast
	cfg.Provider = config.ProviderOpenAI
	cfg.BaseURL = baseURL
	cfg.APIKey = "benchmark-key"
	cfg.Model = "benchmark-model"
	cfg.SetupComplete = true
	cfg.AutoTaskMode = boolPointer(false)
	cfg.Router.Enabled = false
	cfg.Router.UseLLMAdvisor = false
	data, err := yaml.Marshal(cfg)
	if err != nil {
		b.Fatalf("marshal process-envelope config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(home, "config.yaml"), data, 0o600); err != nil {
		b.Fatalf("write process-envelope config: %v", err)
	}
}

func runBenchmarkProcess(binary, workspace, home string) (string, string, int64, error) {
	cmd := exec.Command(binary, "run", "--yes", "--dir", workspace, processEnvelopePrompt)
	cmd.Env = processEnvelopeEnv(home)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.String(), stderr.String(), childMaxRSSBytes(cmd.ProcessState), err
}

// childMaxRSSBytes extracts the exited CLI process's resident-set sample from
// the platform-specific rusage value without importing platform-specific
// syscall types into this benchmark. Darwin reports bytes; Linux reports KiB.
// Unsupported platforms simply omit the metric rather than inventing a unit.
func childMaxRSSBytes(state *os.ProcessState) int64 {
	if state == nil || (runtime.GOOS != "darwin" && runtime.GOOS != "linux") {
		return 0
	}
	usage := state.SysUsage()
	value := reflect.ValueOf(usage)
	if value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return 0
		}
		value = value.Elem()
	}
	if value.Kind() != reflect.Struct {
		return 0
	}
	field := value.FieldByName("Maxrss")
	if !field.IsValid() || !field.CanInt() {
		return 0
	}
	rss := field.Int()
	if rss <= 0 {
		return 0
	}
	if runtime.GOOS == "linux" {
		rss *= 1024
	}
	return rss
}

func assertProcessEnvelope(b *testing.B, stdout, stderr string, err error) {
	b.Helper()
	if err != nil {
		b.Fatalf("process first turn: %v\nstdout=%q\nstderr=%q", err, stdout, stderr)
	}
	if !strings.Contains(stdout, "benchmark response") {
		b.Fatalf("stdout=%q, missing deterministic provider response", stdout)
	}
	if strings.Contains(stderr, "Problem:") {
		b.Fatalf("stderr=%q, contains a process failure", stderr)
	}
}

func processEnvelopeEnv(home string) []string {
	env := make([]string, 0, 16)
	for _, key := range []string{"PATH", "SYSTEMROOT", "WINDIR", "TMPDIR", "TMP", "TEMP"} {
		if value := os.Getenv(key); value != "" {
			env = append(env, key+"="+value)
		}
	}
	env = append(env,
		"HOME="+home,
		"PICOGENT_HOME="+home,
		"PICOGENT_CODEX_HOME="+filepath.Join(home, "codex"),
		"PICOGENT_PROVIDER=",
		"PICOGENT_BASE_URL=",
		"PICOGENT_API_KEY=",
		"OPENAI_API_KEY=",
		"PICOGENT_MODEL=",
		"PICOGENT_ROUTER=0",
		"PICOGENT_MODE=",
	)
	return env
}

func benchmarkModuleRoot(b *testing.B) string {
	b.Helper()
	dir, err := os.Getwd()
	if err != nil {
		b.Fatalf("get benchmark working directory: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			b.Fatal("could not find go.mod for process-envelope benchmark")
		}
		dir = parent
	}
}

func boolPointer(value bool) *bool {
	return &value
}
