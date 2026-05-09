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

### 6. Spotify mapping: prefer MB URL relationships, fall back to verified search

**Decision.** For each Recording, use MusicBrainz's `url-rels` Spotify link
when present. When absent, search Spotify (limit=10) with composer + work
+ conductor + orchestra, score each candidate album by whether the
performance's conductor (worth 2) or orchestra (worth 1) appears in the
album's artist credits or its name, and attach the highest-scoring
candidate's URL. If no candidate scores at all, attach nothing — silent
wrongness is worse than honest absence.

**Why verification.** Live testing revealed Spotify's relevance ranking
returns the *least-bad* candidate even when no real match exists. A
blind first-result pick attached "Celestial Bliss" by Felix Lancaster
to a Mozart Mass performance because the album name happened to contain
"Mass". Verification by performer credit closes that hole.

**Trade-offs.**
- (+) Authoritative when MB has the link; honestly-empty otherwise.
- (−) Performances where MB credits a conductor / orchestra that
  Spotify doesn't list in its album metadata will lose their URL even
  when the album is in fact correct. Mitigations would include matching
  against vocal credits or release year, or introducing an LLM-based
  similarity check.

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
- **Cross-edition aggregation, residual gaps.** Increment 4 closed the
  worst version of this limitation by walking MB's `other version`
  relations two hops out and aggregating recordings across the
  resulting set (`expandEditions` in `musicbrainz.go`). `Mozart Great
  Mass in C` now reaches 6 distinct performances (Bernius, Dijkstra,
  Rilling, Harnoncourt, …) across 5 sibling editions of K.427.
  Residual gaps remain: caps of `maxEditionHops = 2` and
  `maxWorksToBrowse = 10` mean very large edition families get
  truncated, and recordings linked to an edition entity that's
  neither in our top search results nor reachable in two hops are
  still hidden.
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
- **Choir vs. soloist heuristic.** Increment 5 split MB's `vocal`
  credits into separate `Choir:` and `Soloists:` lines via a
  name-pattern heuristic (`isChoir` in `performance.go`): names
  containing keywords like Choir / Chor / Kantorei / Singverein /
  Kammerchor / Coro / Cappella / Ensemble / Cathedral are classified
  as choirs; everything else is a soloist. The MB-correct alternative
  — fetching each artist's entity to read its `type` field
  (Choir vs Person) — would mean one extra API call per vocal credit
  per recording, which is too expensive for the value. The heuristic
  will mis-classify edge cases (e.g. a vocal Ensemble that's actually
  a chamber group, or a choir whose name contains none of the
  keywords).
- **No Spotify URL for sparsely-credited performances.** The Spotify
  match-verification step (decision 6) requires the performance's
  conductor or orchestra to appear in the candidate album's artist
  credits or name. Performances credited only to soloists — no
  conductor, no orchestra — therefore produce a verification score of
  0 and get no URL attached, even when the recording is in fact on
  Spotify. This replaces an earlier worse failure mode where two such
  performances would share a wrong URL. Mitigations would be to
  include soloist names in the verification, or to use Spotify's
  stricter DSL filters with retries.
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
