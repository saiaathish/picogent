// Package releaseartifact builds deterministic production binaries, packages,
// and SPDX SBOMs for release-readiness evidence. It does not authorize a
// release or claim live-provider / rendered-platform readiness.
package releaseartifact

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/saiaathish/picogent/internal/verify"
)

const (
	ManifestSchema   = "picogent.release-artifact.v1"
	SPDXVersion      = "SPDX-2.3"
	PredicateType    = "https://github.com/saiaathish/picogent/attestation/release-artifact/v1"
	MaxManifestBytes = 64 << 10
)

// Target is one GOOS/GOARCH production build.
type Target struct {
	GOOS   string `json:"goos"`
	GOARCH string `json:"goarch"`
}

// Options configures one evidence collection run.
type Options struct {
	Workspace      string
	OutputDir      string
	CandidateSHA   string
	SourceDateEpoch int64
	GoVersion      string
	Targets        []Target
}

// FileEvidence records one retained artifact.
type FileEvidence struct {
	Name   string `json:"name"`
	SHA256 string `json:"sha256"`
	Bytes  int64  `json:"bytes"`
}

// TargetEvidence records one platform's production artifacts.
type TargetEvidence struct {
	GOOS    string       `json:"goos"`
	GOARCH  string       `json:"goarch"`
	Binary  FileEvidence `json:"binary"`
	Package FileEvidence `json:"package"`
	SBOM    FileEvidence `json:"sbom"`
}

// Manifest is the deterministic evidence ledger for one candidate SHA.
type Manifest struct {
	Schema       string           `json:"schema"`
	Status       string           `json:"status"`
	CandidateSHA string           `json:"candidate_sha"`
	Repository   string           `json:"repository,omitempty"`
	GoVersion    string           `json:"go_version"`
	SourceDate   int64            `json:"source_date_epoch"`
	Targets      []TargetEvidence `json:"targets"`
	Reason       string           `json:"reason,omitempty"`
}

// DefaultTargets returns the supported production cross-compile set.
func DefaultTargets() []Target {
	return []Target{
		{GOOS: "linux", GOARCH: "amd64"},
		{GOOS: "darwin", GOARCH: "arm64"},
		{GOOS: "windows", GOARCH: "amd64"},
	}
}

// Build collects deterministic production artifacts for the requested targets.
func Build(opts Options) (Manifest, error) {
	manifest := Manifest{
		Schema:       ManifestSchema,
		Status:       "UNVERIFIED",
		CandidateSHA: strings.TrimSpace(opts.CandidateSHA),
		SourceDate:   opts.SourceDateEpoch,
	}
	if opts.SourceDateEpoch <= 0 {
		return manifest, errors.New("source_date_epoch is required")
	}
	if strings.TrimSpace(opts.CandidateSHA) == "" || !validCommitSHA(opts.CandidateSHA) {
		return manifest, errors.New("candidate_sha must be a full commit id")
	}
	workspace, err := filepath.Abs(strings.TrimSpace(opts.Workspace))
	if err != nil {
		return manifest, fmt.Errorf("resolve workspace: %w", err)
	}
	outputDir, err := filepath.Abs(strings.TrimSpace(opts.OutputDir))
	if err != nil {
		return manifest, fmt.Errorf("resolve output dir: %w", err)
	}
	if err := verify.ValidateReleaseEvidenceDirectory(workspace, outputDir); err != nil {
		return manifest, err
	}
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return manifest, fmt.Errorf("create output dir: %w", err)
	}

	provenance := verify.CollectProvenance(context.Background(), workspace, opts.CandidateSHA)
	if provenance.Match != verify.ManifestPass {
		return manifest, fmt.Errorf("candidate provenance: %s", firstNonEmpty(provenance.Reason, string(provenance.Match)))
	}
	if provenance.Tree != "CLEAN" {
		return manifest, errors.New("workspace is not clean")
	}

	goVersion := strings.TrimSpace(opts.GoVersion)
	if goVersion == "" {
		goVersion = runtime.Version()
	}
	manifest.GoVersion = goVersion

	targets := opts.Targets
	if len(targets) == 0 {
		targets = DefaultTargets()
	}
	for _, target := range targets {
		evidence, err := buildTarget(workspace, outputDir, opts.CandidateSHA, opts.SourceDateEpoch, target)
		if err != nil {
			manifest.Status = "FAIL"
			manifest.Reason = err.Error()
			return manifest, err
		}
		manifest.Targets = append(manifest.Targets, evidence)
	}
	sort.Slice(manifest.Targets, func(i, j int) bool {
		left := manifest.Targets[i].GOOS + "/" + manifest.Targets[i].GOARCH
		right := manifest.Targets[j].GOOS + "/" + manifest.Targets[j].GOARCH
		return left < right
	})
	manifest.Status = "PASS"
	return manifest, nil
}

