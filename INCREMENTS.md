# Increments

A running record of what's been built and what could come next. Design
rationale and known limitations live in [`DESIGN.md`](DESIGN.md); this
file is about progress.

## Completed

### 1. MusicBrainz Work search

Replaced the original Spotify-track search with MB's `/ws/2/work`
endpoint. Refined to:

- filter movements (drop entries with empty `type` or whose title
  contains `": "` / `" from "`), and
- use AND-grouped Lucene queries (`artist:X AND work:(a AND b)`) so
  canonical-title variants like `h-Moll-Messe` still match.

Commits: `da2ce5e`, `a950822`.

### 2. Recording browse and grouping

For each matched Work, browse `/ws/2/recording?work=<MBID>` with
`inc=artist-credits artist-rels`, dedupe to Performances keyed on
`(conductor, orchestra, year)`, merge `vocal` credits across movements
of one performance, sort year-descending. Top 5 Works are visited;
calls are spaced 1s apart for MusicBrainz's public rate limit.

Commit: `410a1df`.

### 3. Spotify URL mapping

#### 3a — MB-supplied URLs

Added `url-rels` to the recording-browse `inc` parameter. Any
`spotify.com` URL on a recording propagates through grouping to the
Performance display. In practice MB's url-rels coverage for Spotify is
essentially zero on classical recordings, so this landed as scaffolding
for 3b.

Commit: `3be1a1a`.

#### 3b — Spotify-search fallback

Client-Credentials auth (`POST /api/token`) + album search
(`GET /v1/search?type=album`) using `composer + work + conductor +
orchestra` as the query. The first hit's URL is attached to the
Performance. Reads `SPOTIFY_CLIENT_ID` / `SPOTIFY_CLIENT_SECRET` from
the env; without credentials the step is skipped with a stderr `Note:`
and the tool continues MusicBrainz-only.

Commit: `3ec4d9f`.

### 4. Cross-edition aggregation

Walks MusicBrainz's `other version` relation graph two hops out from
each matched Work and aggregates recordings across the resulting set.
Closes the worst case of the cross-edition gap noted in `DESIGN.md`'s
Known Limitations: a query for Mozart's K.427 used to find only the
Rilling / Levin recording (because the canonical search returned the
Maunder + Levin editions and only Levin had recordings linked); after
expansion it reaches 6 distinct performances across 5 sibling
editions, including Harnoncourt, Bernius, and Dijkstra. Bach BWV 232's
parent Work has no `other version` edges, so its result is unchanged.

