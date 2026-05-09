package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
)

const (
	musicBrainzWorkURL = "https://musicbrainz.org/ws/2/work/"
	userAgent          = "classical/0.1 ( eddy.bruce@gmail.com )"
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
}
