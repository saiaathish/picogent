package benchmark

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/saiaathish/picogent/internal/procenv"
	"github.com/saiaathish/picogent/internal/workspace"
)

const (
	maxOutcomeQualityLegacyOutputBytes          = 64 << 10
	maxOutcomeQualityLegacyBuildTimeout         = 2 * time.Minute
	maxOutcomeQualityLegacyBinaryBytes          = 128 << 20
	maxOutcomeQualityLegacyProviderPayloadBytes = 8 << 20
	maxOutcomeQualityLegacyWorkspaceEntries     = 512
	maxOutcomeQualityLegacyWorkspaceDepth       = 32
	defaultOutcomeQualityLegacyModel            = "outcome-quality-legacy"
	outcomeQualityLegacyLocalAPIKey             = "picogent-local-provider"
)

// OutcomeQualityLegacySourceHead is the immutable v3 baseline used by this
// adapter. A different source head is a different experiment and must use a
// separately reviewed executor.
const OutcomeQualityLegacySourceHead = "a07943b31044049afb0142f39198244cd3c75218"

// OutcomeQualityLegacyBuildConfig controls the isolated v3 build and its
// local OpenAI-compatible provider endpoint. ProviderURL must point to a
// loopback HTTP server; no upstream credentials are passed to the child.
type OutcomeQualityLegacyBuildConfig struct {
	TempParent  string
	ProviderURL string
	Model       string
}

// OutcomeQualityLegacyBuild owns a v3 binary built from one validated source
// workspace. The binary, Go cache, and per-observation state all live outside
// that workspace. Call Close when the matrix is done.
type OutcomeQualityLegacyBuild struct {
	mu       sync.Mutex
	executor *OutcomeQualityLegacyProcessExecutor
	dir      string
}

// OutcomeQualityLegacyProcessExecutor launches the legacy cmd/picogent binary
// for one bounded matrix observation. It intentionally has no worker protocol:
// the v3 executable is the target being measured.
type OutcomeQualityLegacyProcessExecutor struct {
	Command     string
	Binding     OutcomeQualitySourceBinding
	ProviderURL string
	Model       string
	TempParent  string

	commandPath   string
	commandDigest [sha256.Size]byte
	commandSize   int64
}

func outcomeQualityLegacyInput(scenario OutcomeQualityScenario) OutcomeQualityInput {
	return OutcomeQualityInput{
		Prompt: fmt.Sprintf("Finish the deterministic benchmark fixture for %s", scenario.ID),
		Files: []OutcomeQualityInputFile{
			{
				Path:    "fixture.txt",
				Content: fmt.Sprintf("before %s\n", scenario.ID),
			},
			{
				Path:    "fixture_test.go",
				Content: "package fixture\n\nimport (\n\t\"os\"\n\t\"testing\"\n)\n\nfunc TestFixture(t *testing.T) {\n\tdata, err := os.ReadFile(\"fixture.txt\")\n\tif err != nil {\n\t\tt.Fatal(err)\n\t}\n\tif len(data) == 0 {\n\t\tt.Fatal(\"fixture is empty\")\n\t}\n}\n",
			},
			{
				Path:    "go.mod",
				Content: "module example.com/picogent-outcome-quality-fixture\n\ngo 1.25\n",
			},
		},
		ExpectedChangedPaths: []string{"fixture.txt"},
	}
}

