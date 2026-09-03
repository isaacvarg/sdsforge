package generation

import (
	"bytes"
	"strings"
	"testing"

	"github.com/isaacvarg/sdsforge/internal/config"
	"github.com/isaacvarg/sdsforge/internal/document"
	"github.com/isaacvarg/sdsforge/internal/sections"
)

func fixture(t *testing.T) (document.Data, []sections.ResolvedSection) {
	t.Helper()

	lib, err := sections.NewLibrary(sections.LibraryOptions{})
	if err != nil {
		t.Fatalf("NewLibrary() error = %v", err)
	}

	doc := document.Data{
		ProductName: "Acetone Technical Grade",
		Materials: []document.Material{
			{Name: "Acetone", CASNumber: "67-64-1", Percentage: ">99", HazardCodes: []string{"H225"}},
		},
		Sections: map[string]sections.SectionSelection{
			"first_aid": {Variant: "corrosive"},
		},
	}

	secs, err := sections.ResolveAll(lib, doc.Sections, sections.ResolveContext{})
	if err != nil {
		t.Fatalf("ResolveAll() error = %v", err)
	}
	return doc, secs
}

func TestRenderHTML(t *testing.T) {
	doc, secs := fixture(t)

	var buf bytes.Buffer
	if err := RenderHTML(&buf, NewView(doc, secs, config.Config{}, nil, fixtureVersions())); err != nil {
		t.Fatalf("RenderHTML() error = %v", err)
	}
	out := buf.String()

	wants := []string{
		"<!DOCTYPE html>",
		"Acetone Technical Grade",
		"Version 1.2.0",
		"1. Identification",
		"4. First-aid measures",
		"16. Other information",
		"Do NOT induce vomiting", // the corrosive variant reached the page
		"<table>",                // section 8's exposure limits rendered as a table
		"<th>Basis</th>",         // table headers came through
	}
	for _, want := range wants {
		if !strings.Contains(out, want) {
			t.Errorf("rendered output missing %q", want)
		}
	}

	// All 16 sections present.
	if got := strings.Count(out, `<section class="sds">`); got != 16 {
		t.Errorf("rendered %d sections, want 16", got)
	}
}

// A document with Prop 65 warnings must reach the page as an image (the
// warning symbol) with the computed legal text as its caption -- the same
// path GHS pictograms take through Section 2.
func TestRenderProp65Warning(t *testing.T) {
	lib, err := sections.NewLibrary(sections.LibraryOptions{})
	if err != nil {
		t.Fatalf("NewLibrary() error = %v", err)
	}

	doc := document.Data{
		ProductName: "Acetone Technical Grade",
		Prop65: []document.Prop65Warning{
			{Chemical: "Carbon black", Exposure: "carcinogen"},
		},
	}

	secs, err := sections.ResolveAll(lib, doc.Sections, sections.ResolveContext{
		Sources: doc.SourceData(nil, config.Config{}, fixtureVersions()),
	})
	if err != nil {
		t.Fatalf("ResolveAll() error = %v", err)
	}

	var buf bytes.Buffer
	if err := RenderHTML(&buf, NewView(doc, secs, config.Config{}, nil, fixtureVersions())); err != nil {
		t.Fatalf("RenderHTML() error = %v", err)
	}
	out := buf.String()

	for _, want := range []string{
		// html/template HTML-encodes "+" in a template.URL value, so the
		// literal mime type "svg+xml" comes through as "svg&#43;xml" -- a
		// browser decodes the entity back before resolving the URL.
		`<img src="data:image/svg&#43;xml;base64,`,
		"<figcaption>This product can expose you to Carbon black, which is known to the State of California to cause cancer.",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("rendered output missing %q", want)
		}
	}
}

// html/template escapes by default. A product name containing markup must not
// become live HTML in the finished document.
func TestRenderEscapesContent(t *testing.T) {
	doc, secs := fixture(t)
	doc.ProductName = `<script>alert("xss")</script>`

	var buf bytes.Buffer
	if err := RenderHTML(&buf, NewView(doc, secs, config.Config{}, nil, fixtureVersions())); err != nil {
		t.Fatalf("RenderHTML() error = %v", err)
	}
	out := buf.String()

	if strings.Contains(out, "<script>alert") {
		t.Error("product name was not escaped")
	}
	if !strings.Contains(out, "&lt;script&gt;") {
		t.Error("expected the escaped form in the output")
	}
}

