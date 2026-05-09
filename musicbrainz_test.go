package main

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
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
		if got := q.Get("inc"); got != "artist-credits artist-rels url-rels" {
			t.Errorf("inc param = %q, want space-separated 'artist-credits artist-rels url-rels'", got)
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

func TestResolveComposerMBID(t *testing.T) {
	// Mirrors the live shape for "Rimsky-Korsakov": top result is a
	// Group ("Rimsky-Korsakov Quartet"); next is Maria (Person, "daughter
	// of the composer", score 99); then the actual composer Nikolai
	// (Person, "Russian composer", score 94). Resolver must skip the
	// Group AND skip Maria (relative-of-composer) and land on Nikolai.
	const canned = `{
  "artists": [
    {"id": "quartet-id",  "name": "Rimsky-Korsakov Quartet",  "score": 100, "type": "Group",  "disambiguation": ""},
    {"id": "maria-id",    "name": "Maria Rimsky-Korsakov",    "score": 99,  "type": "Person", "disambiguation": "daughter of the composer"},
    {"id": "composer-id", "name": "Nikolai Rimsky-Korsakov",  "score": 94,  "type": "Person", "disambiguation": "Russian composer"},
    {"id": "andrey-id",   "name": "Andrey Rimsky-Korsakov",   "score": 73,  "type": "Person", "disambiguation": "musicologist, son of the composer"}
  ]
}`
	t.Run("picks the Person whose disambiguation marks them a composer", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if got := r.URL.Query().Get("query"); got != "Rimsky Korsakov" {
				t.Errorf("query param = %q", got)
			}
			w.Write([]byte(canned))
		}))
		defer server.Close()
		got, err := resolveComposerMBID(server.URL, "Rimsky Korsakov")
		if err != nil {
			t.Fatal(err)
		}
		if got != "composer-id" {
			t.Errorf("got %q, want composer-id (Maria's 'daughter of the composer' must not match)", got)
		}
	})

	t.Run("returns empty when no Person has a composer-disambiguation", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(`{"artists":[
                {"id":"g1","name":"Some Quartet","score":100,"type":"Group"},
                {"id":"p1","name":"Some Soprano","score":90,"type":"Person","disambiguation":"soprano"},
                {"id":"o1","name":"Some Orchestra","score":80,"type":"Orchestra"}
            ]}`))
		}))
		defer server.Close()
		got, err := resolveComposerMBID(server.URL, "anything")
		if err != nil {
			t.Fatal(err)
		}
		if got != "" {
			t.Errorf("got %q, want empty (no Person tagged as composer)", got)
		}
	})

	t.Run("empty input returns empty with no HTTP call", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t.Error("HTTP call should not be made for empty input")
		}))
		defer server.Close()
		got, err := resolveComposerMBID(server.URL, "")
		if err != nil {
			t.Fatal(err)
		}
		if got != "" {
			t.Errorf("got %q, want empty", got)
		}
	})
}

func TestIsComposerDisambiguation(t *testing.T) {
	cases := []struct {
		s    string
		want bool
	}{
		{"Russian composer", true},
		{"classical composer", true},
		{"composer", true},
		{"German composer, organist", true},
		{"Baroque composer (1685–1750)", true},
		{"daughter of the composer", false},
		{"musicologist, son of the composer", false},
		{"wife of the composer", false},
		{"soprano", false},
		{"", false},
		{"musicologist", false},
	}
	for _, c := range cases {
		if got := isComposerDisambiguation(c.s); got != c.want {
			t.Errorf("isComposerDisambiguation(%q) = %v, want %v", c.s, got, c.want)
		}
	}
}

