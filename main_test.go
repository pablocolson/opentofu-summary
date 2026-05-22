package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestConfirmApply(t *testing.T) {
	cases := []struct {
		name   string
		input  string
		want   bool
		prompt string // substring that must appear on the writer
	}{
		{"exact yes", "yes\n", true, "Apply this plan?"},
		{"YES uppercase", "YES\n", true, ""},
		{"yes with surrounding whitespace", "  yes  \n", true, ""},
		{"y alone is not enough", "y\n", false, ""},
		{"plain enter", "\n", false, ""},
		{"empty stdin (EOF)", "", false, ""},
		{"no, anything but yes", "no\n", false, ""},
		{"yes but with extra word", "yes please\n", false, ""},
		{"yolo typo", "yse\n", false, ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var w bytes.Buffer
			got := confirmApply(&w, strings.NewReader(tc.input))
			if got != tc.want {
				t.Errorf("confirmApply(%q) = %v, want %v", tc.input, got, tc.want)
			}
			if tc.prompt != "" && !strings.Contains(w.String(), tc.prompt) {
				t.Errorf("prompt %q not written; got: %q", tc.prompt, w.String())
			}
		})
	}
}
