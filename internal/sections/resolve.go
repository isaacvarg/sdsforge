package sections

import (
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strings"
)

// SectionSelection is a document's choice for one section.
//
//	sections:
//	  first_aid:
//	    variant: corrosive
//	    subsections:
//	      inhalation: { variant: acute_toxicity }
//	      ingestion:  { append: ["Do not induce vomiting."] }
type SectionSelection struct {
	// Variant names a PRESET: a bundle of per-subsection picks. Empty means
	// every subsection falls back to its default.
	Variant string `yaml:"variant"`

	// Subsections overrides individual subsections, winning over the preset.
	Subsections map[string]SubsectionOverride `yaml:"subsections"`
}

// SubsectionOverride is a document's adjustment to one subsection.
//
// The pointer fields matter: nil means "the author said nothing", whereas a
// non-nil pointer to an empty block means "the author deliberately blanked
// this". A plain Block value could not tell those apart.
type SubsectionOverride struct {
	// Variant picks a different variant than the preset chose.
	Variant string `yaml:"variant"`

	// Replace discards the variant's content entirely.
	Replace *Block `yaml:"replace"`

	// Append adds to the variant's content.
	Append *Block `yaml:"append"`
}

// ResolvedSubsection is one subsection with its content finally decided.
type ResolvedSubsection struct {
	ID      string
	Title   string
	Kind    string
	Variant string
	Body    Content

	// Source names the document data that supplied this body, or is empty
	// when the content came from the library alone.
	Source string

	// DerivedFrom lists the hazard codes that selected this variant, when it
	// was chosen automatically rather than named. Empty for a manual pick.
	DerivedFrom string

	// SupersededDerived names the variant derivation would have chosen, when a
	// manual selection displaced it. On a safety document, "a human overruled
	// the automatic classification here" is precisely what a reviewer needs to
	// be able to see.
	SupersededDerived string

	// Empty means the resolved body has no content and EmptyText should be
	// rendered in its place.
	Empty     bool
	EmptyText string
}

// ResolvedSection is one section ready to render.
type ResolvedSection struct {
	ID          string
	Number      int
	Title       string
	Subsections []ResolvedSubsection
}

// ResolveContext carries everything the resolver needs from the document
// beyond the section selections themselves.
//
// A zero value reproduces the fully-manual behaviour: no derivation, no
// document data, every subsection falling back to its authored default.
type ResolveContext struct {
	// Sources holds ready-made blocks for subsections that declare a source.
	Sources SourceData

	// HazardCodes drives derivation. When empty, applies_when is never
	// evaluated and variant selection stays entirely manual.
	HazardCodes map[string]bool
}

// Resolve produces the final content for one section.
//
// The order is fixed and deliberate:
//
//	default -> derived -> preset pick -> per-subsection variant -> source
//	        -> replace -> append
//
// Derivation from hazard codes sits just above the default, so every manual
// choice still beats anything computed. The source step sits after the
// authored variant so library content is the fallback when a document carries
// no data, and before replace/append so an explicit override still wins.
func Resolve(fsys fs.FS, def SectionDef, sel SectionSelection, ctx ResolveContext) (ResolvedSection, error) {
	var problems []error

	// STEP 1: the preset, if one was named.
	picks := map[string]string{}
	if sel.Variant != "" {
		ps, err := LoadPreset(fsys, def.Dir, sel.Variant)
		if err != nil {
			// Collected, not returned. Returning here would hide every other
			// problem in the section behind one typo, and the promise of this
			// resolver is that an author sees the whole list in one run.
			problems = append(problems, enrich(fsys, err, func(l optionLister) string {
				return suggest(must(l.ListPresets(def.Dir)))
			}))
		}
		for subID, variant := range ps.Picks {
			// A preset naming a subsection that does not exist is a library
			// bug and would otherwise silently do nothing.
			if _, ok := def.Subsection(subID); !ok {
				problems = append(problems, fmt.Errorf(
					"preset %q picks unknown subsection %q", sel.Variant, subID))
				continue
			}
			picks[subID] = variant
		}
	}

	// Catch document typos: an override for a subsection that does not exist
	// would otherwise be silently ignored, and the author would never learn
	// why their edit had no effect.
	for _, subID := range sortedKeys(sel.Subsections) {
		if _, ok := def.Subsection(subID); !ok {
			problems = append(problems, fmt.Errorf(
				"override for unknown subsection %q in section %q", subID, def.ID))
		}
	}

	out := ResolvedSection{
		ID:     def.ID,
		Number: def.Number,
		Title:  def.Title,
	}

	// STEP 2-4, per subsection, in manifest order.
	for _, sub := range def.Subsections {
		resolved, err := resolveSubsection(fsys, def, sub, picks, sel.Subsections[sub.ID], ctx)
		if err != nil {
			problems = append(problems, err)
			continue
		}
		out.Subsections = append(out.Subsections, resolved)
	}

	if len(problems) > 0 {
		return ResolvedSection{}, fmt.Errorf("resolving section %q:\n%w", def.ID, errors.Join(problems...))
	}
	return out, nil
}

