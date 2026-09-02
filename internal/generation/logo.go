package generation

import (
	"bytes"
	"encoding/base64"
	"encoding/xml"
	"fmt"
	"html/template"
	"image"
	_ "image/jpeg" // registers the JPEG header decoder for image.DecodeConfig
	_ "image/png"  // registers the PNG header decoder
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/isaacvarg/sdsforge/internal/config"
)

// The default box a logo is fitted inside.
//
// The header band is about 7in wide, so 50mm leaves the product name the room
// it needs; 16mm is roughly two lines of the heading beside it, which keeps a
// tall logo from towering over the title it sits next to.
const (
	defaultMaxHeightMM = 16
	defaultMaxWidthMM  = 50

	// mmPerPixel is the CSS reference of 96 pixels to the inch, which is how a
	// renderer decides what an image's "natural size" is on the page.
	mmPerPixel = 25.4 / 96
)

// warnLogoBytes is the encoded size past which a logo is worth a word. Safety
// data sheets get emailed and archived, and a multi-megabyte letterhead rides
// along on every one.
const warnLogoBytes = 1 << 20

// logoMediaTypes maps a file extension to the MIME type used in a data: URI.
// It mirrors the library's own artwork table in internal/sections.
var logoMediaTypes = map[string]string{
	".svg":  "image/svg+xml",
	".png":  "image/png",
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".webp": "image/webp",
}

// Logo is a print-ready company mark: embedded artwork plus the exact space it
// occupies on the page.
type Logo struct {
	// Src is a data: URI. Embedded rather than linked for the same reason the
	// GHS pictograms are: a sheet gets emailed, printed and archived, so it
	// has to stand alone.
	Src string

	// Alt is what a screen reader announces and what survives when images are
	// off. Never empty.
	Alt string

	// Style is the computed size, e.g. "width:50mm;height:12.5mm".
	//
	// template.CSS because html/template sanitises style attributes and would
	// otherwise blank a value it cannot verify. It is safe to mark as trusted
	// precisely because it is built here from measured numbers -- no
	// user-supplied string reaches a CSS context.
	Style template.CSS

	// Bytes is the size of the encoded artwork, for the oversize warning.
	Bytes int

	// Measured reports whether the artwork's own dimensions could be read. When
	// false the style constrains the box only and the renderer keeps the
	// aspect ratio itself.
	Measured bool

	// PixelWidth and PixelHeight are the artwork's intrinsic size, zero when
	// Measured is false. Reported by 'config show'.
	PixelWidth, PixelHeight int

	// WidthMM and HeightMM are the fitted print size, zero when Measured is
	// false.
	WidthMM, HeightMM float64
}

// PrepareLogo reads, measures and fits the configured logo.
//
// It returns nil when no logo is configured, so an unconfigured sheet renders
// exactly as it did before the setting existed.
//
// A path that cannot be read IS an error, matching how the library treats
// missing pictogram artwork: a sheet whose letterhead silently vanished is
// worse than one that refused to build. Artwork that cannot be MEASURED is
// not -- see fit.
func PrepareLogo(cfg config.Logo, companyName string) (*Logo, error) {
	if cfg.IsZero() {
		return nil, nil
	}

	ext := strings.ToLower(filepath.Ext(cfg.Path))
	mediaType, ok := logoMediaTypes[ext]
	if !ok {
		return nil, fmt.Errorf("logo %s: unsupported image type %q; use %s",
			cfg.Path, ext, "svg, png, jpg or webp")
	}

	data, err := os.ReadFile(cfg.Path)
	if err != nil {
		return nil, fmt.Errorf("reading logo %s: %w", cfg.Path, err)
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("logo %s is empty", cfg.Path)
	}

	maxHeight, maxWidth, err := logoBox(cfg)
	if err != nil {
		return nil, err
	}

	src := "data:" + mediaType + ";base64," + base64.StdEncoding.EncodeToString(data)
	logo := &Logo{
		Src: src,
		Alt: logoAlt(cfg, companyName),
		// The encoded length, not the file's: that is what actually rides
		// along in every generated sheet.
		Bytes: len(src),
	}

	w, h, measured := measure(data, mediaType)
	logo.PixelWidth, logo.PixelHeight, logo.Measured = w, h, measured
	logo.WidthMM, logo.HeightMM = fit(w, h, measured, maxWidth, maxHeight)
	logo.Style = logoStyle(logo, maxWidth, maxHeight)

	return logo, nil
}

// Oversized reports whether the encoded logo is large enough to be worth
// mentioning to the user.
func (l *Logo) Oversized() bool { return l != nil && l.Bytes > warnLogoBytes }

// logoBox resolves the fitting bounds, falling back to the defaults.
func logoBox(cfg config.Logo) (height, width float64, err error) {
	height, width = defaultMaxHeightMM, defaultMaxWidthMM

	if cfg.MaxHeight != "" {
		if height, err = config.ParseLength(cfg.MaxHeight); err != nil {
			return 0, 0, fmt.Errorf("logo.max_height: %w", err)
		}
	}
	if cfg.MaxWidth != "" {
		if width, err = config.ParseLength(cfg.MaxWidth); err != nil {
			return 0, 0, fmt.Errorf("logo.max_width: %w", err)
		}
	}
	return height, width, nil
}

