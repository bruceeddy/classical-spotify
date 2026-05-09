package main

import "testing"

func TestParseQueryArgs(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		composer string
		work     string
	}{
		{"no args", []string{}, "", ""},
		{"two args: composer and work", []string{"Mozart", "Great Mass in C"}, "Mozart", "Great Mass in C"},
		{"five unquoted args", []string{"Mozart", "Great", "Mass", "in", "C"}, "Mozart", "Great Mass in C"},
		{"single multi-word arg", []string{"Mozart Great Mass in C"}, "Mozart", "Great Mass in C"},
		{"single word, no work", []string{"Mozart"}, "Mozart", ""},
		{"composer with spaces (quoted)", []string{"Wolfgang Amadeus Mozart", "Great Mass in C"}, "Wolfgang Amadeus Mozart", "Great Mass in C"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotC, gotW := parseQueryArgs(tt.args)
			if gotC != tt.composer || gotW != tt.work {
				t.Errorf("parseQueryArgs(%v) = (%q, %q), want (%q, %q)", tt.args, gotC, gotW, tt.composer, tt.work)
			}
		})
	}
}

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
		{
			name: "hyphenated composer (Rimsky-Korsakov) splits cleanly",
			args: []string{"Rimsky-Korsakov", "Scheherazade"},
			want: `artist:(Rimsky AND Korsakov) AND work:Scheherazade`,
		},
		{
			name: "hyphenated composer with hyphenated work (Saint-Saëns)",
			args: []string{"Saint-Saëns", "Carnival of the Animals"},
			want: `artist:(Saint AND Saëns) AND work:(Carnival AND of AND the AND Animals)`,
		},
		{
			name: "miscellaneous Lucene specials are all neutralised",
			args: []string{"Mozart", `Mass +in C: K.427 / "Great"`},
			want: `artist:Mozart AND work:(Mass AND in AND C AND K.427 AND Great)`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildQuery(tt.args, "")
			if got != tt.want {
				t.Errorf("buildQuery(%v) = %q, want %q", tt.args, got, tt.want)
			}
		})
	}
}

func TestBuildQuery_WithArid(t *testing.T) {
	tests := []struct {
		name string
		args []string
		mbid string
		want string
	}{
		{
			name: "MBID supplied — artist clause becomes arid",
			args: []string{"Rimsky-Korsakov", "Scheherazade"},
			mbid: "4cfe7051-f649-4d07-83b3-7a732abe7249",
			want: `arid:4cfe7051-f649-4d07-83b3-7a732abe7249 AND work:Scheherazade`,
		},
		{
			name: "MBID supplied — composer text is ignored regardless of complexity",
			args: []string{"Wolfgang Amadeus Mozart", "Great Mass in C"},
			mbid: "b972f589-fb0e-474e-b64a-803b0364fa75",
			want: `arid:b972f589-fb0e-474e-b64a-803b0364fa75 AND work:(Great AND Mass AND in AND C)`,
		},
		{
			name: "single-word fallback ignores MBID",
			args: []string{"Mozart"},
			mbid: "b972f589-fb0e-474e-b64a-803b0364fa75",
			want: "Mozart",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildQuery(tt.args, tt.mbid)
			if got != tt.want {
				t.Errorf("buildQuery(%v, %q) = %q, want %q", tt.args, tt.mbid, got, tt.want)
			}
		})
	}
}
