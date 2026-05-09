package main

import "sort"

// Performance is one distinct recording of a Work, identified by the
// (conductor, orchestra, year) fingerprint. Vocals collects every choir
// or vocal-soloist credit seen across the recordings that share the
// fingerprint.
type Performance struct {
	Conductor string
	Orchestra string
	Year      string
	Vocals    []string
}

// groupRecordings deduplicates a list of recordings into Performances by
// (conductor, orchestra, year). Vocal credits seen across grouped
// recordings are merged so that the choir (and any soloists) appear
// once per performance. Results are sorted by year descending; missing
// years sink to the bottom.
func groupRecordings(recs []Recording) []Performance {
	type key struct{ conductor, orchestra, year string }
	byKey := map[key]*Performance{}
	var order []key
	for _, r := range recs {
		var conductor, orchestra string
		var vocals []string
		for _, rel := range r.Relations {
			if rel.Artist == nil {
				continue
			}
			switch rel.Type {
			case "conductor":
				conductor = rel.Artist.Name
			case "performing orchestra":
				orchestra = rel.Artist.Name
			case "vocal":
				vocals = append(vocals, rel.Artist.Name)
			}
		}
		year := ""
		if len(r.FirstReleaseDate) >= 4 {
			year = r.FirstReleaseDate[:4]
		}
		k := key{conductor, orchestra, year}
		p, ok := byKey[k]
		if !ok {
			byKey[k] = &Performance{conductor, orchestra, year, vocals}
			order = append(order, k)
			continue
		}
		seen := make(map[string]bool, len(p.Vocals))
		for _, v := range p.Vocals {
			seen[v] = true
		}
		for _, v := range vocals {
			if !seen[v] {
				p.Vocals = append(p.Vocals, v)
				seen[v] = true
			}
		}
	}
	out := make([]Performance, len(order))
	for i, k := range order {
		out[i] = *byKey[k]
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Year > out[j].Year
	})
	return out
}
