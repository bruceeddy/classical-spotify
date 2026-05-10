package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
)

// llmModel pins the model used for query normalization. Opus 4.7 with
// `effort: low` and adaptive thinking is well-matched to a small,
// structured-extraction task — the cost is a fraction of a cent per
// query.
const llmModel = anthropic.ModelClaudeOpus4_7

const (
	llmMaxTokens     = 500
	llmMaxAlternates = 5
)

// workAlternativesResponse is the JSON shape we ask the LLM to emit.
type workAlternativesResponse struct {
	Alternatives []string `json:"alternatives"`
}

// normalizeWorkQuery asks the LLM for alternative work-search terms when
// the original MusicBrainz query found nothing. The LLM's job is to
// suggest what MB might index the same piece under — canonical
// foreign-language titles, catalog numbers, common aliases.
//
// Returns the list of suggested alternatives (deduped, original-query
// removed). The caller retries each alternative against MusicBrainz
// until one returns results.
//
// `client` is taken as a parameter so tests can construct one with a
// custom base URL (`option.WithBaseURL`) and point at httptest.
func normalizeWorkQuery(ctx context.Context, client anthropic.Client, composer, work string) ([]string, error) {
	prompt := fmt.Sprintf(`A MusicBrainz search for "%s %s" returned no results. MusicBrainz canonicalises classical works under their original-language title or catalog number, so English titles often miss. Suggest up to %d alternative work-search terms.

Examples:
- "Bach Mass in B minor" -> ["BWV 232", "h-Moll-Messe"]
- "Mozart Coronation Mass" -> ["K. 317", "Krönungs-Messe"]
- "Bach St Matthew Passion" -> ["BWV 244", "Matthäus-Passion"]
- "Mussorgsky Pictures at an Exhibition" -> ["Картинки с выставки", "Pictures at an Exhibition"]

Composer: %s
Original work query: %s

Output JSON ONLY in this exact shape, no surrounding prose:
{"alternatives": ["title1", "title2", "BWV 123"]}

If you can't suggest anything useful, return {"alternatives": []}.`,
		composer, work, llmMaxAlternates, composer, work)

	entry := LLMLogEntry{
		Time:     time.Now(),
		Composer: composer,
		Work:     work,
		Model:    string(llmModel),
		Prompt:   prompt,
	}
	defer func() { logLLMCall(entry) }()

	verbosef("LLM normalize: composer=%q work=%q (model=%s)", composer, work, llmModel)
	start := time.Now()
	resp, err := client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     llmModel,
		MaxTokens: llmMaxTokens,
		Thinking: anthropic.ThinkingConfigParamUnion{
			OfAdaptive: &anthropic.ThinkingConfigAdaptiveParam{},
		},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(prompt)),
		},
	})
	entry.LatencyMS = time.Since(start).Milliseconds()
	if err != nil {
		entry.Error = err.Error()
		return nil, fmt.Errorf("anthropic call: %w", err)
	}

	var text string
	for _, block := range resp.Content {
		if tb, ok := block.AsAny().(anthropic.TextBlock); ok {
			text += tb.Text
		}
	}
	text = strings.TrimSpace(text)
	entry.Response = text
	entry.InputTokens = resp.Usage.InputTokens
	entry.OutputTokens = resp.Usage.OutputTokens
	addLLMUsage(entry.InputTokens, entry.OutputTokens)
	verbosef("LLM normalize done: %dms, %d in / %d out tokens", entry.LatencyMS, entry.InputTokens, entry.OutputTokens)

	if text == "" {
		entry.Error = "no text content"
		return nil, fmt.Errorf("LLM returned no text content")
	}

	var out workAlternativesResponse
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		entry.Error = fmt.Sprintf("parse: %v", err)
		return nil, fmt.Errorf("could not parse LLM JSON: %v (raw response: %q)", err, text)
	}

	// Dedupe and remove the original query (it already failed).
	seen := map[string]bool{strings.ToLower(work): true}
	uniq := make([]string, 0, len(out.Alternatives))
	for _, alt := range out.Alternatives {
		alt = strings.TrimSpace(alt)
		if alt == "" {
			continue
		}
		key := strings.ToLower(alt)
		if seen[key] {
			continue
		}
		seen[key] = true
		uniq = append(uniq, alt)
	}
	return uniq, nil
}

// llmCredsAvailable reports whether ANTHROPIC_API_KEY is set, so callers
// can short-circuit without constructing a client that would fail at the
// first request.
func llmCredsAvailable() bool {
	return os.Getenv("ANTHROPIC_API_KEY") != ""
}
