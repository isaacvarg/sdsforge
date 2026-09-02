package document

import "testing"

func TestParseSemver(t *testing.T) {
	tests := []struct {
		in      string
		want    Semver
		wantErr bool
	}{
		{in: "1.0.0", want: Semver{1, 0, 0}},
		{in: "0.0.1", want: Semver{0, 0, 1}},
		{in: "12.4.107", want: Semver{12, 4, 107}},
		// A version label is a regulatory identifier, so anything a reader
		// could not compare against a printed sheet at a glance is rejected.
		{in: "1.0", wantErr: true},
		{in: "1.0.0.0", wantErr: true},
		{in: "v1.0.0", wantErr: true},
		{in: "1.0.0-rc1", wantErr: true},
		{in: "1.0.0+build3", wantErr: true},
		{in: "1.-2.0", wantErr: true},
		{in: "1..0", wantErr: true},
		{in: "", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := ParseSemver(tt.in)
			if tt.wantErr {
				if err == nil {
					t.Errorf("ParseSemver(%q) = %v, want an error", tt.in, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseSemver(%q) error = %v", tt.in, err)
			}
			if got != tt.want {
				t.Errorf("ParseSemver(%q) = %v, want %v", tt.in, got, tt.want)
			}
			if got.String() != tt.in {
				t.Errorf("round trip = %q, want %q", got.String(), tt.in)
			}
		})
	}
}

func TestSemverBump(t *testing.T) {
	// Everything below the bumped part resets: the minor after 1.4.7 is 1.5.0,
	// not 1.5.7.
	tests := []struct {
		in   string
		part BumpPart
		want string
	}{
		{"1.4.7", BumpMajor, "2.0.0"},
		{"1.4.7", BumpMinor, "1.5.0"},
		{"1.4.7", BumpPatch, "1.4.8"},
		{"0.0.0", BumpPatch, "0.0.1"},
	}
	for _, tt := range tests {
		t.Run(tt.in+"->"+tt.want, func(t *testing.T) {
			in, err := ParseSemver(tt.in)
			if err != nil {
				t.Fatalf("ParseSemver(%q) error = %v", tt.in, err)
			}
			if got := in.Bump(tt.part).String(); got != tt.want {
				t.Errorf("%s.Bump(%d) = %s, want %s", tt.in, tt.part, got, tt.want)
			}
		})
	}
}

func TestSemverLess(t *testing.T) {
	tests := []struct {
		a, b string
		want bool
	}{
		{"1.0.0", "1.0.1", true},
		{"1.0.0", "1.1.0", true},
		{"1.9.9", "2.0.0", true},
		{"2.0.0", "1.9.9", false},
		{"1.0.0", "1.0.0", false}, // equal is not less: a reissue needs a new number
		{"1.10.0", "1.9.0", false},
	}
	for _, tt := range tests {
		t.Run(tt.a+"<"+tt.b, func(t *testing.T) {
			a, err := ParseSemver(tt.a)
			if err != nil {
				t.Fatalf("ParseSemver(%q) error = %v", tt.a, err)
			}
			b, err := ParseSemver(tt.b)
			if err != nil {
				t.Fatalf("ParseSemver(%q) error = %v", tt.b, err)
			}
			if got := a.Less(b); got != tt.want {
				t.Errorf("%s.Less(%s) = %v, want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}
