package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

const (
	musicBrainzWorkURL      = "https://musicbrainz.org/ws/2/work/"
	musicBrainzRecordingURL = "https://musicbrainz.org/ws/2/recording"
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

	var resp WorkSearchResponse
	if err := mbGet(baseURL+"?"+params.Encode(), &resp); err != nil {
		return nil, err
	}
	return resp.Works, nil
}

func browseRecordingsByWork(baseURL, workID string) ([]Recording, error) {
	params := url.Values{}
	params.Add("work", workID)
	params.Add("fmt", "json")
	params.Add("inc", "artist-credits artist-rels url-rels")
	params.Add("limit", "100")

	var resp RecordingBrowseResponse
	if err := mbGet(baseURL+"?"+params.Encode(), &resp); err != nil {
		return nil, err
	}
	return resp.Recordings, nil
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
