package document

import "testing"

func TestSlugify(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"Caustic Soda 50%", "caustic-soda-50"},
		{"Acetone", "acetone"},
		{"Sodium Hydroxide (50% w/w)", "sodium-hydroxide-50-w-w"},
		{"  padded  ", "padded"},
		{"%%%", ""},
		{"", ""},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			if got := Slugify(tt.in); got != tt.want {
				t.Errorf("Slugify(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
