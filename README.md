# classical

A command-line tool for browsing classical-music recordings.

Spotify's data model is track-centric, which doesn't match how classical
listeners think about repertoire — you usually want a specific *recording*
of a *work*, where the work spans many tracks (Kyrie, Gloria, Credo, …)
and the recording is identified by its conductor and orchestra/choir.
This tool resolves a free-text query to a canonical Work entity in
MusicBrainz, then lists the recordings linked to that Work, grouped by
performance fingerprint `(conductor, orchestra, year)`.

> **Status.** Early but functional. Search, recording lookup, and
> Spotify URL resolution work end-to-end. Playback / playlist creation
> is deferred. See [`DESIGN.md`](DESIGN.md) for the full design and
> known limitations, or [`INCREMENTS.md`](INCREMENTS.md) for the
> progress log.

## Build

```bash
go build -o classical
```

Requires Go 1.21 or later. The MusicBrainz queries run anonymously, so
the tool works out of the box without credentials.

To also surface Spotify album URLs alongside each recording, register
a free Spotify app at <https://developer.spotify.com/dashboard> and
export your credentials before running:

```bash
export SPOTIFY_CLIENT_ID=your_client_id
export SPOTIFY_CLIENT_SECRET=your_secret
```

(or put them in a sourceable `.env` — see [`.env.example`](.env.example)).
Without these env vars, the tool prints a one-line `Note:` to stderr
explaining the skip and continues with MusicBrainz-only output.

**Optional but useful: an Anthropic API key.** If `ANTHROPIC_API_KEY`
is set, the tool falls back to Claude when a MusicBrainz work search
returns nothing — Claude suggests canonical aliases (e.g. translates
`Mass in B minor` → `h-Moll-Messe` / `BWV 232`) and the search is
retried. Without the key the fallback is skipped and you get the same
"No works found" you'd have got before, so this is purely additive.

```bash
export ANTHROPIC_API_KEY=sk-ant-…
```

## Usage

```bash
./classical <composer> <work>
```

The composer goes first; the rest is the work title. Either pass them as
separate shell args or as one quoted string:

```bash
./classical Mozart Great Mass in C
./classical "Wolfgang Amadeus Mozart" "Great Mass in C"
./classical Bach BWV 232
```

The tool prints the matched MusicBrainz Works (so you can see what your
query resolved to), then the recordings of those Works grouped by
performance.

### Example

```
$ ./classical Bach BWV 232
Searching MusicBrainz: artist:Bach AND work:(BWV AND 232)

Found 1 work(s):

1. h-Moll-Messe, BWV 232
   Composer: Johann Sebastian Bach
   Type:     Mass
   Note:     Mass in B minor
   Score:    97
   MBID:     ef5ff09e-0bf0-4ea1-bbfa-a7e670a0a12d

Recordings (4):

1. 2007  René Jacobs
   Orchestra: Akademie für Alte Musik Berlin
   Choir:     RIAS Kammerchor
   Label:     Berlin Classics
   Spotify:   https://open.spotify.com/album/4GTwxM7IjQI147iGH20Omf

2. 2002  Christian Brembeck
   Orchestra: Capella Istropolitana

3. 1996  (no conductor credited)
   Soloists:  Markus Brutscher, Johanna Koslowsky, …

4. ????  (no conductor credited)
   Soloists:  Hana Blažíková, David Erler, Peter Harvey, …
```

`Choir:` and `Soloists:` are split via a name-pattern heuristic on the
MusicBrainz `vocal` credits (which conflate both). `Label:` and
`Spotify:` are only shown when (a) Spotify credentials are present and
(b) the search-result album passes verification by conductor or
orchestra match — sparsely credited recordings (no conductor and no
orchestra) won't get either.

## How it works

1. **Query.** Your input is split into composer + work. The composer
   string is resolved against MusicBrainz's artist endpoint to find a
   Person whose disambiguation marks them as a composer; their MBID
   is stored. This step bypasses MB's name-matching, which doesn't
   follow Latin aliases — important for composers stored under a
   non-Latin canonical name (Rimsky-Korsakov, Tchaikovsky, etc.).
   The work side is emitted as `work:(<word> AND <word> AND …)`,
   AND-grouped so all words must appear but order and surrounding
   tokens are flexible (this is what lets `Bach BWV 232` match the
   canonical `h-Moll-Messe, BWV 232`). Lucene-special characters in
   the input are replaced with spaces so things like `Rimsky-Korsakov`
   don't get mangled by Lucene's NOT operator.
