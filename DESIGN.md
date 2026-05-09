# Design Decisions

This document captures the design choices behind the classical-music search
behavior of `classical`. Each section states the decision, the alternatives
considered, and the trade-off that drove the choice.

## Problem

Spotify's data model is track-centric: the track is the unit, with album and
artist as secondary attributes. Classical music does not fit this model. A
user typically wants a specific *recording* of a *work*, where:

- A work (e.g. Mozart's Great Mass in C minor, K.427) usually spans multiple
  tracks (Kyrie, Gloria, Credo, ...).
- Many recordings of the same work exist, distinguished by conductor,
  orchestra/choir, and year.
- Work titles are inconsistent across albums and languages
  ("Great Mass in C minor", "Mass in C minor K.427", "Messe in c-Moll").

A track-level Spotify search returns a flat, unstructured list that doesn't
match the way classical listeners think about repertoire.

## Decisions

### 1. Use MusicBrainz as the canonical-identity layer

**Decision.** Resolve the user's query to a MusicBrainz Work, then fetch the
Work's Recordings (with conductor / orchestra / year) from MusicBrainz. Use
MusicBrainz IDs and catalog numbers (K., BWV, Op.) as stable join keys
through the pipeline.

**Alternatives considered.**
- *Spotify-only with heuristics.* Search Spotify, cluster by
  conductor/orchestra, regex-match catalog numbers in titles. Simpler — no new
  dependency — but the accuracy ceiling is low because the underlying metadata
  is the problem we're trying to fix.
- *Hand-curated local catalog.* Maintain a JSON list of works we care about.
  Highest precision for covered works, zero coverage outside the list.
  Reasonable if listening were concentrated on a known repertoire, but
  limits discovery.

**Trade-offs.**
- (+) Stable, citable IDs; canonical work titles; explicit Recording entities
  with performer credits and dates.
- (+) Free, no API key.
- (−) Public API is rate-limited to 1 req/sec and requires a descriptive
  `User-Agent`.
- (−) Coverage gaps, especially for very recent recordings — MusicBrainz lags
  Spotify.

### 2. Free-text search expecting composer + work

**Decision.** Users type a free-text query like `"Mozart Great Mass in C"`.
The CLI expects the query to identify a specific work (composer + work name).

**Alternatives considered.**
- *Browse-by-composer.* Pick composer → list works → pick work → list
  recordings. Rejected: prolific composers (Bach, Mozart) have too many works
  for a useful flat list.
- *Both flows.* Adds complexity without proportional value at this stage.

**Trade-offs.**
- (+) Matches how users already think about classical repertoire.
- (+) Minimal CLI ergonomics — no interactive prompts.
- (−) Ambiguous queries (e.g. "Mozart Mass in C" matches K.317 *and* K.427)
  need handling — see decision 5.

### 3. Defer LLM-based query normalization

**Decision.** Pass the user's query directly to MusicBrainz's search. Do not
introduce an LLM layer for query understanding yet.

**Alternatives considered.**
- *LLM-only pipeline* (skip MusicBrainz). The LLM both normalizes the query
  and identifies recordings. Rejected: no stable identity layer, will
  hallucinate edge-case recordings, and is bounded by training cutoff.
- *Hybrid with LLM upfront.* The LLM extracts `{composer, work, catalog
  number}` from free-text and MusicBrainz looks up the rest. Likely the right
  long-term shape, but adds a dependency before we know whether MB's own
  fuzzy search is good enough.

**Trade-offs.**
- (+) Zero new dependencies for the first version. We can measure how often
  MB's search alone fails before paying for an LLM.
- (−) MB's Lucene-style search will struggle on multilingual / colloquial
  names ("Krönungsmesse", "St Matthew Passion" vs "Matthäus-Passion"). We
  accept this for the first version.

### 4. Replace Spotify track search; don't preserve it

**Decision.** The work-oriented search becomes the primary CLI behavior. The
existing track-search code is removed.

**Rationale.** The track-search behavior is the wrong shape for the use case.
Dual modes would complicate the CLI without serving a real need. Easy to add
back later behind a flag if wanted.

### 5. Show all matched Works and aggregate their recordings

**Decision.** When a query resolves to N Works, display all of them (with
composer + catalog number) and then browse recordings for the top
`maxWorksToBrowse`, aggregating the results into a single Performance
list. The user sees both the breadth of MB matches and the actual
playable recordings in one response.

**Alternatives considered.**
- *Print Works and exit on ambiguity.* The original draft of this
  decision. Rejected during increment 2: classical works in MB are
  typically split across several "edition" entities (Maunder / Levin /
  fragment for K.427), and forcing the user to re-query for each
  edition fragments the result for what is effectively one piece.
- *Auto-pick the top-scoring Work.* Risk of showing only the top
  edition's recordings — which can be empty (Maunder's K.427 has zero
  recordings linked in MB) — while the productive editions are
  silently hidden. Aggregating across matches sidesteps the ranking
  question.
- *Interactive disambiguation prompt.* Cleaner UX for genuinely
  ambiguous cases but breaks scripting and adds a TTY dependency.