// BuildOutcomeQualityLegacy validates the exact v3 source tree and builds
// ./cmd/picogent from that tree. The build output and cache are external to the
// source workspace, and the command uses typed arguments without a shell.
func BuildOutcomeQualityLegacy(ctx context.Context, binding OutcomeQualitySourceBinding, buildConfig OutcomeQualityLegacyBuildConfig) (build *OutcomeQualityLegacyBuild, err error) {
	if ctx == nil {
		ctx = context.Background()
	}
	providerURL, err := normalizeOutcomeQualityLegacyProviderURL(buildConfig.ProviderURL)
	if err != nil {
		return nil, fmt.Errorf("outcome-quality legacy provider: %w", err)
	}
	model, err := normalizeOutcomeQualityLegacyModel(buildConfig.Model)
	if err != nil {
		return nil, fmt.Errorf("outcome-quality legacy model: %w", err)
	}
	if err := validateOutcomeQualityLegacyTarget("legacy build", binding.Target); err != nil {
		return nil, err
	}
	workspaceRoot, err := canonicalOutcomeQualityWorkspace(binding.Workspace)
	if err != nil {
		return nil, fmt.Errorf("outcome-quality legacy build workspace: %w", err)
	}
	if err := validateOutcomeQualitySourceBinding(ctx, "legacy build", binding.Target, workspaceRoot); err != nil {
		return nil, err
	}
	buildParent, err := outcomeQualityLegacyTempParent(buildConfig.TempParent, workspaceRoot)
	if err != nil {
		return nil, fmt.Errorf("outcome-quality legacy build temp parent: %w", err)
	}

	buildDir, err := os.MkdirTemp(buildParent, "picogent-outcome-quality-legacy-")
	if err != nil {
		return nil, fmt.Errorf("create outcome-quality legacy build directory: %w", err)
	}
	keepBuildDir := false
	defer func() {
		if !keepBuildDir {
			if cleanupErr := removeOutcomeQualityLegacyDir(buildDir); cleanupErr != nil {
				cleanupErr = fmt.Errorf("cleanup outcome-quality legacy build directory: %w", cleanupErr)
				if err == nil {
					build = nil
					err = cleanupErr
				} else {
					err = errors.Join(err, cleanupErr)
				}
			}
		}
	}()
	canonicalBuildDir := buildDir
	if resolved, resolveErr := filepath.EvalSymlinks(buildDir); resolveErr == nil {
		canonicalBuildDir = resolved
	}
	if outcomeQualityPathWithin(filepath.Clean(canonicalBuildDir), workspaceRoot) {
		return nil, fmt.Errorf("outcome-quality legacy build directory is inside source workspace")
	}

	goCommand, err := exec.LookPath("go")
	if err != nil {
		return nil, fmt.Errorf("find Go toolchain for outcome-quality legacy build: %w", err)
	}
	binaryName := "picogent"
	if runtime.GOOS == "windows" {
		binaryName += ".exe"
	}
	binaryPath := filepath.Join(buildDir, binaryName)
	buildCtx, cancel := context.WithTimeout(ctx, maxOutcomeQualityLegacyBuildTimeout)
	defer cancel()
	if err := validateOutcomeQualityLegacyToolchain(buildCtx, goCommand, binding.Target.GoVersion); err != nil {
		return nil, fmt.Errorf("validate Go toolchain for outcome-quality legacy build: %w", err)
	}
	command := exec.CommandContext(buildCtx, goCommand, "build", "-mod=readonly", "-trimpath", "-o", binaryPath, "./cmd/picogent")
	command.Dir = workspaceRoot
	command.Env = outcomeQualityLegacyBuildEnvironment(
		filepath.Join(buildDir, "go-cache"),
		filepath.Join(buildDir, "go-mod-cache"),
	)
	var output outcomeQualityLegacyBuffer
	command.Stdout = &output
	command.Stderr = &output
	configureOutcomeQualityWorkerCommand(command)
	if err := runOutcomeQualityWorkerCommand(buildCtx, command); err != nil {
		detail := strings.TrimSpace(output.String())
		if detail != "" {
			return nil, fmt.Errorf("build outcome-quality legacy cmd/picogent: %w: %s", err, detail)
		}
		return nil, fmt.Errorf("build outcome-quality legacy cmd/picogent: %w", err)
	}
	if err := buildCtx.Err(); err != nil {
		return nil, fmt.Errorf("build outcome-quality legacy cmd/picogent: %w", err)
	}
	commandPath, err := validateOutcomeQualityLegacyCommand(binaryPath, workspaceRoot)
	if err != nil {
		return nil, fmt.Errorf("validate built outcome-quality legacy cmd/picogent: %w", err)
	}
	commandDigest, commandSize, err := hashOutcomeQualityLegacyCommand(commandPath)
	if err != nil {
		return nil, fmt.Errorf("identify built outcome-quality legacy cmd/picogent: %w", err)
	}
	if err := validateOutcomeQualitySourceBinding(ctx, "legacy build", binding.Target, workspaceRoot); err != nil {
		return nil, err
	}

	keepBuildDir = true
	return &OutcomeQualityLegacyBuild{
		executor: &OutcomeQualityLegacyProcessExecutor{
			Command:       commandPath,
			Binding:       OutcomeQualitySourceBinding{Target: binding.Target, Workspace: workspaceRoot},
			ProviderURL:   providerURL,
			Model:         model,
			TempParent:    buildDir,
			commandPath:   commandPath,
			commandDigest: commandDigest,
			commandSize:   commandSize,
		},
		dir: buildDir,
	}, nil
}

// ProcessExecutor returns the bounded executor backed by the built v3
// cmd/picogent binary.
func (b *OutcomeQualityLegacyBuild) ProcessExecutor() *OutcomeQualityLegacyProcessExecutor {
	if b == nil {
		return nil
	}
	return b.executor
}