// An empty subsection must say so rather than render blank.
func TestRenderEmptySubsection(t *testing.T) {
	lib, err := sections.NewLibrary(sections.LibraryOptions{})
	if err != nil {
		t.Fatal(err)
	}
	def, err := sections.LoadSection(lib, "04_first_aid")
	if err != nil {
		t.Fatal(err)
	}

	var blank sections.Block
	if err := blankBlock(&blank); err != nil {
		t.Fatal(err)
	}

	sec, err := sections.Resolve(lib, def, sections.SectionSelection{
		Subsections: map[string]sections.SubsectionOverride{
			"symptoms": {Replace: &blank},
		},
	}, sections.ResolveContext{})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}

	var buf bytes.Buffer
	if err := RenderHTML(&buf, NewView(document.Data{ProductName: "Test"}, []sections.ResolvedSection{sec}, config.Config{}, nil, fixtureVersions())); err != nil {
		t.Fatalf("RenderHTML() error = %v", err)
	}
	if !strings.Contains(buf.String(), `<p class="empty">No data available.</p>`) {
		t.Error("empty subsection did not render its empty text")
	}
}

// The issuing company comes from configuration, and must reach both the
// header and the footer of the finished sheet.
func TestRenderCompanyHeaderAndFooter(t *testing.T) {
	doc, secs := fixture(t)

	var buf bytes.Buffer
	if err := RenderHTML(&buf, NewView(doc, secs, config.Config{Company: config.Company{Name: "Acme Chemical Co."}}, nil, fixtureVersions())); err != nil {
		t.Fatalf("RenderHTML() error = %v", err)
	}
	out := buf.String()

	if !strings.Contains(out, "<span>Acme Chemical Co.</span>") {
		t.Error("company name missing from the header")
	}
	if !strings.Contains(out, "Prepared by Acme Chemical Co.") {
		t.Error("company name missing from the footer")
	}
}

// With no company configured, the sheet reads exactly as it did before the
// setting existed -- no stray "Prepared by" and no empty header cell.
func TestRenderWithoutCompany(t *testing.T) {
	doc, secs := fixture(t)

	var buf bytes.Buffer
	if err := RenderHTML(&buf, NewView(doc, secs, config.Config{}, nil, fixtureVersions())); err != nil {
		t.Fatalf("RenderHTML() error = %v", err)
	}
	out := buf.String()

	if strings.Contains(out, "Prepared by") {
		t.Error("footer credits a company that was never configured")
	}
	if strings.Contains(out, "<span></span>") {
		t.Error("header has an empty company cell")
	}
}

// The logo goes in the header beside the title, embedded so the sheet stands
// alone when emailed or printed.
func TestRenderLogo(t *testing.T) {
	doc, secs := fixture(t)

	logo, err := PrepareLogo(config.Logo{Path: writePNG(t, 1600, 400)}, "Acme Chemical Co.")
	if err != nil {
		t.Fatalf("PrepareLogo() error = %v", err)
	}

	var buf bytes.Buffer
	if err := RenderHTML(&buf, NewView(doc, secs, config.Config{Company: config.Company{Name: "Acme Chemical Co."}}, logo, fixtureVersions())); err != nil {
		t.Fatalf("RenderHTML() error = %v", err)
	}
	out := buf.String()

	for _, want := range []string{
		`class="logo"`,
		`src="data:image/png;base64,`,
		`alt="Acme Chemical Co. logo"`,
		`style="width:50mm;height:12.5mm"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("rendered header missing %q", want)
		}
	}

	// html/template blanks a style attribute it cannot verify. Catching that
	// here is the point of building the value from measured numbers.
	if strings.Contains(out, "ZgotmplZ") {
		t.Error("html/template rejected the computed style")
	}
}

// With no logo the header is exactly what it was before the setting existed.
func TestRenderWithoutLogo(t *testing.T) {
	doc, secs := fixture(t)

	var buf bytes.Buffer
	if err := RenderHTML(&buf, NewView(doc, secs, config.Config{}, nil, fixtureVersions())); err != nil {
		t.Fatalf("RenderHTML() error = %v", err)
	}
	if out := buf.String(); strings.Contains(out, `class="logo"`) {
		t.Error("header carries a logo element with no logo configured")
	}
}
