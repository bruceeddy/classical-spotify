package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

// startTime is captured at process start so verbose lines can show
// elapsed-since-startup. Initialised in main.
var startTime time.Time

// verboseWriter is the destination for `-v` pipeline trace output.
// nil means verbose output is disabled — verbosef is a no-op.
var verboseWriter io.Writer

// llmLog and llmLogFile are the JSONL append-target for LLM call
// records. Lazily initialised on the first logLLMCall — no log file
// is created if no LLM call ever happens. nil means the lazy init
// failed (e.g. permission denied) and we silently skip logging.
var (
	llmLogOnce sync.Once
	llmLog     *json.Encoder
	llmLogFile *os.File
	llmLogMu   sync.Mutex
)

// llmInputTokens / llmOutputTokens accumulate over the whole run for
// the end-of-run summary. Single-goroutine, so plain ints are fine.
var (
	llmInputTokens  int64
	llmOutputTokens int64
)

// LLMLogEntry is one line in the JSONL log file.
type LLMLogEntry struct {
	Time         time.Time `json:"time"`
	Composer     string    `json:"composer"`
	Work         string    `json:"work"`
	Model        string    `json:"model"`
	Prompt       string    `json:"prompt"`
	Response     string    `json:"response"`
	InputTokens  int64     `json:"input_tokens"`
	OutputTokens int64     `json:"output_tokens"`
	LatencyMS    int64     `json:"latency_ms"`
	Error        string    `json:"error,omitempty"`
}

// initLLMLog opens the JSONL log file. Path comes from
// CLASSICAL_LOG_FILE if set, else "./classical.jsonl" in the cwd.
// On open failure we surface a stderr note and leave llmLog nil so
// subsequent logLLMCall calls become no-ops.
func initLLMLog() {
	path := os.Getenv("CLASSICAL_LOG_FILE")
	if path == "" {
		path = "classical.jsonl"
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Note: could not open LLM log %s: %v\n", path, err)
		return
	}
	llmLogFile = f
	llmLog = json.NewEncoder(f)
}

// logLLMCall writes one JSONL entry. Lazy-inits the log file on first
// call. Safe to call concurrently.
func logLLMCall(entry LLMLogEntry) {
	llmLogOnce.Do(initLLMLog)
	if llmLog == nil {
		return
	}
	llmLogMu.Lock()
	defer llmLogMu.Unlock()
	_ = llmLog.Encode(entry)
}

// closeLLMLog releases the log file handle. Safe to call multiple
// times; safe if the log was never opened.
func closeLLMLog() {
	if llmLogFile != nil {
		_ = llmLogFile.Close()
		llmLogFile = nil
	}
}

// verbosef writes a printf-style line to verboseWriter, prefixed with
// elapsed time from startTime. No-op if verboseWriter is nil.
func verbosef(format string, args ...any) {
	if verboseWriter == nil {
		return
	}
	fmt.Fprintf(verboseWriter, "[%6s] ", time.Since(startTime).Round(time.Millisecond))
	fmt.Fprintf(verboseWriter, format, args...)
	if len(format) == 0 || format[len(format)-1] != '\n' {
		fmt.Fprintln(verboseWriter)
	}
}

// addLLMUsage records token usage for the end-of-run summary.
func addLLMUsage(in, out int64) {
	llmInputTokens += in
	llmOutputTokens += out
}
