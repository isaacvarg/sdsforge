package generation

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/isaacvarg/sdsforge/internal/config"
	"github.com/isaacvarg/sdsforge/internal/document"
)

func TestRenderFooter(t *testing.T) {
	doc, secs := fixture(t)

	out, err := RenderFooter(NewView(doc, secs, config.Default(), nil))
	if err != nil {
		t.Fatalf("RenderFooter() error = %v", err)
	}

	wants := []string{
		`class="pageNumber"`, // Chrome's substitution classes have to survive
		`class="totalPages"`, //   html/template untouched
		"Acetone Technical Grade",
		"Version 1.2",
		"padding: 0 19.05mm", // the default 0.75in margin, so the footer lines up
	}
	for _, want := range wants {
		if !strings.Contains(out, want) {
			t.Errorf("footer missing %q, got:\n%s", want, out)
		}
	}
}

// The footer is handed to Chrome as raw markup, so a product name carrying an
// angle bracket must not be able to break out of the span it sits in.
func TestRenderFooterEscapesProductName(t *testing.T) {
	doc := document.Data{ProductName: `Acid <script>alert(1)</script> & Base`}

	out, err := RenderFooter(NewView(doc, nil, config.Default(), nil))
	if err != nil {
		t.Fatalf("RenderFooter() error = %v", err)
	}

	if strings.Contains(out, "<script>") {
		t.Errorf("footer did not escape the product name:\n%s", out)
	}
	if !strings.Contains(out, "&amp;") {
		t.Errorf("footer did not escape the ampersand:\n%s", out)
	}
	// html/template blanks a value it cannot verify, leaving this marker.
	if strings.Contains(out, "ZgotmplZ") {
		t.Errorf("html/template rejected a value in the footer:\n%s", out)
	}
}

// A missing version must not leave a dangling separator.
func TestRenderFooterWithoutVersion(t *testing.T) {
	out, err := RenderFooter(NewView(document.Data{ProductName: "NaCl"}, nil, config.Default(), nil))
	if err != nil {
		t.Fatalf("RenderFooter() error = %v", err)
	}
	if strings.Contains(out, "Version") {
		t.Errorf("footer names a version the document does not have:\n%s", out)
	}
	if strings.Contains(out, "&middot;") {
		t.Errorf("footer left a dangling separator:\n%s", out)
	}
}

// The end-of-document footer must not repeat what the running footer already
// says on every page, but the --html output has no running footer to defer to.
func TestLayoutFooterDefersToTheRunningFooter(t *testing.T) {
	doc, secs := fixture(t)
	cfg := config.Config{Company: config.Company{Name: "Acme Chemical Co."}}

	render := func(forPDF bool) string {
		t.Helper()
		view := NewView(doc, secs, cfg, nil)
		view.ForPDF = forPDF

		var buf bytes.Buffer
		if err := RenderHTML(&buf, view); err != nil {
			t.Fatalf("RenderHTML() error = %v", err)
		}
		return buf.String()
	}

	const standard = "29 CFR 1910.1200"

	forHTML := render(false)
	if !strings.Contains(forHTML, "Prepared by Acme Chemical Co.") {
		t.Error("the HTML output dropped the preparer from its footer")
	}
	if !strings.Contains(forHTML, standard) {
		t.Error("the HTML output dropped the standard from its footer")
	}

	forPDF := render(true)
	if strings.Contains(forPDF, "Prepared by Acme Chemical Co.") {
		t.Error("the PDF output repeats the preparer the running footer already carries")
	}
	if !strings.Contains(forPDF, standard) {
		t.Error("the PDF output dropped the standard, which no running footer carries")
	}
}

// The page framing only makes sense in a browser window; on paper @page owns
// the geometry and this would add half an inch above the header.
func TestLayoutKeepsScreenFramingOffThePage(t *testing.T) {
	doc, secs := fixture(t)

	var buf bytes.Buffer
	if err := RenderHTML(&buf, NewView(doc, secs, config.Default(), nil)); err != nil {
		t.Fatalf("RenderHTML() error = %v", err)
	}
	out := buf.String()

	screen := strings.Index(out, "@media screen")
	framing := strings.Index(out, "padding: 0.5in 0")
	if screen < 0 || framing < 0 {
		t.Fatalf("expected the screen framing behind @media screen, got:\n%s", out[:min(len(out), 2000)])
	}
	if framing < screen {
		t.Error("the screen framing is outside @media screen and will reach the printed page")
	}
}