2. **Work search.** A `GET /ws/2/work?query=arid:<MBID> AND work:(…)`
   against MusicBrainz returns matching Works. (When composer
   resolution fails the artist clause falls back to `artist:<name>`.)
   Entries that look like a movement (title contains `": "` or
   `" from "`, or has no top-level `type`) are filtered out so we
   get the parent work rather than its individual movements.
3. **LLM fallback (optional).** If step 2 returned nothing AND
   `ANTHROPIC_API_KEY` is set, Claude is asked for alternative
   work-search terms — canonical foreign-language titles, catalog
   numbers, common aliases. The search is retried with each suggestion
   in turn; the first non-empty result wins. This is what lets
   `Bach Mass in B minor` resolve via `BWV 232` / `h-Moll-Messe`
   even though MB doesn't index the work under any of the English
   words. Without the key the fallback is skipped and the user sees
   the same "No works found" they would have before — purely additive.
4. **Edition expansion.** Classical works in MB are typically split
   across several "edition" Work entities (e.g. K.427 has Maunder,
   Levin, fragment, and several reconstruction Works), connected by
   `other version` relations, and recordings link to whichever
   edition's metadata cites them. We BFS over those relations from
   each matched Work, two hops out, capped at 10 total Works, so a
   single query aggregates recordings across the whole edition family.
5. **Recording browse.** For each Work in the expanded set, we call
   `GET /ws/2/recording?work=<MBID>&inc=artist-credits+artist-rels+url-rels`
   to get the recordings, with one second between calls to respect
   MusicBrainz's public rate limit.
6. **Group.** Recordings are deduplicated into Performances by
   `(conductor, orchestra, year)`. The `vocal` credits MusicBrainz
   reports are split via a name-pattern heuristic (`isChoir`) into
   choirs (named after Choir / Chor / Kantorei / Singverein /
   Kammerchor / Coro / Cappella / etc.) and individual soloists, then
   merged across movements of the same performance. Sorted
   year-descending.
