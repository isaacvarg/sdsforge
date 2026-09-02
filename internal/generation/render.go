// Package generation renders resolved sections into an output document.
package generation

import (
	"bytes"
	"embed"
	"fmt"
	"html/template"
	"io"
	"strings"
	"time"

	"github.com/isaacvarg/sdsforge/internal/config"
	"github.com/isaacvarg/sdsforge/internal/document"
	"github.com/isaacvarg/sdsforge/internal/sections"
)

//go:embed templates/*.tmpl
var templateFS embed.FS

// View is the data handed to the templates.
//
// Templates get a purpose-built view rather than the raw document, so the
// markup never reaches into internals and every value it needs is already
// computed.
type View struct {
	Doc      document.Data
	Sections []sections.ResolvedSection
	// Company is the issuer, named in the header and footer. It comes from the
	// user's config rather than the document, so it is passed in separately.
	Company config.Company
	// Logo is the issuer's mark, nil when none is configured.
	Logo        *Logo
	GeneratedAt string

	// ForPDF reports that this render is on its way to Chrome, which draws a
	// running footer on every page. The layout's own end-of-document footer
	// stands down when that is true rather than saying the same thing twice.
	ForPDF bool

	// MarginCSS is the page margin, for the running footer's padding. Chrome
	// renders header and footer templates outside the page box, so a footer
	// that does not pad itself by the margin sits flush to the paper edge.
	MarginCSS string
}

// NewView assembles what the templates render from.
//
// It takes the whole config rather than the pieces because two separate tables
// now reach the page -- [company] and [pdf] -- and a growing parameter list is
// how call sites start passing them in the wrong order.
func NewView(doc document.Data, secs []sections.ResolvedSection, cfg config.Config, logo *Logo) View {
	return View{
		Doc:         doc,
		Sections:    secs,
		Company:     cfg.Company,
		Logo:        logo,
		GeneratedAt: time.Now().Format("2006-01-02"),
		MarginCSS:   cfg.PDF.MarginCSS(),
	}
}

// funcs are the helpers available inside the templates.
var funcs = template.FuncMap{
	// dict builds a map inline, which is how a template passes more than one
	// value to a sub-template.
	"dict": func(pairs ...any) (map[string]any, error) {
		if len(pairs)%2 != 0 {
			return nil, fmt.Errorf("dict requires an even number of arguments, got %d", len(pairs))
		}
		m := make(map[string]any, len(pairs)/2)
		for i := 0; i < len(pairs); i += 2 {
			key, ok := pairs[i].(string)
			if !ok {
				return nil, fmt.Errorf("dict key %d is %T, want string", i, pairs[i])
			}
			m[key] = pairs[i+1]
		}
		return m, nil
	},
	"hasRows": func(t *sections.Table) bool { return t != nil && len(t.Rows) > 0 },

	// imageURI marks an image source as safe for html/template.
	//
	// html/template rejects data: URIs in src by default, replacing them with
	// a placeholder -- sensible, since a data: URI can carry script. Rather
	// than blanket-trusting the value, only base64 data: URIs of an allowed
	// image type pass. Anything else yields an empty src, so a malformed or
	// hostile value renders as a broken image rather than executing.
	"imageURI": func(src string) template.URL {
		for _, prefix := range []string{
			"data:image/svg+xml;base64,",
			"data:image/png;base64,",
			"data:image/jpeg;base64,",
			"data:image/webp;base64,",
		} {
			if strings.HasPrefix(src, prefix) {
				return template.URL(src)
			}
		}
		return ""
	},
}

// parsed is built once at package load. Parsing templates is not cheap and the
// set never changes, so doing it per render would be pure waste.
//
// template.Must panics on a bad template. That is correct here: the templates
// are compiled into the binary, so a parse failure is a build defect that must
// surface immediately rather than at the moment a user runs a report.
var parsed = template.Must(
	template.New("layout.html.tmpl").Funcs(funcs).ParseFS(templateFS, "templates/*.tmpl"),
)

// RenderHTML writes the finished safety data sheet to w.
func RenderHTML(w io.Writer, view View) error {
	// Render into a buffer first so a mid-template failure cannot leave a
	// half-written file on disk.
	var buf bytes.Buffer
	if err := parsed.ExecuteTemplate(&buf, "layout.html.tmpl", view); err != nil {
		return fmt.Errorf("rendering document: %w", err)
	}

	if _, err := buf.WriteTo(w); err != nil {
		return fmt.Errorf("writing document: %w", err)
	}
	return nil
}

// RenderFooter renders the running footer Chrome draws on every printed page.
//
// It goes through html/template like the page does, so a product name carrying
// an angle bracket is escaped rather than breaking the markup Chrome is handed.
func RenderFooter(view View) (string, error) {
	var buf bytes.Buffer
	if err := parsed.ExecuteTemplate(&buf, "footer.html.tmpl", view); err != nil {
		return "", fmt.Errorf("rendering page footer: %w", err)
	}
	return buf.String(), nil
}
