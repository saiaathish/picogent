package outcome

import (
	"strings"

	"github.com/saiaathish/picogent/internal/taskstate"
)

const maxImpactItems = 8

// ImpactScope is a coarse description of the changed surface. It is a routing
// hint, not a probability estimate or a claim that every affected file was
// discovered.
type ImpactScope string

const (
	ImpactNone      ImpactScope = "NONE"
	ImpactFocused   ImpactScope = "FOCUSED"
	ImpactCrossArea ImpactScope = "CROSS_AREA"
	ImpactBroad     ImpactScope = "BROAD"
	ImpactUnknown   ImpactScope = "UNKNOWN"
)

// ImpactRisk is intentionally categorical. A conservative category can add
// review or verification work, but it can never authorize an action.
type ImpactRisk string

const (
	ImpactRiskLow    ImpactRisk = "LOW"
	ImpactRiskMedium ImpactRisk = "MEDIUM"
	ImpactRiskHigh   ImpactRisk = "HIGH"
)

// ImpactArea is a fixed vocabulary derived from path shape and task intent.
type ImpactArea string

const (
	ImpactAreaSource      ImpactArea = "source"
	ImpactAreaTests       ImpactArea = "tests"
	ImpactAreaUI          ImpactArea = "ui"
	ImpactAreaConfig      ImpactArea = "config"
	ImpactAreaSecurity    ImpactArea = "security"
	ImpactAreaConcurrency ImpactArea = "concurrency"
	ImpactAreaGenerated   ImpactArea = "generated"
	ImpactAreaDocs        ImpactArea = "docs"
)

// ImpactCheck is a fixed verification or review category. It deliberately
// contains no command, path, or repository-derived text.
type ImpactCheck string

const (
	ImpactCheckTargetedTests  ImpactCheck = "targeted_tests"
	ImpactCheckBroader        ImpactCheck = "broader_verification"
	ImpactCheckBuild          ImpactCheck = "build_check"
	ImpactCheckRendered       ImpactCheck = "rendered_ui"
	ImpactCheckSecurity       ImpactCheck = "security_review"
	ImpactCheckConcurrency    ImpactCheck = "concurrency_review"
	ImpactCheckVisualReview   ImpactCheck = "visual_review"
	ImpactCheckConfigReview   ImpactCheck = "configuration_review"
	ImpactCheckDocs           ImpactCheck = "documentation_check"
	ImpactCheckTargetedReview ImpactCheck = "targeted_review"
	ImpactCheckBroaderReview  ImpactCheck = "broader_review"
)

type ImpactCheckpoint string

const (
	ImpactCheckpointNone      ImpactCheckpoint = "none"
	ImpactCheckpointCrossArea ImpactCheckpoint = "before_cross_area_follow_up"
	ImpactCheckpointBroad     ImpactCheckpoint = "before_broad_follow_up"
	ImpactCheckpointHighRisk  ImpactCheckpoint = "before_high_risk_follow_up"
	ImpactCheckpointRecheck   ImpactCheckpoint = "recheck_impact_before_edit"
)

var impactAreaOrder = [...]ImpactArea{
	ImpactAreaSecurity,
	ImpactAreaConcurrency,
	ImpactAreaUI,
	ImpactAreaConfig,
	ImpactAreaTests,
	ImpactAreaGenerated,
	ImpactAreaDocs,
	ImpactAreaSource,
}

// ImpactProfile is the bounded change-impact projection carried by the
// transient Outcome Engine contract. It uses only cheap, already-durable
// signals; it is not a dependency graph, static analyzer, or completion gate.
type ImpactProfile struct {
	Scope              ImpactScope      `json:"scope"`
	Risk               ImpactRisk       `json:"risk"`
	Confidence         string           `json:"confidence"`
	ChangedFiles       int              `json:"changed_files"`
	ChangedFilesCapped bool             `json:"changed_files_capped,omitempty"`
	Areas              []ImpactArea     `json:"areas,omitempty"`
	Verification       []ImpactCheck    `json:"verification,omitempty"`
	Review             []ImpactCheck    `json:"review,omitempty"`
	Checkpoint         ImpactCheckpoint `json:"checkpoint"`
}