7. **Spotify URLs and labels (optional).** For each Performance we
   either reuse a Spotify URL the recording already had via
   MusicBrainz `url-rels`, or, if absent, fall back to a Spotify album
   search (composer + work + conductor + orchestra). Search results
   are then *verified*: a candidate album is only attached if the
   performance's conductor or orchestra appears in the album's artist
   credits or name. If nothing passes verification the URL is left
   empty — better no URL than the wrong one, since Spotify's ranking
   happily returns unrelated albums when no real match exists. After
   the per-performance search, a single batch lookup against
   `/v1/albums?ids=…` fetches the record `Label` for every attached
   album (the simplified search-result album doesn't include it).
   MB's `url-rels` coverage is essentially zero in practice, so the
   verified fallback does almost all the work. Without credentials
   the whole step is skipped.

A descriptive `User-Agent` is sent on every MusicBrainz request, as the
public API requires.

## Tips and gotchas

- **Use catalog numbers if you don't have an Anthropic key.**
  MusicBrainz lists Bach's Mass in B minor under its German title
  (`h-Moll-Messe, BWV 232`), so a query like `Bach Mass in B minor`
  doesn't match directly. With `ANTHROPIC_API_KEY` set the LLM
  fallback closes this gap automatically (suggests `BWV 232` /
  `h-Moll-Messe` and retries). Without the key, fall back to the
  catalog number (`Bach BWV 232`) or canonical title (`Bach
  h-Moll-Messe`) yourself — both work.
- **Quote multi-word composer names.** `./classical Wolfgang Amadeus
  Mozart Great Mass in C` will treat "Wolfgang" as the composer surname.
  Use `./classical "Wolfgang Amadeus Mozart" "Great Mass in C"` instead.
- **Hyphenated names work.** `./classical Rimsky-Korsakov Scheherazade`
  resolves the composer to his MusicBrainz MBID and uses that for the
  work search, so the query reaches works stored under his Cyrillic
  canonical name.
- **Cross-edition gaps (mostly closed).** Classical works often have
  several MusicBrainz "edition" entities (e.g. K.427 has Maunder, Levin,
  fragment, and several reconstructions). Recordings link to whichever
  edition's metadata cites them. The tool walks MB's `other version`
  relations two hops from the matched Works to aggregate across the
  family, so most editions are reached automatically. Recordings linked
  to editions further than two hops, or to editions that don't appear
  in the top search results at all, can still be missed.
- **MusicBrainz isn't uniformly populated.** Some recordings are
  missing a conductor, orchestra, or release date and show as
  `(no conductor credited)` / `????` in the output.

For the full design rationale and the complete list of known
limitations, see [`DESIGN.md`](DESIGN.md).

## Observability

Three things land for free; one is opt-in:

**End-of-run summary (always on).** One stderr line at exit, e.g.
`Done in 2.99s` or — when an LLM call happened —
`Done in 8.4s (Claude: 350 in / 52 out tokens)`. Quick visibility
into Claude cost without grep.

**LLM call log (always on, JSONL).** Every Claude call writes one
JSON object to `./classical.jsonl` containing the timestamp, composer,
work, full prompt, full response text, model, input/output tokens,
latency, and any error. The file is lazily created on first call, so
runs that don't hit Claude don't produce one. Override the path with
`CLASSICAL_LOG_FILE` if you want it elsewhere. Nothing rotates the
file — manage it yourself for long-running use.

```bash
$ jq . classical.jsonl
{"time":"2026-05-10T01:58:29Z","composer":"Bach","work":"Mass in B minor",
 "model":"claude-opus-4-7","prompt":"...","response":"{\"alternatives\":[\"BWV 232\",\"h-Moll-Messe\",\"Messe in h-Moll\",\"Hohe Messe\"]}",
 "input_tokens":350,"output_tokens":52,"latency_ms":5161}
```

**Verbose pipeline trace (`-v`, opt-in).** Adds an elapsed-time-prefixed
line per pipeline stage to stderr: every MusicBrainz call (composer
resolution with all candidates, work search, work-rels lookup,
recording browse) and every Spotify call (auth, album search with
match-score per candidate, label batch). Useful for diagnosing slow
queries or unexpected results without code changes.

```bash
$ ./classical -v Bach Mass in B minor 2> trace.log
[ 495ms] MB composer resolve "Bach": 5 candidates in 495ms
[ 495ms]   - Johann Sebastian Bach  type=Person  score=100  disambig="German Baroque period composer & musician"
[ 495ms]   - Carl Philipp Emanuel Bach  type=Person  score=82  disambig="German classical composer"
[ 495ms]   → picked Johann Sebastian Bach (24f1766e-…)
…
[7.467s] verifying 10 Spotify candidate(s) for René Jacobs/Akademie für Alte Musik Berlin/2007
[7.467s]   score=3  album="Bach: Mass in B Minor, BWV 232"  artists=[Johann Sebastian Bach René Jacobs …]
[7.467s]   score=0  album="Celestial Bliss"  artists=[Felix Lancaster]
[7.467s]   → picked "Bach: Mass in B Minor, BWV 232" (score=3)
```

## Running the tests

```bash
go test ./...
```

Tests are hermetic — no live calls to MusicBrainz or Spotify. The HTTP
paths use Go's `net/http/httptest` with canned responses captured from
the live APIs. For the per-test breakdown:

```bash
go test -v ./...
```

## Project layout

```
main.go            main(), orchestration, display helpers
musicbrainz.go     MB types and HTTP client (mbGet, searchWorks, browseRecordingsByWork)
query.go           parseQueryArgs, buildQuery, Lucene helpers
performance.go     Performance type, groupRecordings
spotify.go         Spotify auth, album search, fillSpotifyURLs
llm.go             Claude-API fallback for canonical-name normalization
logging.go         JSONL LLM log, verbose pipeline trace, run summary
*_test.go          tests, one file per source file
DESIGN.md          design decisions and known limitations
```