func buildTarget(workspace, outputDir, candidateSHA string, epoch int64, target Target) (TargetEvidence, error) {
	if target.GOOS == "" || target.GOARCH == "" {
		return TargetEvidence{}, errors.New("target goos/goarch are required")
	}
	ext := ""
	if target.GOOS == "windows" {
		ext = ".exe"
	}
	base := fmt.Sprintf("picogent-%s-%s", target.GOOS, target.GOARCH)
	binaryName := base + ext
	packageName := base + ".tar.gz"
	sbomName := base + ".spdx.json"

	binaryPath := filepath.Join(outputDir, binaryName)
	packagePath := filepath.Join(outputDir, packageName)
	sbomPath := filepath.Join(outputDir, sbomName)

	if err := buildBinary(workspace, binaryPath, candidateSHA, target); err != nil {
		return TargetEvidence{}, err
	}
	if err := writePackage(packagePath, binaryName, binaryPath, filepath.Join(workspace, "LICENSE"), epoch); err != nil {
		return TargetEvidence{}, err
	}
	modules, err := modulesFromBinary(binaryPath)
	if err != nil {
		return TargetEvidence{}, err
	}
	expected, err := modulesFromSource(workspace, target)
	if err != nil {
		return TargetEvidence{}, err
	}
	if err := compareModuleSets(modules, expected); err != nil {
		return TargetEvidence{}, err
	}
	if err := writeSPDX(sbomPath, candidateSHA, binaryName, modules, epoch); err != nil {
		return TargetEvidence{}, err
	}

	binaryEv, err := fileEvidence(binaryName, binaryPath)
	if err != nil {
		return TargetEvidence{}, err
	}
	packageEv, err := fileEvidence(packageName, packagePath)
	if err != nil {
		return TargetEvidence{}, err
	}
	sbomEv, err := fileEvidence(sbomName, sbomPath)
	if err != nil {
		return TargetEvidence{}, err
	}
	return TargetEvidence{
		GOOS:    target.GOOS,
		GOARCH:  target.GOARCH,
		Binary:  binaryEv,
		Package: packageEv,
		SBOM:    sbomEv,
	}, nil
}

func buildBinary(workspace, outputPath, candidateSHA string, target Target) error {
	args := []string{
		"build",
		"-trimpath",
		"-buildvcs=false",
		"-mod=readonly",
		"-ldflags", fmt.Sprintf("-buildid= -X main.version=%s", candidateSHA),
		"-o", outputPath,
		"./cmd/picogent",
	}
	cmd := exec.Command("go", args...)
	cmd.Dir = workspace
	cmd.Env = append(os.Environ(),
		"CGO_ENABLED=0",
		"GOOS="+target.GOOS,
		"GOARCH="+target.GOARCH,
		"GOFLAGS=",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("build %s/%s: %w\n%s", target.GOOS, target.GOARCH, err, strings.TrimSpace(string(out)))
	}
	return nil
}

