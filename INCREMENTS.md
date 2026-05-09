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

1. **LLM query normalization** (deferred per decision 3 in `DESIGN.md`).
   Most-impactful remaining limitation: would fix `Bach Mass in B minor`
   returning nothing because MusicBrainz's canonical title is
   `h-Moll-Messe`. Adds a third optional credential
   (`ANTHROPIC_API_KEY`) and one external network call per query.
2. **More display polish.** Truncate long soloist lists; add a `-v`
   flag for verbose vs compact output; colorise headings; visually
   group the Works section into "matched" vs "reached via expansion".
3. **Recover URLs for sparsely-credited performances.** Extend
   `albumMatchScore` to also match against soloist names (or use
   Spotify's stricter DSL with retries), so recordings credited only
   to soloists can still get a verified Spotify URL.
4. **Cross into "Out of scope."** Playback (Spotify Player API + user
   OAuth, authorization-code flow rather than client-credentials) or
   playlist creation. Bigger lift, different auth shape.

For the full deferred list, see `DESIGN.md`'s `## Out of scope` section.
