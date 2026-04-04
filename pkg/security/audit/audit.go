package audit

import (
	"crypto"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"time"
)

type Entry struct {
	Timestamp time.Time      `json:"timestamp"`
	Actor     string         `json:"actor"`
	Action    string         `json:"action"`
	Target    string         `json:"target,omitempty"`
	Status    string         `json:"status"`
	TraceID   string         `json:"trace_id,omitempty"`
	SpanID    string         `json:"span_id,omitempty"`
	Details   map[string]any `json:"details,omitempty"`
	Signature []byte         `json:"signature,omitempty"`
}

type Logger struct {
	file   *os.File
	signer crypto.Signer
}

func NewLogger(dataDir string) (*Logger, error) {
	if err := os.MkdirAll(dataDir, 0700); err != nil {
		return nil, err
	}

	path := filepath.Join(dataDir, "audit.log")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return nil, err
	}

	return &Logger{file: f}, nil
}

// SetSigner attaches a cryptographic signer to the logger for producing signed logs.
func (l *Logger) SetSigner(s crypto.Signer) {
	l.signer = s
}

func (l *Logger) Close() error {
	if l.file != nil {
		return l.file.Close()
	}
	return nil
}

func (l *Logger) Log(entry Entry) {
	if entry.Timestamp.IsZero() {
		entry.Timestamp = time.Now()
	}

	if l.signer != nil {
		// Create a hash of the unsigned entry for signature
		entryData, _ := json.Marshal(struct {
			Timestamp time.Time      `json:"timestamp"`
			Actor     string         `json:"actor"`
			Action    string         `json:"action"`
			Target    string         `json:"target,omitempty"`
			Status    string         `json:"status"`
			Details   map[string]any `json:"details,omitempty"`
		}{
			Timestamp: entry.Timestamp,
			Actor:     entry.Actor,
			Action:    entry.Action,
			Target:    entry.Target,
			Status:    entry.Status,
			Details:   entry.Details,
		})

		hash := sha256.Sum256(entryData)
		sig, err := l.signer.Sign(rand.Reader, hash[:], crypto.SHA256)
		if err == nil {
			entry.Signature = sig
		} else {
			slog.Error("failed to sign audit entry", "error", err)
		}
	}

	data, err := json.Marshal(entry)
	if err != nil {
		slog.Error("failed to marshal audit entry", "error", err)
		return
	}

	if _, err := l.file.Write(append(data, '\n')); err != nil {
		slog.Error("failed to write to audit log", "error", err)
	}
}
