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
   Vocal:     RIAS Kammerchor
   Spotify:   https://open.spotify.com/album/4GTwxM7IjQI147iGH20Omf

2. 2002  Christian Brembeck
   Orchestra: Capella Istropolitana
   Spotify:   https://open.spotify.com/album/1MqMfzyUUDKb1hkT8q3AtI

3. 1996  (no conductor credited)
   Vocal:     Markus Brutscher, Johanna Koslowsky, …
   Spotify:   https://open.spotify.com/album/3ELfX4GIPcYOTJBl8PdoKi

4. ????  (no conductor credited)
   Vocal:     Hana Blažíková, David Erler, Peter Harvey, …
   Spotify:   https://open.spotify.com/album/3ELfX4GIPcYOTJBl8PdoKi
```

(Without Spotify credentials the `Spotify:` lines are absent.)

## How it works

1. **Query.** Your input is split into composer + work and emitted as
   `artist:<composer> AND work:(<word> AND <word> AND …)`. Words inside
   each side are AND-grouped so all must appear, but the order and
   surrounding tokens are flexible — this is what lets `Bach BWV 232`
   match the canonical `h-Moll-Messe, BWV 232`.
2. **Work search.** A `GET /ws/2/work?query=…` against MusicBrainz
   returns matching Works. Entries that look like a movement (title
   contains `": "` or `" from "`, or has no top-level `type`) are
   filtered out so we get the parent work rather than its individual
   movements.
3. **Edition expansion.** Classical works in MB are typically split
   across several "edition" Work entities (e.g. K.427 has Maunder,
   Levin, fragment, and several reconstruction Works), connected by
   `other version` relations, and recordings link to whichever
   edition's metadata cites them. We BFS over those relations from
   each matched Work, two hops out, capped at 10 total Works, so a
   single query aggregates recordings across the whole edition family.
4. **Recording browse.** For each Work in the expanded set, we call
   `GET /ws/2/recording?work=<MBID>&inc=artist-credits+artist-rels+url-rels`
   to get the recordings, with one second between calls to respect
   MusicBrainz's public rate limit.
5. **Group.** Recordings are deduplicated into Performances by
   `(conductor, orchestra, year)`. Vocal credits (choirs and soloists,
   both tagged `vocal` by MusicBrainz) are merged across movements of
   the same performance, then printed sorted year-descending.
6. **Spotify URLs (optional).** For each Performance we either reuse a
   Spotify URL the recording already had via MusicBrainz `url-rels`,
   or, if absent, fall back to a Spotify album search (composer + work
   + conductor + orchestra) and attach the first matching album's URL.
   MB's `url-rels` coverage for Spotify is essentially zero in practice
   for classical recordings, so the fallback does almost all the
   work. Without credentials the step is skipped.

A descriptive `User-Agent` is sent on every MusicBrainz request, as the
public API requires.

## Tips and gotchas

- **Use catalog numbers when canonical titles are translated.**
  MusicBrainz lists Bach's Mass in B minor under its German title
  (`h-Moll-Messe, BWV 232`), so `./classical Bach Mass in B minor`
  returns nothing. `./classical Bach BWV 232` and `./classical Bach
  h-Moll-Messe` both find it.
- **Quote multi-word composer names.** `./classical Wolfgang Amadeus
  Mozart Great Mass in C` will treat "Wolfgang" as the composer surname.
  Use `./classical "Wolfgang Amadeus Mozart" "Great Mass in C"` instead.
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
*_test.go          tests, one file per source file
DESIGN.md          design decisions and known limitations
```
