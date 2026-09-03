package cmd

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/isaacvarg/sdsforge/internal/config"
	"github.com/isaacvarg/sdsforge/internal/document"
	"github.com/isaacvarg/sdsforge/internal/ghs"
	"github.com/isaacvarg/sdsforge/internal/sections"
	"github.com/spf13/cobra"
)

// classifyCmd prints what the hazard codes produced, before any of it reaches a
// rendered sheet.
//
// Automatic classification that cannot be inspected is a liability on a safety
// document: someone has to be able to check the tool's work.
var classifyCmd = &cobra.Command{
	Use:   "classify <document-id>",
	Short: "Show what a document's hazard codes produce",
	Long: `Print the GHS classification computed from a document's hazard codes: the
hazard statements, signal word, pictograms and precautionary statements that
will appear in section 2, plus which variant every other section derived and
why.

Nothing is written; this only reports.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id, err := documentID(args[0])
		if err != nil {
			return err
		}
		return runClassify(cmd, id)
	},
}

// runClassify reports what one document's hazard codes produce.
//
// Factored out of classifyCmd so 'document edit --classify' shows exactly what
// 'document classify' shows, rather than growing a second implementation of the
// same pipeline that could drift from it.
func runClassify(cmd *cobra.Command, id int) error {
	doc, versions, err := loadForRender(id)
	if err != nil {
		return err
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	lib, err := openLibraryWith(cfg)
	if err != nil {
		return err
	}
	tables, err := ghs.LoadTables(lib)
	if err != nil {
		return err
	}

	codes := doc.AllHazardCodes()
	classification, err := tables.Classify(codes)
	if err != nil {
		return fmt.Errorf("document %d: %w", id, err)
	}
	if err := classification.ApplyText(doc.PrecautionaryText); err != nil {
		return fmt.Errorf("document %d: %w", id, err)
	}

	out := cmd.OutOrStdout()
	printClassification(out, doc, classification)

	resolved, err := sections.ResolveAll(lib, doc.Sections, sections.ResolveContext{
		Sources:     doc.SourceData(classification, cfg, versions),
		HazardCodes: doc.HazardCodeSet(),
	})
	if err != nil {
		return err
	}
	printDerivation(out, resolved)

	return nil
}

func printClassification(out io.Writer, doc document.Data, c *ghs.Classification) {
	fmt.Fprintf(out, "%s\n", doc.ProductName)

	if len(c.Codes) == 0 {
		fmt.Fprintln(out, "\nNo hazard codes declared. Section 2 will show the")
		fmt.Fprintln(out, "\"not classified\" defaults and no wording will be derived.")
		fmt.Fprintln(out, "\nAdd them to document.yaml, e.g.  hazard_codes: [H315, H350]")
		return
	}

	fmt.Fprintf(out, "Codes: %s\n", strings.Join(c.Codes, ", "))

	fmt.Fprintln(out, "\nCLASSIFICATION")
	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	for _, h := range c.Hazards {
		fmt.Fprintf(w, "  %s\t%s\t%s\t%s\n", h.Code, h.Class, "Cat. "+h.Category, h.Statement+".")
	}
	w.Flush()

	signal := c.SignalWord
	if signal == "" {
		signal = "(none)"
	}
	fmt.Fprintf(out, "\nSignal word: %s\n", signal)

	pictograms := make([]string, 0, len(c.Pictograms))
	for _, p := range c.Pictograms {
		pictograms = append(pictograms, fmt.Sprintf("%s (%s)", p.Code, p.Name))
	}
	if len(pictograms) == 0 {
		pictograms = []string{"(none)"}
	}
	fmt.Fprintf(out, "Pictograms:  %s\n", strings.Join(pictograms, ", "))

	fmt.Fprintf(out, "\nPRECAUTIONARY (%d)\n", len(c.Precautions))
	w = tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	for _, p := range c.Precautions {
		marker := ""
		if p.SupplierSpecified {
			marker = " *"
		}
		fmt.Fprintf(w, "  %s\t%s\t%s%s\n", p.Type, p.Code, p.Statement, marker)
	}
	w.Flush()

	if len(c.Warnings) > 0 {
		fmt.Fprintln(out, "\nREVIEW")
		for _, warning := range c.Warnings {
			fmt.Fprintf(out, "  * %s\n", wrap(warning, 72, "    "))
		}
	}
}

func printDerivation(out io.Writer, resolved []sections.ResolvedSection) {
	type row struct{ section, subsection, variant, why string }
	var rows []row

	for _, sec := range resolved {
		for _, sub := range sec.Subsections {
			// Source is checked first because it wins over the variant. A
			// subsection can have a derived variant that the computed content
			// then supersedes; reporting the variant there would credit
			// wording that never reached the sheet.
			switch {
			case sub.Source != "":
				why := "computed from " + sub.Source
				switch {
				case sub.SupersededDerived != "":
					why += " (manual variant " + sub.Variant + " overrode derived " + sub.SupersededDerived + ")"
				case sub.DerivedFrom != "":
					why += " (superseding variant " + sub.Variant + ")"
				}
				rows = append(rows, row{sec.ID, sub.ID, "-", why})
			case sub.DerivedFrom != "":
				rows = append(rows, row{sec.ID, sub.ID, sub.Variant, "derived from " + sub.DerivedFrom})
			case sub.SupersededDerived != "":
				rows = append(rows, row{sec.ID, sub.ID, sub.Variant,
					"selected manually, OVERRIDING derived " + sub.SupersededDerived})
			case sub.Variant != sections.DefaultVariant:
				rows = append(rows, row{sec.ID, sub.ID, sub.Variant, "selected manually"})
			}
		}
	}

	if len(rows) == 0 {
		fmt.Fprintln(out, "\nNothing derived; every section resolves to its default.")
		return
	}

	sort.SliceStable(rows, func(i, j int) bool { return rows[i].section < rows[j].section })

	fmt.Fprintln(out, "\nSECTION CONTENT")
	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	for _, r := range rows {
		fmt.Fprintf(w, "  %s/%s\t%s\t%s\n", r.section, r.subsection, r.variant, r.why)
	}
	w.Flush()
}

// wrap re-flows a long warning so it stays readable in a terminal.
func wrap(s string, width int, indent string) string {
	var (
		b    strings.Builder
		line int
	)
	for i, word := range strings.Fields(s) {
		if i > 0 {
			if line+1+len(word) > width {
				b.WriteString("\n" + indent)
				line = 0
			} else {
				b.WriteString(" ")
				line++
			}
		}
		b.WriteString(word)
		line += len(word)
	}
	return b.String()
}

func init() {
	documentCmd.AddCommand(classifyCmd)
}
