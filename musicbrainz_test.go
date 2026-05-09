package main

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

// canned MusicBrainz JSON for a Work search, captured live and trimmed to
// the fields the parser cares about.
const cannedMozartGreatMassResponse = `{
  "count": 6,
  "offset": 0,
  "works": [
    {
      "id": "fc221f1e-a2ad-4591-96a5-704c1539fbe3",
      "type": "Mass",
      "score": 100,
      "title": "Great Mass in C minor, K. 427",
      "disambiguation": "ed. Maunder",
      "relations": [
        {"type": "composer", "artist": {"name": "Wolfgang Amadeus Mozart"}},
        {"type": "composer", "artist": {"name": "Richard Maunder"}}
      ]
    },
    {
      "id": "da4d0b2f-421d-4ad8-9ef3-3229f366fc05",
      "type": "",
      "score": 98,
      "title": "Great Mass in C minor, K. 427: II. Gloria",
      "relations": [
        {"type": "composer", "artist": {"name": "Wolfgang Amadeus Mozart"}}
      ]
    }
  ]
}`

// canned MusicBrainz recording-browse response for a Work. Two recordings
// share a (conductor, orchestra, year) fingerprint with different vocal
// credits per movement (so the merge path is exercised); a third is a
// different performance; a fourth has no relations and an unknown date.
const cannedRecordingsResponse = `{
  "recording-count": 4,
  "recordings": [
    {
      "id": "rec-1",
      "title": "Mass: I. Kyrie",
      "first-release-date": "1980-03-01",
      "relations": [
        {"type": "conductor", "artist": {"name": "Karajan"}},
        {"type": "performing orchestra", "artist": {"name": "Berlin Philharmonic"}},
        {"type": "vocal", "artist": {"name": "Vienna Singverein"}}
      ]
    },
    {
      "id": "rec-2",
      "title": "Mass: II. Gloria",
      "first-release-date": "1980-03-01",
      "relations": [
        {"type": "conductor", "artist": {"name": "Karajan"}},
        {"type": "performing orchestra", "artist": {"name": "Berlin Philharmonic"}},
        {"type": "vocal", "artist": {"name": "Vienna Singverein"}},
        {"type": "vocal", "artist": {"name": "Edith Mathis"}}
      ]
    },
    {
      "id": "rec-3",
      "title": "Mass: I. Kyrie",
      "first-release-date": "1985-06-15",
      "relations": [
        {"type": "conductor", "artist": {"name": "Gardiner"}},
        {"type": "performing orchestra", "artist": {"name": "English Baroque Soloists"}},
        {"type": "vocal", "artist": {"name": "Monteverdi Choir"}}
      ]
    },
    {
      "id": "rec-4",
      "title": "Some Excerpt",
      "first-release-date": "",
      "relations": []
    }
  ]
}`

func TestFilterMovements(t *testing.T) {
	in := []Work{
		{Title: "Great Mass in C minor, K. 427", Type: "Mass"},
		{Title: "Great Mass in C minor, K. 427: II. Gloria", Type: ""},
		{Title: "Mass in C: Kyrie", Type: ""},
		{Title: "Mozart!", Type: "Musical"},
		// Movements that carry the parent's type — must still be dropped.
		{Title: "h-Moll-Messe, BWV 232: III. Sanctus", Type: "Mass"},
		{Title: "Benedictus from Mass in B minor, BWV 232", Type: "Mass"},
	}
	got := filterMovements(in)
	wantTitles := []string{
		"Great Mass in C minor, K. 427",
		"Mozart!",
	}
	gotTitles := make([]string, len(got))
	for i, w := range got {
		gotTitles[i] = w.Title
	}
	if !reflect.DeepEqual(gotTitles, wantTitles) {
		t.Errorf("filterMovements titles = %v, want %v", gotTitles, wantTitles)
	}
}

