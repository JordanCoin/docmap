package render

import (
	"testing"
)

func TestFormatTokens(t *testing.T) {
	tests := []struct {
		input    int
		expected string
	}{
		{0, "0"},
		{500, "500"},
		{999, "999"},
		{1000, "1.0k"},
		{1500, "1.5k"},
		{10000, "10.0k"},
	}

	for _, tc := range tests {
		got := formatTokens(tc.input)
		if got != tc.expected {
			t.Errorf("formatTokens(%d) = %q, want %q", tc.input, got, tc.expected)
		}
	}
}

func TestCenterText(t *testing.T) {
	tests := []struct {
		input    string
		width    int
		expected string
	}{
		{"test", 10, "   test"},
		{"hello", 5, "hello"},
		{"hi", 6, "  hi"},
	}

	for _, tc := range tests {
		got := centerText(tc.input, tc.width)
		if got != tc.expected {
			t.Errorf("centerText(%q, %d) = %q, want %q", tc.input, tc.width, got, tc.expected)
		}
	}
}
