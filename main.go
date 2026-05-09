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
	Type   string `json:"type"`
	Artist *struct {
		Name string `json:"name"`
	} `json:"artist,omitempty"`
}

type WorkSearchResponse struct {
	Works []Work `json:"works"`
	Count int    `json:"count"`
}

func searchWorks(query string) ([]Work, error) {
	params := url.Values{}
	params.Add("query", query)
	params.Add("fmt", "json")
	params.Add("limit", "10")

	req, err := http.NewRequest("GET", musicBrainzWorkURL+"?"+params.Encode(), nil)
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
		fmt.Println("Usage: classical <search query>")
		fmt.Println("Example: classical Mozart Great Mass in C")
		os.Exit(1)
	}

	query := strings.Join(args, " ")

	fmt.Printf("Searching MusicBrainz for works matching: %q\n\n", query)

	works, err := searchWorks(query)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	if len(works) == 0 {
		fmt.Println("No works found.")
		return
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
