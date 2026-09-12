package runtimeboundary

import (
	"errors"
	"strings"

	"github.com/saiaathish/picogent/internal/securefile"
)

const maxEvidenceDocumentBytes = 128 << 10

type evidenceDocumentResult struct {
	valid  bool
	reason string
}

// checkEvidenceDocument validates documentation-backed evidence without
// promoting file existence into a runtime claim. The expected source SHA and
// fixed markers are checked in one bounded read; raw document content and
// read errors never enter the matrix report.
func checkEvidenceDocument(path, expectedSHA string, lookup func(string) (bool, error), read func(string) ([]byte, error), markers ...string) evidenceDocumentResult {
	if !validCommitSHA(strings.TrimSpace(expectedSHA)) {
		return evidenceDocumentResult{reason: "evidence document expected source identity is invalid"}
	}
	exists, err := lookup(path)
	if err != nil {
		return evidenceDocumentResult{reason: "evidence document lookup failed"}
	}
	if !exists {
		return evidenceDocumentResult{reason: "evidence document is missing"}
	}
	data, err := read(path)
	if err != nil {
		if errors.Is(err, securefile.ErrReadLimit) {
			return evidenceDocumentResult{reason: "evidence document exceeds bounded read limit"}
		}
		return evidenceDocumentResult{reason: "evidence document could not be read"}
	}
	if len(data) > maxEvidenceDocumentBytes {
		return evidenceDocumentResult{reason: "evidence document exceeds bounded read limit"}
	}
	text := string(data)
	if !strings.Contains(text, strings.TrimSpace(expectedSHA)) {
		return evidenceDocumentResult{reason: "evidence document source identity is stale or absent"}
	}
	for _, marker := range markers {
		if strings.TrimSpace(marker) == "" || !strings.Contains(text, marker) {
			return evidenceDocumentResult{reason: "evidence document provenance markers are incomplete"}
		}
	}
	return evidenceDocumentResult{valid: true}
}
