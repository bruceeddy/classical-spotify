package main

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSpotifyToken_ParsesAndSendsBasicAuth(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("method = %q, want POST", r.Method)
		}
		if got := r.Header.Get("Content-Type"); got != "application/x-www-form-urlencoded" {
			t.Errorf("Content-Type = %q", got)
		}
		want := "Basic " + base64.StdEncoding.EncodeToString([]byte("id:secret"))
		if got := r.Header.Get("Authorization"); got != want {
			t.Errorf("Authorization = %q, want %q", got, want)
		}
		w.Write([]byte(`{"access_token":"abc123","token_type":"Bearer","expires_in":3600}`))
	}))
	defer server.Close()

	tok, err := spotifyToken(server.URL, "id", "secret")
	if err != nil {
		t.Fatalf("spotifyToken: %v", err)
	}
	if tok != "abc123" {
		t.Errorf("token = %q, want abc123", tok)
	}
}

func TestSpotifyToken_NonOKStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":"invalid_client"}`))
	}))
	defer server.Close()
	if _, err := spotifyToken(server.URL, "id", "wrong"); err == nil {
		t.Fatal("expected error on non-200, got nil")
	}
}

const cannedSpotifyAlbumSearchResponse = `{
  "albums": {
    "items": [
      {
        "name": "Mass in B Minor",
        "external_urls": {"spotify": "https://open.spotify.com/album/jacobs-bwv232"},
        "artists": [
          {"name": "Johann Sebastian Bach"},
          {"name": "RIAS Kammerchor"},
          {"name": "Akademie für Alte Musik Berlin"},
          {"name": "René Jacobs"}
        ]
      },
      {
        "name": "Bach: Mass in B Minor",
        "external_urls": {"spotify": "https://open.spotify.com/album/another"},
        "artists": [{"name": "Some other conductor"}]
      }
    ]
  }
}`

func TestSearchSpotifyAlbums_ParsesAndSendsBearer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Errorf("Authorization = %q, want %q", got, "Bearer test-token")
		}
		q := r.URL.Query()
		if got := q.Get("q"); got != "Bach Mass in B minor René Jacobs" {
			t.Errorf("q = %q", got)
		}
		if got := q.Get("type"); got != "album" {
			t.Errorf("type = %q, want album", got)
		}
		if got := q.Get("limit"); got != "10" {
			t.Errorf("limit = %q, want 10", got)
		}
		w.Write([]byte(cannedSpotifyAlbumSearchResponse))
	}))
	defer server.Close()

	albums, err := searchSpotifyAlbums(server.URL, "test-token", "Bach Mass in B minor René Jacobs")
	if err != nil {
		t.Fatalf("searchSpotifyAlbums: %v", err)
	}
	if len(albums) != 2 {
		t.Fatalf("got %d albums, want 2", len(albums))
	}
	if albums[0].Name != "Mass in B Minor" {
		t.Errorf("albums[0].Name = %q", albums[0].Name)
	}
	if albums[0].ExternalURLs.Spotify != "https://open.spotify.com/album/jacobs-bwv232" {
		t.Errorf("albums[0].ExternalURLs.Spotify = %q", albums[0].ExternalURLs.Spotify)
	}
	if len(albums[0].Artists) != 4 || albums[0].Artists[3].Name != "René Jacobs" {
		t.Errorf("albums[0].Artists = %+v", albums[0].Artists)
	}
}

func TestSearchSpotifyAlbums_NonOKStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":{"status":401,"message":"bad token"}}`))
	}))
	defer server.Close()
	if _, err := searchSpotifyAlbums(server.URL, "expired", "anything"); err == nil {
		t.Fatal("expected error on 401, got nil")
	}
}

func TestFillSpotifyURLs_MissingCreds(t *testing.T) {
	t.Setenv("SPOTIFY_CLIENT_ID", "")
	t.Setenv("SPOTIFY_CLIENT_SECRET", "")

	perfs := []Performance{{Conductor: "Karajan"}}
	err := fillSpotifyURLs(perfs, "Mozart", "Great Mass in C")
	if err == nil {
		t.Fatal("expected error when creds missing, got nil")
	}
	if perfs[0].SpotifyURL != "" {
		t.Errorf("SpotifyURL was populated despite missing creds: %q", perfs[0].SpotifyURL)
	}
}

