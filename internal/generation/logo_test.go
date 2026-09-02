package generation

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/isaacvarg/sdsforge/internal/config"
)

// writePNG builds a PNG of the given pixel size and returns its path.
func writePNG(t *testing.T, w, h int) string {
	t.Helper()

	img := image.NewRGBA(image.Rect(0, 0, w, h))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(t.TempDir(), "logo.png")
	if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// writeFile puts content in a temp file with the given name.
func writeFile(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestPrepareLogoUnconfigured(t *testing.T) {
	logo, err := PrepareLogo(config.Logo{}, "Acme")
	if err != nil {
		t.Fatalf("PrepareLogo() error = %v", err)
	}
	if logo != nil {
		t.Errorf("PrepareLogo() = %+v, want nil when no logo is configured", logo)
	}
}

// The artwork is measured and fitted into the box with its ratio preserved,
// so a user never has to work out print dimensions themselves.
func TestPrepareLogoFits(t *testing.T) {
	tests := []struct {
		name                  string
		pxW, pxH              int
		maxWidth, maxHeight   string
		wantWidth, wantHeight string
	}{
		// 1600x400 px is 423.33 x 105.83 mm naturally: far wider than the
		// 50mm bound, so width binds and the 4:1 ratio gives 12.5mm of height.
		{"wide is width-bound", 1600, 400, "", "", "50mm", "12.5mm"},
		// 400x1600 is the same shape turned over: height binds at 16mm.
		{"tall is height-bound", 400, 1600, "", "", "4mm", "16mm"},
		{"square is height-bound", 800, 800, "", "", "16mm", "16mm"},
		// Halving the height bound halves both dimensions.
		{"max_height override", 1600, 400, "", "8mm", "32mm", "8mm"},
		{"max_width override", 1600, 400, "25mm", "", "25mm", "6.25mm"},
		{"units convert", 1600, 400, "", "0.5in", "50mm", "12.5mm"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logo, err := PrepareLogo(config.Logo{
				Path:      writePNG(t, tt.pxW, tt.pxH),
				MaxWidth:  tt.maxWidth,
				MaxHeight: tt.maxHeight,
			}, "Acme")
			if err != nil {
				t.Fatalf("PrepareLogo() error = %v", err)
			}

			if !logo.Measured {
				t.Fatal("a PNG must be measurable")
			}
			if logo.PixelWidth != tt.pxW || logo.PixelHeight != tt.pxH {
				t.Errorf("measured %dx%d, want %dx%d",
					logo.PixelWidth, logo.PixelHeight, tt.pxW, tt.pxH)
			}

			want := "width:" + tt.wantWidth + ";height:" + tt.wantHeight
			if got := string(logo.Style); got != want {
				t.Errorf("Style = %q, want %q", got, want)
			}
		})
	}
}

// An upscaled raster mark prints blurred, so a small logo is left at its own
// size; a user who wants it bigger says so with max_height.
func TestPrepareLogoIsNeverUpscaled(t *testing.T) {
	// 100x50 px is 26.46 x 13.23 mm, inside the default 50x16 box.
	logo, err := PrepareLogo(config.Logo{Path: writePNG(t, 100, 50)}, "Acme")
	if err != nil {
		t.Fatalf("PrepareLogo() error = %v", err)
	}
	if got := string(logo.Style); got != "width:26.46mm;height:13.23mm" {
		t.Errorf("Style = %q, want the artwork's own size", got)
	}
}

func TestPrepareLogoSVG(t *testing.T) {
	tests := []struct {
		name string
		svg  string
		want string
	}{
		{
			"width and height",
			`<svg xmlns="http://www.w3.org/2000/svg" width="1600" height="400"></svg>`,
			"width:50mm;height:12.5mm",
		},
		{
			"px suffix",
			`<svg xmlns="http://www.w3.org/2000/svg" width="1600px" height="400px"></svg>`,
			"width:50mm;height:12.5mm",
		},
		{
			// A percentage says nothing about the ratio, so the viewBox wins.
			"viewBox fallback",
			`<svg xmlns="http://www.w3.org/2000/svg" width="100%" viewBox="0 0 1600 400"></svg>`,
			"width:50mm;height:12.5mm",
		},
		{
			"viewBox only",
			`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 800 800"></svg>`,
			"width:16mm;height:16mm",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logo, err := PrepareLogo(config.Logo{Path: writeFile(t, "logo.svg", tt.svg)}, "Acme")
			if err != nil {
				t.Fatalf("PrepareLogo() error = %v", err)
			}
			if got := string(logo.Style); got != tt.want {
				t.Errorf("Style = %q, want %q", got, tt.want)
			}
		})
	}
}

