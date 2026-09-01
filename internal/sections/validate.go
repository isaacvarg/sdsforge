package sections

import (
	"errors"
	"fmt"
	"path"
	"sort"
)

// ValidateLibrary checks an entire content library for the mistakes that would
// otherwise surface as a silently wrong safety data sheet.
//
// It reports every problem it finds rather than stopping at the first: someone
// authoring content wants the full list, not one error per run.
//
// Checks performed:
//
//   - every section in layout.yaml has a loadable, valid section.yaml
//   - every subsection has a default.yaml (the fallback must always exist)
//   - every variant parses and its content kind matches the manifest
//   - every preset picks real subsections and real variants
//   - no two variants of one subsection share a derivation priority
func ValidateLibrary(lib *Library) error {
	layout, err := LoadLayout(lib)
	if err != nil {
		return err
	}

	var problems []error
	seenIDs := map[string]string{}

	for _, dir := range layout.Sections {
		def, err := LoadSection(lib, dir)
		if err != nil {
			problems = append(problems, err)
			continue
		}

		// Section ids key the document YAML, so a collision would make one
		// section unaddressable.
		if prev, dup := seenIDs[def.ID]; dup {
			problems = append(problems, fmt.Errorf(
				"%s: section id %q already used by %s", dir, def.ID, prev))
		}
		seenIDs[def.ID] = dir

		problems = append(problems, validateSectionContent(lib, def)...)
	}

	if len(problems) == 0 {
		return nil
	}
	return errors.Join(problems...)
}

func validateSectionContent(lib *Library, def SectionDef) []error {
	var problems []error

	for _, sub := range def.Subsections {
		where := path.Join(def.Dir, sub.ID)

		// LoadSection already rejects an unknown source, but validating here
		// too means `sections validate` reports it alongside everything else
		// rather than aborting the whole section on a manifest error.
		if sub.Source != "" && !knownSources[sub.Source] {
			problems = append(problems, fmt.Errorf(
				"%s: unknown source %q; known sources: %s", where, sub.Source, suggestSources()))
		}

		variants, err := lib.ListVariants(def.Dir, sub.ID)
		if err != nil {
			problems = append(problems, fmt.Errorf("%s: %w", where, err))
			continue
		}

		// The default is the fallback the resolver reaches for when nothing
		// selects otherwise. Without it, an unselected subsection is a hard
		// error at render time.
		hasDefault := false
		for _, v := range variants {
			if v == DefaultVariant {
				hasDefault = true
				break
			}
		}
		if !hasDefault {
			problems = append(problems, fmt.Errorf(
				"%s: no %s.yaml; every subsection needs a fallback", where, DefaultVariant))
		}

		// priorities maps a priority value to the variants claiming it, so a
		// collision can name both culprits.
		priorities := map[int][]string{}

		for _, name := range variants {
			vf, err := LoadVariant(lib, def.Dir, sub.ID, name)
			if err != nil {
				problems = append(problems, err)
				continue
			}
			if got := vf.Content.Body.Kind(); got != sub.Kind {
				problems = append(problems, fmt.Errorf(
					"%s/%s.yaml: content is %s but the manifest declares %s",
					where, name, got, sub.Kind))
			}
			// Only predicated variants participate in derivation, so only they
			// can collide. The default deliberately carries priority 0 and no
			// predicate.
			if !vf.AppliesWhen.IsZero() {
				priorities[vf.Priority] = append(priorities[vf.Priority], name)
			}
		}

		for _, p := range sortedIntKeys(priorities) {
			if names := priorities[p]; len(names) > 1 {
				sort.Strings(names)
				problems = append(problems, fmt.Errorf(
					"%s: variants %v all claim priority %d; derivation requires exactly one winner",
					where, names, p))
			}
		}
	}

	problems = append(problems, validatePresets(lib, def)...)
	return problems
}

func validatePresets(lib *Library, def SectionDef) []error {
	var problems []error

	presets, err := lib.ListPresets(def.Dir)
	if err != nil {
		return []error{fmt.Errorf("%s: listing presets: %w", def.Dir, err)}
	}

	for _, name := range presets {
		ps, err := LoadPreset(lib, def.Dir, name)
		if err != nil {
			problems = append(problems, err)
			continue
		}
		for _, subID := range sortedKeys(ps.Picks) {
			variant := ps.Picks[subID]
			if _, ok := def.Subsection(subID); !ok {
				problems = append(problems, fmt.Errorf(
					"%s/presets/%s.yaml: picks unknown subsection %q",
					def.Dir, name, subID))
				continue
			}
			if !lib.Exists(path.Join(def.Dir, subID, variant+".yaml")) {
				available, _ := lib.ListVariants(def.Dir, subID)
				problems = append(problems, fmt.Errorf(
					"%s/presets/%s.yaml: picks variant %q for %q, which does not exist; %s",
					def.Dir, name, variant, subID, suggest(available)))
			}
		}
	}

	return problems
}

func sortedIntKeys[V any](m map[int]V) []int {
	keys := make([]int, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Ints(keys)
	return keys
}