// PredictImpact derives a safe, deterministic profile from the current task.
// Repository paths are inspected as opaque labels only; they are never
// resolved, opened, or copied into a prompt. Missing or capped paths lower
// confidence and increase the recommended verification depth.
func PredictImpact(task *taskstate.Task) ImpactProfile {
	profile := ImpactProfile{
		Scope:      ImpactNone,
		Risk:       ImpactRiskLow,
		Confidence: "low",
		Checkpoint: ImpactCheckpointNone,
	}
	if task == nil {
		return profile
	}
	profile.ChangedFiles = len(task.ChangedFiles)
	profile.ChangedFilesCapped = task.ChangedFilesCapped
	if len(task.ChangedFiles) == 0 {
		if task.ChangeSeq == 0 && !task.ChangedFilesCapped {
			return profile
		}
		return unknownImpact(profile, task)
	}

	directories := make(map[string]struct{}, len(task.ChangedFiles))
	topLevels := make(map[string]struct{}, len(task.ChangedFiles))
	areas := make(map[ImpactArea]struct{})
	validPaths := 0
	invalidPath := false
	for _, raw := range task.ChangedFiles {
		path := normalizeImpactPath(raw)
		if path == "" {
			invalidPath = true
			continue
		}
		validPaths++
		directories[impactDirectory(path)] = struct{}{}
		topLevels[impactTopLevel(path)] = struct{}{}
		for _, area := range impactAreasForPath(path) {
			areas[area] = struct{}{}
		}
	}
	if validPaths == 0 {
		return unknownImpact(profile, task)
	}
	profile.Areas = orderedImpactAreas(areas)
	profile.Confidence = "high"
	if profile.ChangedFilesCapped || len(task.ChangedFiles) > maxImpactItems || invalidPath {
		profile.Confidence = "medium"
	}
	switch {
	case profile.ChangedFilesCapped || profile.ChangedFiles > maxImpactItems:
		profile.Scope = ImpactBroad
	case profile.ChangedFiles > 3 || len(directories) > 1 || len(topLevels) > 2:
		profile.Scope = ImpactCrossArea
	default:
		profile.Scope = ImpactFocused
	}
	profile.Risk = impactRisk(profile.Scope, profile.Areas, task)
	profile.Verification = impactVerification(profile.Scope, profile.Areas)
	profile.Review = impactReview(profile.Scope, profile.Areas)
	if invalidPath {
		profile.Verification = appendImpactCheck(profile.Verification, ImpactCheckBroader)
		profile.Review = appendImpactCheck(profile.Review, ImpactCheckBroaderReview)
	}
	switch {
	case profile.Scope == ImpactBroad:
		profile.Checkpoint = ImpactCheckpointBroad
	case profile.Risk == ImpactRiskHigh:
		profile.Checkpoint = ImpactCheckpointHighRisk
	case profile.Scope == ImpactCrossArea:
		profile.Checkpoint = ImpactCheckpointCrossArea
	}
	return boundImpact(profile)
}

func unknownImpact(profile ImpactProfile, task *taskstate.Task) ImpactProfile {
	profile.Scope = ImpactUnknown
	profile.Risk = impactRisk(profile.Scope, nil, task)
	profile.Checkpoint = ImpactCheckpointRecheck
	profile.Verification = []ImpactCheck{ImpactCheckBroader}
	profile.Review = []ImpactCheck{ImpactCheckBroaderReview}
	return boundImpact(profile)
}

func normalizeImpactPath(path string) string {
	path = strings.TrimSpace(strings.ReplaceAll(path, "\\", "/"))
	path = strings.TrimPrefix(path, "./")
	return strings.ToLower(path)
}

func impactDirectory(path string) string {
	if index := strings.LastIndexByte(path, '/'); index >= 0 {
		if index == 0 {
			return "/"
		}
		return path[:index]
	}
	return "."
}

func impactTopLevel(path string) string {
	if index := strings.IndexByte(path, '/'); index >= 0 {
		return path[:index]
	}
	return "."
}

