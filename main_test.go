package main

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestBuildQuery(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "two args: composer and work",
			args: []string{"Mozart", "Great Mass in C"},
			want: `artist:Mozart AND work:(Great AND Mass AND in AND C)`,
		},
		{
			name: "single arg with multiple words splits on whitespace",
			args: []string{"Mozart Great Mass in C"},
			want: `artist:Mozart AND work:(Great AND Mass AND in AND C)`,
		},
		{
			name: "many unquoted shell args join into work name",
			args: []string{"Mozart", "Great", "Mass", "in", "C"},
			want: `artist:Mozart AND work:(Great AND Mass AND in AND C)`,
		},
		{
			name: "composer with spaces uses AND inside parens",
			args: []string{"Wolfgang Amadeus Mozart", "Great Mass in C"},
			want: `artist:(Wolfgang AND Amadeus AND Mozart) AND work:(Great AND Mass AND in AND C)`,
		},
		{
			name: "single word falls back to unstructured query",
			args: []string{"Mozart"},
			want: "Mozart",
		},
		{
			name: "Bach Mass in B minor uses AND so all words must match",
			args: []string{"Bach", "Mass in B minor"},
			want: `artist:Bach AND work:(Mass AND in AND B AND minor)`,
		},
		{
			name: "Lucene-special characters are stripped from input",
			args: []string{"Mozart", `Great "Mass" (extra) in C`},
			want: `artist:Mozart AND work:(Great AND Mass AND extra AND in AND C)`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildQuery(tt.args)
			if got != tt.want {
				t.Errorf("buildQuery(%v) = %q, want %q", tt.args, got, tt.want)
			}
		})
	}
}

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

// canned MusicBrainz JSON for "artist:Mozart AND work:\"Great Mass in C\""
// captured live and trimmed to the relevant fields the parser cares about.
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

	_, err := searchWorks(server.URL, "anything")
	if err == nil {
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
