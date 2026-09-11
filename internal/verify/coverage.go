package verify

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/saiaathish/picogent/internal/securefile"
)

const MaxCoverageProfileBytes = 8 << 20

// ParseCoverProfile reads a Go coverprofile and returns package statement
// coverage. An empty, missing, or malformed profile remains UNVERIFIED.
func ParseCoverProfile(path string) CoverageEvidence {
	path = strings.TrimSpace(path)
	if path == "" {
		return CoverageEvidence{Status: ManifestUnverified, Reason: "coverprofile path is empty"}
	}
	data, err := securefile.ReadFileLimited(path, MaxCoverageProfileBytes)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return CoverageEvidence{Status: ManifestUnverified, Reason: "coverprofile was not written"}
		}
		if errors.Is(err, securefile.ErrReadLimit) {
			return CoverageEvidence{Status: ManifestUnverified, Reason: "coverprofile exceeds size limit"}
		}
		return CoverageEvidence{Status: ManifestUnverified, Reason: "coverprofile is unreadable"}
	}

	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 0, 64*1024), 1<<20)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return CoverageEvidence{Status: ManifestUnverified, Reason: "coverprofile is unreadable"}
		}
		return CoverageEvidence{Status: ManifestUnverified, Reason: "coverprofile is empty"}
	}
	modeLine := strings.TrimSpace(scanner.Text())
	if !strings.HasPrefix(modeLine, "mode:") {
		return CoverageEvidence{Status: ManifestUnverified, Reason: "coverprofile is missing a mode line"}
	}

	var statements, covered int
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		numStmt, count, ok := parseCoverProfileRecord(line)
		if !ok {
			return CoverageEvidence{Status: ManifestUnverified, Reason: "coverprofile contains a malformed record"}
		}
		statements += numStmt
		if count > 0 {
			covered += numStmt
		}
	}
	if err := scanner.Err(); err != nil {
		return CoverageEvidence{Status: ManifestUnverified, Reason: "coverprofile is unreadable"}
	}
	if statements == 0 {
		return CoverageEvidence{Status: ManifestUnverified, Reason: "coverprofile has no statements"}
	}
	percent := 100 * float64(covered) / float64(statements)
	return CoverageEvidence{Status: ManifestPass, Percent: &percent}
}

func parseCoverProfileRecord(line string) (numStmt, count int, ok bool) {
	// Format: file.go:startLine.col,endLine.col numStmt count
	fields := strings.Fields(line)
	if len(fields) != 3 {
		return 0, 0, false
	}
	if !strings.Contains(fields[0], ":") || !strings.Contains(fields[0], ",") {
		return 0, 0, false
	}
	numStmt, err := strconv.Atoi(fields[1])
	if err != nil || numStmt < 0 {
		return 0, 0, false
	}
	count, err = strconv.Atoi(fields[2])
	if err != nil || count < 0 {
		return 0, 0, false
	}
	return numStmt, count, true
}

func attachCoverProfile(result Result, path string) Result {
	if result.Status != StatusPass {
		if result.Coverage.Status == "" {
			result.Coverage = unverifiedCoverage()
		}
		return result
	}
	result.Coverage = ParseCoverProfile(path)
	if result.Coverage.Status != ManifestPass {
		result.OK = false
		result.Status = StatusInconclusive
		result.Reason = firstReason(result.Coverage.Reason, "targeted coverage evidence is incomplete")
	}
	return result
}

func withGoCoverProfile(command Command, path string) (Command, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return command, fmt.Errorf("coverprofile path is empty")
	}
	if command.Runner != "go" {
		return command, fmt.Errorf("coverprofile collection requires the go runner")
	}
	args := append([]string(nil), command.Args...)
	args = append(args, "-coverprofile="+path, "-covermode=set", "-count=1")
	command.Args = args
	command.Display = displayCommand(command.Runner, command.Args)
	return command, nil
}
