package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"
)

const (
	musicBrainzWorkURL      = "https://musicbrainz.org/ws/2/work/"
	musicBrainzRecordingURL = "https://musicbrainz.org/ws/2/recording"
	userAgent               = "classical/0.1 ( eddy.bruce@gmail.com )"

	// maxWorksToBrowse caps how many of the matched Works we fetch
	// recordings for, to keep us inside MusicBrainz's 1 req/sec budget
	// even when the search returns many edition-variants of the same
	// piece.
	maxWorksToBrowse = 5
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
	Type   string      `json:"type"`
	Artist *WorkArtist `json:"artist,omitempty"`
}

type WorkArtist struct {
	Name string `json:"name"`
}

type Recording struct {
	ID               string              `json:"id"`
	Title            string              `json:"title"`
	FirstReleaseDate string              `json:"first-release-date"`
	Relations        []RecordingRelation `json:"relations"`
}

type RecordingRelation struct {
	Type   string      `json:"type"`
	Artist *WorkArtist `json:"artist,omitempty"`
}

type RecordingBrowseResponse struct {
	Count      int         `json:"recording-count"`
	Recordings []Recording `json:"recordings"`
}

// Performance is one distinct recording of a Work, identified by the
// (conductor, orchestra, year) fingerprint. Vocals collects every choir
// or vocal-soloist credit seen across the recordings that share the
// fingerprint.
type Performance struct {
	Conductor string
	Orchestra string
	Year      string
	Vocals    []string
}

type WorkSearchResponse struct {
	Works []Work `json:"works"`
	Count int    `json:"count"`
}

// buildQuery turns the user's args into a structured MusicBrainz Lucene
// query. If the user supplies multiple shell args, the first is the
// composer and the rest is the work name. If a single arg is supplied,
// it's split on whitespace under the same convention. A single word is
// passed through unstructured.
//
// Multi-word fields are joined with AND inside parens rather than wrapped
// as a quoted phrase: a quoted phrase requires the words consecutively in
// the indexed title and misses canonical titles whose word-order or
// language differs (e.g. "h-Moll-Messe" for Bach's Mass in B minor),
// while AND requires every word but tolerates surrounding tokens.
func buildQuery(args []string) string {
	var composer, work string
	if len(args) >= 2 {
		composer = args[0]
		work = strings.Join(args[1:], " ")
	} else {
		parts := strings.Fields(args[0])
		if len(parts) < 2 {
			return args[0]
		}
		composer = parts[0]
		work = strings.Join(parts[1:], " ")
	}
	return fmt.Sprintf(`artist:%s AND work:%s`,
		luceneAndGroup(sanitizeLucene(composer)),
		luceneAndGroup(sanitizeLucene(work)))
}

// luceneAndGroup joins the words in s with AND and wraps them in parens
// so the field constraint applies to every word. A single word is left
// bare.
func luceneAndGroup(s string) string {
	words := strings.Fields(s)
	if len(words) == 1 {
		return words[0]
	}
	return "(" + strings.Join(words, " AND ") + ")"
}