func impactAreasForPath(path string) []ImpactArea {
	base := path
	if index := strings.LastIndexByte(base, '/'); index >= 0 {
		base = base[index+1:]
	}
	words := impactWords(path)
	isTest := strings.HasSuffix(base, "_test.go") || strings.Contains(base, ".test.") || strings.Contains(base, ".spec.") || impactHasWord(words, "test", "tests", "__tests__")
	isUI := impactHasSuffix(base, ".html", ".css", ".scss", ".tsx", ".jsx", ".vue", ".svelte") || impactHasWord(words, "ui", "web", "frontend", "component", "components", "template", "templates")
	isConfig := impactConfigBase(base) || impactHasSuffix(base, ".yaml", ".yml", ".toml", ".ini", ".json") || impactHasWord(words, "config", "configs", "manifest", "manifests", "deployment", "deployments")
	isDocs := impactHasSuffix(base, ".md", ".mdx", ".rst", ".adoc") || impactHasWord(words, "docs", "documentation", "readme", "changelog")
	isGenerated := impactHasWord(words, "generated", "gen") || strings.HasSuffix(base, ".gen.go") || strings.Contains(base, ".generated.")
	areas := make(map[ImpactArea]struct{})
	if isTest {
		areas[ImpactAreaTests] = struct{}{}
	}
	if isUI {
		areas[ImpactAreaUI] = struct{}{}
	}
	if isConfig {
		areas[ImpactAreaConfig] = struct{}{}
	}
	if isDocs {
		areas[ImpactAreaDocs] = struct{}{}
	}
	if isGenerated {
		areas[ImpactAreaGenerated] = struct{}{}
	}
	if impactHasWord(words, "auth", "authentication", "authorization", "login", "oauth", "credential", "credentials", "secret", "token", "permission", "permissions", "policy", "mcp", "sandbox", "crypto", "cryptography", "tls", "certificate", "cert", "cookie", "session") {
		areas[ImpactAreaSecurity] = struct{}{}
	}
	if impactHasWord(words, "concurrency", "concurrent", "goroutine", "mutex", "lock", "race", "worker", "queue", "turn", "taskstate", "process", "atomic", "session") {
		areas[ImpactAreaConcurrency] = struct{}{}
	}
	if !isTest && !isUI && !isConfig && !isDocs && !isGenerated {
		areas[ImpactAreaSource] = struct{}{}
	}
	return orderedImpactAreas(areas)
}

func impactConfigBase(base string) bool {
	switch base {
	case "go.mod", "go.sum", "package.json", "package-lock.json", "pnpm-lock.yaml", "yarn.lock", "cargo.toml", "cargo.lock", "pyproject.toml", "requirements.txt", "makefile", "dockerfile", ".env", ".env.local":
		return true
	default:
		return false
	}
}

func impactHasSuffix(value string, wanted ...string) bool {
	for _, suffix := range wanted {
		if strings.HasSuffix(value, suffix) {
			return true
		}
	}
	return false
}

func impactWords(path string) []string {
	return strings.FieldsFunc(path, func(r rune) bool {
		return !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'))
	})
}

func impactHasWord(words []string, wanted ...string) bool {
	for _, word := range words {
		for _, candidate := range wanted {
			if word == candidate {
				return true
			}
		}
	}
	return false
}

func orderedImpactAreas(areas map[ImpactArea]struct{}) []ImpactArea {
	out := make([]ImpactArea, 0, len(areas))
	for _, area := range impactAreaOrder {
		if _, ok := areas[area]; ok {
			out = append(out, area)
		}
	}
	return out
}

func impactRisk(scope ImpactScope, areas []ImpactArea, task *taskstate.Task) ImpactRisk {
	for _, area := range areas {
		if area == ImpactAreaSecurity || area == ImpactAreaConcurrency {
			return ImpactRiskHigh
		}
	}
	if task != nil && task.Intent != nil && strings.EqualFold(strings.TrimSpace(task.Intent.Risk), "high") {
		return ImpactRiskHigh
	}
	if scope == ImpactBroad || scope == ImpactCrossArea || scope == ImpactUnknown {
		return ImpactRiskMedium
	}
	for _, area := range areas {
		if area == ImpactAreaSource || area == ImpactAreaUI || area == ImpactAreaConfig || area == ImpactAreaGenerated {
			return ImpactRiskMedium
		}
	}
	return ImpactRiskLow
}

func impactVerification(scope ImpactScope, areas []ImpactArea) []ImpactCheck {
	out := make([]ImpactCheck, 0, maxImpactItems)
	for _, area := range areas {
		switch area {
		case ImpactAreaSource, ImpactAreaTests, ImpactAreaGenerated:
			out = appendImpactCheck(out, ImpactCheckTargetedTests)
		case ImpactAreaConfig:
			out = appendImpactCheck(out, ImpactCheckBuild)
		case ImpactAreaUI:
			out = appendImpactCheck(out, ImpactCheckRendered)
		case ImpactAreaSecurity:
			out = appendImpactCheck(out, ImpactCheckSecurity)
		case ImpactAreaConcurrency:
			out = appendImpactCheck(out, ImpactCheckConcurrency)
		case ImpactAreaDocs:
			out = appendImpactCheck(out, ImpactCheckDocs)
		}
	}
	if scope != ImpactFocused && scope != ImpactNone {
		out = appendImpactCheck(out, ImpactCheckBroader)
	}
	return out
}