// logoAlt is never empty: a mark identifying who issued the sheet is
// information, not decoration, so it must survive a screen reader or images
// being off.
func logoAlt(cfg config.Logo, companyName string) string {
	if alt := strings.TrimSpace(cfg.Alt); alt != "" {
		return alt
	}
	if companyName != "" {
		return companyName + " logo"
	}
	return "Company logo"
}

// fit scales the artwork into the box, preserving its aspect ratio.
//
// The scale is capped at 1: a logo smaller than the box is left alone rather
// than blown up, because an upscaled raster mark prints blurred and a user who
// wanted it bigger can say so with max_height.
func fit(pxW, pxH int, measured bool, maxWidthMM, maxHeightMM float64) (widthMM, heightMM float64) {
	if !measured || pxW <= 0 || pxH <= 0 {
		return 0, 0
	}

	// The artwork's own pixels are read at the CSS reference of 96 per inch,
	// which is what "natural size" means to a browser laying the page out.
	naturalW := float64(pxW) * mmPerPixel
	naturalH := float64(pxH) * mmPerPixel

	scale := min(maxWidthMM/naturalW, maxHeightMM/naturalH, 1)
	return naturalW * scale, naturalH * scale
}

// logoStyle renders the CSS for the fitted size.
//
// Unmeasurable artwork gets the bounds instead of an exact size, so the
// renderer preserves the aspect ratio itself. Less deterministic on paper, but
// a working logo beats a refused build: nothing about a company mark is
// regulated content.
func logoStyle(l *Logo, maxWidthMM, maxHeightMM float64) template.CSS {
	if !l.Measured {
		return template.CSS(fmt.Sprintf("max-width:%s;max-height:%s;height:auto",
			config.FormatLength(maxWidthMM), config.FormatLength(maxHeightMM)))
	}
	return template.CSS(fmt.Sprintf("width:%s;height:%s",
		config.FormatLength(l.WidthMM), config.FormatLength(l.HeightMM)))
}

// measure reads the artwork's intrinsic dimensions without decoding it.
func measure(data []byte, mediaType string) (w, h int, ok bool) {
	if mediaType == "image/svg+xml" {
		return measureSVG(data)
	}

	// DecodeConfig reads the header only, so a large photograph costs nothing
	// here. WebP has no stdlib decoder; it falls through as unmeasurable.
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || cfg.Width <= 0 || cfg.Height <= 0 {
		return 0, 0, false
	}
	return cfg.Width, cfg.Height, true
}

// measureSVG reads the root element's dimensions.
//
// Only the RATIO matters -- a vector scales losslessly into whatever box wins
// -- so width/height are preferred and viewBox is the fallback, which is the
// same order a browser resolves them in.
func measureSVG(data []byte) (w, h int, ok bool) {
	dec := xml.NewDecoder(bytes.NewReader(data))
	// SVG in the wild carries entity references and mixed namespaces that a
	// strict parse rejects; the root element's attributes are all that is
	// wanted here.
	dec.Strict = false
	dec.AutoClose = xml.HTMLAutoClose

	for {
		token, err := dec.Token()
		if err == io.EOF {
			return 0, 0, false
		}
		if err != nil {
			return 0, 0, false
		}

		start, isStart := token.(xml.StartElement)
		if !isStart {
			continue
		}
		if !strings.EqualFold(start.Name.Local, "svg") {
			return 0, 0, false // the root element is not an <svg>
		}

		var width, height, viewBox string
		for _, attr := range start.Attr {
			switch strings.ToLower(attr.Name.Local) {
			case "width":
				width = attr.Value
			case "height":
				height = attr.Value
			case "viewbox":
				viewBox = attr.Value
			}
		}

		if w, okW := svgLength(width); okW {
			if h, okH := svgLength(height); okH {
				return w, h, true
			}
		}
		return svgViewBox(viewBox)
	}
}

// svgLength reads an SVG width or height attribute. Only unitless values and
// px are usable as pixels; a percentage or a physical unit says nothing about
// the ratio, so it is rejected in favour of the viewBox.
func svgLength(s string) (int, bool) {
	trimmed := strings.TrimSuffix(strings.TrimSpace(strings.ToLower(s)), "px")
	value, err := strconv.ParseFloat(trimmed, 64)
	if err != nil || value <= 0 {
		return 0, false
	}
	return int(value), true
}

// svgViewBox reads the width and height from "min-x min-y width height".
func svgViewBox(s string) (w, h int, ok bool) {
	fields := strings.FieldsFunc(strings.TrimSpace(s), func(r rune) bool {
		return r == ' ' || r == ',' || r == '\t' || r == '\n'
	})
	if len(fields) != 4 {
		return 0, 0, false
	}

	width, errW := strconv.ParseFloat(fields[2], 64)
	height, errH := strconv.ParseFloat(fields[3], 64)
	if errW != nil || errH != nil || width <= 0 || height <= 0 {
		return 0, 0, false
	}
	return int(width), int(height), true
}
