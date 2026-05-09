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

Increment 4 was originally drafted as "grouping / display polish," but
grouping had to happen in Increment 2 to print anything useful and
display was iterated through 2 and 3 — so the original Increment 4 is
largely already done. Three reasonable directions for what to do next:

1. **Display / UX polish.** Truncate long vocal lists; add a flag for
   verbose vs compact output; cap displayed performances; colorise
   headings. Small and contained.
2. **Address a known limitation from `DESIGN.md`.** Most impactful
   candidates:
   - *Cross-edition aggregation* — follow MB `other version` relations
     so a query for K.427 surfaces Karajan and Gardiner (currently
     hidden because their recordings link to a different edition
     entity).
   - *LLM query normalization* (deferred per decision 3) — would fix
     `Bach Mass in B minor` returning nothing.
3. **Cross into "Out of scope."** Playback (Spotify Player API + user
   OAuth, authorization-code flow rather than client-credentials) or
   playlist creation. Bigger lift, different auth shape.

For the full deferred list, see `DESIGN.md`'s `## Out of scope` section.