// Close removes only the temporary directory created by
// BuildOutcomeQualityLegacy.
func (b *OutcomeQualityLegacyBuild) Close() error {
	if b == nil {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.dir == "" {
		return nil
	}
	dir := b.dir
	if err := removeOutcomeQualityLegacyDir(dir); err != nil {
		return err
	}
	b.dir = ""
	return nil
}

func (e *OutcomeQualityLegacyProcessExecutor) outcomeQualitySourceBinding() OutcomeQualitySourceBinding {
	if e == nil {
		return OutcomeQualitySourceBinding{}
	}
	return e.Binding
}

func (e *OutcomeQualityLegacyProcessExecutor) validateOutcomeQualitySource(ctx context.Context) error {
	if e == nil {
		return errors.New("outcome-quality legacy executor is nil")
	}
	workspaceRoot, err := canonicalOutcomeQualityWorkspace(e.Binding.Workspace)
	if err != nil {
		return fmt.Errorf("outcome-quality legacy workspace: %w", err)
	}
	if err := validateOutcomeQualityLegacyTarget("legacy", e.Binding.Target); err != nil {
		return err
	}
	return validateOutcomeQualitySourceBinding(ctx, "legacy", e.Binding.Target, workspaceRoot)
}

// Execute runs one exact v3 headless invocation with a fresh fixture and
// external PICOGENT_HOME/cache. Filesystem contents and the v3 verification
// line are measured directly. v3 does not expose provider token/context or
// structured repair telemetry, so those gaps are returned as explicit
// unverified reasons rather than being represented as zero measurements.
func (e *OutcomeQualityLegacyProcessExecutor) Execute(ctx context.Context, request OutcomeQualityExecutionRequest) (execution OutcomeQualityExecution, err error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if e == nil {
		return OutcomeQualityExecution{}, errors.New("outcome-quality legacy executor is nil")
	}
	input, err := validateOutcomeQualityLegacyRequest(request)
	if err != nil {
		return OutcomeQualityExecution{}, err
	}
	if err := validateOutcomeQualityProviderTarget(e.Binding.Target, request.Target); err != nil {
		return OutcomeQualityExecution{}, err
	}
	providerURL, err := normalizeOutcomeQualityLegacyProviderURL(e.ProviderURL)
	if err != nil {
		return OutcomeQualityExecution{}, fmt.Errorf("outcome-quality legacy provider: %w", err)
	}
	model, err := normalizeOutcomeQualityLegacyModel(e.Model)
	if err != nil {
		return OutcomeQualityExecution{}, fmt.Errorf("outcome-quality legacy model: %w", err)
	}
	workspaceRoot, err := canonicalOutcomeQualityWorkspace(e.Binding.Workspace)
	if err != nil {
		return OutcomeQualityExecution{}, fmt.Errorf("outcome-quality legacy workspace: %w", err)
	}
	if err := validateOutcomeQualitySourceBinding(ctx, "legacy", e.Binding.Target, workspaceRoot); err != nil {
		return OutcomeQualityExecution{}, err
	}
	commandPath, err := validateOutcomeQualityLegacyExecutable(e, workspaceRoot)
	if err != nil {
		return OutcomeQualityExecution{}, err
	}
	tempParent, err := outcomeQualityLegacyTempParent(e.TempParent, workspaceRoot)
	if err != nil {
		return OutcomeQualityExecution{}, fmt.Errorf("outcome-quality legacy temp parent: %w", err)
	}

	runRoot, err := os.MkdirTemp(tempParent, "picogent-outcome-quality-legacy-run-")
	if err != nil {
		return OutcomeQualityExecution{}, fmt.Errorf("create outcome-quality legacy run directory: %w", err)
	}
	defer func() {
		if cleanupErr := removeOutcomeQualityLegacyDir(runRoot); cleanupErr != nil {
			cleanupErr = fmt.Errorf("cleanup outcome-quality legacy run directory: %w", cleanupErr)
			if err == nil {
				err = cleanupErr
				return
			}
			err = errors.Join(err, cleanupErr)
		}
	}()
	fixtureRoot := filepath.Join(runRoot, "workspace")
	homeRoot := filepath.Join(runRoot, "home")
	cacheRoot := filepath.Join(runRoot, "go-cache")
	tempRoot := filepath.Join(runRoot, "tmp")
	for _, dir := range []string{fixtureRoot, homeRoot, cacheRoot, tempRoot} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return OutcomeQualityExecution{}, fmt.Errorf("create outcome-quality legacy directory: %w", err)
		}
	}
	if err := writeOutcomeQualityFixture(ctx, fixtureRoot, input); err != nil {
		return OutcomeQualityExecution{}, err
	}
	fixturePaths := outcomeQualityInputPaths(input)
	before, err := workspace.Capture(ctx, fixtureRoot, fixturePaths)
	if err != nil {
		return OutcomeQualityExecution{}, fmt.Errorf("capture legacy fixture before run: %w", err)
	}
	if comparison := workspace.Compare(before, before); !comparison.Fresh {
		return OutcomeQualityExecution{}, fmt.Errorf("legacy fixture before-run observation is not fresh: %s", comparison.Reason)
	}
	expected := outcomeQualityExpectedContents(request.Scenario, input)

	command := exec.Command(commandPath, "run", "--yes", "--dir", fixtureRoot, input.Prompt)
	command.Dir = fixtureRoot
	budgetProxy, err := newOutcomeQualityLegacyBudgetProxy(providerURL, request.Policy)
	if err != nil {
		return OutcomeQualityExecution{}, fmt.Errorf("start legacy provider budget proxy: %w", err)
	}
	defer budgetProxy.Close()
	command.Env = outcomeQualityLegacyEnvironment(homeRoot, tempRoot, cacheRoot, budgetProxy.URL(), model)
	command.Stdin = strings.NewReader("")
	var stdout, stderr outcomeQualityLegacyBuffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	configureOutcomeQualityWorkerCommand(command)
	runCtx, cancel := context.WithTimeout(ctx, time.Duration(request.Policy.TimeoutMillis)*time.Millisecond)
	runErr := runOutcomeQualityWorkerCommand(runCtx, command)
	runCtxErr := runCtx.Err()
	cancel()
	if runErr != nil {
		detail := strings.TrimSpace(stderr.String())
		if detail != "" {
			return OutcomeQualityExecution{}, fmt.Errorf("legacy cmd/picogent command failed: %w: %s", runErr, detail)
		}
		return OutcomeQualityExecution{}, fmt.Errorf("legacy cmd/picogent command failed: %w", runErr)
	}
	if runCtxErr != nil {
		return OutcomeQualityExecution{}, fmt.Errorf("legacy cmd/picogent command failed: %w", runCtxErr)
	}
	if stdout.truncated || stderr.truncated {
		return OutcomeQualityExecution{}, fmt.Errorf("legacy cmd/picogent output exceeded %d bytes", maxOutcomeQualityLegacyOutputBytes)
	}
	if err := validateOutcomeQualitySourceBinding(ctx, "legacy", e.Binding.Target, workspaceRoot); err != nil {
		return OutcomeQualityExecution{}, err
	}

	after, err := workspace.Capture(ctx, fixtureRoot, fixturePaths)
	if err != nil {
		return OutcomeQualityExecution{}, fmt.Errorf("capture legacy fixture after run: %w", err)
	}
	if err := after.Validate(); err != nil {
		return OutcomeQualityExecution{}, fmt.Errorf("legacy fixture after-run observation: %w", err)
	}
	if comparison := workspace.Compare(after, after); !comparison.Fresh {
		return OutcomeQualityExecution{}, fmt.Errorf("legacy fixture after-run observation is not fresh: %s", comparison.Reason)
	}

	correctContent, actual, changedPaths, inspectErr := inspectOutcomeQualityLegacyWorkspace(ctx, fixtureRoot, input, expected)
	if inspectErr != nil {
		return OutcomeQualityExecution{}, fmt.Errorf("inspect legacy fixture: %w", inspectErr)
	}
	wantChanged := append([]string(nil), input.ExpectedChangedPaths...)
	sort.Strings(wantChanged)
	changedPathsMatch := sameOutcomeQualityStrings(changedPaths, wantChanged)
	verification := outcomeQualityLegacyVerificationStatus(stderr.String())
	evidence := EvidenceUnverified
	if verification == OutcomeVerificationPass || verification == OutcomeVerificationFail {
		evidence = EvidenceCurrent
	}
	tokens, modelCalls, toolCalls := budgetProxy.Metrics()
	metrics := OutcomeQualityMetrics{
		OutcomeSuccess:      OutcomeAssessmentInconclusive,
		Correctness:         OutcomeAssessmentInconclusive,
		UserQuestions:       0,
		Tokens:              tokens,
		ModelCalls:          modelCalls,
		ToolCalls:           toolCalls,
		ChangedLines:        outcomeQualityChangedLines(input, actual),
		UnnecessaryChanges:  outcomeQualityUnnecessaryChanges(changedPaths, wantChanged),
		VerificationQuality: verification,
		RepairCount:         0,
		ContextGrowthBytes:  0,
		Evidence:            evidence,
	}

	var reasons []string
	if verification == OutcomeVerificationUnverified {
		reasons = append(reasons, "legacy v3 did not emit a parseable verification result")
	} else if verification == OutcomeVerificationInconclusive || verification == OutcomeVerificationSkipped {
		reasons = append(reasons, fmt.Sprintf("legacy v3 verification was %s", verification))
	}
	reasons = append(reasons,
		"legacy v3 does not expose structured repair counts",
		"legacy v3 does not expose context-growth measurement",
		"legacy v3 token and model-call counts are observed at the local provider boundary, not emitted by v3",
	)
	if verification == OutcomeVerificationPass && correctContent && changedPathsMatch {
		metrics.OutcomeSuccess = OutcomeAssessmentPass
		metrics.Correctness = OutcomeAssessmentPass
	} else if verification == OutcomeVerificationFail || !correctContent || !changedPathsMatch {
		metrics.OutcomeSuccess = OutcomeAssessmentFail
		metrics.Correctness = OutcomeAssessmentFail
	}
	return OutcomeQualityExecution{Metrics: metrics, Unverified: boundedOutcomeQualityReasons(reasons, 8)}, nil
}

