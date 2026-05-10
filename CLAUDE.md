# CLAUDE.md

Loaded on every session — kept tight. Full rationale in
[`DESIGN.md`](DESIGN.md), progress in [`INCREMENTS.md`](INCREMENTS.md),
user-facing notes in [`README.md`](README.md).

Personal Go CLI: free-text classical-music query → MusicBrainz Work →
recordings grouped by performance, with optional Spotify URLs and a
Claude fallback for canonical-name mismatches.

## Conventions

- **Three docs stay in sync with code.** Functional change → update
  DESIGN.md / INCREMENTS.md / README.md in a separate follow-up commit.
  Reframe entries that moved from "deferred" to "shipped" or from "open
  limitation" to "residual gap".
- **Hermetic tests, no live API calls in CI.** `httptest.Server` +
  canned responses captured live. Curls are fine for discovery; the
  lesson lands in a test before the commit.
- **Plan → spot-check → commit.** Brief recommendation + main trade-off,
  wait for OK, implement, run live, show output before committing. Each
  conceptual step gets its own commit + tests + docs cycle.
- **Use the `Explore` subagent for multi-file codebase searches.** Its
  results don't bloat the main context.
- **`/compact` between increments** to drop accumulated history;
  `/clear` only when switching project entirely.

## Optional env vars (all degrade gracefully)

- `SPOTIFY_CLIENT_ID` + `SPOTIFY_CLIENT_SECRET` — URLs + labels
- `ANTHROPIC_API_KEY` — canonical-name LLM fallback
- `CLASSICAL_LOG_FILE` — JSONL log path (default `./classical.jsonl`)

## Domain gotchas — design around these

- **Multiple edition entities per work.** MB splits a piece (e.g. K.427)
  across "edition" Works (Maunder, Levin, fragment) linked by
  `other version`. `expandEditions` walks them.
- **MB's `artist:` field doesn't follow Latin aliases.** Non-Latin
  canonical names (Cyrillic Rimsky-Korsakov, etc.) need MBID resolution
  then `arid:<MBID>`. `resolveComposerMBID`.
- **MB tags choirs and soloists alike** (`vocal`). Heuristic split in
  `isChoir`.
- **Spotify search returns least-bad results** even when nothing matches
  — verify performer credits before attaching. `bestMatchingAlbum`.
- **Spotify search-result album lacks `label`** — follow-up batch lookup
  in `getSpotifyAlbumLabels`.
- **Lucene specials break queries** — hyphens are Lucene's NOT operator.
  `sanitizeLucene` replaces them all with spaces.
