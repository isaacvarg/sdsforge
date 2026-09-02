package config

import (
	"math"
	"strings"
	"testing"
)

// A file with no [pdf] table is the normal case; toml.Unmarshal leaves the
// fields empty rather than at what Default() put there, so validate has to
// fill them back in.
func TestLoadPDFDefaults(t *testing.T) {
	write(t, "config.toml", "[company]\nname = \"Acme\"\n")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.PDF.Paper != defaultPaper {
		t.Errorf("paper = %q, want %q", cfg.PDF.Paper, defaultPaper)
	}
	if cfg.PDF.Margin != defaultMargin {
		t.Errorf("margin = %q, want %q", cfg.PDF.Margin, defaultMargin)
	}
	if cfg.PDF.Browser != "" {
		t.Errorf("browser = %q, want empty so PATH is searched", cfg.PDF.Browser)
	}
}

func TestLoadPDF(t *testing.T) {
	write(t, "config.toml", `
[pdf]
browser = "/usr/bin/chromium"
paper   = "A4"
margin  = "20mm"
`)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.PDF.Browser != "/usr/bin/chromium" {
		t.Errorf("browser = %q", cfg.PDF.Browser)
	}
	// Normalised on the way in, so Geometry need not case-fold at every lookup.
	if cfg.PDF.Paper != "a4" {
		t.Errorf("paper = %q, want %q", cfg.PDF.Paper, "a4")
	}
	if cfg.PDF.Margin != "20mm" {
		t.Errorf("margin = %q", cfg.PDF.Margin)
	}
}

// Caught when the file is read rather than at the moment a user prints.
func TestLoadRejectsBadPaper(t *testing.T) {
	write(t, "config.toml", "[pdf]\npaper = \"tabloid\"\n")

	_, err := Load()
	if err == nil {
		t.Fatal("Load() accepted an unknown paper size")
	}
	if !strings.Contains(err.Error(), "pdf.paper") {
		t.Errorf("error does not name the key: %v", err)
	}
	// The message has to list the accepted values, or the user is guessing.
	for _, name := range PaperNames() {
		if !strings.Contains(err.Error(), name) {
			t.Errorf("error does not offer %q: %v", name, err)
		}
	}
}

func TestLoadRejectsBadMargin(t *testing.T) {
	tests := []struct {
		name   string
		margin string
	}{
		{"no unit", "20"},
		// The running footer is drawn inside the margin, so it needs room.
		{"zero", "0in"},
		{"not a number", "wide"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			write(t, "config.toml", "[pdf]\nmargin = \""+tt.margin+"\"\n")

			_, err := Load()
			if err == nil {
				t.Fatalf("Load() accepted margin = %q", tt.margin)
			}
			if !strings.Contains(err.Error(), "pdf.margin") {
				t.Errorf("error does not name the key: %v", err)
			}
		})
	}
}

func TestPDFGeometry(t *testing.T) {
	tests := []struct {
		name                     string
		pdf                      PDF
		wantW, wantH, wantMargin float64
	}{
		{"letter default", PDF{Paper: "letter", Margin: "0.75in"}, 8.5, 11, 0.75},
		{"a4", PDF{Paper: "a4", Margin: "20mm"}, 210 / 25.4, 297 / 25.4, 20 / 25.4},
		{"legal", PDF{Paper: "legal", Margin: "1in"}, 8.5, 14, 1},
		// An empty margin falls back rather than printing edge to edge.
		{"empty margin", PDF{Paper: "a5", Margin: ""}, 148 / 25.4, 210 / 25.4, 0.75},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w, h, margin, err := tt.pdf.Geometry()
			if err != nil {
				t.Fatalf("Geometry() error = %v", err)
			}
			for _, c := range []struct {
				what      string
				got, want float64
			}{
				{"width", w, tt.wantW},
				{"height", h, tt.wantH},
				{"margin", margin, tt.wantMargin},
			} {
				if math.Abs(c.got-c.want) > 0.001 {
					t.Errorf("%s = %.4fin, want %.4fin", c.what, c.got, c.want)
				}
			}
		})
	}
}

// The footer pads itself by the page margin; if these two disagree the footer
// does not line up with the content above it.
func TestPDFMarginCSS(t *testing.T) {
	tests := []struct {
		margin string
		want   string
	}{
		{"0.75in", "19.05mm"},
		{"20mm", "20mm"},
		{"", "19.05mm"}, // the default, normalised like any other value
	}
	for _, tt := range tests {
		t.Run("margin="+tt.margin, func(t *testing.T) {
			if got := (PDF{Margin: tt.margin}).MarginCSS(); got != tt.want {
				t.Errorf("MarginCSS() = %q, want %q", got, tt.want)
			}
		})
	}
}

// Load runs for every command; 'sections list' has no business failing because
// Chrome is not installed.
func TestLoadDoesNotCheckBrowserExists(t *testing.T) {
	write(t, "config.toml", "[pdf]\nbrowser = \"/nowhere/chrome\"\n")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v, want nil for a browser that is not installed", err)
	}
	if cfg.PDF.Browser != "/nowhere/chrome" {
		t.Errorf("browser = %q", cfg.PDF.Browser)
	}
}
