package main

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// withVerboseBuffer redirects verbosef output to a bytes.Buffer for the
// duration of the test, then restores the previous writer. Returns the
// buffer for assertions.
func withVerboseBuffer(t *testing.T) *bytes.Buffer {
	t.Helper()
	prev := verboseWriter
	prevStart := startTime
	buf := &bytes.Buffer{}
	verboseWriter = buf
	startTime = time.Now()
	t.Cleanup(func() {
		verboseWriter = prev
		startTime = prevStart
	})
	return buf
}

// withLLMLogBuffer redirects the JSONL log encoder to a bytes.Buffer.
// Skips the lazy-init path (which would touch the filesystem).
func withLLMLogBuffer(t *testing.T) *bytes.Buffer {
	t.Helper()
	prevEnc := llmLog
	prevTokensIn, prevTokensOut := llmInputTokens, llmOutputTokens
	buf := &bytes.Buffer{}
	llmLog = json.NewEncoder(buf)
	llmInputTokens = 0
	llmOutputTokens = 0
	// Mark the lazy-init Once as already-done so logLLMCall doesn't
	// try to open a real log file.
	llmLogOnce.Do(func() {})
	t.Cleanup(func() {
		llmLog = prevEnc
		llmInputTokens = prevTokensIn
		llmOutputTokens = prevTokensOut
	})
	return buf
}

func TestVerbosef_NoOpWhenWriterNil(t *testing.T) {
	prev := verboseWriter
	verboseWriter = nil
	t.Cleanup(func() { verboseWriter = prev })
	// Should not panic, should not write anywhere.
	verbosef("hello %s", "world")
}

func TestVerbosef_WritesWithElapsedPrefix(t *testing.T) {
	buf := withVerboseBuffer(t)
	verbosef("MB work search %q: %d results", "Bach BWV 232", 4)
	got := buf.String()
	if !strings.Contains(got, "MB work search") {
		t.Errorf("verbose output missing message text: %q", got)
	}
	if !strings.HasPrefix(got, "[") || !strings.Contains(got, "] ") {
		t.Errorf("verbose output missing elapsed-time prefix: %q", got)
	}
	if !strings.HasSuffix(got, "\n") {
		t.Errorf("verbose output should end in newline: %q", got)
	}
}

func TestLogLLMCall_WritesJSONLEntry(t *testing.T) {
	buf := withLLMLogBuffer(t)
	logLLMCall(LLMLogEntry{
		Time:         time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC),
		Composer:     "Bach",
		Work:         "Mass in B minor",
		Model:        "claude-opus-4-7",
		Prompt:       "Suggest alternatives.",
		Response:     `{"alternatives":["BWV 232","h-Moll-Messe"]}`,
		InputTokens:  120,
		OutputTokens: 18,
		LatencyMS:    1450,
	})
	line := buf.String()
	// Should be a single JSONL line.
	if strings.Count(line, "\n") != 1 {
		t.Errorf("expected exactly one newline, got %d: %q", strings.Count(line, "\n"), line)
	}
	var got LLMLogEntry
	if err := json.Unmarshal([]byte(line), &got); err != nil {
		t.Fatalf("unmarshal log line: %v", err)
	}
	if got.Composer != "Bach" || got.Work != "Mass in B minor" || got.Model != "claude-opus-4-7" {
		t.Errorf("unexpected log entry: %+v", got)
	}
	if got.InputTokens != 120 || got.OutputTokens != 18 || got.LatencyMS != 1450 {
		t.Errorf("token / latency fields wrong: %+v", got)
	}
}

func TestLogLLMCall_NoOpWhenLogNil(t *testing.T) {
	prev := llmLog
	llmLog = nil
	t.Cleanup(func() { llmLog = prev })
	// Should not panic.
	logLLMCall(LLMLogEntry{Composer: "x"})
}

// TestNormalizeWorkQuery_LogsCallIncludingTokens verifies the
// integration glue: a successful LLM call writes a JSONL entry with the
// usage tokens populated and increments the run-summary counters.
func TestNormalizeWorkQuery_LogsCallIncludingTokens(t *testing.T) {
	buf := withLLMLogBuffer(t)
	server := fakeAnthropic(t, `{"alternatives": ["BWV 232"]}`)
	defer server.Close()

	_, err := normalizeWorkQuery(context.Background(), clientForServer(server), "Bach", "Mass in B minor")
	if err != nil {
		t.Fatal(err)
	}

	var entry LLMLogEntry
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &entry); err != nil {
		t.Fatalf("unmarshal log line: %v (raw: %q)", err, buf.String())
	}
	if entry.Composer != "Bach" || entry.Work != "Mass in B minor" {
		t.Errorf("unexpected log entry: %+v", entry)
	}
	if entry.Prompt == "" {
		t.Error("prompt should be logged")
	}
	if entry.Response != `{"alternatives": ["BWV 232"]}` {
		t.Errorf("response not logged correctly: %q", entry.Response)
	}
	if entry.InputTokens != 100 || entry.OutputTokens != 20 {
		t.Errorf("token usage not logged: in=%d out=%d", entry.InputTokens, entry.OutputTokens)
	}
	if llmInputTokens != 100 || llmOutputTokens != 20 {
		t.Errorf("run-summary counters not updated: in=%d out=%d", llmInputTokens, llmOutputTokens)
	}
}

// TestNormalizeWorkQuery_LogsErrorPath verifies that a parse failure
// still produces a log entry, with the error captured.
func TestNormalizeWorkQuery_LogsErrorPath(t *testing.T) {
	buf := withLLMLogBuffer(t)
	server := fakeAnthropic(t, `definitely not JSON`)
	defer server.Close()

	_, err := normalizeWorkQuery(context.Background(), clientForServer(server), "Bach", "X")
	if err == nil {
		t.Fatal("expected parse error")
	}
	var entry LLMLogEntry
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &entry); err != nil {
		t.Fatalf("unmarshal: %v (raw: %q)", err, buf.String())
	}
	if entry.Error == "" {
		t.Error("expected log entry to have an error message")
	}
	if !strings.Contains(entry.Error, "parse") {
		t.Errorf("error should mention parse failure, got %q", entry.Error)
	}
}
