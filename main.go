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
	composer = strings.ReplaceAll(composer, `"`, ``)
	work = strings.ReplaceAll(work, `"`, ``)
	return fmt.Sprintf(`artist:%s AND work:"%s"`, composer, work)
}

// filterMovements drops Work entities that are individual movements of a
// parent work. Movements come back with an empty `type`, while parent
// works carry a type such as "Mass", "Symphony", or "Concerto".
func filterMovements(works []Work) []Work {
	out := works[:0]
	for _, w := range works {
		if w.Type == "" {
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
