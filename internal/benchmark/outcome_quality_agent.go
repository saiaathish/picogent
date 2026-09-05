package benchmark

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/saiaathish/picogent/internal/agent"
	"github.com/saiaathish/picogent/internal/config"
	"github.com/saiaathish/picogent/internal/llm"
	"github.com/saiaathish/picogent/internal/perm"
	"github.com/saiaathish/picogent/internal/taskstate"
	"github.com/saiaathish/picogent/internal/tools"
	"github.com/saiaathish/picogent/internal/workspace"
)

// OutcomeQualityAgentExecutor is the M-lane's provider-independent executor.
// It deliberately runs a real Agent against an ephemeral fixture rather than
// interpreting tool calls in the matrix runner. The fixture provider is
// deterministic and local; it is not evidence about a live model or an
// arbitrary repository.
type OutcomeQualityAgentExecutor struct {
	// TempParent optionally selects the parent directory for ephemeral
	// workspaces. The executor removes only the directories it creates.
	TempParent string
}

// NewOutcomeQualityAgentExecutor creates the bounded scripted executor used
// by focused M-lane tests and local benchmark runs.
func NewOutcomeQualityAgentExecutor() *OutcomeQualityAgentExecutor {
	return &OutcomeQualityAgentExecutor{}
}

// Execute runs one scenario/variant/repetition with the same input contract
// received by both source-head variants. The fixture is copied into a fresh
// workspace, and the task store is intentionally kept outside that workspace
// so the benchmark observes the existing taskstate seam without introducing a
// benchmark-owned persistence format.
func (e *OutcomeQualityAgentExecutor) Execute(ctx context.Context, request OutcomeQualityExecutionRequest) (OutcomeQualityExecution, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := validateOutcomeQualityAgentRequest(request); err != nil {
		return OutcomeQualityExecution{}, err
	}
	input, err := normalizeOutcomeQualityInput(request.Input)
	if err != nil {
		return OutcomeQualityExecution{}, fmt.Errorf("normalize scripted input: %w", err)
	}
	if digest := outcomeQualityInputDigest(input); digest != request.InputSHA256 {
		return OutcomeQualityExecution{}, fmt.Errorf("input digest=%q does not match request input_sha256=%q", digest, request.InputSHA256)
	}

	workspaceRoot, err := os.MkdirTemp(e.tempParent(), "picogent-outcome-quality-")
	if err != nil {
		return OutcomeQualityExecution{}, fmt.Errorf("create fixture workspace: %w", err)
	}
	defer os.RemoveAll(workspaceRoot)
	storeRoot, err := os.MkdirTemp(e.tempParent(), "picogent-outcome-quality-task-")
	if err != nil {
		return OutcomeQualityExecution{}, fmt.Errorf("create fixture task store: %w", err)
	}
	defer os.RemoveAll(storeRoot)

	if err := writeOutcomeQualityFixture(ctx, workspaceRoot, input); err != nil {
		return OutcomeQualityExecution{}, err
	}
	fixturePaths := outcomeQualityInputPaths(input)
	before, err := workspace.Capture(ctx, workspaceRoot, fixturePaths)
	if err != nil {
		return OutcomeQualityExecution{}, fmt.Errorf("capture fixture before agent run: %w", err)
	}
	if comparison := workspace.Compare(before, before); !comparison.Fresh {
		return OutcomeQualityExecution{}, fmt.Errorf("fixture before-agent observation is not fresh: %s", comparison.Reason)
	}

	expected := outcomeQualityExpectedContents(request.Scenario, input)
	verifyCallback := func(verifyCtx context.Context, targets []string) (string, error) {
		return verifyOutcomeQualityFixture(verifyCtx, workspaceRoot, targets, input, expected)
	}
	script, err := outcomeQualityScript(input, expected)
	if err != nil {
		return OutcomeQualityExecution{}, err
	}
	client := &outcomeQualityCountingClient{scripted: &llm.Scripted{Responses: script}}
	handler := &outcomeQualityAgentHandler{}
	registry := tools.NewRegistry(tools.Context{
		Workspace:     workspaceRoot,
		VerifyTargets: verifyCallback,
	})
	gate := perm.New(config.ModeFast, workspaceRoot, nil)
	cfg := config.Default()
	cfg.Workspace = workspaceRoot
	cfg.Mode = config.ModeFast
	cfg.Provider = config.ProviderOllama
	cfg.Model = "scripted-outcome-quality"
	cfg.MaxToolRounds = request.Policy.MaxTurns
	if cfg.MaxToolRounds <= 0 {
		return OutcomeQualityExecution{}, errors.New("scripted executor requires a positive max_turns policy")
	}

	a := agent.New(cfg, client, registry, gate)
	defer a.Close()
	a.SetTaskStore(taskstate.NewStore(storeRoot))
	sessionID := fmt.Sprintf("outcome-%s-%s-%d", request.Scenario.ID, request.Variant, request.Repetition)
	if err := a.SetTaskSession(sessionID); err != nil {
		return OutcomeQualityExecution{}, fmt.Errorf("attach scripted task session: %w", err)
	}
	_, result, runErr := a.RunWithOptions(ctx, nil, llm.Message{
		Role:    "user",
		Content: input.Prompt,
	}, handler, agent.RunOptions{
		// Scenario labels are deliberately part of the shared input, but words
		// such as "security" or "performance" must not silently add a
		// taskstate quality requirement to this seam-level fixture. The fixture
		// still exercises the real user prompt while its durable task contract
		// remains the same for every catalog entry.
		DurablePrompt: "Fix the deterministic fixture and verify the result",
		TracePrompt:   input.Prompt,
	})

	after, captureErr := workspace.Capture(ctx, workspaceRoot, fixturePaths)
	if captureErr != nil {
		if runErr != nil {
			return OutcomeQualityExecution{}, errors.Join(runErr, fmt.Errorf("capture fixture after agent run: %w", captureErr))
		}
		return OutcomeQualityExecution{}, fmt.Errorf("capture fixture after agent run: %w", captureErr)
	}
	if comparison := workspace.Compare(after, after); !comparison.Fresh {
		return OutcomeQualityExecution{}, fmt.Errorf("fixture after-agent observation is not fresh: %s", comparison.Reason)
	}

	metrics, reasons := outcomeQualityAgentMetrics(workspaceRoot, input, expected, before, after, result, client, handler, runErr)
	return OutcomeQualityExecution{Metrics: metrics, Unverified: reasons}, runErr
}