func validateOutcomeQualityLegacyRequest(request OutcomeQualityExecutionRequest) (OutcomeQualityInput, error) {
	if !request.Variant.valid() {
		return OutcomeQualityInput{}, fmt.Errorf("unknown legacy variant %q", request.Variant)
	}
	if request.Repetition < 1 {
		return OutcomeQualityInput{}, fmt.Errorf("legacy repetition=%d is invalid", request.Repetition)
	}
	if !outcomeQualityScenarioMatchesCatalog(request.Scenario) {
		return OutcomeQualityInput{}, errors.New("legacy scenario is not in the stable catalog")
	}
	if err := validateOutcomeQualityLegacyTarget("legacy request", request.Target); err != nil {
		return OutcomeQualityInput{}, err
	}
	if err := validateOutcomeQualityPolicy(request.Policy); err != nil {
		return OutcomeQualityInput{}, fmt.Errorf("legacy policy: %w", err)
	}
	if request.Repetition > request.Policy.Repetitions {
		return OutcomeQualityInput{}, fmt.Errorf("legacy repetition=%d exceeds policy repetitions=%d", request.Repetition, request.Policy.Repetitions)
	}
	input, err := normalizeOutcomeQualityInput(request.Input)
	if err != nil {
		return OutcomeQualityInput{}, fmt.Errorf("normalize legacy input: %w", err)
	}
	digest := outcomeQualityInputDigest(input)
	if request.InputSHA256 != digest || request.Scenario.InputSHA256 != digest {
		return OutcomeQualityInput{}, fmt.Errorf("legacy input digest=%q does not match request/scenario", digest)
	}
	return input, nil
}

func validateOutcomeQualityProviderTarget(binding, request OutcomeQualityTarget) error {
	if err := validateOutcomeQualityLegacyTarget("legacy binding", binding); err != nil {
		return err
	}
	if err := validateOutcomeQualityLegacyTarget("legacy request", request); err != nil {
		return err
	}
	if !outcomeQualityTargetsEqual(binding, request) {
		return errors.New("legacy binding and request target must match")
	}
	return nil
}

func validateOutcomeQualityLegacyTarget(name string, target OutcomeQualityTarget) error {
	if err := validateOutcomeQualityTarget(name, target); err != nil {
		return err
	}
	if target.SourceHead != OutcomeQualityLegacySourceHead {
		return fmt.Errorf("%s source_head=%q is not the allowlisted exact v3 baseline %q", name, target.SourceHead, OutcomeQualityLegacySourceHead)
	}
	return nil
}

