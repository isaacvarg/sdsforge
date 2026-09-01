package sections

import (
	"errors"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestImagesKind(t *testing.T) {
	if got := (&Images{}).Kind(); got != "images" {
		t.Errorf("Kind() = %q, want %q", got, "images")
	}
}

// An image nobody can identify is not information on a safety document, so a
// missing alt makes an entry count as empty.
func TestImagesIsEmpty(t *testing.T) {
	tests := []struct {
		name   string
		images []Image
		want   bool
	}{
		{"zero value", nil, true},
		{"empty slice", []Image{}, true},
		{"src but no alt", []Image{{Src: "data:image/png;base64,AAA"}}, true},
		{"alt but no src", []Image{{Alt: "GHS05"}}, true},
		{"complete", []Image{{Src: "data:image/png;base64,AAA", Alt: "GHS05"}}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := (&Images{Images: tt.images}).IsEmpty(); got != tt.want {
				t.Errorf("IsEmpty() = %v, want %v", got, tt.want)
			}
		})
	}
}

// Two hazards can carry the same pictogram; it must appear once.
func TestImagesAppendDeduplicates(t *testing.T) {
	base := &Images{Images: []Image{{Src: "a.svg", Alt: "A"}}}
	result, err := base.Append(&Images{Images: []Image{
		{Src: "a.svg", Alt: "A"},
		{Src: "b.svg", Alt: "B"},
	}})
	if err != nil {
		t.Fatalf("Append() error = %v", err)
	}
	got := result.(*Images).Images
	if len(got) != 2 || got[0].Src != "a.svg" || got[1].Src != "b.svg" {
		t.Errorf("Append() = %+v, want a.svg then b.svg once each", got)
	}
	if len(base.Images) != 1 {
		t.Errorf("Append mutated its receiver: %+v", base.Images)
	}
}

func TestImagesAppendKindMismatch(t *testing.T) {
	_, err := (&Images{}).Append(&Prose{Text: []string{"x"}})
	if !errors.Is(err, ErrKindMismatch) {
		t.Fatalf("error = %v, want one wrapping ErrKindMismatch", err)
	}
}

func TestBlockUnmarshalImages(t *testing.T) {
	src := `
kind: images
images:
  - src: ghs/pictograms/GHS05.svg
    alt: "GHS05 pictogram: corrosion"
    caption: "GHS05 (corrosion)"
`
	var blk Block
	if err := yaml.Unmarshal([]byte(src), &blk); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	imgs, ok := blk.Body.(*Images)
	if !ok {
		t.Fatalf("Body is %T, want *Images", blk.Body)
	}
	if len(imgs.Images) != 1 || imgs.Images[0].Caption != "GHS05 (corrosion)" {
		t.Errorf("decoded %+v", imgs.Images)
	}
}

// A path becomes a data: URI so the finished sheet stands alone.
func TestEmbedImagesFromLibrary(t *testing.T) {
	lib := realLibrary(t)

	body, err := embedImages(lib, &Images{Images: []Image{
		{Src: "ghs/pictograms/GHS05.svg", Alt: "GHS05 pictogram: corrosion"},
	}})
	if err != nil {
		t.Fatalf("embedImages() error = %v", err)
	}

	got := body.(*Images).Images[0].Src
	if !strings.HasPrefix(got, "data:image/svg+xml;base64,") {
		t.Fatalf("src = %.60q, want an svg data URI", got)
	}
	if len(got) < 1000 {
		t.Errorf("data URI is only %d bytes; the artwork looks truncated", len(got))
	}
}

// An already-embedded URI passes through untouched.
func TestEmbedImagesLeavesDataURIs(t *testing.T) {
	in := &Images{Images: []Image{{Src: "data:image/png;base64,AAAA", Alt: "x"}}}
	out, err := embedImages(realLibrary(t), in)
	if err != nil {
		t.Fatalf("embedImages() error = %v", err)
	}
	if out != Content(in) {
		t.Error("embedImages copied a block that needed no change")
	}
}

// Artwork that cannot be read must fail loudly: a pictogram silently missing
// from a safety data sheet is the failure worth being loudest about.
func TestEmbedImagesFailsLoudly(t *testing.T) {
	lib := realLibrary(t)

	tests := []struct {
		name, src, want string
	}{
		{"missing file", "ghs/pictograms/GHS99.svg", "GHS99"},
		{"unsupported type", "ghs/pictograms/GHS05.tiff", "unsupported extension"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := embedImages(lib, &Images{Images: []Image{{Src: tt.src, Alt: "x"}}})
			if err == nil {
				t.Fatalf("embedImages(%q) = nil error, want an error", tt.src)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error %q should mention %q", err, tt.want)
			}
		})
	}
}

// Every pictogram named by section 2's variants must actually exist.
func TestSectionTwoPictogramsResolve(t *testing.T) {
	lib := realLibrary(t)
	def := loadDef(t, lib, "02_hazards")

	variants, err := lib.ListVariants(def.Dir, "pictograms")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range variants {
		t.Run(name, func(t *testing.T) {
			sec, err := Resolve(lib, def,
				SectionSelection{Subsections: map[string]SubsectionOverride{
					"pictograms": {Variant: name},
				}}, ResolveContext{})
			if err != nil {
				t.Fatalf("Resolve() error = %v", err)
			}
			sub := find(t, sec, "pictograms")
			if name == DefaultVariant {
				if !sub.Empty {
					t.Error("the default variant should carry no pictogram")
				}
				return
			}
			for _, img := range sub.Body.(*Images).Images {
				if !strings.HasPrefix(img.Src, "data:image/svg+xml;base64,") {
					t.Errorf("%s: src was not embedded: %.40q", name, img.Src)
				}
				if img.Alt == "" {
					t.Errorf("%s: image has no alt text", name)
				}
			}
		})
	}
}
