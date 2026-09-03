package sections

import (
	"sort"
	"strings"
)

// Source names identify where a subsection's content comes from when it is
// derived from the document's own data rather than authored in the library.
//
// A variant file cannot know a specific product's composition, supplier, or
// revision history, so those subsections declare a source in section.yaml:
//
//   - { id: ingredients, title: Hazardous ingredients, kind: table, source: materials }
//
// This package owns the VOCABULARY -- a closed set it can validate on its own --
// while the document package owns POPULATING it. That split is what keeps
// sections free of any dependency on document, which imports sections.
const (
	SourceMaterials      = "materials"
	SourceIdentification = "identification"
	SourceRecommendedUse = "recommended_use"
	SourceSupplier       = "supplier"
	SourceEmergencyPhone = "emergency_phone"
	SourceRevisions      = "revisions"
	SourceProp65         = "prop65"

	// Section 2 is computed from the document's hazard codes rather than
	// selected from authored variants: an arbitrary code set matches no
	// hand-written profile. See internal/ghs.
	SourceClassification = "classification"
	SourceSignalWord     = "signal_word"
	SourcePictograms     = "pictograms"
	SourcePrecautionary  = "precautionary"
)

// knownSources is the set a manifest's `source:` field is validated against.
var knownSources = map[string]bool{
	SourceMaterials:      true,
	SourceIdentification: true,
	SourceRecommendedUse: true,
	SourceSupplier:       true,
	SourceEmergencyPhone: true,
	SourceRevisions:      true,
	SourceProp65:         true,
	SourceClassification: true,
	SourceSignalWord:     true,
	SourcePictograms:     true,
	SourcePrecautionary:  true,
}

// SourceNames returns every valid source name, sorted, for error messages.
// Mirrors RegisteredKinds in content.go.
func SourceNames() []string {
	names := make([]string, 0, len(knownSources))
	for name := range knownSources {
		names = append(names, name)
	}
	sort.Strings(names) // map order is random; sort for stable messages
	return names
}

// SourceData carries ready-made content blocks keyed by source name.
//
// The resolver never learns what a document is: the caller builds these blocks
// and hands them over. A source whose data is empty must be OMITTED from the
// map rather than added as an empty block, so the authored placeholder in the
// library survives.
type SourceData map[string]Content

// Block returns the block for a source, and whether one was supplied with any
// content in it.
func (s SourceData) Block(source string) (Content, bool) {
	if source == "" || s == nil {
		return nil, false
	}
	body, ok := s[source]
	if !ok || body == nil || body.IsEmpty() {
		return nil, false
	}
	return body, true
}

// suggestSources formats the valid source names for an error message.
func suggestSources() string {
	return strings.Join(SourceNames(), ", ")
}