func (e *OutcomeQualityAgentExecutor) tempParent() string {
	if e == nil {
		return ""
	}
	return strings.TrimSpace(e.TempParent)
}

func validateOutcomeQualityAgentRequest(request OutcomeQualityExecutionRequest) error {
	if !request.Variant.valid() {
		return fmt.Errorf("unknown scripted variant %q", request.Variant)
	}
	if request.Repetition < 1 {
		return fmt.Errorf("scripted repetition=%d is invalid", request.Repetition)
	}
	if !validSHA(request.Target.SourceHead) {
		return errors.New("scripted target source_head must be a full 40-character commit SHA")
	}
	if strings.TrimSpace(request.Scenario.ID) == "" {
		return errors.New("scripted scenario id is required")
	}
	if err := validateOutcomeQualityPolicy(request.Policy); err != nil {
		return fmt.Errorf("scripted policy: %w", err)
	}
	return nil
}

func writeOutcomeQualityFixture(ctx context.Context, root string, input OutcomeQualityInput) error {
	for _, file := range input.Files {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("write fixture %q: %w", file.Path, err)
		}
		if err := workspace.WriteAtomic(root, file.Path, []byte(file.Content)); err != nil {
			return fmt.Errorf("write fixture %q: %w", file.Path, err)
		}
	}
	return nil
}

func outcomeQualityInputPaths(input OutcomeQualityInput) []string {
	paths := make([]string, 0, len(input.Files))
	for _, file := range input.Files {
		paths = append(paths, file.Path)
	}
	sort.Strings(paths)
	return paths
}

