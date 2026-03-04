package main

import (
	"encoding/base64"
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
	spotifyAuthURL   = "https://accounts.spotify.com/api/token"
	spotifySearchURL = "https://api.spotify.com/v1/search"
	clientID         = ""
	clientSecret     = ""
)

type Track struct {
	Name    string `json:"name"`
	Artists []struct {
		Name string `json:"name"`
	} `json:"artists"`
	Album struct {
		Name string `json:"name"`
	} `json:"album"`
	ExternalURLs struct {
		Spotify string `json:"spotify"`
	} `json:"external_urls"`
}

type SearchResponse struct {
	Tracks struct {
		Items []Track `json:"items"`
	} `json:"tracks"`
}

type TokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
}

// getSpotifyToken retrieves an access token from the Spotify API
func getSpotifyToken() (string, error) {
	auth := base64.StdEncoding.EncodeToString([]byte(clientID + ":" + clientSecret))

	req, err := http.NewRequest("POST", spotifyAuthURL, strings.NewReader("grant_type=client_credentials"))
	if err != nil {
		return "", err
	}

	req.Header.Add("Authorization", "Basic "+auth)
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var tokenResp TokenResponse
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	err = json.Unmarshal(body, &tokenResp)
	if err != nil {
		return "", err
	}

	return tokenResp.AccessToken, nil
}

// searchTracks searches Spotify for tracks matching the query
func searchTracks(query string, accessToken string) ([]Track, error) {
	params := url.Values{}
	params.Add("q", query)
	params.Add("type", "track")
	params.Add("limit", "20")

	fullURL := spotifySearchURL + "?" + params.Encode()

	req, err := http.NewRequest("GET", fullURL, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Add("Authorization", "Bearer "+accessToken)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var searchResp SearchResponse
	err = json.Unmarshal(body, &searchResp)
	if err != nil {
		return nil, err
	}

	return searchResp.Tracks.Items, nil
}

func main() {
	flag.Parse()
	args := flag.Args()

	if len(args) == 0 {
		fmt.Println("Usage: classical <search query>")
		fmt.Println("Example: classical \"Mozart Symphony No. 40\"")
		os.Exit(1)
	}

	query := strings.Join(args, " ")

	fmt.Printf("Searching Spotify for: \"%s\"\n\n", query)

	// Get access token
	token, err := getSpotifyToken()
	if err != nil {
		fmt.Printf("Error getting Spotify token: %v\n", err)
		os.Exit(1)
	}

	// Search for tracks
	tracks, err := searchTracks(query, token)
	if err != nil {
		fmt.Printf("Error searching Spotify: %v\n", err)
		os.Exit(1)
	}

	if len(tracks) == 0 {
		fmt.Println("No tracks found.")
		return
	}

	fmt.Printf("Found %d track(s):\n\n", len(tracks))
	for i, track := range tracks {
		artists := make([]string, len(track.Artists))
		for j, artist := range track.Artists {
			artists[j] = artist.Name
		}

		fmt.Printf("%d. %s\n", i+1, track.Name)
		fmt.Printf("   Artists: %s\n", strings.Join(artists, ", "))
		fmt.Printf("   Album: %s\n", track.Album.Name)
		fmt.Printf("   URL: %s\n\n", track.ExternalURLs.Spotify)
	}
}