func resolveSubsection(
	fsys fs.FS,
	def SectionDef,
	sub SubsectionDef,
	picks map[string]string,
	override SubsectionOverride,
	ctx ResolveContext,
) (ResolvedSubsection, error) {
	// Precedence, lowest to highest.
	variant := DefaultVariant
	derivedFrom, derivedVariant := "", ""
	if len(ctx.HazardCodes) > 0 {
		derived, from, err := derive(fsys, def, sub, ctx.HazardCodes)
		if err != nil {
			return ResolvedSubsection{}, err
		}
		if derived != "" {
			variant, derivedFrom, derivedVariant = derived, from, derived
		}
	}
	if pick, ok := picks[sub.ID]; ok && pick != "" {
		variant = pick
		derivedFrom = ""
	}
	if override.Variant != "" {
		variant = override.Variant
		derivedFrom = ""
	}

	// Record only a genuine disagreement: a manual pick that lands on the same
	// variant derivation would have chosen is not an override worth flagging.
	superseded := ""
	if derivedVariant != "" && derivedVariant != variant {
		superseded = derivedVariant
	}

	vf, err := LoadVariant(fsys, def.Dir, sub.ID, variant)
	if err != nil {
		return ResolvedSubsection{}, fmt.Errorf("subsection %q: %w", sub.ID,
			enrich(fsys, err, func(l optionLister) string {
				return suggest(must(l.ListVariants(def.Dir, sub.ID)))
			}))
	}

	body := vf.Content.Body
	if body.Kind() != sub.Kind {
		return ResolvedSubsection{}, fmt.Errorf(
			"subsection %q: variant %q declares %s content but the manifest declares %s",
			sub.ID, variant, body.Kind(), sub.Kind)
	}

	// STEP 3: document data, when this subsection declares a source and the
	// document actually carries some. An absent or empty source leaves the
	// authored placeholder in place -- a sheet with no material list should
	// still render the library's row rather than a blank table.
	source := ""
	if body2, ok := ctx.Sources.Block(sub.Source); ok {
		if got := body2.Kind(); got != sub.Kind {
			return ResolvedSubsection{}, fmt.Errorf(
				"subsection %q: source %q supplied %s content but the subsection is %s",
				sub.ID, sub.Source, got, sub.Kind)
		}
		body = body2
		source = sub.Source
	}

	// STEP 4a: replace wins outright over both the variant and the source.
	if override.Replace != nil {
		if got := override.Replace.Body.Kind(); got != sub.Kind {
			return ResolvedSubsection{}, fmt.Errorf(
				"subsection %q: replace block is %s content but the subsection is %s",
				sub.ID, got, sub.Kind)
		}
		body = override.Replace.Body
	}

	// STEP 4b: append adds to whatever survived.
	if override.Append != nil {
		if got := override.Append.Body.Kind(); got != sub.Kind {
			return ResolvedSubsection{}, fmt.Errorf(
				"subsection %q: append block is %s content but the subsection is %s",
				sub.ID, got, sub.Kind)
		}
		merged, err := body.Append(override.Append.Body)
		if err != nil {
			return ResolvedSubsection{}, fmt.Errorf("subsection %q: %w", sub.ID, err)
		}
		body = merged
	}

	// Artwork referenced by path becomes a data: URI here, so the finished
	// document stands alone once written.
	body, err = embedImages(fsys, body)
	if err != nil {
		return ResolvedSubsection{}, fmt.Errorf("subsection %q: %w", sub.ID, err)
	}

	// STEP 5: nothing to say means say so explicitly. An SDS with a silently
	// blank section is worse than one that states the absence of data.
	emptyText := sub.EmptyText
	if emptyText == "" {
		emptyText = def.EmptyText
	}
	if emptyText == "" {
		emptyText = DefaultEmptyText
	}

	return ResolvedSubsection{
		ID:                sub.ID,
		Title:             sub.Title,
		Kind:              sub.Kind,
		Variant:           variant,
		DerivedFrom:       derivedFrom,
		SupersededDerived: superseded,
		Source:            source,
		Body:              body,
		Empty:             body.IsEmpty(),
		EmptyText:         emptyText,
	}, nil
}