func TestComposer(t *testing.T) {
	tests := []struct {
		name string
		work Work
		want string
	}{
		{
			name: "composer relation present",
			work: Work{Relations: []WorkRelation{
				{Type: "composer", Artist: &WorkArtist{Name: "Wolfgang Amadeus Mozart"}},
			}},
			want: "Wolfgang Amadeus Mozart",
		},
		{
			name: "first composer wins when several are present",
			work: Work{Relations: []WorkRelation{
				{Type: "composer", Artist: &WorkArtist{Name: "Wolfgang Amadeus Mozart"}},
				{Type: "composer", Artist: &WorkArtist{Name: "Richard Maunder"}},
			}},
			want: "Wolfgang Amadeus Mozart",
		},
		{
			name: "non-composer relations are ignored",
			work: Work{Relations: []WorkRelation{
				{Type: "writer", Artist: &WorkArtist{Name: "Some Writer"}},
				{Type: "performance", Artist: &WorkArtist{Name: "Some Performer"}},
			}},
			want: "",
		},
		{
			name: "no relations at all",
			work: Work{},
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := composer(tt.work); got != tt.want {
				t.Errorf("composer() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSearchWorks_ParsesResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("User-Agent"); got != userAgent {
			t.Errorf("User-Agent = %q, want %q", got, userAgent)
		}
		if got := r.URL.Query().Get("query"); got != `artist:Mozart AND work:(Great AND Mass AND in AND C)` {
			t.Errorf("query param = %q, want structured query", got)
		}
		if got := r.URL.Query().Get("fmt"); got != "json" {
			t.Errorf("fmt param = %q, want json", got)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(cannedMozartGreatMassResponse))
	}))
	defer server.Close()

	works, err := searchWorks(server.URL, `artist:Mozart AND work:(Great AND Mass AND in AND C)`)
	if err != nil {
		t.Fatalf("searchWorks returned error: %v", err)
	}
	if len(works) != 2 {
		t.Fatalf("got %d works, want 2", len(works))
	}
	if works[0].Title != "Great Mass in C minor, K. 427" {
		t.Errorf("works[0].Title = %q", works[0].Title)
	}
	if works[0].Type != "Mass" {
		t.Errorf("works[0].Type = %q, want Mass", works[0].Type)
	}
	if works[0].Score != 100 {
		t.Errorf("works[0].Score = %d, want 100", works[0].Score)
	}
	if got := composer(works[0]); got != "Wolfgang Amadeus Mozart" {
		t.Errorf("composer(works[0]) = %q, want Wolfgang Amadeus Mozart", got)
	}
	if works[0].Disambiguation != "ed. Maunder" {
		t.Errorf("works[0].Disambiguation = %q", works[0].Disambiguation)
	}

	// The end-to-end pipeline filters movements; verify that path too.
	kept := filterMovements(works)
	if len(kept) != 1 || kept[0].Title != "Great Mass in C minor, K. 427" {
		t.Errorf("after filterMovements got %v, want only the parent Mass", kept)
	}
}

func TestSearchWorks_NonOKStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		w.Write([]byte("upstream error"))
	}))
	defer server.Close()

	if _, err := searchWorks(server.URL, "anything"); err == nil {
		t.Fatal("expected error for non-200 response, got nil")
	}
}

func TestSearchWorks_EmptyResults(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"count":0,"offset":0,"works":[]}`))
	}))
	defer server.Close()

	works, err := searchWorks(server.URL, "anything")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(works) != 0 {
		t.Errorf("got %d works, want 0", len(works))
	}
}

func TestBrowseRecordingsByWork_ParsesAndSendsParams(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("User-Agent"); got != userAgent {
			t.Errorf("User-Agent = %q, want %q", got, userAgent)
		}
		q := r.URL.Query()
		if got := q.Get("work"); got != "ee2b44c5-ba74-4435-ab94-b82c9da84054" {
			t.Errorf("work param = %q", got)
		}
		if got := q.Get("fmt"); got != "json" {
			t.Errorf("fmt param = %q", got)
		}
		if got := q.Get("inc"); got != "artist-credits artist-rels" {
			t.Errorf("inc param = %q, want space-separated 'artist-credits artist-rels'", got)
		}
		w.Write([]byte(cannedRecordingsResponse))
	}))
	defer server.Close()

	recs, err := browseRecordingsByWork(server.URL, "ee2b44c5-ba74-4435-ab94-b82c9da84054")
	if err != nil {
		t.Fatalf("browseRecordingsByWork: %v", err)
	}
	if len(recs) != 4 {
		t.Fatalf("got %d recordings, want 4", len(recs))
	}
	if recs[0].FirstReleaseDate != "1980-03-01" {
		t.Errorf("recs[0].FirstReleaseDate = %q", recs[0].FirstReleaseDate)
	}
	if len(recs[0].Relations) != 3 {
		t.Errorf("recs[0] has %d relations, want 3", len(recs[0].Relations))
	}
}

func TestBrowseRecordingsByWork_NonOKStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":"bad work id"}`))
	}))
	defer server.Close()
	if _, err := browseRecordingsByWork(server.URL, "garbage"); err == nil {
		t.Fatal("expected error for HTTP 400, got nil")
	}
}
