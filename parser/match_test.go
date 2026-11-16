package parser

import "testing"

func TestContainsText(t *testing.T) {
	text := "public trust in the research are under-\nmined [41]. As a community,\nwe must do   better. The key-\nvalue store"
	cases := []struct {
		q    string
		want bool
	}{
		{"undermined", true},           // end-of-line hyphenation
		{"under-", true},               // literal text still matches
		{"a community, we must", true}, // phrase across a line wrap
		{"do better", true},            // collapsed spaces
		{"key-value", true},            // hyphen kept when the query has it
		{"keyvalue", true},             // and seen through when it does not
		{"public  trust", true},        // query whitespace is normalized too
		{"mined", true},
		{"overmined", false},
		{"", false},
	}
	for _, c := range cases {
		if got := ContainsText(text, c.q); got != c.want {
			t.Errorf("ContainsText(%q) = %v, want %v", c.q, got, c.want)
		}
	}
	// "- " that is punctuation, not hyphenation, must not join words.
	if ContainsText("costs fell -\nprices rose", "fellprices") {
		t.Error("a spaced dash is not a hyphen")
	}
}
