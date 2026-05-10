package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const (
	musicBrainzWorkURL      = "https://musicbrainz.org/ws/2/work/"
	musicBrainzRecordingURL = "https://musicbrainz.org/ws/2/recording"
	musicBrainzArtistURL    = "https://musicbrainz.org/ws/2/artist/"
	userAgent               = "classical/0.1 ( eddy.bruce@gmail.com )"
)

type Work struct {
	ID             string         `json:"id"`
	Title          string         `json:"title"`
	Type           string         `json:"type"`
	Score          int            `json:"score"`
	Disambiguation string         `json:"disambiguation"`
	Relations      []WorkRelation `json:"relations"`
}

type WorkRelation struct {
	Type      string      `json:"type"`
	Direction string      `json:"direction,omitempty"`
	Artist    *WorkArtist `json:"artist,omitempty"`
	// Work is populated for work-to-work relations (e.g. "other version",
	// "parts"). The embedded Work is a stub: id, title, type — no nested
	// relations.
	Work *Work `json:"work,omitempty"`
}

type WorkArtist struct {
	Name string `json:"name"`
}

type WorkSearchResponse struct {
	Works []Work `json:"works"`
	Count int    `json:"count"`
}

type Recording struct {
	ID               string              `json:"id"`
	Title            string              `json:"title"`
	FirstReleaseDate string              `json:"first-release-date"`
	Relations        []RecordingRelation `json:"relations"`
}

type RecordingRelation struct {
	Type   string       `json:"type"`
	Artist *WorkArtist  `json:"artist,omitempty"`
	URL    *RelationURL `json:"url,omitempty"`
}

type RelationURL struct {
	Resource string `json:"resource"`
}

type RecordingBrowseResponse struct {
	Count      int         `json:"recording-count"`
	Recordings []Recording `json:"recordings"`
}

// mbGet performs a GET against the MusicBrainz API with the required
// User-Agent header, treats any non-200 status as an error (with the
// response body in the message), and decodes the JSON body into out.
func mbGet(url string, out interface{}) error {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("musicbrainz request failed (HTTP %d): %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return json.Unmarshal(body, out)
}

func searchWorks(baseURL, query string) ([]Work, error) {
	params := url.Values{}
	params.Add("query", query)
	params.Add("fmt", "json")
	params.Add("limit", "25")

	start := time.Now()
	var resp WorkSearchResponse
	if err := mbGet(baseURL+"?"+params.Encode(), &resp); err != nil {
		verbosef("MB work search %q: error after %dms — %v", query, time.Since(start).Milliseconds(), err)
		return nil, err
	}
	verbosef("MB work search %q: %d results in %dms", query, len(resp.Works), time.Since(start).Milliseconds())
	return resp.Works, nil
}

func browseRecordingsByWork(baseURL, workID string) ([]Recording, error) {
	params := url.Values{}
	params.Add("work", workID)
	params.Add("fmt", "json")
	params.Add("inc", "artist-credits artist-rels url-rels")
	params.Add("limit", "100")

	start := time.Now()
	var resp RecordingBrowseResponse
	if err := mbGet(baseURL+"?"+params.Encode(), &resp); err != nil {
		verbosef("MB browse recordings work=%s: error after %dms — %v", workID, time.Since(start).Milliseconds(), err)
		return nil, err
	}
	verbosef("MB browse recordings work=%s: %d recordings in %dms", workID, len(resp.Recordings), time.Since(start).Milliseconds())
	return resp.Recordings, nil
}

type ArtistRef struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Score          int    `json:"score"`
	Type           string `json:"type"`
	Disambiguation string `json:"disambiguation"`
}

type ArtistSearchResponse struct {
	Artists []ArtistRef `json:"artists"`
}

// resolveComposerMBID searches MusicBrainz for an artist matching the
// given name and returns the MBID of the highest-scoring Person whose
// disambiguation marks them as a composer. Returns "" when no such
// candidate exists in the top 5 hits.
//
// Two filters are necessary to land on the right artist for composers
// like Rimsky-Korsakov:
//
//   - Group/Orchestra/Choir artists (e.g. "Rimsky-Korsakov Quartet")
//     are skipped — the Type=Person filter handles that.
//   - Persons related to the composer (e.g. "Maria Rimsky-Korsakov,
//     daughter of the composer") often outscore the composer themselves
//     in MB's ranking. The disambiguation check ("composer" yes,
//     "the composer" / "of composer" no) discriminates the actual
//     composer from their relatives.
//
// Composer resolution lets us substitute `arid:<MBID>` for `artist:<name>`
// in the work search, which works around MB's index not following Latin
// aliases on the work-search artist field — important for composers
// stored under their non-Latin canonical name (Cyrillic for Russians,
// for example).
func resolveComposerMBID(baseURL, name string) (string, error) {
	if name == "" {
		return "", nil
	}
	params := url.Values{}
	params.Add("query", name)
	params.Add("fmt", "json")
	params.Add("limit", "5")

	start := time.Now()
	var resp ArtistSearchResponse
	if err := mbGet(baseURL+"?"+params.Encode(), &resp); err != nil {
		verbosef("MB composer resolve %q: error after %dms — %v", name, time.Since(start).Milliseconds(), err)
		return "", err
	}
	verbosef("MB composer resolve %q: %d candidates in %dms", name, len(resp.Artists), time.Since(start).Milliseconds())
	for _, a := range resp.Artists {
		verbosef("  - %s  type=%s  score=%d  disambig=%q", a.Name, a.Type, a.Score, a.Disambiguation)
	}
	for _, a := range resp.Artists {
		if a.Type == "Person" && isComposerDisambiguation(a.Disambiguation) {
			verbosef("  → picked %s (%s)", a.Name, a.ID)
			return a.ID, nil
		}
	}
	verbosef("  → no Person+composer match; falling back to free-text artist:%s", name)
	return "", nil
}

