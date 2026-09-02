package config

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// Lengths are handled in millimetres throughout.
//
// The sheet is a physical letter page -- the stylesheet sizes pictograms in mm
// and @page in inches -- so a print measurement is the honest unit. Pixels are
// accepted for convenience and converted at the CSS reference of 96 per inch.
const (
	mmPerInch  = 25.4
	mmPerPoint = mmPerInch / 72
	mmPerPixel = mmPerInch / 96
)

// lengthUnits maps a CSS unit suffix to its size in millimetres.
var lengthUnits = map[string]float64{
	"mm": 1,
	"cm": 10,
	"in": mmPerInch,
	"pt": mmPerPoint,
	"px": mmPerPixel,
}

// ParseLength converts a CSS length such as "16mm", "0.75in", "48pt", "2cm" or
// "64px" into millimetres.
//
// A unit is required: a bare number on a document that gets printed is
// ambiguous, and guessing at one would be guessing at how big a logo appears
// on paper.
func ParseLength(s string) (float64, error) {
	trimmed := strings.TrimSpace(s)
	if trimmed == "" {
		return 0, fmt.Errorf("empty length")
	}

	for unit, scale := range lengthUnits {
		digits, found := strings.CutSuffix(strings.ToLower(trimmed), unit)
		if !found {
			continue
		}
		value, err := strconv.ParseFloat(strings.TrimSpace(digits), 64)
		if err != nil {
			return 0, fmt.Errorf("%q is not a number followed by a unit", s)
		}
		if value <= 0 || math.IsInf(value, 0) {
			return 0, fmt.Errorf("%q must be greater than zero", s)
		}
		return value * scale, nil
	}

	return 0, fmt.Errorf("%q has no unit; use mm, cm, in, pt or px", s)
}

// FormatLength renders millimetres as a CSS length, trimming trailing zeros so
// a whole number reads as "16mm" rather than "16.00mm".
func FormatLength(mm float64) string {
	return strconv.FormatFloat(roundTo(mm, 2), 'f', -1, 64) + "mm"
}

// roundTo rounds to n decimal places. Two is finer than any printer resolves
// and keeps the generated CSS readable.
func roundTo(v float64, n int) float64 {
	scale := math.Pow(10, float64(n))
	return math.Round(v*scale) / scale
}