func validateOutcomeQualityLegacyCommand(raw, sourceWorkspace string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", errors.New("outcome-quality legacy command is required")
	}
	if !filepath.IsAbs(raw) {
		return "", errors.New("outcome-quality legacy command must be an absolute path")
	}
	path, err := filepath.Abs(raw)
	if err != nil {
		return "", fmt.Errorf("resolve outcome-quality legacy command: %w", err)
	}
	info, err := os.Lstat(path)
	if err != nil {
		return "", fmt.Errorf("stat outcome-quality legacy command: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return "", errors.New("outcome-quality legacy command must not be a symlink")
	}
	if !info.Mode().IsRegular() {
		return "", errors.New("outcome-quality legacy command is not a regular file")
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o111 == 0 {
		return "", errors.New("outcome-quality legacy command is not executable")
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", fmt.Errorf("resolve outcome-quality legacy command: %w", err)
	}
	if outcomeQualityPathWithin(filepath.Clean(resolved), sourceWorkspace) {
		return "", errors.New("outcome-quality legacy command must be outside source workspace")
	}
	base := strings.TrimSuffix(strings.ToLower(filepath.Base(resolved)), ".exe")
	if base == "outcome-quality-worker" {
		return "", errors.New("outcome-quality legacy executor cannot use the v4 outcome-quality worker")
	}
	return filepath.Clean(resolved), nil
}

func validateOutcomeQualityLegacyExecutable(executor *OutcomeQualityLegacyProcessExecutor, sourceWorkspace string) (string, error) {
	if executor == nil {
		return "", errors.New("outcome-quality legacy executor is nil")
	}
	if executor.commandPath == "" || executor.commandSize <= 0 {
		return "", errors.New("outcome-quality legacy executable identity is unavailable; use BuildOutcomeQualityLegacy")
	}
	commandPath, err := validateOutcomeQualityLegacyCommand(executor.Command, sourceWorkspace)
	if err != nil {
		return "", err
	}
	if !outcomeQualityLegacyPathsEqual(commandPath, executor.commandPath) {
		return "", errors.New("outcome-quality legacy command path changed after build")
	}
	digest, size, err := hashOutcomeQualityLegacyCommand(commandPath)
	if err != nil {
		return "", err
	}
	if size != executor.commandSize || digest != executor.commandDigest {
		return "", errors.New("outcome-quality legacy command bytes changed after build")
	}
	return commandPath, nil
}

func hashOutcomeQualityLegacyCommand(path string) ([sha256.Size]byte, int64, error) {
	var digest [sha256.Size]byte
	file, err := os.Open(path)
	if err != nil {
		return digest, 0, fmt.Errorf("open outcome-quality legacy command: %w", err)
	}
	info, statErr := file.Stat()
	if statErr != nil {
		_ = file.Close()
		return digest, 0, fmt.Errorf("stat outcome-quality legacy command: %w", statErr)
	}
	if !info.Mode().IsRegular() {
		_ = file.Close()
		return digest, 0, errors.New("outcome-quality legacy command is not a regular file")
	}
	if info.Size() <= 0 || info.Size() > maxOutcomeQualityLegacyBinaryBytes {
		_ = file.Close()
		return digest, 0, fmt.Errorf("outcome-quality legacy command size=%d outside 1..%d", info.Size(), maxOutcomeQualityLegacyBinaryBytes)
	}
	hasher := sha256.New()
	size, copyErr := io.Copy(hasher, io.LimitReader(file, maxOutcomeQualityLegacyBinaryBytes+1))
	closeErr := file.Close()
	if copyErr != nil {
		return digest, 0, fmt.Errorf("read outcome-quality legacy command: %w", copyErr)
	}
	if closeErr != nil {
		return digest, 0, fmt.Errorf("close outcome-quality legacy command: %w", closeErr)
	}
	if size != info.Size() {
		return digest, 0, errors.New("outcome-quality legacy command changed while being identified")
	}
	if size > maxOutcomeQualityLegacyBinaryBytes {
		return digest, 0, fmt.Errorf("outcome-quality legacy command exceeds %d bytes", maxOutcomeQualityLegacyBinaryBytes)
	}
	copy(digest[:], hasher.Sum(nil))
	return digest, size, nil
}

func validateOutcomeQualityLegacyToolchain(ctx context.Context, command, expectedVersion string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	command = strings.TrimSpace(command)
	if command == "" {
		return errors.New("Go toolchain command is required")
	}
	expectedVersion = strings.TrimSpace(expectedVersion)
	if expectedVersion == "" {
		return errors.New("expected Go toolchain version is required")
	}
	output, err := exec.CommandContext(ctx, command, "version").Output()
	if err != nil {
		return fmt.Errorf("read Go toolchain version: %w", err)
	}
	fields := strings.Fields(string(output))
	if len(fields) < 3 || fields[0] != "go" || fields[1] != "version" {
		return fmt.Errorf("unexpected Go toolchain version output %q", strings.TrimSpace(string(output)))
	}
	if fields[2] != expectedVersion {
		return fmt.Errorf("Go toolchain version=%q does not match declared %q", fields[2], expectedVersion)
	}
	return nil
}

func outcomeQualityLegacyPathsEqual(left, right string) bool {
	if runtime.GOOS == "windows" {
		return strings.EqualFold(filepath.Clean(left), filepath.Clean(right))
	}
	return filepath.Clean(left) == filepath.Clean(right)
}

func outcomeQualityLegacyTempParent(raw, sourceWorkspace string) (string, error) {
	parent := strings.TrimSpace(raw)
	if parent == "" {
		parent = os.TempDir()
	}
	if !filepath.IsAbs(parent) {
		return "", errors.New("temp parent must be an absolute path")
	}
	parent, err := filepath.Abs(parent)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(parent)
	if err != nil {
		return "", fmt.Errorf("temp parent is unavailable: %w", err)
	}
	if !info.IsDir() {
		return "", errors.New("temp parent is not a directory")
	}
	resolved, err := filepath.EvalSymlinks(parent)
	if err != nil {
		return "", fmt.Errorf("resolve temp parent: %w", err)
	}
	resolved = filepath.Clean(resolved)
	if outcomeQualityPathWithin(resolved, sourceWorkspace) {
		return "", errors.New("temp parent must be outside source workspace")
	}
	return resolved, nil
}

func normalizeOutcomeQualityLegacyProviderURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", errors.New("provider URL is required")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("provider URL is invalid: %w", err)
	}
	if u.Scheme != "http" {
		return "", errors.New("provider URL must use http")
	}
	if u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", errors.New("provider URL must be a credential-free loopback origin")
	}
	if strings.HasSuffix(u.Host, ":") {
		return "", errors.New("provider URL port is invalid")
	}
	host := u.Hostname()
	if host == "" {
		return "", errors.New("provider URL host is required")
	}
	if !strings.EqualFold(host, "localhost") {
		ip := net.ParseIP(host)
		if ip == nil || !ip.IsLoopback() {
			return "", fmt.Errorf("provider URL host %q is not loopback", host)
		}
	}
	if port := u.Port(); port != "" {
		value, err := strconv.Atoi(port)
		if err != nil || value < 1 || value > 65535 {
			return "", errors.New("provider URL port is invalid")
		}
	}
	return strings.TrimRight(u.String(), "/"), nil
}

