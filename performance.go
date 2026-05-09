package main

import (
	"regexp"
	"sort"
	"strings"
)

// Performance is one distinct recording of a Work, identified by the
// (conductor, orchestra, year) fingerprint. Choirs and Soloists split
// the credits MusicBrainz tags as `vocal` (which conflates both) using
// a name-pattern heuristic. SpotifyURL holds the first Spotify URL
// surfaced via MusicBrainz `url-rels` on any of the grouped recordings;
// it stays empty when no MB-supplied URL is present (the Spotify-search
// fallback fills it in later).
type Performance struct {
	Conductor      string
	Orchestra      string
	Year           string
	Choirs         []string
	Soloists       []string
	SpotifyURL     string
	SpotifyAlbumID string // populated alongside SpotifyURL; used for the label lookup
	Label          string // record label, populated by the Spotify album-detail call
}

// choirNameRegex matches strings that look like a choir or vocal
// ensemble. It's a heuristic over common naming patterns in classical
// performance credits — multilingual keywords (Choir / Chor /
// Kantorei / Singverein / Kammerchor / Coro / Choeur / Chorale /
// Cappella / Consort / Cathedral / Singers / Voices / Ensemble) plus
// the bare word "chor" which appears in many German names.
//
// The MB-correct alternative would be a follow-up artist lookup to
// check the entity's `type` (Person vs Choir/Group), but that means
// one extra API call per vocal credit per recording — too expensive
// for the value it adds.
var choirNameRegex = regexp.MustCompile(`(?i)\b(choir|chorus|chorale|choeur|chor|kantorei|singverein|kammerchor|singers|voices|cathedral|coro|coral|cappella|consort|ensemble)\b`)

// isChoir applies the heuristic. Empty input returns false.
func isChoir(name string) bool {
	if name == "" {
		return false
	}
	return choirNameRegex.MatchString(name)
}

// mergeUnique appends items from `incoming` to `dst` preserving order
// and skipping any duplicates already present.
func mergeUnique(dst, incoming []string) []string {
	seen := make(map[string]bool, len(dst))
	for _, s := range dst {
		seen[s] = true
	}
	for _, s := range incoming {
		if !seen[s] {
			dst = append(dst, s)
			seen[s] = true
		}
	}
	return dst
}

// groupRecordings deduplicates a list of recordings into Performances by
// (conductor, orchestra, year). Vocal credits seen across grouped
// recordings are merged so that the choir (and any soloists) appear
// once per performance. Results are sorted by year descending; missing
// years sink to the bottom.
func groupRecordings(recs []Recording) []Performance {
	type key struct{ conductor, orchestra, year string }
	byKey := map[key]*Performance{}
	var order []key
	for _, r := range recs {
		var conductor, orchestra, spotifyURL string
		var choirs, soloists []string
		for _, rel := range r.Relations {
			if rel.Artist != nil {
				switch rel.Type {
				case "conductor":
					conductor = rel.Artist.Name
				case "performing orchestra":
					orchestra = rel.Artist.Name
				case "vocal":
					if isChoir(rel.Artist.Name) {
						choirs = append(choirs, rel.Artist.Name)
					} else {
						soloists = append(soloists, rel.Artist.Name)
					}
				}
			}
			if rel.URL != nil && spotifyURL == "" && strings.Contains(rel.URL.Resource, "spotify.com") {
				spotifyURL = rel.URL.Resource
			}
		}
		year := ""
		if len(r.FirstReleaseDate) >= 4 {
			year = r.FirstReleaseDate[:4]
		}
		k := key{conductor, orchestra, year}
		p, ok := byKey[k]
		if !ok {
			byKey[k] = &Performance{
				Conductor:  conductor,
				Orchestra:  orchestra,
				Year:       year,
				Choirs:     choirs,
				Soloists:   soloists,
				SpotifyURL: spotifyURL,
			}
			order = append(order, k)
			continue
		}
		p.Choirs = mergeUnique(p.Choirs, choirs)
		p.Soloists = mergeUnique(p.Soloists, soloists)
		if p.SpotifyURL == "" && spotifyURL != "" {
			p.SpotifyURL = spotifyURL
		}
	}
	out := make([]Performance, len(order))
	for i, k := range order {
		out[i] = *byKey[k]
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Year > out[j].Year
	})
	return out
}
