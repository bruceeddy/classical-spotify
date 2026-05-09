package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
)

const (
	spotifyAuthURL   = "https://accounts.spotify.com/api/token"
	spotifySearchURL = "https://api.spotify.com/v1/search"
	spotifyAlbumsURL = "https://api.spotify.com/v1/albums"
)

type SpotifyAlbum struct {
	ID           string              `json:"id"`
	Name         string              `json:"name"`
	ExternalURLs SpotifyExternalURLs `json:"external_urls"`
	Artists      []SpotifyArtistRef  `json:"artists"`
}

type SpotifyExternalURLs struct {
	Spotify string `json:"spotify"`
}

type SpotifyArtistRef struct {
	Name string `json:"name"`
}

type SpotifyAlbumSearchResponse struct {
	Albums struct {
		Items []SpotifyAlbum `json:"items"`
	} `json:"albums"`
}

type spotifyTokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
}

// spotifyToken obtains an access token via Spotify's Client Credentials
// flow. The auth URL is parameterised so tests can inject httptest.
func spotifyToken(authURL, clientID, clientSecret string) (string, error) {
	auth := base64.StdEncoding.EncodeToString([]byte(clientID + ":" + clientSecret))

	req, err := http.NewRequest("POST", authURL, strings.NewReader("grant_type=client_credentials"))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Basic "+auth)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("spotify auth failed (HTTP %d): %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var tr spotifyTokenResponse
	if err := json.Unmarshal(body, &tr); err != nil {
		return "", err
	}
	return tr.AccessToken, nil
}

// searchSpotifyAlbums runs an album search; the search URL is
// parameterised so tests can inject httptest.
func searchSpotifyAlbums(searchURL, token, query string) ([]SpotifyAlbum, error) {
	params := url.Values{}
	params.Add("q", query)
	params.Add("type", "album")
	params.Add("limit", "10")

	req, err := http.NewRequest("GET", searchURL+"?"+params.Encode(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)

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
		return nil, fmt.Errorf("spotify search failed (HTTP %d): %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var sr SpotifyAlbumSearchResponse
	if err := json.Unmarshal(body, &sr); err != nil {
		return nil, err
	}
	return sr.Albums.Items, nil
}

// albumMatchScore scores how strongly a Spotify album appears to
// belong to a given Performance. Conductor matches (in the album's
// artist credits or its name) are worth 2; orchestra matches are
// worth 1. Empty Performance fields contribute 0 and don't penalise.
//
// This exists because Spotify's relevance ranking returns the
// "least bad" candidate even when no real match exists, so a
// blind first-result pick attached unrelated albums (e.g. "Rain
// Sounds Symphony") to genuine performances.
func albumMatchScore(alb SpotifyAlbum, p Performance) int {
	artistsBlob := ""
	for _, a := range alb.Artists {
		artistsBlob += strings.ToLower(a.Name) + " | "
	}
	name := strings.ToLower(alb.Name)
	score := 0
	if c := strings.ToLower(p.Conductor); c != "" {
		if strings.Contains(artistsBlob, c) || strings.Contains(name, c) {
			score += 2
		}
	}
	if o := strings.ToLower(p.Orchestra); o != "" {
		if strings.Contains(artistsBlob, o) || strings.Contains(name, o) {
			score += 1
		}
	}
	return score
}

// bestMatchingAlbum picks the highest-scoring album for the given
// Performance. Returns ok=false when no candidate scores at all —
// callers should leave the SpotifyURL empty in that case rather than
// attaching a wrong URL.
func bestMatchingAlbum(albums []SpotifyAlbum, p Performance) (SpotifyAlbum, bool) {
	bestScore := 0
	var best SpotifyAlbum
	for _, alb := range albums {
		if s := albumMatchScore(alb, p); s > bestScore {
			bestScore = s
			best = alb
		}
	}
	if bestScore == 0 {
		return SpotifyAlbum{}, false
	}
	return best, true
}

// getSpotifyAlbumLabels batch-fetches the `label` field for up to 20
// Spotify album IDs in a single call (`GET /v1/albums?ids=…`). Returns
// a map keyed on album ID. Empty input is a no-op (no HTTP call).
//
// The simplified album object returned by /v1/search doesn't include
// `label`, so we need this follow-up lookup to get record-label
// metadata onto each performance.
func getSpotifyAlbumLabels(albumsURL, token string, ids []string) (map[string]string, error) {
	if len(ids) == 0 {
		return map[string]string{}, nil
	}
	if len(ids) > 20 {
		ids = ids[:20]
	}
	params := url.Values{}
	params.Add("ids", strings.Join(ids, ","))

	req, err := http.NewRequest("GET", albumsURL+"?"+params.Encode(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)

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
		return nil, fmt.Errorf("spotify album lookup failed (HTTP %d): %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var r struct {
		Albums []struct {
			ID    string `json:"id"`
			Label string `json:"label"`
		} `json:"albums"`
	}
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, err
	}
	out := make(map[string]string, len(r.Albums))
	for _, a := range r.Albums {
		out[a.ID] = a.Label
	}
	return out, nil
}

// spotifyQueryFor builds an album-search query from the user's
// composer/work plus a Performance's distinguishing fields. Empty
// fields are skipped so a partial Performance still gets a sensible
// query.
func spotifyQueryFor(composer, work string, p Performance) string {
	parts := make([]string, 0, 4)
	for _, s := range []string{composer, work, p.Conductor, p.Orchestra} {
		if s != "" {
			parts = append(parts, s)
		}
	}
	return strings.Join(parts, " ")
}

// fillSpotifyURLs is the integration glue: it reads SPOTIFY_CLIENT_ID
// and SPOTIFY_CLIENT_SECRET from the environment, obtains an access
// token, and for every Performance that doesn't already have an
// MB-supplied SpotifyURL runs an album search and stores the first
// hit's external Spotify URL on the performance. Errors on individual
// searches are logged to stderr; only auth/setup failures bubble up.
func fillSpotifyURLs(performances []Performance, composer, work string) error {
	clientID := os.Getenv("SPOTIFY_CLIENT_ID")
	clientSecret := os.Getenv("SPOTIFY_CLIENT_SECRET")
	if clientID == "" || clientSecret == "" {
		return fmt.Errorf("SPOTIFY_CLIENT_ID and SPOTIFY_CLIENT_SECRET must be set")
	}
	token, err := spotifyToken(spotifyAuthURL, clientID, clientSecret)
	if err != nil {
		return err
	}
	for i := range performances {
		if performances[i].SpotifyURL != "" {
			continue
		}
		query := spotifyQueryFor(composer, work, performances[i])
		albums, err := searchSpotifyAlbums(spotifySearchURL, token, query)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Spotify search failed for performance %d: %v\n", i+1, err)
			continue
		}
		if best, ok := bestMatchingAlbum(albums, performances[i]); ok {
			performances[i].SpotifyURL = best.ExternalURLs.Spotify
			performances[i].SpotifyAlbumID = best.ID
		}
	}

	// Batch-fetch record labels for every album we attached.
	var ids []string
	for _, p := range performances {
		if p.SpotifyAlbumID != "" {
			ids = append(ids, p.SpotifyAlbumID)
		}
	}
	if len(ids) > 0 {
		labels, err := getSpotifyAlbumLabels(spotifyAlbumsURL, token, ids)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Spotify album label lookup failed: %v\n", err)
		} else {
			for i := range performances {
				if l, ok := labels[performances[i].SpotifyAlbumID]; ok {
					performances[i].Label = l
				}
			}
		}
	}
	return nil
}