func outcomeQualityExpectedContents(scenario OutcomeQualityScenario, input OutcomeQualityInput) map[string]string {
	expectedPaths := make(map[string]struct{}, len(input.ExpectedChangedPaths))
	for _, path := range input.ExpectedChangedPaths {
		expectedPaths[path] = struct{}{}
	}
	expected := make(map[string]string, len(input.Files))
	for _, file := range input.Files {
		content := file.Content
		if _, shouldChange := expectedPaths[file.Path]; shouldChange {
			content = fmt.Sprintf("after %s seed=%d\n", scenario.ID, scenario.Seed)
		}
		expected[file.Path] = content
	}
	return expected
}

func outcomeQualityScript(input OutcomeQualityInput, expected map[string]string) ([]llm.ChatResponse, error) {
	responses := make([]llm.ChatResponse, 0, 4)
	readCalls := make([]llm.ToolCall, 0, len(input.Files))
	for index, file := range input.Files {
		args, err := json.Marshal(map[string]string{"path": file.Path})
		if err != nil {
			return nil, fmt.Errorf("encode scripted read %q: %w", file.Path, err)
		}
		readCalls = append(readCalls, llm.ToolCall{
			ID:        fmt.Sprintf("read-%d", index+1),
			Name:      "read_file",
			Arguments: string(args),
		})
	}
	responses = append(responses, scriptedOutcomeToolResponse(readCalls, 80, 40))

	expectedPaths := append([]string(nil), input.ExpectedChangedPaths...)
	sort.Strings(expectedPaths)
	writeCalls := make([]llm.ToolCall, 0, len(expectedPaths))
	for index, path := range expectedPaths {
		args, err := json.Marshal(map[string]string{
			"path":    path,
			"content": expected[path],
		})
		if err != nil {
			return nil, fmt.Errorf("encode scripted write %q: %w", path, err)
		}
		writeCalls = append(writeCalls, llm.ToolCall{
			ID:        fmt.Sprintf("write-%d", index+1),
			Name:      "write_file",
			Arguments: string(args),
		})
	}
	if len(writeCalls) > 0 {
		responses = append(responses, scriptedOutcomeToolResponse(writeCalls, 96, 48))
	}

	// Verification must bind every input file, not only files expected to
	// change. The taskstate proof is compared with the full after-run fixture
	// observation; targeting only changed files makes multi-file fixtures look
	// stale even when their unchanged support files were checked successfully.
	verifyPaths := outcomeQualityInputPaths(input)
	verifyArgs, err := json.Marshal(map[string]any{"targets": verifyPaths})
	if err != nil {
		return nil, fmt.Errorf("encode scripted verification: %w", err)
	}
	responses = append(responses, scriptedOutcomeToolResponse([]llm.ToolCall{{
		ID:        "verify-1",
		Name:      "verify",
		Arguments: string(verifyArgs),
	}}, 112, 56))
	responses = append(responses, llm.ChatResponse{
		Message:          llm.Message{Role: "assistant", Content: "Goal complete: the deterministic fixture is complete and verified."},
		PromptTokens:     128,
		CompletionTokens: 48,
	})
	return responses, nil
}

func scriptedOutcomeToolResponse(calls []llm.ToolCall, promptTokens, completionTokens int) llm.ChatResponse {
	return llm.ChatResponse{
		Message:          llm.Message{Role: "assistant", ToolCalls: calls},
		PromptTokens:     promptTokens,
		CompletionTokens: completionTokens,
	}
}

func verifyOutcomeQualityFixture(ctx context.Context, root string, targets []string, input OutcomeQualityInput, expected map[string]string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	// The verification observation is the complete fixture boundary. Content
	// comparison below still decides which files were expected to change.
	want := outcomeQualityInputPaths(input)
	sort.Strings(want)
	got := append([]string(nil), targets...)
	sort.Strings(got)
	if !sameOutcomeQualityStrings(got, want) {
		return fmt.Sprintf("verify FAIL\nexpected targets %v, got %v", want, got), nil
	}
	for _, file := range input.Files {
		f, err := workspace.OpenRead(root, file.Path)
		if err != nil {
			return fmt.Sprintf("verify FAIL\nread %s: %v", file.Path, err), nil
		}
		data, readErr := io.ReadAll(io.LimitReader(f, int64(maxOutcomeQualityFixtureBytes)+1))
		closeErr := f.Close()
		if readErr != nil {
			return fmt.Sprintf("verify FAIL\nread %s: %v", file.Path, readErr), nil
		}
		if closeErr != nil {
			return fmt.Sprintf("verify FAIL\nclose %s: %v", file.Path, closeErr), nil
		}
		if string(data) != expected[file.Path] {
			return fmt.Sprintf("verify FAIL\ncontent mismatch in %s", file.Path), nil
		}
	}
	return fmt.Sprintf("verify PASS\n%d fixture paths pass", len(input.Files)), nil
}

func sameOutcomeQualityStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

type outcomeQualityCountingClient struct {
	scripted             *llm.Scripted
	modelCalls           int
	tokens               int
	initialContextBytes  int64
	peakContextBytes     int64
	invalidTokenResponse error
}

func (c *outcomeQualityCountingClient) Chat(ctx context.Context, request llm.ChatRequest) (llm.ChatResponse, error) {
	if err := ctx.Err(); err != nil {
		return llm.ChatResponse{}, err
	}
	if c == nil || c.scripted == nil {
		return llm.ChatResponse{}, errors.New("scripted client is unavailable")
	}
	response, err := c.scripted.Chat(ctx, request)
	if err != nil {
		return response, err
	}
	if response.PromptTokens < 0 || response.CompletionTokens < 0 {
		c.invalidTokenResponse = errors.New("scripted response contained a negative token count")
		return llm.ChatResponse{}, c.invalidTokenResponse
	}
	c.modelCalls++
	c.tokens += response.PromptTokens + response.CompletionTokens
	requestBytes := outcomeQualityRequestBytes(request)
	if c.modelCalls == 1 {
		c.initialContextBytes = requestBytes
		c.peakContextBytes = requestBytes
	} else if requestBytes > c.peakContextBytes {
		c.peakContextBytes = requestBytes
	}
	return response, nil
}

func outcomeQualityRequestBytes(request llm.ChatRequest) int64 {
	total := int64(len(request.Model) + len(request.TaskMode) + len(request.LastToolKind))
	for _, message := range request.Messages {
		total += int64(len(message.Role) + len(message.Content) + len(message.ToolCallID) + len(message.Name))
		for _, part := range message.Parts {
			total += int64(len(part.Type) + len(part.Text) + len(part.MIME) + len(part.Name) + len(part.Data))
		}
		for _, call := range message.ToolCalls {
			total += int64(len(call.ID) + len(call.ItemID) + len(call.Name) + len(call.Arguments))
		}
	}
	return total
}

type outcomeQualityAgentHandler struct {
	agent.NopHandler
	permissionPrompts int
	toolCalls         int
	repairCount       int
	pendingRepair     bool
	errorCount        int
	lastError         string
}

func (h *outcomeQualityAgentHandler) OnNeedPermission(ctx context.Context, _ perm.Request) (perm.Decision, error) {
	if err := ctx.Err(); err != nil {
		return perm.Deny, err
	}
	h.permissionPrompts++
	return perm.Allow, nil
}

func (h *outcomeQualityAgentHandler) OnToolStart(call llm.ToolCall) {
	h.toolCalls++
	if h.pendingRepair && (call.Name == "write_file" || call.Name == "edit_file") {
		h.repairCount++
		h.pendingRepair = false
	}
}

func (h *outcomeQualityAgentHandler) OnToolEnd(call llm.ToolCall, result string, err error) {
	if call.Name == "verify" && err == nil {
		status := outcomeQualityStatus(result)
		switch status {
		case "FAIL":
			h.pendingRepair = true
		case "PASS":
			h.pendingRepair = false
		}
	}
}

func (h *outcomeQualityAgentHandler) OnError(err error) {
	h.errorCount++
	if err != nil {
		h.lastError = err.Error()
	}
}

