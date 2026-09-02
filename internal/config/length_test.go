package config

import (
	"math"
	"strings"
	"testing"
)

func TestParseLength(t *testing.T) {
	tests := []struct {
		in   string
		want float64
	}{
		{"16mm", 16},
		{"2cm", 20},
		{"1in", 25.4},
		{"0.75in", 19.05},
		{"72pt", 25.4},
		{"96px", 25.4},
		{" 16mm ", 16},
		{"16MM", 16},
		{"12.5mm", 12.5},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := ParseLength(tt.in)
			if err != nil {
				t.Fatalf("ParseLength(%q) error = %v", tt.in, err)
			}
			if math.Abs(got-tt.want) > 1e-9 {
				t.Errorf("ParseLength(%q) = %v mm, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestParseLengthErrors(t *testing.T) {
	tests := []struct{ in, wantMsg string }{
		{"", "empty"},
		// A bare number on a printed document is ambiguous, so it is refused.
		{"16", "no unit"},
		{"mm", "not a number"},
		{"0mm", "greater than zero"},
		{"-4mm", "greater than zero"},
		{"16em", "no unit"},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			_, err := ParseLength(tt.in)
			if err == nil {
				t.Fatalf("ParseLength(%q) error = nil", tt.in)
			}
			if !strings.Contains(err.Error(), tt.wantMsg) {
				t.Errorf("ParseLength(%q) error = %v, want it to mention %q", tt.in, err, tt.wantMsg)
			}
		})
	}
}

func TestFormatLength(t *testing.T) {
	tests := []struct {
		in   float64
		want string
	}{
		{16, "16mm"},
		{12.5, "12.5mm"},
		{6.25, "6.25mm"},
		{26.458333, "26.46mm"},
	}
	for _, tt := range tests {
		if got := FormatLength(tt.in); got != tt.want {
			t.Errorf("FormatLength(%v) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