func impactReview(scope ImpactScope, areas []ImpactArea) []ImpactCheck {
	out := make([]ImpactCheck, 0, maxImpactItems)
	if scope == ImpactBroad || scope == ImpactCrossArea {
		out = appendImpactCheck(out, ImpactCheckBroaderReview)
	} else if scope == ImpactFocused {
		out = appendImpactCheck(out, ImpactCheckTargetedReview)
	}
	for _, area := range areas {
		switch area {
		case ImpactAreaSecurity:
			out = appendImpactCheck(out, ImpactCheckSecurity)
		case ImpactAreaConcurrency:
			out = appendImpactCheck(out, ImpactCheckConcurrency)
		case ImpactAreaUI:
			out = appendImpactCheck(out, ImpactCheckVisualReview)
		case ImpactAreaConfig:
			out = appendImpactCheck(out, ImpactCheckConfigReview)
		}
	}
	return out
}

func appendImpactCheck(values []ImpactCheck, check ImpactCheck) []ImpactCheck {
	if len(values) >= maxImpactItems || !knownImpactCheck(check) {
		return values
	}
	for _, existing := range values {
		if existing == check {
			return values
		}
	}
	return append(values, check)
}

func knownImpactCheck(check ImpactCheck) bool {
	switch check {
	case ImpactCheckTargetedTests, ImpactCheckBroader, ImpactCheckBuild, ImpactCheckRendered,
		ImpactCheckSecurity, ImpactCheckConcurrency, ImpactCheckVisualReview, ImpactCheckConfigReview,
		ImpactCheckDocs, ImpactCheckTargetedReview, ImpactCheckBroaderReview:
		return true
	default:
		return false
	}
}

func boundImpact(profile ImpactProfile) ImpactProfile {
	switch profile.Scope {
	case ImpactNone, ImpactFocused, ImpactCrossArea, ImpactBroad, ImpactUnknown:
	default:
		profile.Scope = ImpactUnknown
	}
	switch profile.Risk {
	case ImpactRiskLow, ImpactRiskMedium, ImpactRiskHigh:
	default:
		profile.Risk = ImpactRiskMedium
	}
	switch profile.Confidence {
	case "low", "medium", "high":
	default:
		profile.Confidence = "low"
	}
	if profile.ChangedFiles < 0 {
		profile.ChangedFiles = 0
	}
	if profile.ChangedFiles > maxImpactItems {
		profile.ChangedFiles = maxImpactItems
		profile.ChangedFilesCapped = true
	}
	profile.Areas = boundImpactAreas(profile.Areas)
	profile.Verification = boundImpactChecks(profile.Verification)
	profile.Review = boundImpactChecks(profile.Review)
	switch profile.Checkpoint {
	case ImpactCheckpointNone, ImpactCheckpointCrossArea, ImpactCheckpointBroad, ImpactCheckpointHighRisk, ImpactCheckpointRecheck:
	default:
		profile.Checkpoint = ImpactCheckpointRecheck
	}
	return profile
}

func boundImpactAreas(values []ImpactArea) []ImpactArea {
	if len(values) == 0 {
		return nil
	}
	out := make([]ImpactArea, 0, len(values))
	for _, area := range impactAreaOrder {
		for _, value := range values {
			if value == area {
				out = append(out, area)
				break
			}
		}
	}
	if len(out) > maxImpactItems {
		return out[:maxImpactItems]
	}
	return out
}

func boundImpactChecks(values []ImpactCheck) []ImpactCheck {
	out := make([]ImpactCheck, 0, len(values))
	for _, value := range values {
		out = appendImpactCheck(out, value)
	}
	return out
}

func impactAreasSummary(areas []ImpactArea) string {
	if len(areas) == 0 {
		return "none"
	}
	values := make([]string, 0, len(areas))
	for _, area := range boundImpactAreas(areas) {
		values = append(values, string(area))
	}
	return strings.Join(values, ",")
}

func impactChecksSummary(checks []ImpactCheck) string {
	if len(checks) == 0 {
		return "none"
	}
	values := make([]string, 0, len(checks))
	for _, check := range boundImpactChecks(checks) {
		values = append(values, string(check))
	}
	return strings.Join(values, ",")
}
