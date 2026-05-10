# CLAUDE.md

Project guidance for Claude Code working in this repo. Loaded on every
session — kept tight; full design rationale lives in
[`DESIGN.md`](DESIGN.md) and the progress log in
[`INCREMENTS.md`](INCREMENTS.md).

## What this is

A Go CLI that takes a free-text classical-music query
(`./classical Mozart Great Mass in C`), resolves the composer to a
MusicBrainz MBID, finds the matching Work plus its sibling editions,
browses recordings, groups them into Performances by
`(conductor, orchestra, year)`, and — when Spotify creds are set —
attaches a verified Spotify album URL and record label to each.
Falls back to Claude when MusicBrainz can't match the work title
(canonical-name issues like `Bach Mass in B minor` →
`h-Moll-Messe, BWV 232`). Personal project.

## Working conventions

- **Three docs stay in sync with code.** When a functional change ships,
  update `DESIGN.md` (rationale + known limitations), `INCREMENTS.md`
  (progress log), and `README.md` (user-facing) to match. Reframe
  affected entries — "deferred" becomes "shipped"; "open limitation"
  becomes "residual gap"; numbered "To come" candidates get
  renumbered or removed. Commit the doc update separately so the diff
  is clean.
- **Tests are hermetic.** Every behaviour change comes with tests
  using `httptest.Server` + canned API responses captured live. One-off
  curls are fine for discovery, but the lesson goes into a test before
  the commit lands. Tests must never hit MusicBrainz, Spotify, or
  Anthropic in CI.
- **Plan → spot-check → commit.** For non-trivial changes: state the
  shape (recommendation + main trade-off), wait for OK, implement,
  run live on representative queries, show the output before
  committing. Each conceptual step gets its own commit + tests + docs
  cycle.

## Architecture

Per-concern files, mirrored test files:

- `main.go` — `main()`, orchestration, display helpers
- `musicbrainz.go` — MB types and HTTP client (`mbGet`, `searchWorks`,
  `lookupWorkOtherVersions`, `browseRecordingsByWork`,
  `expandEditions`, `resolveComposerMBID`)
- `query.go` — `parseQueryArgs`, `buildQuery`, Lucene helpers
- `performance.go` — `Performance` type, `groupRecordings`, choir
  heuristic
- `spotify.go` — Spotify auth, album search, match-score verification,
  label batch lookup
- `llm.go` — Anthropic SDK fallback for canonical-name normalization
- `logging.go` — JSONL LLM log, `-v` pipeline trace, run summary

Two optional credentials, both degrade gracefully when unset:

- `SPOTIFY_CLIENT_ID` / `SPOTIFY_CLIENT_SECRET` — Spotify URLs + labels
- `ANTHROPIC_API_KEY` — LLM canonical-name fallback

Observability:

- `./classical.jsonl` — always-on JSONL log of every LLM call
- `CLASSICAL_LOG_FILE` — override path
- `-v` — stderr pipeline trace with elapsed-time prefixes

## Domain gotchas — know these upfront

Each was discovered live and patched in increments 4–7. Design new
features with these in mind:

- **Multiple edition entities per work.** MB splits a single classical
  piece (e.g. K.427) across several "edition" Works (Maunder, Levin,
  fragment) linked by `other version` relations. Recordings link to
  specific editions. `expandEditions` BFSes over these, two hops out,
  capped at 10 total Works.
- **MB's `artist:` field doesn't follow Latin aliases.** Composers
  stored under non-Latin canonical names (Cyrillic
  Rimsky-Korsakov, etc.) need MBID resolution via `/ws/2/artist`
  first, then `arid:<MBID>` in the work search.
  `resolveComposerMBID` handles this.
- **MB tags choirs and soloists with the same `vocal` relation.**
  Distinguishing them needs a name heuristic (`isChoir`) or extra
  artist lookups. We use the heuristic.
- **Spotify search returns least-bad results.** Even when nothing
  matches, the relevance ranker picks *something* — `Mass`-titled
  albums for Mozart Mass queries, `Symphony`-titled for Esterházy.
  Always verify against performer credits before attaching a URL.
  `bestMatchingAlbum` does this.
- **Spotify's simplified album object lacks `label`.** The search
  endpoint returns a stripped-down shape; the record label needs a
  follow-up `/v1/albums?ids=…` batch lookup
  (`getSpotifyAlbumLabels`).
- **Lucene specials in user input.** Hyphens (`-`) are Lucene's NOT
  operator, so `artist:Rimsky-Korsakov` parses as "Rimsky NOT
  Korsakov". `sanitizeLucene` replaces every special character
  with a space.