// The real round-trip. Skipped rather than failed when no browser is
// installed: not every machine running the unit tests can print.
func TestRenderPDF(t *testing.T) {
	if testing.Short() {
		t.Skip("starting a browser is not a short test")
	}
	browser, err := FindBrowser("")
	if err != nil {
		t.Skipf("no browser to print with: %v", err)
	}

	doc, secs := fixture(t)
	view := NewView(doc, secs, config.Default(), nil)
	view.ForPDF = true

	var html bytes.Buffer
	if err := RenderHTML(&html, view); err != nil {
		t.Fatalf("RenderHTML() error = %v", err)
	}
	footer, err := RenderFooter(view)
	if err != nil {
		t.Fatalf("RenderFooter() error = %v", err)
	}

	width, height, margin, err := config.Default().PDF.Geometry()
	if err != nil {
		t.Fatalf("Geometry() error = %v", err)
	}

	pdf, err := RenderPDF(context.Background(), browser, html.Bytes(), PDFOptions{
		PaperWidth:  width,
		PaperHeight: height,
		Margin:      margin,
		Footer:      footer,
	})
	if err != nil {
		t.Fatalf("RenderPDF() error = %v", err)
	}

	if !bytes.HasPrefix(pdf, []byte("%PDF-")) {
		t.Errorf("output is not a PDF, starts with %q", pdf[:min(len(pdf), 16)])
	}
	// A 16-section sheet with an embedded stylesheet is tens of kilobytes; a
	// few hundred bytes would mean Chrome printed a blank page.
	if len(pdf) < 10_000 {
		t.Errorf("PDF is %d bytes, which is too small to be the sheet", len(pdf))
	}
}

// A cancelled run reports the cancellation, not whatever the browser was doing
// when it was killed.
func TestRenderPDFCancelled(t *testing.T) {
	if testing.Short() {
		t.Skip("starting a browser is not a short test")
	}
	browser, err := FindBrowser("")
	if err != nil {
		t.Skipf("no browser to print with: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err = RenderPDF(ctx, browser, []byte("<html><body>x</body></html>"), PDFOptions{
		PaperWidth: 8.5, PaperHeight: 11, Margin: 0.75,
	})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("RenderPDF() error = %v, want context.Canceled", err)
	}
}

// A section is not an atom. Making one unbreakable pushes any section that
// does not fit onto the next page whole, which is how a sheet ends up with
// pages that stop halfway down.
func TestLayoutLetsSectionsBreakAcrossPages(t *testing.T) {
	doc, secs := fixture(t)

	var buf bytes.Buffer
	if err := RenderHTML(&buf, NewView(doc, secs, config.Default(), nil)); err != nil {
		t.Fatalf("RenderHTML() error = %v", err)
	}
	out := buf.String()

	if rule := cssRule(t, out, "section.sds"); strings.Contains(rule, "break-inside: avoid") {
		t.Errorf("section.sds is unbreakable, which strands whole pages: %s", rule)
	}

	// What replaces it: the small things a reader would notice being torn.
	wants := map[string]string{
		"a section heading can be orphaned at the foot of a page": "section.sds > h2,\n    .subsection > h3 { break-after: avoid; }",
		"a table row can be split":                                "tr { break-inside: avoid; }",
		"a split table does not repeat its header":                "thead { display: table-header-group; }",
		"a paragraph can leave a single line behind":              "orphans: 2; widows: 2;",
		"a pictogram can be split from its caption":               ".pictograms figure {",
	}
	for problem, css := range wants {
		if !strings.Contains(out, css) {
			t.Errorf("%s: the stylesheet is missing %q", problem, css)
		}
	}
}

// cssRule returns the declaration block of the first rule whose selector list
// starts with selector.
func cssRule(t *testing.T, html, selector string) string {
	t.Helper()

	at := strings.Index(html, "\n    "+selector+" {")
	if at < 0 {
		t.Fatalf("no %q rule in the stylesheet", selector)
	}
	rest := html[at:]
	end := strings.Index(rest, "}")
	if end < 0 {
		t.Fatalf("unterminated %q rule", selector)
	}
	return rest[:end+1]
}
