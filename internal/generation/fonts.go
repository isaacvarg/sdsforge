package generation

import (
	"embed"
	"encoding/base64"
	"fmt"
	"html/template"
)

// fontFS holds the document's typeface. Fira Sans is embedded rather than
// linked for the same reason the logo and GHS pictograms are: a sheet gets
// emailed, printed and archived, so it has to stand alone, and PDF rendering
// injects HTML directly into headless Chrome with no navigation to a real
// URL for a stylesheet link to resolve against.
//
//go:embed fonts/*.woff2
var fontFS embed.FS

// fontWeights lists the static weights vendored above, each a Latin-subset
// woff2 from Google Fonts. These three cover every weight used in the
// document's type hierarchy -- no selector needs a fourth.
var fontWeights = []struct {
	file   string
	weight int
}{
	{"fonts/FiraSans-Regular.woff2", 400},
	{"fonts/FiraSans-SemiBold.woff2", 600},
	{"fonts/FiraSans-Bold.woff2", 700},
}

// fontFaceCSS is the @font-face rules for every embedded weight, built once
// at package load. It is template.CSS -- trusted, unescaped -- because it is
// assembled entirely from embedded binary data baked into the binary, never
// from user input, the same trust rationale as Logo.Style and imageURI.
var fontFaceCSS = template.CSS(buildFontFaceCSS())

func buildFontFaceCSS() string {
	var css string
	for _, w := range fontWeights {
		data, err := fontFS.ReadFile(w.file)
		if err != nil {
			panic(fmt.Sprintf("generation: reading embedded font %s: %v", w.file, err))
		}
		css += fmt.Sprintf(
			`@font-face{font-family:"Fira Sans";font-style:normal;font-weight:%d;font-display:swap;src:url(data:font/woff2;base64,%s) format("woff2")}`,
			w.weight, base64.StdEncoding.EncodeToString(data))
	}
	return css
}
