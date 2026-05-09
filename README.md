# classical

A command-line tool for browsing classical-music recordings.

Spotify's data model is track-centric, which doesn't match how classical
listeners think about repertoire — you usually want a specific *recording*
of a *work*, where the work spans many tracks (Kyrie, Gloria, Credo, …)
and the recording is identified by its conductor and orchestra/choir.
This tool resolves a free-text query to a canonical Work entity in
MusicBrainz, then lists the recordings linked to that Work, grouped by
performance fingerprint `(conductor, orchestra, year)`.

> **Status.** Early. The MusicBrainz-backed search and grouping work
> end-to-end. Linking each recording to its Spotify URL is planned
> (Increment 3); playback / playlist creation is deferred. See
> [`DESIGN.md`](DESIGN.md) for the full design and known limitations.

## Build

```bash
go build -o classical
```

Requires Go 1.21 or later. No API keys are needed at the moment;
MusicBrainz's public API is open.

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

2. 2002  Christian Brembeck
   Orchestra: Capella Istropolitana

3. 1996  (no conductor credited)
   Vocal:     Markus Brutscher, Johanna Koslowsky, …

4. ????  (no conductor credited)
   Vocal:     Hana Blažíková, David Erler, Peter Harvey, …
```

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
3. **Recording browse.** For each remaining Work (capped at the top 5),
   we call `GET /ws/2/recording?work=<MBID>&inc=artist-credits+artist-rels`
   to get the recordings, with one second between calls to respect
   MusicBrainz's public rate limit.
4. **Group.** Recordings are deduplicated into Performances by
   `(conductor, orchestra, year)`. Vocal credits (choirs and soloists,
   both tagged `vocal` by MusicBrainz) are merged across movements of
   the same performance, then printed sorted year-descending.

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
- **Cross-edition gaps.** Classical works often have several MusicBrainz
  "edition" entities (e.g. K.427 has Maunder, Levin, and original-fragment
  entries). Recordings link to whichever edition's metadata cites them,
  so a query may miss famous recordings linked to an edition that
  doesn't surface in the top matches. Try a more specific query or use
  the catalog number.
- **MusicBrainz isn't uniformly populated.** Some recordings are
  missing a conductor, orchestra, or release date and show as
  `(no conductor credited)` / `????` in the output.

For the full design rationale and the complete list of known
limitations, see [`DESIGN.md`](DESIGN.md).

## Project layout

```
main.go            main(), orchestration, display
musicbrainz.go     MB types and HTTP client (mbGet, searchWorks, browseRecordingsByWork)
query.go           buildQuery and Lucene helpers
performance.go     Performance type, groupRecordings
*_test.go          tests, mirroring each file
DESIGN.md          design decisions and known limitations
```