func normalizeOutcomeQualityLegacyModel(raw string) (string, error) {
	model := strings.TrimSpace(raw)
	if model == "" {
		model = defaultOutcomeQualityLegacyModel
	}
	if len(model) > MaxOutcomeQualityTextBytes || strings.ContainsAny(model, "\x00\r\n") {
		return "", errors.New("model is invalid")
	}
	return model, nil
}

func outcomeQualityLegacyBuildEnvironment(cacheDir, moduleCacheDir string) []string {
	env := procenv.Sanitized()
	return outcomeQualityOverrideEnvironment(env, map[string]string{
		"GOCACHE":     cacheDir,
		"GOMODCACHE":  moduleCacheDir,
		"GOPATH":      filepath.Join(filepath.Dir(cacheDir), "go-path"),
		"GOTOOLCHAIN": "local",
		"GOWORK":      "off",
	})
}

func removeOutcomeQualityLegacyDir(root string) error {
	root = strings.TrimSpace(root)
	if root == "" {
		return nil
	}
	walkErr := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return nil
		}
		return os.Chmod(path, 0o700)
	})
	removeErr := os.RemoveAll(root)
	if errors.Is(walkErr, os.ErrNotExist) {
		walkErr = nil
	}
	if walkErr != nil && removeErr != nil {
		return errors.Join(walkErr, removeErr)
	}
	if walkErr != nil {
		return walkErr
	}
	return removeErr
}

func outcomeQualityLegacyEnvironment(homeDir, tempDir, cacheDir, providerURL, model string) []string {
	allowed := map[string]struct{}{
		"PATH": {}, "LANG": {}, "LC_ALL": {}, "LC_CTYPE": {}, "TZ": {},
		"SYSTEMROOT": {}, "WINDIR": {},
	}
	env := make([]string, 0, len(allowed)+12)
	for _, entry := range procenv.Sanitized() {
		key, _, ok := strings.Cut(entry, "=")
		if !ok {
			continue
		}
		if _, ok := allowed[strings.ToUpper(key)]; ok {
			env = append(env, entry)
		}
	}
	return outcomeQualityOverrideEnvironment(env, map[string]string{
		"HOME":                                  homeDir,
		"USERPROFILE":                           homeDir,
		"TMPDIR":                                tempDir,
		"TMP":                                   tempDir,
		"TEMP":                                  tempDir,
		"GOCACHE":                               cacheDir,
		"PICOGENT_API_KEY":                      outcomeQualityLegacyLocalAPIKey,
		"PICOGENT_HOME":                         homeDir,
		"PICOGENT_PROVIDER":                     "openai",
		"PICOGENT_BASE_URL":                     providerURL,
		"PICOGENT_MODEL":                        model,
		"PICOGENT_ROUTER":                       "0",
		"PICOGENT_MODE":                         "fast",
		"PICOGENT_OUTCOME_QUALITY_LEGACY_CHILD": "1",
	})
}

type outcomeQualityLegacyBudgetProxy struct {
	server *httptest.Server
	target *url.URL
	client *http.Client
	policy OutcomeQualityPolicy

	mu         sync.Mutex
	modelCalls int
	toolCalls  int
	tokens     int
}