func outcomeQualityAgentMetrics(workspaceRoot string, input OutcomeQualityInput, expected map[string]string, before, after workspace.Observation, result agent.Result, client *outcomeQualityCountingClient, handler *outcomeQualityAgentHandler, runErr error) (OutcomeQualityMetrics, []string) {
	changedPaths := normalizeOutcomeQualityChangedPaths(workspaceRoot, result.FilesChanged)
	wantChanged := append([]string(nil), input.ExpectedChangedPaths...)
	sort.Strings(wantChanged)
	correctContent, actualContents, contentErr := outcomeQualityContentsMatch(workspaceRoot, input, expected)
	changedPathsMatch := sameOutcomeQualityStrings(changedPaths, wantChanged)
	verification := outcomeQualityVerificationState(result)
	evidenceCurrent := verification == OutcomeVerificationPass && outcomeQualityCompletionEvidenceCurrent(result, after)

	metrics := OutcomeQualityMetrics{
		OutcomeSuccess:      OutcomeAssessmentInconclusive,
		Correctness:         OutcomeAssessmentInconclusive,
		UserQuestions:       handler.permissionPrompts,
		Tokens:              client.tokens,
		ModelCalls:          client.modelCalls,
		ToolCalls:           handler.toolCalls,
		ChangedLines:        outcomeQualityChangedLines(input, actualContents),
		UnnecessaryChanges:  outcomeQualityUnnecessaryChanges(changedPaths, wantChanged),
		VerificationQuality: verification,
		RepairCount:         handler.repairCount,
		ContextGrowthBytes:  outcomeQualityContextGrowth(client),
		Evidence:            EvidenceUnverified,
	}
	if evidenceCurrent {
		metrics.Evidence = EvidenceCurrent
	}

	var reasons []string
	if client.invalidTokenResponse != nil {
		reasons = append(reasons, client.invalidTokenResponse.Error())
	}
	if runErr != nil {
		reasons = append(reasons, "agent run failed: "+runErr.Error())
	}
	if handler.errorCount > 0 && runErr == nil {
		reason := fmt.Sprintf("agent emitted %d error event(s)", handler.errorCount)
		if handler.lastError != "" {
			reason += ": " + handler.lastError
		}
		reasons = append(reasons, reason)
	}
	if !workspaceObservationCurrent(before, after) {
		reasons = append(reasons, "fixture workspace observation is not current")
	}
	if contentErr != nil {
		reasons = append(reasons, "fixture content could not be checked: "+contentErr.Error())
	}
	if verification == OutcomeVerificationPass && !evidenceCurrent {
		reasons = append(reasons, "current proof unavailable: scripted task completion evidence is not ready")
	}
	if len(reasons) > 0 {
		return metrics, boundedOutcomeQualityReasons(reasons, 8)
	}

	if !correctContent || !changedPathsMatch {
		metrics.Correctness = OutcomeAssessmentFail
		metrics.OutcomeSuccess = OutcomeAssessmentFail
		return metrics, nil
	}
	if verification != OutcomeVerificationPass || !evidenceCurrent {
		detail := fmt.Sprintf("verification=%s verified=%q task=%t completion_ready=%t", verification, result.Verified, result.Task != nil, result.Completion.Ready)
		if result.Task != nil {
			proof := result.Task.CompletionCheck()
			detail += fmt.Sprintf(" proof=%#v change_seq=%d verified_change_seq=%d verifications=%d", proof, result.Task.ChangeSeq, result.Task.VerifiedChangeSeq, len(result.Task.Verification))
		}
		return metrics, []string{"scripted agent did not produce current passing verification evidence: " + detail}
	}
	if !result.GoalDone || !result.Completion.Ready {
		return metrics, []string{"scripted agent did not satisfy the durable completion projection"}
	}
	metrics.Correctness = OutcomeAssessmentPass
	metrics.OutcomeSuccess = OutcomeAssessmentPass
	return metrics, nil
}

func normalizeOutcomeQualityChangedPaths(root string, paths []string) []string {
	result := make([]string, 0, len(paths))
	seen := make(map[string]struct{}, len(paths))
	for _, raw := range paths {
		path := filepath.ToSlash(filepath.Clean(strings.TrimSpace(raw)))
		if path == "." || path == "" {
			continue
		}
		if filepath.IsAbs(path) {
			rel, err := filepath.Rel(root, path)
			if err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				path = filepath.ToSlash(filepath.Clean(rel))
			} else {
				path = filepath.ToSlash(path)
			}
		}
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		result = append(result, path)
	}
	sort.Strings(result)
	return result
}

