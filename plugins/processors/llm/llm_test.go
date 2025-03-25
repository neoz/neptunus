package llm

import (
	"strings"
	"testing"
)

func TestTrim(t *testing.T) {
	// Test cases
	tests := []struct {
		input    string
		expected string
	}{
		{"", ""},
		{" ", ""},
		{"  ", ""},
		{"   ", ""},
		{"    ", ""},
		{"     ", ""},
		{"      ", ""},
		{"\n\n", ""},
		{"\n\n\n", ""},
		{"\n\n\n\n", ""},
	}

	for _, test := range tests {
		actual := strings.Trim(test.input, "\t\r\n ")
		if actual != test.expected {
			t.Errorf("Trim(%s): expected %s, actual %s", test.input, test.expected, actual)
		}
	}
}