// ResolveAll resolves every section in the library's layout order.
func ResolveAll(lib *Library, selections map[string]SectionSelection, ctx ResolveContext) ([]ResolvedSection, error) {
	layout, err := LoadLayout(lib)
	if err != nil {
		return nil, err
	}

	var (
		out      []ResolvedSection
		problems []error
	)

	// known records every section the library DEFINES, independent of whether
	// it resolved cleanly. Deriving it from successful results instead would
	// report a section that failed to resolve as "unknown" too -- a phantom
	// second error naming a section that plainly exists.
	known := map[string]bool{}

	for _, dir := range layout.Sections {
		def, err := LoadSection(lib, dir)
		if err != nil {
			problems = append(problems, err)
			continue
		}
		known[def.ID] = true

		// Selections are keyed by section id ("first_aid"), never by number or
		// directory, so renumbering a section never breaks a saved document.
		resolved, err := Resolve(lib, def, selections[def.ID], ctx)
		if err != nil {
			problems = append(problems, err)
			continue
		}
		out = append(out, resolved)
	}
	for _, id := range sortedKeys(selections) {
		if !known[id] {
			problems = append(problems, fmt.Errorf("document selects unknown section %q", id))
		}
	}

	if len(problems) > 0 {
		return nil, errors.Join(problems...)
	}
	return out, nil
}

// sortedKeys returns a map's keys in a stable order.
//
// Go randomizes map iteration on purpose. Anything that turns a map into
// output -- error messages especially -- must sort, or the same broken
// document produces differently-ordered errors on every run.
func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// optionLister is an OPTIONAL interface: a filesystem that can also enumerate
// what is available. *Library implements it; a plain fs.FS (os.DirFS, an
// embed.FS, a testdata directory) does not.
//
// Type-asserting to a narrow interface like this is how Go asks "can you also
// do X?" without forcing every caller to supply it. The resolver keeps working
// on any fs.FS; it just produces better errors when handed something richer.
type optionLister interface {
	ListPresets(sectionDir string) ([]string, error)
	ListVariants(sectionDir, subsectionID string) ([]string, error)
}

// enrich appends an "available: ..." clause to err when fsys can enumerate the
// options. A "no such variant" error is far more useful when it also says what
// the real variants are.
func enrich(fsys fs.FS, err error, options func(optionLister) string) error {
	lister, ok := fsys.(optionLister)
	if !ok {
		return err
	}
	return fmt.Errorf("%w; %s", err, options(lister))
}

// must discards a listing error: this is only decorating a message that is
// already being reported, so a failure to enumerate should never replace the
// real error.
func must(names []string, _ error) []string { return names }

// derive picks a variant for a subsection from the document's hazard codes.
//
// Every variant of the subsection is evaluated; the highest priority among
// those whose applies_when matches wins. Returning "" means nothing matched,
// leaving the default in place.
//
// A tie is an error rather than an arbitrary pick. ValidateLibrary rejects ties
// statically for the shipped library, but a custom layer can introduce one, and
// choosing silently between two hazard profiles is not something this tool
// should ever do.
func derive(fsys fs.FS, def SectionDef, sub SubsectionDef, codes map[string]bool) (string, string, error) {
	lister, ok := fsys.(optionLister)
	if !ok {
		// A plain fs.FS cannot enumerate variants, so derivation is simply
		// unavailable; manual selection still works.
		return "", "", nil
	}

	names, err := lister.ListVariants(def.Dir, sub.ID)
	if err != nil {
		return "", "", fmt.Errorf("subsection %q: listing variants for derivation: %w", sub.ID, err)
	}

	var (
		bestName     string
		bestPriority int
		bestCodes    []string
		tied         []string
	)

	for _, name := range names {
		if name == DefaultVariant {
			continue // the fallback never participates
		}
		vf, err := LoadVariant(fsys, def.Dir, sub.ID, name)
		if err != nil {
			return "", "", err
		}
		if vf.AppliesWhen.IsZero() || !vf.AppliesWhen.Matches(codes) {
			continue
		}

		switch {
		case bestName == "" || vf.Priority > bestPriority:
			bestName, bestPriority = name, vf.Priority
			bestCodes = matchedCodes(vf.AppliesWhen, codes)
			tied = nil
		case vf.Priority == bestPriority:
			tied = append(tied, name)
		}
	}

	if len(tied) > 0 {
		all := append([]string{bestName}, tied...)
		sort.Strings(all)
		return "", "", fmt.Errorf(
			"subsection %q: hazard codes match variants %v at the same priority %d; "+
				"one must outrank the others, or name a variant explicitly",
			sub.ID, all, bestPriority)
	}

	if bestName == "" {
		return "", "", nil
	}
	return bestName, strings.Join(bestCodes, ", "), nil
}

// matchedCodes reports which of the document's codes a predicate matched on,
// for the derivation trace.
func matchedCodes(p *Predicate, codes map[string]bool) []string {
	var out []string
	for _, list := range [][]string{p.AnyOf, p.AllOf} {
		for _, c := range list {
			if codes[c] {
				out = append(out, c)
			}
		}
	}
	sort.Strings(out)
	return slicesCompact(out)
}

func slicesCompact(in []string) []string {
	out := in[:0]
	var prev string
	for i, s := range in {
		if i == 0 || s != prev {
			out = append(out, s)
		}
		prev = s
	}
	return out
}
