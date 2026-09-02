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
		ProductName:     "Acetone Technical Grade",
		DocumentVersion: "1.2",
		LastRevision:    "2026-08-14",
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
	if err := RenderHTML(&buf, doc, secs, config.Company{}); err != nil {
		t.Fatalf("RenderHTML() error = %v", err)
	}
	out := buf.String()

	wants := []string{
		"<!DOCTYPE html>",
		"Acetone Technical Grade",
		"Version 1.2",
		"1. Identification",
		"4. First-aid measures",
		"16. Other information",
		"Do NOT induce vomiting",  // the corrosive variant reached the page
		"<table>",                 // section 8's exposure limits rendered as a table
		"<th>OSHA PEL (TWA)</th>", // table headers came through
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

// html/template escapes by default. A product name containing markup must not
// become live HTML in the finished document.
func TestRenderEscapesContent(t *testing.T) {
	doc, secs := fixture(t)
	doc.ProductName = `<script>alert("xss")</script>`

	var buf bytes.Buffer
	if err := RenderHTML(&buf, doc, secs, config.Company{}); err != nil {
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
	if err := RenderHTML(&buf, document.Data{ProductName: "Test"}, []sections.ResolvedSection{sec}, config.Company{}); err != nil {
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
	if err := RenderHTML(&buf, doc, secs, config.Company{Name: "Acme Chemical Co."}); err != nil {
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
	if err := RenderHTML(&buf, doc, secs, config.Company{}); err != nil {
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