func writePackage(packagePath, binaryName, binaryPath, licensePath string, epoch int64) error {
	binaryData, err := os.ReadFile(binaryPath)
	if err != nil {
		return fmt.Errorf("read binary: %w", err)
	}
	licenseData, err := os.ReadFile(licensePath)
	if err != nil {
		return fmt.Errorf("read LICENSE: %w", err)
	}
	file, err := os.OpenFile(packagePath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()
	gz := gzip.NewWriter(file)
	gz.Name = ""
	gz.ModTime = time.Unix(epoch, 0).UTC()
	tw := tar.NewWriter(gz)

	modTime := time.Unix(epoch, 0).UTC()
	entries := []struct {
		name string
		mode int64
		data []byte
	}{
		{name: binaryName, mode: 0o755, data: binaryData},
		{name: "LICENSE", mode: 0o644, data: licenseData},
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].name < entries[j].name })
	for _, entry := range entries {
		hdr := &tar.Header{
			Name:    entry.name,
			Mode:    entry.mode,
			Size:    int64(len(entry.data)),
			ModTime: modTime,
			Uid:     0,
			Gid:     0,
			Uname:   "",
			Gname:   "",
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		if _, err := tw.Write(entry.data); err != nil {
			return err
		}
	}
	if err := tw.Close(); err != nil {
		return err
	}
	if err := gz.Close(); err != nil {
		return err
	}
	return file.Close()
}

type moduleRef struct {
	Path    string
	Version string
	Hash    string
}

func modulesFromBinary(binaryPath string) ([]moduleRef, error) {
	cmd := exec.Command("go", "version", "-m", binaryPath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("go version -m: %w\n%s", err, strings.TrimSpace(string(out)))
	}
	var modules []moduleRef
	seen := map[string]struct{}{}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "dep\t") {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) < 3 {
			return nil, fmt.Errorf("malformed go version -m dep line: %q", line)
		}
		ref := moduleRef{Path: fields[1], Version: fields[2]}
		if len(fields) > 3 {
			ref.Hash = fields[3]
		}
		key := ref.Path + "@" + ref.Version
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		modules = append(modules, ref)
	}
	sort.Slice(modules, func(i, j int) bool {
		if modules[i].Path == modules[j].Path {
			return modules[i].Version < modules[j].Version
		}
		return modules[i].Path < modules[j].Path
	})
	if len(modules) == 0 {
		return nil, nil
	}
	return modules, nil
}

func modulesFromSource(workspace string, target Target) ([]moduleRef, error) {
	cmd := exec.Command("go", "list", "-deps", "-f", `{{if and .Module (not .Standard)}}{{.Module.Path}}{{"\t"}}{{.Module.Version}}{{"\t"}}{{.Module.Sum}}{{end}}`, "./cmd/picogent")
	cmd.Dir = workspace
	cmd.Env = append(os.Environ(),
		"CGO_ENABLED=0",
		"GOOS="+target.GOOS,
		"GOARCH="+target.GOARCH,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("go list -deps: %w\n%s", err, strings.TrimSpace(string(out)))
	}
	seen := map[string]moduleRef{}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) < 2 {
			continue
		}
		ref := moduleRef{Path: fields[0], Version: fields[1]}
		if len(fields) > 2 {
			ref.Hash = fields[2]
		}
		// Main module often has empty version in list output; skip matching
		// against binary dep lines which omit the main module.
		if ref.Version == "" || ref.Version == "(devel)" {
			continue
		}
		seen[ref.Path+"@"+ref.Version] = ref
	}
	modules := make([]moduleRef, 0, len(seen))
	for _, ref := range seen {
		modules = append(modules, ref)
	}
	sort.Slice(modules, func(i, j int) bool {
		if modules[i].Path == modules[j].Path {
			return modules[i].Version < modules[j].Version
		}
		return modules[i].Path < modules[j].Path
	})
	if len(modules) == 0 {
		return nil, nil
	}
	return modules, nil
}

