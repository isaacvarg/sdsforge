package sections

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

// builtinFS holds the shipped content library, compiled into the binary.
//
// The `all:` prefix tells the embedder to include files that start with "." or
// "_", which it would otherwise skip. Everything under osha/ is embedded at
// BUILD time, so a released binary needs no content files on disk.
//
//go:embed all:osha
var builtinFS embed.FS

// DefaultJurisdiction is the only jurisdiction shipped today. The directory
// level exists so adding CLP or WHMIS later is a new folder, not a rewrite of
// every path in the codebase.
const DefaultJurisdiction = "osha"

// Library is a stack of content sources consulted in order.
//
// It implements fs.FS itself, which means it can be passed directly to
// LoadSection, LoadVariant, and anything else in the stdlib that reads from a
// filesystem -- no adapter, no special-casing.
type Library struct {
	// layers are searched in order; the first hit wins. Index 0 is the user's
	// overlay when enabled, so a custom file shadows the built-in one.
	layers []fs.FS

	// names labels each layer for error messages ("custom", "built-in").
	names []string

	jurisdiction string
}

// LibraryOptions configures which sources a Library draws on.
type LibraryOptions struct {
	// Jurisdiction selects the built-in tree. Empty means DefaultJurisdiction.
	Jurisdiction string

	// CustomVariants gates the user overlay entirely. When false, CustomDir is
	// ignored and nothing outside the binary is read or scanned -- which is
	// the point: the cost of a custom library is not paid unless it is on.
	CustomVariants bool

	// CustomDir is the root of the user's own content, containing a
	// jurisdiction directory (e.g. <CustomDir>/osha/04_first_aid/...).
	CustomDir string
}

// NewLibrary builds a Library from the given options.
func NewLibrary(opts LibraryOptions) (*Library, error) {
	jurisdiction := opts.Jurisdiction
	if jurisdiction == "" {
		jurisdiction = DefaultJurisdiction
	}

	// fs.Sub returns a view rooted at a subdirectory, so callers address
	// "04_first_aid/section.yaml" rather than "osha/04_first_aid/section.yaml".
	builtin, err := fs.Sub(builtinFS, jurisdiction)
	if err != nil {
		return nil, fmt.Errorf("no built-in library for jurisdiction %q: %w", jurisdiction, err)
	}

	lib := &Library{jurisdiction: jurisdiction}

	if opts.CustomVariants && opts.CustomDir != "" {
		root := filepath.Join(opts.CustomDir, jurisdiction)
		info, err := os.Stat(root)
		if err != nil {
			return nil, fmt.Errorf("custom variant library %s: %w", root, err)
		}
		if !info.IsDir() {
			return nil, fmt.Errorf("custom variant library %s is not a directory", root)
		}
		lib.layers = append(lib.layers, os.DirFS(root))
		lib.names = append(lib.names, "custom")
	}

	lib.layers = append(lib.layers, builtin)
	lib.names = append(lib.names, "built-in")

	return lib, nil
}

// Open implements fs.FS. The first layer holding the file wins.
func (l *Library) Open(name string) (fs.File, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrInvalid}
	}
	for _, layer := range l.layers {
		f, err := layer.Open(name)
		if err == nil {
			return f, nil
		}
		// Only a genuine "not here" falls through to the next layer. A
		// permission error or corrupt archive must surface, not be masked by
		// the built-in copy.
		if !isNotExist(err) {
			return nil, err
		}
	}
	return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
}

// isNotExist reports whether err means the file simply is not in this layer.
func isNotExist(err error) bool {
	return err != nil && (os.IsNotExist(err) || strings.Contains(err.Error(), "file does not exist"))
}

// Jurisdiction reports which jurisdiction this library was built for.
func (l *Library) Jurisdiction() string { return l.jurisdiction }

// Layers reports the layer names in search order, for diagnostics.
func (l *Library) Layers() []string { return append([]string(nil), l.names...) }

// Exists reports whether any layer holds the named file.
func (l *Library) Exists(name string) bool {
	f, err := l.Open(name)
	if err != nil {
		return false
	}
	_ = f.Close()
	return true
}

// ListVariants returns the variant names available for one subsection, across
// every layer, sorted and de-duplicated.
//
// A name present in both layers appears once: the custom layer shadows the
// built-in file rather than adding a second entry.
func (l *Library) ListVariants(sectionDir, subsectionID string) ([]string, error) {
	dir := path.Join(sectionDir, subsectionID)
	seen := map[string]struct{}{}
	found := false

	for _, layer := range l.layers {
		entries, err := fs.ReadDir(layer, dir)
		if err != nil {
			if isNotExist(err) {
				continue // this layer simply does not carry that subsection
			}
			return nil, fmt.Errorf("listing %s: %w", dir, err)
		}
		found = true
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
				continue
			}
			seen[strings.TrimSuffix(e.Name(), ".yaml")] = struct{}{}
		}
	}

	if !found {
		return nil, fmt.Errorf("no variant directory %s: %w", dir, fs.ErrNotExist)
	}

	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	sort.Strings(names) // map order is random; sort for stable output
	return names, nil
}

// ListPresets returns the preset names available for a section.
func (l *Library) ListPresets(sectionDir string) ([]string, error) {
	dir := path.Join(sectionDir, "presets")
	seen := map[string]struct{}{}

	for _, layer := range l.layers {
		entries, err := fs.ReadDir(layer, dir)
		if err != nil {
			if isNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("listing %s: %w", dir, err)
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
				continue
			}
			seen[strings.TrimSuffix(e.Name(), ".yaml")] = struct{}{}
		}
	}

	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	sort.Strings(names)
	return names, nil
}
