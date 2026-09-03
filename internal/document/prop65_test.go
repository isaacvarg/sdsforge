package document

import (
	"strings"
	"testing"

	"github.com/isaacvarg/sdsforge/internal/config"
	"github.com/isaacvarg/sdsforge/internal/sections"
)

func TestProp65StatementEmpty(t *testing.T) {
	for _, warnings := range [][]Prop65Warning{
		nil,
		{{Chemical: "Acetone", Exposure: "not_a_real_exposure"}},
		{{Chemical: "", Exposure: "carcinogen"}},
	} {
		if got := prop65Statement(warnings); got != "" {
			t.Errorf("prop65Statement(%v) = %q, want empty", warnings, got)
		}
	}
}

func TestProp65Statement(t *testing.T) {
	tests := []struct {
		name     string
		warnings []Prop65Warning
		want     string
	}{
		{
			name:     "single carcinogen (A, trimmed)",
			warnings: []Prop65Warning{{Chemical: "Carbon black", Exposure: "carcinogen"}},
			want:     "This product can expose you to Carbon black, which is known to the State of California to cause cancer. For more information go to www.P65Warnings.ca.gov.",
		},
		{
			name: "multiple carcinogens (A)",
			warnings: []Prop65Warning{
				{Chemical: "Carbon black", Exposure: "carcinogen"},
				{Chemical: "Benzene", Exposure: "carcinogen"},
			},
			want: "This product can expose you to chemicals including Carbon black and Benzene, which are known to the State of California to cause cancer. For more information go to www.P65Warnings.ca.gov.",
		},
		{
			name:     "single reproductive toxicant (B, trimmed)",
			warnings: []Prop65Warning{{Chemical: "Toluene", Exposure: "reproductive_toxicant"}},
			want:     "This product can expose you to Toluene, which is known to the State of California to cause birth defects or other reproductive harm. For more information go to www.P65Warnings.ca.gov.",
		},
		{
			name: "separate carcinogen and reproductive toxicant chemicals (C)",
			warnings: []Prop65Warning{
				{Chemical: "Carbon black", Exposure: "carcinogen"},
				{Chemical: "Toluene", Exposure: "reproductive_toxicant"},
			},
			want: "This product can expose you to chemicals including Carbon black, which is known to the State of California to cause cancer, and Toluene, which is known to the State of California to cause birth defects or other reproductive harm. For more information go to www.P65Warnings.ca.gov.",
		},
		{
			name:     "single chemical listed as both (D, trimmed)",
			warnings: []Prop65Warning{{Chemical: "Lead", Exposure: "both"}},
			want:     "This product can expose you to Lead, which is known to the State of California to cause cancer and birth defects or other reproductive harm. For more information go to www.P65Warnings.ca.gov.",
		},
		{
			name: "same chemical declared via two separate entries collapses to (D)",
			warnings: []Prop65Warning{
				{Chemical: "Lead", Exposure: "carcinogen"},
				{Chemical: "Lead", Exposure: "reproductive_toxicant"},
			},
			want: "This product can expose you to Lead, which is known to the State of California to cause cancer and birth defects or other reproductive harm. For more information go to www.P65Warnings.ca.gov.",
		},
		{
			name:     "unrecognized exposure is ignored",
			warnings: []Prop65Warning{{Chemical: "Water", Exposure: "harmless"}, {Chemical: "Lead", Exposure: "carcinogen"}},
			want:     "This product can expose you to Lead, which is known to the State of California to cause cancer. For more information go to www.P65Warnings.ca.gov.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := prop65Statement(tt.warnings); got != tt.want {
				t.Errorf("prop65Statement() =\n%q\nwant\n%q", got, tt.want)
			}
		})
	}
}

func TestProp65BlockOmittedWhenEmpty(t *testing.T) {
	if block := prop65Block(nil); block != nil {
		t.Errorf("prop65Block(nil) = %v, want nil", block)
	}
}

func TestProp65BlockRendersSymbolAndCaption(t *testing.T) {
	block := prop65Block([]Prop65Warning{{Chemical: "Lead", Exposure: "both"}})
	images, ok := block.(*sections.Images)
	if !ok {
		t.Fatalf("prop65Block() = %T, want *sections.Images", block)
	}
	if len(images.Images) != 1 {
		t.Fatalf("len(Images) = %d, want 1", len(images.Images))
	}

	img := images.Images[0]
	if !strings.HasPrefix(img.Src, "data:image/svg+xml;base64,") {
		t.Errorf("Src = %q, want an embedded svg data URI", img.Src)
	}
	if img.Alt == "" {
		t.Error("Alt is empty")
	}
	if !strings.Contains(img.Caption, "Lead") {
		t.Errorf("Caption = %q, want it to name the chemical", img.Caption)
	}
}

func TestSourceDataIncludesProp65(t *testing.T) {
	d := Data{Prop65: []Prop65Warning{{Chemical: "Lead", Exposure: "both"}}}
	got := d.SourceData(nil, config.Config{}, VersionIndex{})
	if _, ok := got.Block(sections.SourceProp65); !ok {
		t.Error("SourceData() did not populate SourceProp65")
	}
}

// TestExposureValuesAllNormalize guards the list the JSON Schema generator
// publishes against the switch that actually decides. A spelling advertised in
// the schema but rejected by the parser would be worse than no schema at all:
// the editor would bless a value that silently drops the entry.
func TestExposureValuesAllNormalize(t *testing.T) {
	for _, v := range ExposureValues() {
		if got := normalizeExposure(v); got == "" {
			t.Errorf("ExposureValues lists %q, but normalizeExposure rejects it", v)
		}
	}
}
