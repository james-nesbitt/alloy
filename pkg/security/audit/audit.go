package audit

import (
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
}

type Logger struct {
	file *os.File
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

	data, err := json.Marshal(entry)
	if err != nil {
		slog.Error("failed to marshal audit entry", "error", err)
		return
	}

	if _, err := l.file.Write(append(data, '\n')); err != nil {
		slog.Error("failed to write to audit log", "error", err)
	}
}
