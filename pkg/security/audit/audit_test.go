package audit

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestAuditLogger(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "audit-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	l, err := NewLogger(tempDir)
	if err != nil {
		t.Fatalf("failed to create logger: %v", err)
	}

	l.Log(Entry{
		Actor:  "test-actor",
		Action: "test-action",
		Status: "success",
	})

	err = l.Close()
	if err != nil {
		t.Errorf("failed to close logger: %v", err)
	}

	// Verify file exists and has content
	content, err := os.ReadFile(tempDir + "/audit.log")
	if err != nil {
		t.Fatalf("failed to read audit log: %v", err)
	}
	if len(content) == 0 {
		t.Error("audit log is empty")
	}
}

func TestSignedAuditLogger(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "audit-signed-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	l, err := NewLogger(tempDir)
	if err != nil {
		t.Fatalf("failed to create logger: %v", err)
	}

	// Use a software key for testing

	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	l.SetSigner(key)

	l.Log(Entry{
		Actor:  "test-actor-signed",
		Action: "test-action-signed",
		Status: "success",
	})

	l.Close()

	content, _ := os.ReadFile(tempDir + "/audit.log")
	var entry Entry
	if err := json.Unmarshal(content, &entry); err != nil {
		t.Fatalf("failed to unmarshal log entry: %v", err)
	}

	if len(entry.Signature) == 0 {
		t.Error("log entry signature is missing")
	}

	if !strings.Contains(string(content), "signature") {
		t.Error("log entry signature not found in JSON data")
	}
}

func TestNewLoggerFail(t *testing.T) {
	// Use a path that is unlikely to be writable or valid as a directory
	_, err := NewLogger("/dev/null/invalid")
	if err == nil {
		t.Error("expected error creating logger in invalid directory")
	}
}