func TestLookupWorkOtherVersions_ParsesAndFilters(t *testing.T) {
	// Mixed relations: composer, parts, performance, and two "other
	// version" links. Only the other-version targets should come back.
	const canned = `{
  "id": "fc221f1e-a2ad-4591-96a5-704c1539fbe3",
  "title": "Great Mass in C minor, K. 427",
  "type": "Mass",
  "relations": [
    {"type": "composer", "direction": "backward", "artist": {"name": "Mozart"}},
    {"type": "parts", "direction": "forward", "work": {"id": "movement-1", "title": "Kyrie", "type": ""}},
    {"type": "other version", "direction": "backward",
     "work": {"id": "712210fe-fragment", "title": "Missa in c-Moll, K. 427/417a", "type": "Mass"}},
    {"type": "other version", "direction": "forward",
     "work": {"id": "ee2b44c5-levin", "title": "Mass no. 17 ... Levin", "type": "Mass"}},
    {"type": "performance", "direction": "backward",
     "recording": {"id": "rec-x", "title": "Some Kyrie"}}
  ]
}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "fc221f1e") {
			t.Errorf("URL path = %q, expected MBID in path", r.URL.Path)
		}
		if got := r.URL.Query().Get("inc"); got != "work-rels" {
			t.Errorf("inc = %q, want work-rels", got)
		}
		if got := r.URL.Query().Get("fmt"); got != "json" {
			t.Errorf("fmt = %q", got)
		}
		w.Write([]byte(canned))
	}))
	defer server.Close()

	siblings, err := lookupWorkOtherVersions(server.URL+"/", "fc221f1e-a2ad-4591-96a5-704c1539fbe3")
	if err != nil {
		t.Fatalf("lookupWorkOtherVersions: %v", err)
	}
	if len(siblings) != 2 {
		t.Fatalf("got %d siblings, want 2 (other-version only, ignoring composer/parts/performance)", len(siblings))
	}
	gotIDs := []string{siblings[0].ID, siblings[1].ID}
	wantIDs := []string{"712210fe-fragment", "ee2b44c5-levin"}
	if !reflect.DeepEqual(gotIDs, wantIDs) {
		t.Errorf("sibling IDs = %v, want %v", gotIDs, wantIDs)
	}
	if siblings[0].Title != "Missa in c-Moll, K. 427/417a" {
		t.Errorf("siblings[0].Title = %q", siblings[0].Title)
	}
}

func TestExpandEditions_BFSAndDedup(t *testing.T) {
	// Topology: A → B; B → C, D, A. A is the seed. Expansion should
	// reach {A, B, C, D} once and not re-add A on the back-edge.
	responses := map[string]string{
		"A": `{"id":"A","title":"A-work","type":"Mass","relations":[
              {"type":"other version","direction":"forward","work":{"id":"B","title":"B-work","type":"Mass"}}]}`,
		"B": `{"id":"B","title":"B-work","type":"Mass","relations":[
              {"type":"other version","direction":"backward","work":{"id":"A","title":"A-work","type":"Mass"}},
              {"type":"other version","direction":"forward","work":{"id":"C","title":"C-work","type":"Mass"}},
              {"type":"other version","direction":"forward","work":{"id":"D","title":"D-work","type":"Mass"}}]}`,
		"C": `{"id":"C","title":"C-work","type":"Mass","relations":[]}`,
		"D": `{"id":"D","title":"D-work","type":"Mass","relations":[]}`,
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/")
		body, ok := responses[id]
		if !ok {
			t.Errorf("unexpected lookup for %q", id)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Write([]byte(body))
	}))
	defer server.Close()

	got := expandEditions(server.URL+"/", []Work{{ID: "A"}}, 3, 10, 0)
	gotIDs := make([]string, len(got))
	for i, w := range got {
		gotIDs[i] = w.ID
	}
	wantIDs := []string{"A", "B", "C", "D"}
	if !reflect.DeepEqual(gotIDs, wantIDs) {
		t.Errorf("expanded IDs = %v, want %v (BFS order, no re-addition of seed)", gotIDs, wantIDs)
	}
}

func TestExpandEditions_RespectsMaxExpandedCap(t *testing.T) {
	// A → B → C → D. With maxExpanded=2 the walk should stop after A, B.
	responses := map[string]string{
		"A": `{"id":"A","title":"A","type":"Mass","relations":[{"type":"other version","work":{"id":"B","title":"B","type":"Mass"}}]}`,
		"B": `{"id":"B","title":"B","type":"Mass","relations":[{"type":"other version","work":{"id":"C","title":"C","type":"Mass"}}]}`,
		"C": `{"id":"C","title":"C","type":"Mass","relations":[{"type":"other version","work":{"id":"D","title":"D","type":"Mass"}}]}`,
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/")
		w.Write([]byte(responses[id]))
	}))
	defer server.Close()
	got := expandEditions(server.URL+"/", []Work{{ID: "A"}}, 5, 2, 0)
	if len(got) != 2 {
		t.Fatalf("got %d expanded, want 2 (cap)", len(got))
	}
	if got[0].ID != "A" || got[1].ID != "B" {
		t.Errorf("expanded IDs = [%s, %s], want [A, B]", got[0].ID, got[1].ID)
	}
}

func TestExpandEditions_RespectsMaxHops(t *testing.T) {
	// A → B → C. With maxHops=1 only A's children (B) should be added.
	responses := map[string]string{
		"A": `{"id":"A","title":"A","type":"Mass","relations":[{"type":"other version","work":{"id":"B","title":"B","type":"Mass"}}]}`,
		"B": `{"id":"B","title":"B","type":"Mass","relations":[{"type":"other version","work":{"id":"C","title":"C","type":"Mass"}}]}`,
		"C": `{"id":"C","title":"C","type":"Mass","relations":[]}`,
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/")
		w.Write([]byte(responses[id]))
	}))
	defer server.Close()
	got := expandEditions(server.URL+"/", []Work{{ID: "A"}}, 1, 10, 0)
	gotIDs := make([]string, len(got))
	for i, w := range got {
		gotIDs[i] = w.ID
	}
	wantIDs := []string{"A", "B"}
	if !reflect.DeepEqual(gotIDs, wantIDs) {
		t.Errorf("expanded IDs = %v, want %v (hop cap should stop before C)", gotIDs, wantIDs)
	}
}

func TestExpandEditions_NoOtherVersions(t *testing.T) {
	// Single Work with no other-version relations (Bach BWV 232 case).
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"id":"A","title":"A","type":"Mass","relations":[]}`))
	}))
	defer server.Close()
	got := expandEditions(server.URL+"/", []Work{{ID: "A", Title: "Solo"}}, 2, 10, 0)
	if len(got) != 1 || got[0].ID != "A" {
		t.Errorf("got %v, want just [A] when there are no other-version edges", got)
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
