package runtimeboundary

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckEvidenceDocumentRequiresExactSourceAndMarkers(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "evidence.md")
	sha := strings.Repeat("a", 40)
	if err := os.WriteFile(path, []byte("Status: `PASS`\nsource: "+sha+"\nmarker: retained\nsecret instruction\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	lookup := func(path string) (bool, error) {
		_, err := os.Stat(path)
		return err == nil, err
	}
	read := os.ReadFile

	valid := checkEvidenceDocument(path, sha, lookup, read, "Status: `PASS`", "marker: retained")
	if !valid.valid || valid.reason != "" {
		t.Fatalf("valid evidence document = %#v", valid)
	}
	stale := checkEvidenceDocument(path, strings.Repeat("b", 40), lookup, read, "Status: `PASS`")
	if stale.valid || stale.reason != "evidence document source identity is stale or absent" {
		t.Fatalf("stale evidence document = %#v", stale)
	}
	missingMarker := checkEvidenceDocument(path, sha, lookup, read, "Status: `FAIL`")
	if missingMarker.valid || missingMarker.reason != "evidence document provenance markers are incomplete" {
		t.Fatalf("marker-incomplete evidence document = %#v", missingMarker)
	}
	if strings.Contains(valid.reason+stale.reason+missingMarker.reason, "secret instruction") {
		t.Fatal("raw document text escaped the validation result")
	}
}

func TestCheckEvidenceDocumentFailsClosedForMissingUnreadableAndOversized(t *testing.T) {
	sha := strings.Repeat("c", 40)
	missing := checkEvidenceDocument("/does/not/exist", sha, func(string) (bool, error) {
		return false, nil
	}, func(string) ([]byte, error) {
		return nil, errors.New("should not read missing document")
	})
	if missing.valid || missing.reason != "evidence document is missing" {
		t.Fatalf("missing document = %#v", missing)
	}

	lookupFailed := checkEvidenceDocument("ignored", sha, func(string) (bool, error) {
		return false, errors.New("raw lookup failure")
	}, func(string) ([]byte, error) {
		return nil, nil
	})
	if lookupFailed.valid || lookupFailed.reason != "evidence document lookup failed" {
		t.Fatalf("lookup failure = %#v", lookupFailed)
	}

	readFailed := checkEvidenceDocument("unreadable", sha, func(string) (bool, error) {
		return true, nil
	}, func(string) ([]byte, error) {
		return nil, errors.New("raw read failure")
	})
	if readFailed.valid || readFailed.reason != "evidence document could not be read" {
		t.Fatalf("read failure = %#v", readFailed)
	}

	oversized := checkEvidenceDocument("oversized", sha, func(string) (bool, error) {
		return true, nil
	}, func(string) ([]byte, error) {
		return []byte(strings.Repeat("x", maxEvidenceDocumentBytes+1)), nil
	})
	if oversized.valid || oversized.reason != "evidence document exceeds bounded read limit" {
		t.Fatalf("oversized document = %#v", oversized)
	}
}
