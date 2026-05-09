package main

import (
	"fmt"
	"strings"
)

// parseQueryArgs splits the user's shell args into a (composer, work)
// pair. Multiple args: the first is the composer, the rest joined is
// the work. Single arg: split on whitespace and apply the same rule;
// if the result is one token, only composer is set and work is empty.
func parseQueryArgs(args []string) (composer, work string) {
	if len(args) == 0 {
		return
	}
	if len(args) >= 2 {
		composer = args[0]
		work = strings.Join(args[1:], " ")
		return
	}
	parts := strings.Fields(args[0])
	if len(parts) < 2 {
		composer = args[0]
		return
	}
	composer = parts[0]
	work = strings.Join(parts[1:], " ")
	return
}

// buildQuery turns the user's args into a structured MusicBrainz Lucene
// query. A single-token query is passed through unstructured.
//
// Multi-word fields are joined with AND inside parens rather than wrapped
// as a quoted phrase: a quoted phrase requires the words consecutively in
// the indexed title and misses canonical titles whose word-order or
// language differs (e.g. "h-Moll-Messe" for Bach's Mass in B minor),
// while AND requires every word but tolerates surrounding tokens.
func buildQuery(args []string) string {
	composer, work := parseQueryArgs(args)
	if work == "" {
		return composer
	}
	return fmt.Sprintf(`artist:%s AND work:%s`,
		luceneAndGroup(sanitizeLucene(composer)),
		luceneAndGroup(sanitizeLucene(work)))
}

// luceneAndGroup joins the words in s with AND and wraps them in parens
// so the field constraint applies to every word. A single word is left
// bare.
func luceneAndGroup(s string) string {
	words := strings.Fields(s)
	if len(words) == 1 {
		return words[0]
	}
	return "(" + strings.Join(words, " AND ") + ")"
}

// sanitizeLucene replaces every Lucene-special character in s with a
// single space and collapses runs of whitespace. The replacement
// preserves word boundaries — important for hyphenated composer names
// like "Rimsky-Korsakov" or "Saint-Saëns", where a bare hyphen is
// Lucene's NOT operator and would otherwise turn `artist:Rimsky-Korsakov`
// into "Rimsky NOT Korsakov" and return zero results.
//
// The full set of Lucene-special characters is:
//
//	+ - && || ! ( ) { } [ ] ^ " ~ * ? : \ /
//
// We treat each as whitespace; word-level operators (AND / OR / NOT in
// uppercase) are left alone since users don't typically type them.
func sanitizeLucene(s string) string {
	const specials = `"()-+!^~*?:\/{}[]&|`
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if strings.ContainsRune(specials, r) {
			b.WriteRune(' ')
		} else {
			b.WriteRune(r)
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}