Capped at `maxEditionHops = 2` and `maxWorksToBrowse = 10` to keep
the API budget under MusicBrainz's 1 req/sec ceiling. A note in
the output ("(N additional Work(s) reached via MusicBrainz 'other
version' relations.)") makes the expansion transparent.

Commit: `afe36a8`.

### 5. Display polish: choir/soloist split and record label

#### 5a — Choir / soloist split

MusicBrainz tags both choirs and individual vocal soloists with the
same `vocal` relation type. They used to share a single `Vocal:`
output line which the user had to mentally untangle. Increment 5a
introduces an `isChoir(name)` heuristic — a regex over multilingual
choir-naming keywords (Choir / Chor / Kantorei / Singverein /
Kammerchor / Coro / Cappella / Cathedral / Ensemble / Singers /
Voices) — and splits the credits into separate `Choir:` and
`Soloists:` output lines.

`Performance.Vocals` was replaced by `Performance.Choirs` and
`Performance.Soloists`; `groupRecordings` classifies each `vocal`
relation at extraction time and merges via a new `mergeUnique`
helper.

Commit: `d508e81`.

#### 5b — Record label

Spotify's simplified album object (returned by `/v1/search`) doesn't
include the `label` field. Increment 5b adds a follow-up batch lookup
against `/v1/albums?ids=…` after the per-performance search
completes; one HTTP call covers up to 20 albums. The retrieved label
is stored on `Performance.Label` and displayed on a new `Label:`
output line. `SpotifyAlbum` gained an `ID` field so we can capture
the album ID at search time and feed it into the lookup.

Commit: `fec345c`.

### 6. Composer query precision

Two related fixes for a broader class of query bug discovered when
testing `./classical Rimsky-Korsakov Scheherazade`, which used to
return zero results.

#### 6a — Full Lucene-special-character sanitisation

The previous `sanitizeLucene` only stripped `"`, `(`, and `)`. A bare
hyphen — Lucene's NOT operator — broke any query containing a
hyphenated composer name: `artist:Rimsky-Korsakov` parsed as
"Rimsky NOT Korsakov" and returned zero. The sanitiser now replaces
the full set of Lucene-special characters (`+ - && || ! ( ) { } [ ]
^ " ~ * ? : \ /`) with single spaces and collapses runs, preserving
word boundaries — `Rimsky-Korsakov` becomes the AND-grouped
`(Rimsky AND Korsakov)`. Also unblocks Saint-Saëns and any other
composer with hyphens or other specials in their name.

Commit: `84fb367`.

#### 6b — Composer MBID resolution via the artist endpoint

Even after 6a, `artist:Rimsky AND Korsakov` returned zero — MB stores
the composer's canonical name in Cyrillic (`Николай Андреевич
Римский‐Корсаков`) and the work-search `artist:` field doesn't
follow Latin aliases. New `resolveComposerMBID` (in `musicbrainz.go`)
calls `/ws/2/artist/?query=…`, picks the highest-scoring Person
whose disambiguation marks them as a composer, and returns their
MBID. `buildQuery` then uses `arid:<MBID>` instead of `artist:<name>`,
which bypasses name-matching entirely.

The disambiguation filter (`isComposerDisambiguation`) is critical:
it accepts "Russian composer", "classical composer", "composer"
alone, but rejects "daughter of the composer" / "musicologist, son
of the composer", which Maria and Andrey Rimsky-Korsakov carry and
which would otherwise outrank Nikolai by score (99 / 73 vs his 94).

Live: `./classical Rimsky-Korsakov Scheherazade` now returns 21
recordings, including Muti / Philadelphia (Warner Classics),
Goossens / LSO (Everest), and several Sony Classical releases.

Commit: `847b7f2`.

### 7. LLM fallback for canonical-name work-query mismatches

Closes the worst case of the multilingual work-title gap noted in
`DESIGN.md`'s known limitations. MB indexes works under their original-
language canonical title, so an English query like `Bach Mass in B
minor` returned zero results because MB has it as `h-Moll-Messe, BWV
232`. Increment 7 adds an LLM-fallback step: when the initial work
search returns zero results AND `ANTHROPIC_API_KEY` is set, Claude
(Opus 4.7, adaptive thinking) is asked for alternative work-search
terms — canonical foreign-language titles, catalog numbers, common
aliases — and the search is retried with each. The first non-empty
alternative wins.

The LLM is never on the happy path: queries that work directly
(catalog-number queries, exact-canonical-title queries, queries the
composer-MBID resolution from increment 6 already handles) bypass it
entirely. Without `ANTHROPIC_API_KEY` set the fallback is skipped and
the user sees "No works found" exactly as before — no regression.

Implementation uses the official Anthropic Go SDK
(`github.com/anthropics/anthropic-sdk-go`), added as a dependency.
Tests are hermetic via `httptest` + `option.WithBaseURL` to inject the
test server URL into the SDK client.

Commit: `3f91904`.

### 8. Observability

Adds five observability surfaces so the user can audit Claude usage,
diagnose latency, and debug Spotify match scoring without adding
print statements.

1. **LLM call log (always-on JSONL).** Every call to
   `normalizeWorkQuery` writes a structured entry — timestamp,
   composer, work, full prompt, response text, model, input/output
   tokens, latency, error — to `./classical.jsonl`. Path overridable
   via `CLASSICAL_LOG_FILE`. Lazily opened on first call, so the
   file never appears for users without an Anthropic key.
2. **`-v` pipeline trace (stderr).** Each pipeline stage prints
   timing and result counts: composer-MBID resolution (with all
   candidate artists, type, score, disambiguation), MB work search,
   edition expansion (per hop), recording browse, Spotify auth /
   search / label batch — each line prefixed with elapsed-since-
   startup so the user can see where time is going.
3. **End-of-run summary (always on).** One stderr line at exit:
   `Done in 2.99s` or, when an LLM call happened,
   `Done in 8.4s (Claude: 350 in / 52 out tokens)`. Surfaces cost
   without grep.
4. **Spotify match-score trace (under `-v`).** `bestMatchingAlbum`
   logs every candidate album with its score and artist list, then
   the picked album or a clear "no candidate verified" line. Would
   have flagged the Celestial-Bliss / Rain-Sounds-Symphony failures
   at runtime instead of click-through-and-discover.
5. **Composer-resolution trace (under `-v`).** `resolveComposerMBID`
   logs all artist candidates with their disambiguation strings so
   the user can see surprises like J.S. Bach vs C.P.E. Bach being
   picked.

Implementation: new `logging.go` (~80 LOC) plus light instrumentation
in each per-API-call function. Tests redirect both writers to
`bytes.Buffer` so the suite never touches the real filesystem or
stderr.

Commit: `2c169e1`.

## Cross-cutting

Alongside the numbered increments:

- **Refactor.** Extracted the duplicated MB HTTP boilerplate into
  `mbGet`. Split the package into files by concern (`main.go`,
  `musicbrainz.go`, `query.go`, `performance.go`, `spotify.go`) with
  mirrored test files.
- **DESIGN.md.** Initial design doc, "Known limitations" section,
  later correction (decision 5 rewritten to match the shipped
  show-and-aggregate behaviour).
- **README.md.** Two rewrites (after Increment 1+2, after Increment 3)
  to track actual behaviour, plus a `## Running the tests` section.
- **Test suite.** Hermetic, `httptest.Server`-based. Closed gaps in
  `parseQueryArgs` and `fillSpotifyURLs`-missing-creds coverage.

## To come

Reasonable next directions:

1. **Refine composer-disambiguation heuristic.** Composers whose MB
   `disambiguation` is empty or doesn't include "composer" don't
   resolve via `resolveComposerMBID` and fall back to free-text. Could
   refine by adding a tier-2 fallback (first Person if no composer-
   disambiguated Person exists), or by checking work-count signals.
2. **More display polish.** Truncate long soloist lists; colorise
   headings; visually group the Works section into "matched" vs
   "reached via expansion". (The verbose-mode plumbing from increment
   8 covers the diagnostics use case; this is purely the
   user-facing-display side.)
3. **Recover URLs for sparsely-credited performances.** Extend
   `albumMatchScore` to also match against soloist names (or use
   Spotify's stricter DSL with retries), so recordings credited only
   to soloists can still get a verified Spotify URL.
4. **Extend the LLM fallback's reach.** Currently the fallback fires
   only on *empty* MB results. Queries that return *irrelevant* hits
   (rather than zero) won't trigger it. Could extend by also asking
   the LLM when the top result's score is below a threshold, or by
   running it always for cost-tolerant deployments.
5. **Observability follow-ups.** Log rotation (the JSONL file grows
   indefinitely); cost-in-dollars in the end-of-run summary
   (currently shows tokens only); per-call MB / Spotify request logs
   to file (only LLM calls are file-logged today — `-v` covers the
   stderr side).
6. **Cross into "Out of scope."** Playback (Spotify Player API + user
   OAuth, authorization-code flow rather than client-credentials) or
   playlist creation. Bigger lift, different auth shape.

For the full deferred list, see `DESIGN.md`'s `## Out of scope` section.