// sanitizeLucene removes characters from user input that have special
// meaning in Lucene queries and would otherwise break our `field:(...)`
// wrapping.
func sanitizeLucene(s string) string {
	for _, c := range []string{`"`, `(`, `)`} {
		s = strings.ReplaceAll(s, c, "")
	}
	return s
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

func searchWorks(baseURL, query string) ([]Work, error) {
	params := url.Values{}
	params.Add("query", query)
	params.Add("fmt", "json")
	params.Add("limit", "25")

	req, err := http.NewRequest("GET", baseURL+"?"+params.Encode(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("musicbrainz work search failed (HTTP %d): %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var searchResp WorkSearchResponse
	if err := json.Unmarshal(body, &searchResp); err != nil {
		return nil, err
	}

	return searchResp.Works, nil
}

func browseRecordingsByWork(baseURL, workID string) ([]Recording, error) {
	params := url.Values{}
	params.Add("work", workID)
	params.Add("fmt", "json")
	params.Add("inc", "artist-credits artist-rels")
	params.Add("limit", "100")

	req, err := http.NewRequest("GET", baseURL+"?"+params.Encode(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("musicbrainz recording browse failed (HTTP %d): %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var browseResp RecordingBrowseResponse
	if err := json.Unmarshal(body, &browseResp); err != nil {
		return nil, err
	}
	return browseResp.Recordings, nil
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
		var conductor, orchestra string
		var vocals []string
		for _, rel := range r.Relations {
			if rel.Artist == nil {
				continue
			}
			switch rel.Type {
			case "conductor":
				conductor = rel.Artist.Name
			case "performing orchestra":
				orchestra = rel.Artist.Name
			case "vocal":
				vocals = append(vocals, rel.Artist.Name)
			}
		}
		year := ""
		if len(r.FirstReleaseDate) >= 4 {
			year = r.FirstReleaseDate[:4]
		}
		k := key{conductor, orchestra, year}
		p, ok := byKey[k]
		if !ok {
			byKey[k] = &Performance{conductor, orchestra, year, vocals}
			order = append(order, k)
			continue
		}
		seen := make(map[string]bool, len(p.Vocals))
		for _, v := range p.Vocals {
			seen[v] = true
		}
		for _, v := range vocals {
			if !seen[v] {
				p.Vocals = append(p.Vocals, v)
				seen[v] = true
			}
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

func composer(w Work) string {
	for _, r := range w.Relations {
		if r.Type == "composer" && r.Artist != nil {
			return r.Artist.Name
		}
	}
	return ""
}

func main() {
	flag.Parse()
	args := flag.Args()

	if len(args) == 0 {
		fmt.Println("Usage: classical <composer> <work>")
		fmt.Println("Example: classical Mozart Great Mass in C")
		fmt.Println("Example: classical \"Wolfgang Amadeus Mozart\" \"Great Mass in C\"")
		os.Exit(1)
	}

	query := buildQuery(args)
	fmt.Printf("Searching MusicBrainz: %s\n\n", query)

	works, err := searchWorks(musicBrainzWorkURL, query)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	works = filterMovements(works)
	if len(works) == 0 {
		fmt.Println("No works found.")
		return
	}

	if len(works) > 10 {
		works = works[:10]
	}

	fmt.Printf("Found %d work(s):\n\n", len(works))
	for i, w := range works {
		fmt.Printf("%d. %s\n", i+1, w.Title)
		if c := composer(w); c != "" {
			fmt.Printf("   Composer: %s\n", c)
		}
		if w.Type != "" {
			fmt.Printf("   Type:     %s\n", w.Type)
		}
		if w.Disambiguation != "" {
			fmt.Printf("   Note:     %s\n", w.Disambiguation)
		}
		fmt.Printf("   Score:    %d\n", w.Score)
		fmt.Printf("   MBID:     %s\n\n", w.ID)
	}

	toBrowse := works
	if len(toBrowse) > maxWorksToBrowse {
		toBrowse = toBrowse[:maxWorksToBrowse]
	}
	var recs []Recording
	for i, w := range toBrowse {
		if i > 0 {
			time.Sleep(time.Second)
		}
		got, err := browseRecordingsByWork(musicBrainzRecordingURL, w.ID)
		if err != nil {
			fmt.Printf("Error browsing recordings for %s: %v\n", w.ID, err)
			continue
		}
		recs = append(recs, got...)
	}

	performances := groupRecordings(recs)
	if len(performances) == 0 {
		fmt.Println("No recordings linked in MusicBrainz for the matched work(s).")
		return
	}

	fmt.Printf("Recordings (%d):\n\n", len(performances))
	for i, p := range performances {
		year := p.Year
		if year == "" {
			year = "????"
		}
		conductor := p.Conductor
		if conductor == "" {
			conductor = "(no conductor credited)"
		}
		fmt.Printf("%d. %s  %s\n", i+1, year, conductor)
		if p.Orchestra != "" {
			fmt.Printf("   Orchestra: %s\n", p.Orchestra)
		}
		if len(p.Vocals) > 0 {
			fmt.Printf("   Vocal:     %s\n", strings.Join(p.Vocals, ", "))
		}
		fmt.Println()
	}
}
