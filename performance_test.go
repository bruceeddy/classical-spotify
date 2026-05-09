package main

import (
	"reflect"
	"testing"
)

func TestGroupRecordings(t *testing.T) {
	// Two recordings share a (Karajan / Berlin Phil / 1980) fingerprint
	// with different vocal credits per movement, exercising the merge
	// path. A third is a different performance. A fourth has no
	// relations and an unknown date — its empty fingerprint must not
	// merge with anything else.
	recs := []Recording{
		{
			Title:            "Mass: I. Kyrie",
			FirstReleaseDate: "1980-03-01",
			Relations: []RecordingRelation{
				{Type: "conductor", Artist: &WorkArtist{Name: "Karajan"}},
				{Type: "performing orchestra", Artist: &WorkArtist{Name: "Berlin Philharmonic"}},
				{Type: "vocal", Artist: &WorkArtist{Name: "Vienna Singverein"}},
			},
		},
		{
			Title:            "Mass: II. Gloria",
			FirstReleaseDate: "1980-03-01",
			Relations: []RecordingRelation{
				{Type: "conductor", Artist: &WorkArtist{Name: "Karajan"}},
				{Type: "performing orchestra", Artist: &WorkArtist{Name: "Berlin Philharmonic"}},
				{Type: "vocal", Artist: &WorkArtist{Name: "Vienna Singverein"}},
				{Type: "vocal", Artist: &WorkArtist{Name: "Edith Mathis"}},
			},
		},
		{
			Title:            "Mass: I. Kyrie",
			FirstReleaseDate: "1985-06-15",
			Relations: []RecordingRelation{
				{Type: "conductor", Artist: &WorkArtist{Name: "Gardiner"}},
				{Type: "performing orchestra", Artist: &WorkArtist{Name: "English Baroque Soloists"}},
				{Type: "vocal", Artist: &WorkArtist{Name: "Monteverdi Choir"}},
			},
		},
		{
			Title:            "Some Excerpt",
			FirstReleaseDate: "",
		},
	}

	got := groupRecordings(recs)
	if len(got) != 3 {
		t.Fatalf("got %d performances, want 3 (Karajan grouped, Gardiner, no-relations)", len(got))
	}

	// Sorted year-desc: 1985, 1980, "".
	if got[0].Year != "1985" || got[0].Conductor != "Gardiner" {
		t.Errorf("got[0] = %+v, want Gardiner 1985", got[0])
	}
	if got[1].Year != "1980" || got[1].Conductor != "Karajan" {
		t.Errorf("got[1] = %+v, want Karajan 1980", got[1])
	}
	if got[2].Year != "" || got[2].Conductor != "" {
		t.Errorf("got[2] = %+v, want empty conductor and year", got[2])
	}

	// Karajan's two movements should have merged vocals: Vienna Singverein
	// from the Kyrie plus Edith Mathis from the Gloria, in first-seen order.
	want := []string{"Vienna Singverein", "Edith Mathis"}
	if !reflect.DeepEqual(got[1].Vocals, want) {
		t.Errorf("got[1].Vocals = %v, want %v", got[1].Vocals, want)
	}

	if got[1].Orchestra != "Berlin Philharmonic" {
		t.Errorf("got[1].Orchestra = %q", got[1].Orchestra)
	}
}