// A working logo beats a refused build: nothing about a company mark is
// regulated content, so unmeasurable artwork is bounded rather than rejected.
func TestPrepareLogoUnmeasurable(t *testing.T) {
	for _, tt := range []struct{ name, file, content string }{
		{"svg without dimensions", "logo.svg", `<svg xmlns="http://www.w3.org/2000/svg"></svg>`},
		{"webp has no stdlib decoder", "logo.webp", "RIFF????WEBPVP8 "},
	} {
		t.Run(tt.name, func(t *testing.T) {
			logo, err := PrepareLogo(config.Logo{Path: writeFile(t, tt.file, tt.content)}, "Acme")
			if err != nil {
				t.Fatalf("PrepareLogo() error = %v, want a bounded logo instead", err)
			}
			if logo.Measured {
				t.Fatal("Measured = true for artwork with no readable dimensions")
			}
			if got := string(logo.Style); got != "max-width:50mm;max-height:16mm;height:auto" {
				t.Errorf("Style = %q, want the bounds", got)
			}
		})
	}
}

// A sheet whose letterhead silently vanished is worse than one that refused to
// build -- the same call the library makes for missing pictogram artwork.
func TestPrepareLogoMissingFile(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "nope.png")

	_, err := PrepareLogo(config.Logo{Path: missing}, "Acme")
	if err == nil {
		t.Fatal("PrepareLogo() error = nil for a missing file")
	}
	if !strings.Contains(err.Error(), missing) {
		t.Errorf("error does not name the file: %v", err)
	}
}

func TestPrepareLogoUnsupportedType(t *testing.T) {
	_, err := PrepareLogo(config.Logo{Path: writeFile(t, "logo.tiff", "II*")}, "Acme")
	if err == nil || !strings.Contains(err.Error(), ".tiff") {
		t.Errorf("error = %v, want one naming the extension", err)
	}
}

// A mark identifying who issued the sheet is information, not decoration, so
// it always carries alt text.
func TestPrepareLogoAlt(t *testing.T) {
	path := writePNG(t, 100, 50)

	tests := []struct{ name, alt, company, want string }{
		{"from config", "Acme mark", "Acme Chemical Co.", "Acme mark"},
		{"from company name", "", "Acme Chemical Co.", "Acme Chemical Co. logo"},
		{"last resort", "", "", "Company logo"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logo, err := PrepareLogo(config.Logo{Path: path, Alt: tt.alt}, tt.company)
			if err != nil {
				t.Fatal(err)
			}
			if logo.Alt != tt.want {
				t.Errorf("Alt = %q, want %q", logo.Alt, tt.want)
			}
		})
	}
}

func TestPrepareLogoRejectsBadBounds(t *testing.T) {
	_, err := PrepareLogo(config.Logo{Path: writePNG(t, 100, 50), MaxHeight: "16"}, "Acme")
	if err == nil || !strings.Contains(err.Error(), "logo.max_height") {
		t.Errorf("error = %v, want one naming logo.max_height", err)
	}
}

func TestLogoOversized(t *testing.T) {
	if (*Logo)(nil).Oversized() {
		t.Error("a nil logo is not oversized")
	}
	if (&Logo{Bytes: 1024}).Oversized() {
		t.Error("a small logo is not oversized")
	}
	if !(&Logo{Bytes: 4 << 20}).Oversized() {
		t.Error("a 4 MB logo is worth a warning")
	}
}
