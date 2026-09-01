package sections

import (
	"encoding/base64"
	"fmt"
	"io/fs"
	"path"
	"strings"
)

// Images is a row of pictures with alternative text -- GHS pictograms in
// section 2, and transport labels in section 14 should that follow.
//
// Src is normally a data: URI so a generated sheet stands alone: these
// documents get emailed and printed, and a sheet with broken image icons is
// worse than one that fell back to text.
type Images struct {
	Images []Image `yaml:"images"`
}

// Image is one picture.
type Image struct {
	Src string `yaml:"src"`

	// Alt is what a screen reader announces and what a text-only rendering
	// falls back to. For a hazard pictogram it is the difference between
	// "corrosion" and silence, so an Image without it is treated as invalid.
	Alt string `yaml:"alt"`

	// Caption is printed under the image.
	Caption string `yaml:"caption"`
}

var _ Content = (*Images)(nil)

func (i *Images) Kind() string {
	return "images"
}

// IsEmpty reports whether anything renderable is present. An entry missing
// either its source or its alt text does not count: a picture nobody can
// identify is not information on a safety document.
func (i *Images) IsEmpty() bool {
	for _, img := range i.Images {
		if strings.TrimSpace(img.Src) != "" && strings.TrimSpace(img.Alt) != "" {
			return false
		}
	}
	return true
}

// Append adds further images, skipping ones already present by source so the
// same pictogram cannot appear twice when two hazards both carry it.
func (i *Images) Append(other Content) (Content, error) {
	oi, isImages := other.(*Images)
	if !isImages {
		return nil, fmt.Errorf("%w: cannot append %s content to images",
			ErrKindMismatch, other.Kind())
	}

	seen := make(map[string]bool, len(i.Images))
	merged := make([]Image, 0, len(i.Images)+len(oi.Images))
	for _, img := range i.Images {
		seen[img.Src] = true
		merged = append(merged, img)
	}
	for _, img := range oi.Images {
		if seen[img.Src] {
			continue
		}
		seen[img.Src] = true
		merged = append(merged, img)
	}

	return &Images{Images: merged}, nil
}

func init() {
	Register("images", func() Content {
		return &Images{}
	})
}

// imageMediaTypes maps a file extension to the MIME type used in a data: URI.
var imageMediaTypes = map[string]string{
	".svg":  "image/svg+xml",
	".png":  "image/png",
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".webp": "image/webp",
}

// embedImages rewrites library-relative image paths into data: URIs.
//
// Variant files reference artwork by path -- `src: ghs/pictograms/GHS05.svg` --
// because pasting base64 into a YAML file nobody can read is a poor way to
// author content. Embedding happens here, at resolve time, because this is
// where the library filesystem is in hand and the renderer's is not.
//
// A src that is already a data: URI (as the computed pictograms are) passes
// through untouched. A path that cannot be read is an error: a pictogram
// silently missing from a safety data sheet is exactly the failure worth being
// loud about.
func embedImages(fsys fs.FS, body Content) (Content, error) {
	images, isImages := body.(*Images)
	if !isImages {
		return body, nil
	}

	out := make([]Image, 0, len(images.Images))
	changed := false

	for _, img := range images.Images {
		if img.Src == "" || strings.HasPrefix(img.Src, "data:") {
			out = append(out, img)
			continue
		}

		mediaType, ok := imageMediaTypes[strings.ToLower(path.Ext(img.Src))]
		if !ok {
			return nil, fmt.Errorf("image %q has unsupported extension %q",
				img.Src, path.Ext(img.Src))
		}

		data, err := fs.ReadFile(fsys, img.Src)
		if err != nil {
			return nil, fmt.Errorf("reading image %q: %w", img.Src, err)
		}

		img.Src = "data:" + mediaType + ";base64," + base64.StdEncoding.EncodeToString(data)
		out = append(out, img)
		changed = true
	}

	if !changed {
		return body, nil
	}
	return &Images{Images: out}, nil
}