func newOutcomeQualityLegacyBudgetProxy(providerURL string, policy OutcomeQualityPolicy) (*outcomeQualityLegacyBudgetProxy, error) {
	providerURL, err := normalizeOutcomeQualityLegacyProviderURL(providerURL)
	if err != nil {
		return nil, fmt.Errorf("normalize provider URL: %w", err)
	}
	if err := validateOutcomeQualityPolicy(policy); err != nil {
		return nil, fmt.Errorf("validate provider policy: %w", err)
	}
	target, err := url.Parse(providerURL)
	if err != nil {
		return nil, fmt.Errorf("parse provider URL: %w", err)
	}
	proxy := &outcomeQualityLegacyBudgetProxy{
		target: target,
		client: &http.Client{
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
		policy: policy,
	}
	proxy.server = httptest.NewServer(http.HandlerFunc(proxy.handle))
	return proxy, nil
}

func (p *outcomeQualityLegacyBudgetProxy) URL() string {
	if p == nil || p.server == nil {
		return ""
	}
	return p.server.URL
}

func (p *outcomeQualityLegacyBudgetProxy) Close() {
	if p == nil || p.server == nil {
		return
	}
	p.server.Close()
}

func (p *outcomeQualityLegacyBudgetProxy) Metrics() (tokens, modelCalls, toolCalls int) {
	if p == nil {
		return 0, 0, 0
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.tokens, p.modelCalls, p.toolCalls
}

func (p *outcomeQualityLegacyBudgetProxy) handle(w http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost || request.URL.Path != "/chat/completions" || request.URL.RawQuery != "" {
		http.Error(w, "legacy provider proxy only accepts POST /chat/completions", http.StatusNotFound)
		return
	}
	body, err := io.ReadAll(io.LimitReader(request.Body, maxOutcomeQualityLegacyProviderPayloadBytes+1))
	if err != nil {
		http.Error(w, "could not read legacy provider request", http.StatusBadRequest)
		return
	}
	if len(body) > maxOutcomeQualityLegacyProviderPayloadBytes {
		http.Error(w, "legacy provider request is too large", http.StatusRequestEntityTooLarge)
		return
	}
	modelLimit := p.policy.MaxModelCalls
	if p.policy.MaxTurns < modelLimit {
		modelLimit = p.policy.MaxTurns
	}
	p.mu.Lock()
	if p.modelCalls >= modelLimit {
		p.mu.Unlock()
		http.Error(w, "legacy provider model-call budget exceeded", http.StatusTooManyRequests)
		return
	}
	p.modelCalls++
	p.mu.Unlock()

	endpoint := *p.target
	endpoint.Path = strings.TrimRight(p.target.Path, "/") + "/chat/completions"
	endpoint.RawPath = ""
	endpoint.RawQuery = ""
	upstream, err := http.NewRequestWithContext(request.Context(), http.MethodPost, endpoint.String(), bytes.NewReader(body))
	if err != nil {
		http.Error(w, "could not create legacy provider request", http.StatusBadGateway)
		return
	}
	for _, header := range []string{"Accept", "Authorization", "Content-Type"} {
		for _, value := range request.Header.Values(header) {
			upstream.Header.Add(header, value)
		}
	}
	response, err := p.client.Do(upstream)
	if err != nil {
		http.Error(w, "legacy provider request failed", http.StatusBadGateway)
		return
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, maxOutcomeQualityLegacyProviderPayloadBytes+1))
	if err != nil {
		http.Error(w, "could not read legacy provider response", http.StatusBadGateway)
		return
	}
	if len(responseBody) > maxOutcomeQualityLegacyProviderPayloadBytes {
		http.Error(w, "legacy provider response is too large", http.StatusBadGateway)
		return
	}
	if response.StatusCode >= http.StatusMultipleChoices && response.StatusCode < http.StatusBadRequest {
		http.Error(w, "legacy provider redirects are not allowed", http.StatusBadGateway)
		return
	}
	if response.StatusCode >= http.StatusBadRequest {
		writeOutcomeQualityLegacyProviderResponse(w, response.StatusCode, response.Header, responseBody)
		return
	}

	usage, toolCalls, err := outcomeQualityLegacyProviderUsage(responseBody)
	if err != nil {
		http.Error(w, "legacy provider response cannot prove benchmark budgets: "+err.Error(), http.StatusBadGateway)
		return
	}
	p.mu.Lock()
	if usage > p.policy.MaxTokens {
		p.mu.Unlock()
		http.Error(w, "legacy provider token budget exceeded", http.StatusTooManyRequests)
		return
	}
	if p.tokens+usage > p.policy.MaxTokens {
		p.mu.Unlock()
		http.Error(w, "legacy provider token budget exceeded", http.StatusTooManyRequests)
		return
	}
	if p.toolCalls+toolCalls > p.policy.MaxToolCalls {
		p.mu.Unlock()
		http.Error(w, "legacy provider tool-call budget exceeded", http.StatusTooManyRequests)
		return
	}
	p.tokens += usage
	p.toolCalls += toolCalls
	p.mu.Unlock()
	writeOutcomeQualityLegacyProviderResponse(w, response.StatusCode, response.Header, responseBody)
}

func outcomeQualityLegacyProviderUsage(payload []byte) (int, int, error) {
	var response struct {
		Choices []struct {
			Message struct {
				ToolCalls []json.RawMessage `json:"tool_calls"`
			} `json:"message"`
		} `json:"choices"`
		Usage *struct {
			PromptTokens     *int `json:"prompt_tokens"`
			CompletionTokens *int `json:"completion_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(payload, &response); err != nil {
		return 0, 0, fmt.Errorf("invalid JSON")
	}
	if response.Usage == nil || response.Usage.PromptTokens == nil || response.Usage.CompletionTokens == nil {
		return 0, 0, errors.New("usage is missing prompt_tokens or completion_tokens")
	}
	if *response.Usage.PromptTokens < 0 || *response.Usage.CompletionTokens < 0 {
		return 0, 0, errors.New("usage contains a negative token count")
	}
	maxInt := int(^uint(0) >> 1)
	if *response.Usage.PromptTokens > maxInt-*response.Usage.CompletionTokens {
		return 0, 0, errors.New("usage token count overflows integer range")
	}
	usage := *response.Usage.PromptTokens + *response.Usage.CompletionTokens
	toolCalls := 0
	for _, choice := range response.Choices {
		toolCalls += len(choice.Message.ToolCalls)
	}
	return usage, toolCalls, nil
}

func writeOutcomeQualityLegacyProviderResponse(w http.ResponseWriter, status int, headers http.Header, body []byte) {
	for key, values := range headers {
		if strings.EqualFold(key, "Content-Length") {
			continue
		}
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

func outcomeQualityOverrideEnvironment(env []string, overrides map[string]string) []string {
	keys := make([]string, 0, len(overrides))
	for key := range overrides {
		keys = append(keys, strings.ToUpper(key))
	}
	sort.Strings(keys)
	values := make(map[string]string, len(overrides))
	for key, value := range overrides {
		values[strings.ToUpper(key)] = value
	}
	out := make([]string, 0, len(env)+len(values))
	seen := make(map[string]struct{}, len(env)+len(values))
	for _, entry := range env {
		key, _, ok := strings.Cut(entry, "=")
		if !ok {
			continue
		}
		upper := strings.ToUpper(key)
		if value, overridden := values[upper]; overridden {
			if _, already := seen[upper]; already {
				continue
			}
			out = append(out, key+"="+value)
			seen[upper] = struct{}{}
			continue
		}
		if _, already := seen[upper]; already {
			continue
		}
		out = append(out, entry)
		seen[upper] = struct{}{}
	}
	for _, key := range keys {
		if _, already := seen[key]; already {
			continue
		}
		out = append(out, key+"="+values[key])
		seen[key] = struct{}{}
	}
	return out
}

func inspectOutcomeQualityLegacyWorkspace(ctx context.Context, root string, input OutcomeQualityInput, expected map[string]string) (bool, map[string]string, []string, error) {
	actual := make(map[string]string, len(input.Files))
	changed := make([]string, 0, len(input.Files))
	known := make(map[string]struct{}, len(input.Files))
	correct := true
	for _, file := range input.Files {
		if err := ctx.Err(); err != nil {
			return false, actual, nil, err
		}
		known[file.Path] = struct{}{}
		opened, err := workspace.OpenRead(root, file.Path)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				actual[file.Path] = ""
				correct = false
				changed = append(changed, file.Path)
				continue
			}
			return false, actual, nil, err
		}
		data, readErr := io.ReadAll(io.LimitReader(opened, int64(maxOutcomeQualityFixtureBytes)+1))
		closeErr := opened.Close()
		if readErr != nil {
			return false, actual, nil, readErr
		}
		if closeErr != nil {
			return false, actual, nil, closeErr
		}
		if len(data) > maxOutcomeQualityFixtureBytes {
			return false, actual, nil, fmt.Errorf("file %q exceeds bounded fixture size", file.Path)
		}
		content := string(data)
		actual[file.Path] = content
		if content != expected[file.Path] {
			correct = false
		}
		if content != file.Content {
			changed = append(changed, file.Path)
		}
	}
	entries := 0
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if walkErr != nil {
			return walkErr
		}
		if path == root {
			return nil
		}
		entries++
		if entries > maxOutcomeQualityLegacyWorkspaceEntries {
			return fmt.Errorf("fixture contains more than %d filesystem entries", maxOutcomeQualityLegacyWorkspaceEntries)
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("fixture contains symlink %q", path)
		}
		if entry.IsDir() {
			return nil
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("fixture contains non-regular file %q", path)
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if depth := strings.Count(filepath.ToSlash(rel), "/") + 1; depth > maxOutcomeQualityLegacyWorkspaceDepth {
			return fmt.Errorf("fixture path %q exceeds depth %d", rel, maxOutcomeQualityLegacyWorkspaceDepth)
		}
		rel, err = normalizeOutcomeQualityFixturePath(rel)
		if err != nil {
			return err
		}
		if _, ok := known[rel]; ok {
			return nil
		}
		correct = false
		changed = append(changed, rel)
		return nil
	})
	if err != nil {
		return false, actual, nil, err
	}
	sort.Strings(changed)
	changed = uniqueOutcomeQualityStrings(changed)
	return correct, actual, changed, nil
}

func outcomeQualityLegacyVerificationStatus(stderr string) OutcomeQualityVerification {
	status := OutcomeVerificationUnverified
	verifyResultPending := false
	for _, line := range strings.Split(stderr, "\n") {
		if strings.HasPrefix(line, "→ ") {
			fields := strings.Fields(strings.TrimPrefix(line, "→ "))
			verifyResultPending = len(fields) > 0 && strings.EqualFold(fields[0], "verify")
			continue
		}
		if !verifyResultPending {
			continue
		}
		if !strings.HasPrefix(line, "  ") {
			verifyResultPending = false
			continue
		}
		verifyResultPending = false
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) < 2 || !strings.EqualFold(fields[0], "verify") {
			continue
		}
		switch strings.ToUpper(strings.Trim(fields[1], " \t:(),[]")) {
		case "PASS":
			status = OutcomeVerificationPass
		case "FAIL":
			status = OutcomeVerificationFail
		case "INCONCLUSIVE":
			status = OutcomeVerificationInconclusive
		case "SKIPPED":
			status = OutcomeVerificationSkipped
		}
	}
	return status
}

type outcomeQualityLegacyBuffer struct {
	mu        sync.Mutex
	data      bytes.Buffer
	truncated bool
}

func (b *outcomeQualityLegacyBuffer) Write(data []byte) (int, error) {
	if b == nil {
		return len(data), nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	remaining := maxOutcomeQualityLegacyOutputBytes - b.data.Len()
	if remaining <= 0 {
		b.truncated = true
		return len(data), nil
	}
	if len(data) > remaining {
		_, _ = b.data.Write(data[:remaining])
		b.truncated = true
		return len(data), nil
	}
	_, _ = b.data.Write(data)
	return len(data), nil
}

func (b *outcomeQualityLegacyBuffer) String() string {
	if b == nil {
		return ""
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.data.String()
}

func uniqueOutcomeQualityStrings(values []string) []string {
	if len(values) < 2 {
		return values
	}
	out := values[:0]
	for _, value := range values {
		if len(out) == 0 || out[len(out)-1] != value {
			out = append(out, value)
		}
	}
	return out
}