func TestAlbumMatchScore(t *testing.T) {
	tests := []struct {
		name string
		alb  SpotifyAlbum
		perf Performance
		want int
	}{
		{
			name: "conductor in artists scores 2",
			alb: SpotifyAlbum{
				Name:    "Mozart: Mass in C minor",
				Artists: []SpotifyArtistRef{{Name: "Frieder Bernius"}, {Name: "Hofkapelle Stuttgart"}},
			},
			perf: Performance{Conductor: "Frieder Bernius"},
			want: 2,
		},
		{
			name: "conductor in album name (not artists) also scores 2",
			alb: SpotifyAlbum{
				Name:    "Karajan conducts Mozart",
				Artists: []SpotifyArtistRef{{Name: "Berlin Philharmonic"}},
			},
			perf: Performance{Conductor: "Karajan"},
			want: 2,
		},
		{
			name: "orchestra in artists scores 1",
			alb: SpotifyAlbum{
				Name:    "Mozart: Great Mass",
				Artists: []SpotifyArtistRef{{Name: "Hofkapelle Stuttgart"}},
			},
			perf: Performance{Orchestra: "Hofkapelle Stuttgart"},
			want: 1,
		},
		{
			name: "conductor + orchestra both match: scores add to 3",
			alb: SpotifyAlbum{
				Name:    "Mozart: Great Mass in C",
				Artists: []SpotifyArtistRef{{Name: "Frieder Bernius"}, {Name: "Hofkapelle Stuttgart"}},
			},
			perf: Performance{Conductor: "Frieder Bernius", Orchestra: "Hofkapelle Stuttgart"},
			want: 3,
		},
		{
			name: "irrelevant album scores 0 (the Bernius -> Celestial Bliss case)",
			alb: SpotifyAlbum{
				Name:    "Celestial Bliss",
				Artists: []SpotifyArtistRef{{Name: "Felix Lancaster"}},
			},
			perf: Performance{Conductor: "Frieder Bernius", Orchestra: "Hofkapelle Stuttgart"},
			want: 0,
		},
		{
			name: "irrelevant album scores 0 (the Esterházy -> Rain Sounds Symphony case)",
			alb: SpotifyAlbum{
				Name:    "Rain Sounds Symphony",
				Artists: []SpotifyArtistRef{{Name: "Sleep Inducers"}},
			},
			perf: Performance{Orchestra: "Nicolaus Esterházy Sinfonia"},
			want: 0,
		},
		{
			name: "case-insensitive substring match",
			alb: SpotifyAlbum{
				Name:    "MOZART MASS IN C",
				Artists: []SpotifyArtistRef{{Name: "FRIEDER BERNIUS"}},
			},
			perf: Performance{Conductor: "Frieder Bernius"},
			want: 2,
		},
		{
			name: "empty performance scores 0 (no signals to match)",
			alb: SpotifyAlbum{
				Name:    "Mozart: Mass in C",
				Artists: []SpotifyArtistRef{{Name: "Some Conductor"}},
			},
			perf: Performance{},
			want: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := albumMatchScore(tt.alb, tt.perf); got != tt.want {
				t.Errorf("albumMatchScore() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestBestMatchingAlbum(t *testing.T) {
	perf := Performance{Conductor: "Bernius", Orchestra: "Hofkapelle Stuttgart"}

	t.Run("picks highest scoring album", func(t *testing.T) {
		albums := []SpotifyAlbum{
			{Name: "Random Mass", Artists: []SpotifyArtistRef{{Name: "Hofkapelle Stuttgart"}}}, // score 1
			{Name: "Mozart: Mass in C", Artists: []SpotifyArtistRef{{Name: "Bernius"}, {Name: "Hofkapelle Stuttgart"}}, ExternalURLs: SpotifyExternalURLs{Spotify: "right"}}, // score 3
			{Name: "Other Mozart", Artists: []SpotifyArtistRef{{Name: "Bernius"}}}, // score 2
		}
		got, ok := bestMatchingAlbum(albums, perf)
		if !ok {
			t.Fatal("expected ok=true, got false")
		}
		if got.ExternalURLs.Spotify != "right" {
			t.Errorf("picked %q, want the score-3 album", got.ExternalURLs.Spotify)
		}
	})

	t.Run("rejects when all candidates score zero", func(t *testing.T) {
		albums := []SpotifyAlbum{
			{Name: "Celestial Bliss", Artists: []SpotifyArtistRef{{Name: "Felix Lancaster"}}, ExternalURLs: SpotifyExternalURLs{Spotify: "wrong-1"}},
			{Name: "Rain Sounds Symphony", Artists: []SpotifyArtistRef{{Name: "Sleep Inducers"}}, ExternalURLs: SpotifyExternalURLs{Spotify: "wrong-2"}},
		}
		_, ok := bestMatchingAlbum(albums, perf)
		if ok {
			t.Error("expected ok=false when no candidate matches; got true")
		}
	})

	t.Run("empty candidate list returns ok=false", func(t *testing.T) {
		_, ok := bestMatchingAlbum(nil, perf)
		if ok {
			t.Error("expected ok=false on empty list")
		}
	})
}

func TestSpotifyQueryFor(t *testing.T) {
	tests := []struct {
		name     string
		composer string
		work     string
		perf     Performance
		want     string
	}{
		{
			name:     "all fields populated",
			composer: "Bach",
			work:     "Mass in B minor",
			perf:     Performance{Conductor: "René Jacobs", Orchestra: "Akademie für Alte Musik Berlin", Year: "2007"},
			want:     "Bach Mass in B minor René Jacobs Akademie für Alte Musik Berlin",
		},
		{
			name:     "missing conductor is omitted",
			composer: "Bach",
			work:     "Mass in B minor",
			perf:     Performance{Orchestra: "Capella Istropolitana"},
			want:     "Bach Mass in B minor Capella Istropolitana",
		},
		{
			name:     "missing orchestra is omitted",
			composer: "Bach",
			work:     "Mass in B minor",
			perf:     Performance{Conductor: "Brembeck"},
			want:     "Bach Mass in B minor Brembeck",
		},
		{
			name:     "no performance details, only composer/work",
			composer: "Bach",
			work:     "Mass in B minor",
			perf:     Performance{},
			want:     "Bach Mass in B minor",
		},
		{
			name:     "single-word query (work empty) still works",
			composer: "Mozart",
			work:     "",
			perf:     Performance{Conductor: "Karajan", Orchestra: "Berlin Philharmonic"},
			want:     "Mozart Karajan Berlin Philharmonic",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := spotifyQueryFor(tt.composer, tt.work, tt.perf); got != tt.want {
				t.Errorf("spotifyQueryFor() = %q, want %q", got, tt.want)
			}
		})
	}
}