**Trade-offs.**
- (+) Useful output even for ambiguous queries; the displayed Works
  section is the disambiguation signal if the user needs it.
- (+) Predictable, scriptable, no hidden state.
- (−) Genuinely distinct works that match the same query (e.g. K.317
  *and* K.427 both match `Mozart Mass in C`) merge into one recording
  list; the user has to read titles in the Works section to tell
  which Performance belongs to which Work.

### 6. Spotify mapping: prefer MB URL relationships, fall back to search

**Decision.** For each Recording, use MusicBrainz's `url-rels` Spotify link
when present. When absent, search Spotify with composer + work + conductor +
orchestra and pick the best string-similarity match against the album title.

**Trade-offs.**
- (+) Authoritative when MB has the link.
- (−) Fallback search will sometimes pick the wrong album or miss the
  recording entirely. Accepted as a known limitation; scoring can be improved
  later, including by introducing an LLM (see decision 3).

### 7. Group recordings by performer fingerprint

**Decision.** Dedupe Recordings by `(conductor, ensemble, year)` and sort by
year descending. Catalog number remains the join key through the pipeline.

## Known limitations

Things we've observed or accepted as trade-offs in the current
implementation. Listed centrally so future work can pick them up
explicitly.

- **Multilingual / colloquial work titles.** A query like `Bach Mass in B
  minor` returns no results because MB's canonical title for that work is
  `h-Moll-Messe, BWV 232` and none of the English words appear in it.
  Workarounds: use the catalog number (`Bach BWV 232`) or the canonical
  title (`Bach h-Moll-Messe`). The long-term fix is the deferred LLM
  normalization layer (decision 3).
- **Cross-edition aggregation.** MB models a single classical work as
  several sibling "edition" Works (e.g. K.427 has Maunder, Levin, and the
  original-fragment "Große Messe" as separate MBIDs) linked by
  `other version` relations. Recordings link to whichever edition their
  album metadata cites. We don't follow those relations. Concrete
  consequence: `Mozart Great Mass in C` finds the Rilling / Levin
  recording but misses Karajan and Gardiner, whose recordings are linked
  to a different edition Work that doesn't surface in our top matches.
- **Composer-as-first-token heuristic.** `buildQuery` treats the first
  shell arg (or, with a single arg, the first whitespace-separated token)
  as the composer surname. Multi-word composers must be shell-quoted as
  one arg, e.g. `./classical "Wolfgang Amadeus Mozart" "Great Mass in C"`.
- **Top-5 cap on Works to browse.** The recording-browse phase visits at
  most the top 5 matched Works, both to bound latency and to stay inside
  MB's 1 req/sec public rate limit. Recordings linked to a 6th+ matched
  edition are not surfaced.
- **Movement filter is heuristic.** We drop Work entries whose title
  contains `": "` or `" from "`, on the assumption that those are
  movement-style sub-works. The risk is a false positive on a parent
  work whose canonical title genuinely contains either substring (none
  observed yet).
- **MB recording sparsity.** Some MB recordings have no `conductor`, no
  `performing orchestra`, or no `first-release-date`. Such recordings
  collapse into a single `(no conductor credited) / ????` Performance
  bucket, which can mask distinct performances when MB is unevenly
  populated.
- **Choir vs. soloist conflation.** MB uses one relation type (`vocal`)
  for both choirs and individual vocal soloists, with no further
  distinguishing field on the relation itself. Both appear together
  under a shared `Vocal:` line in the output. Distinguishing them would
  require a follow-up lookup on each artist's MB entity type
  (Choir vs Person).
- **Same Spotify URL for sparsely-credited performances.** The Spotify
  search query is built from `(composer, work, conductor, orchestra)`
  with empty fields skipped. Two distinct performances that both lack
  a conductor *and* an orchestra (e.g. recordings credited only to
  soloists) produce identical search queries, and Spotify's relevance
  ranking returns the same top album for both — so they end up sharing
  a Spotify URL even though they're different performances. Observed
  live in `./classical Bach BWV 232` for the two no-conductor entries.
  Mitigations would be to include soloist names in the query, or to
  switch from free-text search to Spotify's stricter DSL filters
  (`album:` + `artist:`).
- **Lucene escaping is partial.** `buildQuery` strips `"`, `(`, and `)`
  from user input so they can't break our `field:(...)` wrapping. Other
  Lucene-special characters (`+ - && || ! { } [ ] ^ ~ * ? : \ /`) pass
  through unsanitised. Unlikely to bite for classical-music names but
  worth knowing if a query suddenly errors.

## Out of scope (for now)

- **Playback** (queueing tracks in Spotify). Requires user OAuth
  (authorization-code flow, not client-credentials). Deferred until the search
  side is solid.
- **Playlist creation.**
- **LLM-based query normalization** — see decision 3.
- **Browse-by-composer flow** — see decision 2.
- **Evaluation harness / golden set.** Useful for comparing implementations
  but not gating; can be added when we want to measure changes.