func outcomeQualityContentsMatch(root string, input OutcomeQualityInput, expected map[string]string) (bool, map[string]string, error) {
	actual := make(map[string]string, len(input.Files))
	for _, file := range input.Files {
		opened, err := workspace.OpenRead(root, file.Path)
		if err != nil {
			return false, actual, err
		}
		data, readErr := io.ReadAll(io.LimitReader(opened, int64(maxOutcomeQualityFixtureBytes)+1))
		closeErr := opened.Close()
		if readErr != nil {
			return false, actual, readErr
		}
		if closeErr != nil {
			return false, actual, closeErr
		}
		if len(data) > maxOutcomeQualityFixtureBytes {
			return false, actual, fmt.Errorf("file %q exceeds bounded fixture size", file.Path)
		}
		content := string(data)
		actual[file.Path] = content
		if content != expected[file.Path] {
			return false, actual, nil
		}
	}
	return true, actual, nil
}

func outcomeQualityChangedLines(input OutcomeQualityInput, expected map[string]string) int {
	total := 0
	for _, file := range input.Files {
		total += lineDifference(file.Content, expected[file.Path])
	}
	return total
}

func lineDifference(before, after string) int {
	left := strings.Split(before, "\n")
	right := strings.Split(after, "\n")
	length := len(left)
	if len(right) > length {
		length = len(right)
	}
	different := 0
	for index := 0; index < length; index++ {
		var l, r string
		if index < len(left) {
			l = left[index]
		}
		if index < len(right) {
			r = right[index]
		}
		if l != r {
			different++
		}
	}
	return different
}

func outcomeQualityUnnecessaryChanges(changed, expected []string) int {
	want := make(map[string]struct{}, len(expected))
	for _, path := range expected {
		want[path] = struct{}{}
	}
	count := 0
	for _, path := range changed {
		if _, ok := want[path]; !ok {
			count++
		}
	}
	return count
}

func outcomeQualityVerificationState(result agent.Result) OutcomeQualityVerification {
	status := outcomeQualityStatus(result.Verified)
	switch status {
	case "PASS":
		return OutcomeVerificationPass
	case "FAIL":
		return OutcomeVerificationFail
	case "INCONCLUSIVE":
		return OutcomeVerificationInconclusive
	case "SKIPPED":
		return OutcomeVerificationSkipped
	default:
		return OutcomeVerificationUnverified
	}
}

func outcomeQualityStatus(output string) string {
	for _, field := range strings.Fields(output) {
		switch status := strings.ToUpper(strings.TrimSpace(field)); status {
		case "PASS", "FAIL", "INCONCLUSIVE", "SKIPPED":
			return status
		}
	}
	return ""
}

func outcomeQualityCompletionEvidenceCurrent(result agent.Result, current workspace.Observation) bool {
	if result.Task == nil {
		return false
	}
	proof := result.Task.CompletionCheck()
	if !result.Completion.Proof.Ready || !proof.Ready || len(result.Task.Verification) == 0 {
		return false
	}
	latest := result.Task.Verification[len(result.Task.Verification)-1]
	if !latest.Passed || latest.Observation == nil || latest.Observation.FilesTruncated || result.Task.VerifiedChangeSeq != result.Task.ChangeSeq {
		return false
	}
	return workspace.Compare(*latest.Observation, current).Fresh
}

func workspaceObservationCurrent(before, after workspace.Observation) bool {
	if err := before.Validate(); err != nil {
		return false
	}
	if err := after.Validate(); err != nil {
		return false
	}
	return workspace.Compare(after, after).Fresh
}

func outcomeQualityContextGrowth(client *outcomeQualityCountingClient) int64 {
	if client == nil || client.peakContextBytes <= client.initialContextBytes {
		return 0
	}
	return client.peakContextBytes - client.initialContextBytes
}
