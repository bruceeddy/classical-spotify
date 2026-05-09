package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"time"
)

// maxWorksToBrowse caps how many of the matched Works we fetch
// recordings for, to keep us inside MusicBrainz's 1 req/sec budget
// even when the search returns many edition-variants of the same
// piece.
const maxWorksToBrowse = 5

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

	displayWorks(works)

	recs := gatherRecordings(works)
	performances := groupRecordings(recs)
	if len(performances) == 0 {
		fmt.Println("No recordings linked in MusicBrainz for the matched work(s).")
		return
	}
	displayPerformances(performances)
}

// gatherRecordings browses MusicBrainz for recordings linked to each
// Work, capped at maxWorksToBrowse and spaced out by 1s to respect MB's
// public rate limit. Errors on individual browse calls are reported to
// stderr; successful results are concatenated.
func gatherRecordings(works []Work) []Recording {
	toBrowse := works
	if len(toBrowse) > maxWorksToBrowse {
		toBrowse = toBrowse[:maxWorksToBrowse]
	}
	var recs []Recording
	for i, w := range toBrowse {
		if i > 0 {
			time.Sleep(time.Second)
		}
		got, err := browseRecordingsByWork(musicBrainzRecordingURL, w.ID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error browsing recordings for %s: %v\n", w.ID, err)
			continue
		}
		recs = append(recs, got...)
	}
	return recs
}

func displayWorks(works []Work) {
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

func displayPerformances(performances []Performance) {
	fmt.Printf("Recordings (%d):\n\n", len(performances))
	for i, p := range performances {
		year := p.Year
		if year == "" {
			year = "????"
		}
		conductor := p.Conductor
		if conductor == "" {
			conductor = "(no conductor credited)"
		}
		fmt.Printf("%d. %s  %s\n", i+1, year, conductor)
		if p.Orchestra != "" {
			fmt.Printf("   Orchestra: %s\n", p.Orchestra)
		}
		if len(p.Vocals) > 0 {
			fmt.Printf("   Vocal:     %s\n", strings.Join(p.Vocals, ", "))
		}
		if p.SpotifyURL != "" {
			fmt.Printf("   Spotify:   %s\n", p.SpotifyURL)
		}
		fmt.Println()
	}
}
