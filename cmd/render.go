package cmd

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strconv"

	"github.com/isaacvarg/sdsforge/internal/config"
	"github.com/isaacvarg/sdsforge/internal/document"
	"github.com/isaacvarg/sdsforge/internal/generation"
	"github.com/isaacvarg/sdsforge/internal/ghs"
	"github.com/isaacvarg/sdsforge/internal/sections"
)

// sheet is one rendered safety data sheet, held in memory.
//
// Nothing is written to disk here. On the PDF path the markup is an
// intermediate that goes to the browser and nowhere else, and on either path a
// half-written sheet is worse than none -- so the caller decides where the bytes
// land, and gets them only once they are all there.
type sheet struct {
	// Slug is the document name reduced for use as a filename, without an
	// extension.
	Slug string
	HTML []byte
	// PDF is nil when the caller asked for HTML only.
	PDF []byte
}

// buildSheet resolves a document against the content library and renders it.
//
// versions supplies the header's version number and section 16's revision
// history, so a caller recording a new version passes an index that already
// contains the pending entry: the archived sheet then shows its own row rather
// than the history as it stood one version ago.
//
// warn receives notices that are worth saying but are not failures -- a legacy
// supplier block being ignored, an oversized logo.
func buildSheet(
	ctx context.Context,
	id int,
	doc document.Data,
	versions document.VersionIndex,
	warn io.Writer,
	wantPDF bool,
) (sheet, error) {
	cfg, err := config.Load()
	if err != nil {
		return sheet{}, err
	}

	lib, err := openLibraryWith(cfg)
	if err != nil {
		return sheet{}, err
	}

	// Supplier details used to live in the document. They are read from the
	// config now, so say the old block is being skipped rather than leave the
	// user wondering why what they typed is not on the sheet.
	if doc.HasLegacySupplier() {
		path, err := config.Path()
		if err != nil {
			return sheet{}, err
		}
		fmt.Fprintf(warn,
			"document %d: the 'supplier:' block is no longer read; company and emergency details come from %s\n",
			id, path)
	}

	// Section 2 is computed from the document's hazard codes, and those same
	// codes derive the wording of every other section.
	tables, err := ghs.LoadTables(lib)
	if err != nil {
		return sheet{}, err
	}
	classification, err := tables.Classify(doc.AllHazardCodes())
	if err != nil {
		return sheet{}, fmt.Errorf("document %d: %w", id, err)
	}
	if err := classification.ApplyText(doc.PrecautionaryText); err != nil {
		return sheet{}, fmt.Errorf("document %d: %w", id, err)
	}

	resolved, err := sections.ResolveAll(lib, doc.Sections, sections.ResolveContext{
		Sources:     doc.SourceData(classification, cfg, versions),
		HazardCodes: doc.HazardCodeSet(),
	})
	if err != nil {
		return sheet{}, fmt.Errorf("resolving document %d:\n%w", id, err)
	}

	// Prepared before anything is rendered, so a bad logo path fails without
	// leaving a half-built sheet behind.
	logo, err := generation.PrepareLogo(cfg.Logo, cfg.Company.Name)
	if err != nil {
		return sheet{}, err
	}
	if logo.Oversized() {
		fmt.Fprintf(warn,
			"warning: the logo adds %s to every sheet; a smaller file would travel better\n",
			formatBytes(logo.Bytes))
	}

	view := generation.NewView(doc, resolved, cfg, logo, versions)
	view.ForPDF = wantPDF

	var html bytes.Buffer
	if err := generation.RenderHTML(&html, view); err != nil {
		return sheet{}, err
	}

	out := sheet{
		Slug: document.Slugify(doc.ProductName),
		HTML: html.Bytes(),
	}
	if !wantPDF {
		return out, nil
	}

	// Located before anything is printed, for the same reason the logo is
	// prepared before the sheet is rendered.
	browser, err := generation.FindBrowser(cfg.PDF.Browser)
	if err != nil {
		return out, err
	}

	width, height, margin, err := cfg.PDF.Geometry()
	if err != nil {
		return out, err
	}
	footer, err := generation.RenderFooter(view)
	if err != nil {
		return out, err
	}

	pdf, err := generation.RenderPDF(ctx, browser, html.Bytes(), generation.PDFOptions{
		PaperWidth:  width,
		PaperHeight: height,
		Margin:      margin,
		Footer:      footer,
	})
	if err != nil {
		return out, err
	}

	out.PDF = pdf
	return out, nil
}

// loadForRender reads the document and its version history together, which is
// what every rendering command needs before it can call buildSheet.
func loadForRender(id int) (document.Data, document.VersionIndex, error) {
	doc, err := document.Load(id)
	if err != nil {
		return document.Data{}, document.VersionIndex{}, err
	}
	versions, err := document.LoadVersions(id)
	if err != nil {
		return document.Data{}, document.VersionIndex{}, err
	}
	return doc, versions, nil
}

// documentID parses the id argument shared by every per-document command.
func documentID(arg string) (int, error) {
	id, err := strconv.Atoi(arg)
	if err != nil {
		return 0, fmt.Errorf("document id must be a number, got %q", arg)
	}
	return id, nil
}

// formatBytes renders a byte count for a warning message.
func formatBytes(n int) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for size := int64(n) / unit; size >= unit; size /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGT"[exp])
}
