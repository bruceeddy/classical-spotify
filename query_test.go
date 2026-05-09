package main

import "testing"

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
