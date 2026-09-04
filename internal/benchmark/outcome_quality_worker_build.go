package benchmark

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

const maxOutcomeQualityWorkerBuildTimeout = 2 * time.Minute

// OutcomeQualityWorkerBuild owns a worker binary built from one validated
// source workspace. The binary is deliberately outside that workspace so the
// build cannot make the source tree dirty. Call Close when the matrix is done.
type OutcomeQualityWorkerBuild struct {
	executor *OutcomeQualityProcessExecutor
	dir      string
}

// BuildOutcomeQualityWorker validates the exact source tree, builds the
// versioned worker package from that tree, and returns a process executor for
// subsequent bounded requests. The build uses typed arguments and never a
// shell. A target that predates the worker package returns an explicit build
// error and must remain unverified by the caller.
func BuildOutcomeQualityWorker(ctx context.Context, binding OutcomeQualitySourceBinding, tempParent string) (*OutcomeQualityWorkerBuild, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	workspace, err := canonicalOutcomeQualityWorkspace(binding.Workspace)
	if err != nil {
		return nil, fmt.Errorf("outcome-quality worker build workspace: %w", err)
	}
	if err := validateOutcomeQualitySourceBinding(ctx, "worker build", binding.Target, workspace); err != nil {
		return nil, err
	}
	buildTempParent := strings.TrimSpace(tempParent)
	if buildTempParent != "" {
		parent, err := filepath.Abs(buildTempParent)
		if err != nil {
			return nil, fmt.Errorf("outcome-quality worker build temp parent: %w", err)
		}
		if resolved, resolveErr := filepath.EvalSymlinks(parent); resolveErr == nil {
			parent = resolved
		}
		if outcomeQualityPathWithin(filepath.Clean(parent), workspace) {
			return nil, fmt.Errorf("outcome-quality worker build temp parent must be outside source workspace")
		}
		buildTempParent = parent
	}

	buildDir, err := os.MkdirTemp(buildTempParent, "picogent-outcome-quality-worker-")
	if err != nil {
		return nil, fmt.Errorf("create outcome-quality worker build directory: %w", err)
	}
	removeBuildDir := func() { _ = os.RemoveAll(buildDir) }
	canonicalBuildDir := buildDir
	if resolved, resolveErr := filepath.EvalSymlinks(buildDir); resolveErr == nil {
		canonicalBuildDir = resolved
	}
	if outcomeQualityPathWithin(canonicalBuildDir, workspace) {
		removeBuildDir()
		return nil, fmt.Errorf("outcome-quality worker build directory is inside source workspace")
	}

	goCommand, err := exec.LookPath("go")
	if err != nil {
		removeBuildDir()
		return nil, fmt.Errorf("find Go toolchain for outcome-quality worker build: %w", err)
	}
	binaryName := "outcome-quality-worker"
	if runtime.GOOS == "windows" {
		binaryName += ".exe"
	}
	binaryPath := filepath.Join(buildDir, binaryName)
	buildCtx, cancel := context.WithTimeout(ctx, maxOutcomeQualityWorkerBuildTimeout)
	defer cancel()
	command := exec.CommandContext(buildCtx, goCommand, "build", "-o", binaryPath, "./cmd/outcome-quality-worker")
	command.Dir = workspace
	command.Env = outcomeQualityWorkerEnvironment()
	var output outcomeQualityBuildBuffer
	command.Stdout = &output
	command.Stderr = &output
	configureOutcomeQualityWorkerCommand(command)
	if err := runOutcomeQualityWorkerCommand(buildCtx, command); err != nil {
		removeBuildDir()
		detail := strings.TrimSpace(output.String())
		if detail != "" {
			return nil, fmt.Errorf("build outcome-quality worker: %w: %s", err, detail)
		}
		return nil, fmt.Errorf("build outcome-quality worker: %w", err)
	}
	if err := buildCtx.Err(); err != nil {
		removeBuildDir()
		return nil, fmt.Errorf("build outcome-quality worker: %w", err)
	}
	info, err := os.Stat(binaryPath)
	if err != nil {
		removeBuildDir()
		return nil, fmt.Errorf("stat built outcome-quality worker: %w", err)
	}
	if !info.Mode().IsRegular() {
		removeBuildDir()
		return nil, fmt.Errorf("built outcome-quality worker is not a regular file")
	}

	return &OutcomeQualityWorkerBuild{
		executor: &OutcomeQualityProcessExecutor{
			Command: binaryPath,
			Binding: OutcomeQualitySourceBinding{Target: binding.Target, Workspace: workspace},
		},
		dir: buildDir,
	}, nil
}

// ProcessExecutor returns the bounded executor backed by the built worker.
func (b *OutcomeQualityWorkerBuild) ProcessExecutor() *OutcomeQualityProcessExecutor {
	if b == nil {
		return nil
	}
	return b.executor
}

// Close removes only the temporary directory created by BuildOutcomeQualityWorker.
func (b *OutcomeQualityWorkerBuild) Close() error {
	if b == nil || b.dir == "" {
		return nil
	}
	dir := b.dir
	b.dir = ""
	return os.RemoveAll(dir)
}

func outcomeQualityPathWithin(path, root string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

type outcomeQualityBuildBuffer struct {
	mu        sync.Mutex
	data      bytes.Buffer
	truncated bool
}

func (b *outcomeQualityBuildBuffer) Write(data []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	remaining := maxOutcomeQualityWorkerResponseBytes - b.data.Len()
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

func (b *outcomeQualityBuildBuffer) String() string {
	if b == nil {
		return ""
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.truncated {
		return string(append(append([]byte(nil), b.data.Bytes()...), []byte(" [output truncated]")...))
	}
	return b.data.String()
}

var _ io.Writer = (*outcomeQualityBuildBuffer)(nil)