// isComposerDisambiguation returns true if a MusicBrainz disambiguation
// string identifies its artist as a composer rather than a relative or
// other adjacent role. The pattern: contains "composer" but not as
// "<relation> of the composer" / "of composer X". Examples:
//
//	"Russian composer"           -> true
//	"classical composer"         -> true
//	"composer"                   -> true
//	"daughter of the composer"   -> false
//	"musicologist, son of the composer" -> false
//	"soprano"                    -> false
func isComposerDisambiguation(s string) bool {
	s = strings.ToLower(s)
	if !strings.Contains(s, "composer") {
		return false
	}
	if strings.Contains(s, "the composer") || strings.Contains(s, "of composer") {
		return false
	}
	return true
}

// lookupWorkOtherVersions fetches a single Work with `inc=work-rels` and
// returns the Works it points at via "other version" relations. The
// returned Work entries are stubs (id / title / type only) — enough to
// drive a follow-up recording-browse but not full Work entities.
func lookupWorkOtherVersions(baseURL, workID string) ([]Work, error) {
	params := url.Values{}
	params.Add("fmt", "json")
	params.Add("inc", "work-rels")

	start := time.Now()
	var w Work
	if err := mbGet(baseURL+workID+"?"+params.Encode(), &w); err != nil {
		verbosef("MB work-rels lookup %s: error after %dms — %v", workID, time.Since(start).Milliseconds(), err)
		return nil, err
	}
	var siblings []Work
	for _, rel := range w.Relations {
		if rel.Type == "other version" && rel.Work != nil {
			siblings = append(siblings, *rel.Work)
		}
	}
	verbosef("MB work-rels lookup %s (%q): %d other-version siblings in %dms", workID, w.Title, len(siblings), time.Since(start).Milliseconds())
	return siblings, nil
}

// expandEditions performs a breadth-first walk over the "other version"
// relation graph starting from `initial`, returning the deduplicated
// union (originals + reachable siblings) capped at maxExpanded. Hops
// beyond maxHops are not walked. sleepBetween is the per-call delay
// inserted between MusicBrainz lookups; tests can pass 0.
//
// MusicBrainz models classical works as multiple sibling "edition"
// Work entities (Maunder, Levin, Bärenreiter, ...) with `other version`
// relations between them, and recordings link to whichever edition
// their album metadata cites. Walking these edges materially improves
// recording coverage for queries that hit a leaf edition.
func expandEditions(baseURL string, initial []Work, maxHops, maxExpanded int, sleepBetween time.Duration) []Work {
	seen := make(map[string]bool, len(initial))
	expanded := make([]Work, 0, len(initial))
	for _, w := range initial {
		if !seen[w.ID] {
			seen[w.ID] = true
			expanded = append(expanded, w)
		}
	}

	frontier := append([]Work(nil), expanded...)
	calls := 0
	for hop := 0; hop < maxHops && len(frontier) > 0 && len(expanded) < maxExpanded; hop++ {
		verbosef("edition expansion hop %d: %d frontier work(s), %d total expanded", hop+1, len(frontier), len(expanded))
		var next []Work
		for _, w := range frontier {
			if len(expanded) >= maxExpanded {
				break
			}
			if calls > 0 && sleepBetween > 0 {
				time.Sleep(sleepBetween)
			}
			calls++
			siblings, err := lookupWorkOtherVersions(baseURL, w.ID)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error looking up other-versions for %s: %v\n", w.ID, err)
				continue
			}
			for _, s := range siblings {
				if seen[s.ID] {
					continue
				}
				seen[s.ID] = true
				expanded = append(expanded, s)
				next = append(next, s)
				if len(expanded) >= maxExpanded {
					break
				}
			}
		}
		frontier = next
	}
	return expanded
}

// filterMovements drops Work entities that are individual movements of a
// parent work. We use two signals: an empty `type` (most movements), and
// a movement-style title (": " separator, or "X from Y") which catches
// the cases where the movement inherits the parent's type.
func filterMovements(works []Work) []Work {
	out := works[:0]
	for _, w := range works {
		if w.Type == "" {
			continue
		}
		if strings.Contains(w.Title, ": ") || strings.Contains(w.Title, " from ") {
			continue
		}
		out = append(out, w)
	}
	return out
}

// composer returns the first artist credited as a "composer" of the
// Work, or "" if no composer relation is set.
func composer(w Work) string {
	for _, r := range w.Relations {
		if r.Type == "composer" && r.Artist != nil {
			return r.Artist.Name
		}
	}
	return ""
}
