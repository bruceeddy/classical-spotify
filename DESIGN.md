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

### 5. Print-and-exit on ambiguous queries

**Decision.** When a query resolves to multiple plausible Works, print all of
them (with composer + catalog number) and exit. The user re-queries with more
specificity (e.g. `"Mozart Mass in C K.427"`).

**Alternatives considered.**
- *Interactive disambiguation prompt.* Cleaner UX but breaks scripting and
  adds a TTY dependency.
- *Pick the top match silently.* Risk of acting on the wrong work — frustrating
  when wrong, opaque when right.

**Trade-offs.**
- (+) Predictable, scriptable, no hidden state.
- (−) Two-step interaction for ambiguous queries.

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

## Out of scope (for now)

- **Playback** (queueing tracks in Spotify). Requires user OAuth
  (authorization-code flow, not client-credentials). Deferred until the search
  side is solid.
- **Playlist creation.**
- **LLM-based query normalization** — see decision 3.
- **Browse-by-composer flow** — see decision 2.
- **Evaluation harness / golden set.** Useful for comparing implementations
  but not gating; can be added when we want to measure changes.
