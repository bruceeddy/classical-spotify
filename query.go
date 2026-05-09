package main

import (
	"fmt"
	"strings"
)

// buildQuery turns the user's args into a structured MusicBrainz Lucene
// query. If the user supplies multiple shell args, the first is the
// composer and the rest is the work name. If a single arg is supplied,
// it's split on whitespace under the same convention. A single word is
// passed through unstructured.
//
// Multi-word fields are joined with AND inside parens rather than wrapped
// as a quoted phrase: a quoted phrase requires the words consecutively in
// the indexed title and misses canonical titles whose word-order or
// language differs (e.g. "h-Moll-Messe" for Bach's Mass in B minor),
// while AND requires every word but tolerates surrounding tokens.
func buildQuery(args []string) string {
	var composer, work string
	if len(args) >= 2 {
		composer = args[0]
		work = strings.Join(args[1:], " ")
	} else {
		parts := strings.Fields(args[0])
		if len(parts) < 2 {
			return args[0]
		}
		composer = parts[0]
		work = strings.Join(parts[1:], " ")
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

// sanitizeLucene removes characters from user input that have special
// meaning in Lucene queries and would otherwise break our `field:(...)`
// wrapping.
func sanitizeLucene(s string) string {
	for _, c := range []string{`"`, `(`, `)`} {
		s = strings.ReplaceAll(s, c, "")
	}
	return s
}