func compareModuleSets(binaryMods, sourceMods []moduleRef) error {
	source := map[string]struct{}{}
	for _, mod := range sourceMods {
		source[mod.Path+"@"+mod.Version] = struct{}{}
	}
	var missing []string
	for _, mod := range binaryMods {
		key := mod.Path + "@" + mod.Version
		if _, ok := source[key]; !ok {
			missing = append(missing, key)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return fmt.Errorf("binary modules missing from source deps: %s", strings.Join(missing, ", "))
	}
	return nil
}

type spdxDocument struct {
	SPDXVersion       string        `json:"spdxVersion"`
	DataLicense       string        `json:"dataLicense"`
	SPDXID            string        `json:"SPDXID"`
	Name              string        `json:"name"`
	DocumentNamespace string        `json:"documentNamespace"`
	CreationInfo      spdxCreation  `json:"creationInfo"`
	Packages          []spdxPackage `json:"packages"`
	Relationships     []spdxRel     `json:"relationships"`
}

type spdxCreation struct {
	Created            string   `json:"created"`
	Creators           []string `json:"creators"`
	LicenseListVersion string   `json:"licenseListVersion"`
}

type spdxPackage struct {
	SPDXID           string `json:"SPDXID"`
	Name             string `json:"name"`
	VersionInfo      string `json:"versionInfo,omitempty"`
	DownloadLocation string `json:"downloadLocation"`
	FilesAnalyzed    bool   `json:"filesAnalyzed"`
	Checksums        []any  `json:"checksums,omitempty"`
	ExternalRefs     []any  `json:"externalRefs,omitempty"`
	Supplier         string `json:"supplier"`
}

type spdxRel struct {
	SPDXElementID      string `json:"spdxElementId"`
	RelatedSPDXElement string `json:"relatedSpdxElement"`
	RelationshipType   string `json:"relationshipType"`
}

func writeSPDX(path, candidateSHA, binaryName string, modules []moduleRef, epoch int64) error {
	created := time.Unix(epoch, 0).UTC().Format(time.RFC3339)
	doc := spdxDocument{
		SPDXVersion:       SPDXVersion,
		DataLicense:       "CC0-1.0",
		SPDXID:            "SPDXRef-DOCUMENT",
		Name:              "picogent-" + candidateSHA[:12] + "-" + binaryName,
		DocumentNamespace: "https://github.com/saiaathish/picogent/spdx/" + candidateSHA + "/" + binaryName,
		CreationInfo: spdxCreation{
			Created:            created,
			Creators:           []string{"Tool: picogent-release-artifact", "Organization: saiaathish/picogent"},
			LicenseListVersion: "3.23",
		},
	}
	rootID := "SPDXRef-Package-picogent"
	doc.Packages = append(doc.Packages, spdxPackage{
		SPDXID:           rootID,
		Name:             "picogent",
		VersionInfo:      candidateSHA,
		DownloadLocation: "NOASSERTION",
		FilesAnalyzed:    false,
		Supplier:         "Organization: saiaathish/picogent",
	})
	doc.Relationships = append(doc.Relationships, spdxRel{
		SPDXElementID:      "SPDXRef-DOCUMENT",
		RelatedSPDXElement: rootID,
		RelationshipType:   "DESCRIBES",
	})
	for i, mod := range modules {
		id := fmt.Sprintf("SPDXRef-Package-%d", i+1)
		doc.Packages = append(doc.Packages, spdxPackage{
			SPDXID:           id,
			Name:             mod.Path,
			VersionInfo:      mod.Version,
			DownloadLocation: "NOASSERTION",
			FilesAnalyzed:    false,
			Supplier:         "NOASSERTION",
		})
		doc.Relationships = append(doc.Relationships, spdxRel{
			SPDXElementID:      rootID,
			RelatedSPDXElement: id,
			RelationshipType:   "DEPENDS_ON",
		})
	}
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

func fileEvidence(name, path string) (FileEvidence, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return FileEvidence{}, err
	}
	sum := sha256.Sum256(data)
	return FileEvidence{
		Name:   name,
		SHA256: hex.EncodeToString(sum[:]),
		Bytes:  int64(len(data)),
	}, nil
}

func WriteManifest(w io.Writer, manifest Manifest) error {
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	if len(data)+1 > MaxManifestBytes {
		return errors.New("release artifact manifest exceeds size limit")
	}
	_, err = w.Write(append(data, '\n'))
	return err
}

func validCommitSHA(sha string) bool {
	if len(sha) != 40 {
		return false
	}
	for _, r := range sha {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return false
		}
	}
	return true
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
