package document

import (
	"testing"
	"time"
)

func TestNewIDCarriesItsTime(t *testing.T) {
	// Millisecond precision is all a ULID records.
	at := time.Date(2026, 9, 2, 21, 23, 58, 0, time.UTC)

	id, err := NewID(at)
	if err != nil {
		t.Fatalf("NewID() error = %v", err)
	}
	if len(id) != IDLength {
		t.Errorf("NewID() = %q, want %d characters", id, IDLength)
	}
	if got := id.Time(); !got.Equal(at) {
		t.Errorf("Time() = %s, want %s", got, at)
	}
}

// Ids sort chronologically as plain text. The listing and the migration both
// lean on it, so that the store reads in the order it was written.
func TestIDsSortByTime(t *testing.T) {
	older, err := NewID(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("NewID() error = %v", err)
	}
	newer, err := NewID(time.Date(2026, 9, 24, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("NewID() error = %v", err)
	}
	if !(older < newer) {
		t.Errorf("%s should sort before %s", older, newer)
	}
}

// Two ids minted in the same millisecond still differ: 80 bits of randomness
// is the whole reason this replaces a shared counter.
func TestNewIDIsUniqueWithinAMillisecond(t *testing.T) {
	at := time.Date(2026, 9, 2, 21, 23, 58, 0, time.UTC)

	seen := make(map[ID]bool, 500)
	for range 500 {
		id, err := NewID(at)
		if err != nil {
			t.Fatalf("NewID() error = %v", err)
		}
		if seen[id] {
			t.Fatalf("NewID() repeated %s", id)
		}
		seen[id] = true
	}
}

func TestParseID(t *testing.T) {
	id, err := NewID(time.Now())
	if err != nil {
		t.Fatalf("NewID() error = %v", err)
	}

	tests := []struct {
		name  string
		input string
		want  ID
		ok    bool
	}{
		{"canonical", string(id), id, true},
		// A shell completion or a copy-paste may lowercase it.
		{"lowercased", lower(string(id)), id, true},
		{"surrounding space", " " + string(id) + " ", id, true},
		{"a legacy numeric id", "7", "", false},
		{"a prefix", string(id)[:6], "", false},
		{"a product slug", "suspension-shower-gel", "", false},
		{"right length, wrong alphabet", "01K6H3PZ8Q7XN4V2R9BKTC5MIL", "", false},
		{"empty", "", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseID(tt.input)
			if tt.ok && err != nil {
				t.Fatalf("ParseID(%q) error = %v", tt.input, err)
			}
			if !tt.ok && err == nil {
				t.Fatalf("ParseID(%q) = %q, want an error", tt.input, got)
			}
			if got != tt.want {
				t.Errorf("ParseID(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// IsID is what tells a document directory from anything else in the store, so
// it has to reject the numeric names the store used to use.
func TestIsID(t *testing.T) {
	id, err := NewID(time.Now())
	if err != nil {
		t.Fatalf("NewID() error = %v", err)
	}

	for _, name := range []string{string(id)} {
		if !IsID(name) {
			t.Errorf("IsID(%q) = false, want true", name)
		}
	}
	for _, name := range []string{"1", "10", "index.yaml", ".git", "versions", ""} {
		if IsID(name) {
			t.Errorf("IsID(%q) = true, want false", name)
		}
	}
}

func TestShort(t *testing.T) {
	id, err := NewID(time.Now())
	if err != nil {
		t.Fatalf("NewID() error = %v", err)
	}
	if got := id.Short(); len(got) != ShortIDLength || string(id[:ShortIDLength]) != got {
		t.Errorf("Short() = %q, want the first %d characters of %s", got, ShortIDLength, id)
	}
	// A malformed id must not panic; Short is display-only.
	if got := ID("abc").Short(); got != "abc" {
		t.Errorf("Short() on a short string = %q, want %q", got, "abc")
	}
}

func lower(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + ('a' - 'A')
		}
	}
	return string(b)
}
