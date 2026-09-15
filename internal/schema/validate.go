package schema

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/isaacvarg/sdsforge/internal/sections"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/santhosh-tekuri/jsonschema/v6/kind"
	"golang.org/x/text/language"
	"golang.org/x/text/message"
	"gopkg.in/yaml.v3"
)

// Problem is one place a document.yaml disagrees with the schema.
type Problem struct {
	// Line and Column are 1-based positions in the YAML source, pointing at the
	// offending key or value. Both are 0 when the problem is with the document
	// as a whole.
	Line, Column int
	// Path is the dotted location of the value, e.g. sections.regulatory.
	Path    string
	Message string
}

func (p Problem) String() string {
	return fmt.Sprintf("%d:%d: %s: %s", p.Line, p.Column, p.Path, p.Message)
}

var printer = message.NewPrinter(language.English)

// Validate checks raw document.yaml content against the schema generated from
// lib.
//
// The schema is built from the same library the document will be resolved
// against, not read from docs/document.schema.json: that file describes the
// built-in library only, and would reject a custom variant the user is entitled
// to select.
//
// A returned error means validation could not run -- the YAML does not parse,
// or the schema does not compile. Problems with the document itself come back
// as Problems, all of them, sorted by position.
func Validate(lib *sections.Library, raw []byte) ([]Problem, error) {
	// Decoding into a node tolerates a repeated mapping key, which document.Load
	// rejects -- and which would leave problems pointing at the wrong copy. The
	// generic decode applies the same rules as Load, so it goes first.
	var probe any
	if err := yaml.Unmarshal(raw, &probe); err != nil {
		return nil, err
	}
	var root yaml.Node
	if err := yaml.Unmarshal(raw, &root); err != nil {
		return nil, err
	}

	schemaJSON, err := Generate(lib)
	if err != nil {
		return nil, err
	}
	sch, err := compile(schemaJSON)
	if err != nil {
		return nil, err
	}

	// An empty file has no node to point at, and "got null, want object" would
	// be a baffling way to say so.
	doc := documentNode(&root)
	if doc == nil {
		return []Problem{{Path: displayPath(nil), Message: "document is empty"}}, nil
	}
	instance, err := toInstance(doc)
	if err != nil {
		return nil, err
	}

	err = sch.Validate(instance)
	if err == nil {
		return nil, nil
	}
	var verr *jsonschema.ValidationError
	if !errors.As(err, &verr) {
		return nil, err
	}

	var problems []Problem
	collect(verr, doc, &problems)
	sort.SliceStable(problems, func(i, j int) bool {
		if problems[i].Line != problems[j].Line {
			return problems[i].Line < problems[j].Line
		}
		return problems[i].Column < problems[j].Column
	})
	return dedupe(problems), nil
}

func compile(schemaJSON []byte) (*jsonschema.Schema, error) {
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(schemaJSON))
	if err != nil {
		return nil, fmt.Errorf("reading generated schema: %w", err)
	}
	c := jsonschema.NewCompiler()
	if err := c.AddResource(CanonicalURL, doc); err != nil {
		return nil, fmt.Errorf("loading generated schema: %w", err)
	}
	sch, err := c.Compile(CanonicalURL)
	if err != nil {
		return nil, fmt.Errorf("compiling generated schema: %w", err)
	}
	return sch, nil
}

// documentNode unwraps the document node yaml.v3 puts around the content.
func documentNode(n *yaml.Node) *yaml.Node {
	switch n.Kind {
	case 0:
		// Empty input: yaml.Unmarshal leaves the node untouched.
		return nil
	case yaml.DocumentNode:
		if len(n.Content) == 0 {
			return nil
		}
		return n.Content[0]
	}
	return n
}

// toInstance converts a YAML node into the JSON-shaped value the validator
// understands.
//
// Walked by hand rather than decoded into an `any`, because the generic decode
// disagrees with how document.Load reads the same text: it turns an unquoted
// date into a time.Time, where Data's string fields receive the text as written.
// Scalars are typed by their resolved tag instead, which is exactly the view the
// schema's "type" keyword should see.
func toInstance(n *yaml.Node) (any, error) {
	switch n.Kind {
	case yaml.AliasNode:
		return toInstance(n.Alias)
	case yaml.MappingNode:
		m := make(map[string]any, len(n.Content)/2)
		for i := 0; i+1 < len(n.Content); i += 2 {
			v, err := toInstance(n.Content[i+1])
			if err != nil {
				return nil, err
			}
			m[n.Content[i].Value] = v
		}
		return m, nil
	case yaml.SequenceNode:
		s := make([]any, 0, len(n.Content))
		for _, item := range n.Content {
			v, err := toInstance(item)
			if err != nil {
				return nil, err
			}
			s = append(s, v)
		}
		return s, nil
	case yaml.ScalarNode:
		switch n.ShortTag() {
		case "!!null":
			return nil, nil
		case "!!bool":
			var b bool
			if err := n.Decode(&b); err != nil {
				return nil, err
			}
			return b, nil
		case "!!int", "!!float":
			var v any
			if err := n.Decode(&v); err != nil {
				return nil, err
			}
			num, err := json.Marshal(v)
			if err != nil {
				// .inf and .nan have no JSON form; the schema has no use for
				// them either, so let them fail as the wrong type.
				return n.Value, nil
			}
			return json.Number(num), nil
		default:
			return n.Value, nil
		}
	}
	return nil, fmt.Errorf("line %d: unsupported YAML node", n.Line)
}

// collect flattens a validation error tree into one Problem per real mistake.
func collect(e *jsonschema.ValidationError, doc *yaml.Node, out *[]Problem) {
	switch k := e.ErrorKind.(type) {
	case *kind.AdditionalProperties:
		// Reported against the object, which for a typo'd key means a line
		// number nowhere near the typo. Point at each unknown key instead.
		for _, prop := range k.Properties {
			loc := append(append([]string{}, e.InstanceLocation...), prop)
			*out = append(*out, problemAt(doc, loc, true, "unknown key "+strconv.Quote(prop)))
		}
		return

	case *kind.AnyOf, *kind.OneOf:
		// Every branch that failed reports its own reasons, most of them
		// irrelevant: a hazard code written as a string does not care that the
		// integer branch wanted an integer. Branches rejected purely on type are
		// the wrong shape and are dropped; so are branches rejected because
		// their `kind` const disagreed, which is a subsection accepting several
		// content kinds saying "you did not mean this one". If exactly one
		// remains, its reasons are the useful ones -- without the second filter
		// a table missing its `rows` in such a subsection reports only "not one
		// of the accepted forms" instead of naming the missing key.
		var plausible []*jsonschema.ValidationError
		for _, c := range e.Causes {
			if !onlyTypeMismatch(c) && !wrongDiscriminator(c) {
				plausible = append(plausible, c)
			}
		}
		if len(plausible) == 1 {
			collect(plausible[0], doc, out)
			return
		}
		if len(e.Causes) > 0 {
			*out = append(*out, problemAt(doc, e.InstanceLocation, false,
				"not one of the accepted forms"+valueSuffix(doc, e.InstanceLocation)))
			return
		}
	}

	if len(e.Causes) == 0 {
		*out = append(*out, problemAt(doc, e.InstanceLocation, false, e.ErrorKind.LocalizedString(printer)))
		return
	}
	for _, c := range e.Causes {
		collect(c, doc, out)
	}
}

// onlyTypeMismatch reports whether every leaf under e is a "type" failure.
func onlyTypeMismatch(e *jsonschema.ValidationError) bool {
	if len(e.Causes) == 0 {
		_, ok := e.ErrorKind.(*kind.Type)
		return ok
	}
	for _, c := range e.Causes {
		if !onlyTypeMismatch(c) {
			return false
		}
	}
	return true
}

func valueSuffix(doc *yaml.Node, loc []string) string {
	n, _ := lookup(doc, loc)
	if n != nil && n.Kind == yaml.ScalarNode {
		return ": " + strconv.Quote(n.Value)
	}
	return ""
}

// problemAt builds a Problem positioned at loc. atKey places it on the final
// key rather than its value, which is where an unknown key's mistake is.
func problemAt(doc *yaml.Node, loc []string, atKey bool, msg string) Problem {
	p := Problem{Path: displayPath(loc), Message: msg}
	n, key := lookup(doc, loc)
	switch {
	case atKey && key != nil:
		p.Line, p.Column = key.Line, key.Column
	case n != nil:
		p.Line, p.Column = n.Line, n.Column
	case doc != nil:
		p.Line, p.Column = doc.Line, doc.Column
	}
	return p
}

// lookup follows an instance location through the YAML tree, returning the
// value node and, when the last step was a mapping key, that key's node.
func lookup(n *yaml.Node, loc []string) (value, key *yaml.Node) {
	for _, seg := range loc {
		for n != nil && n.Kind == yaml.AliasNode {
			n = n.Alias
		}
		if n == nil {
			return nil, nil
		}
		key = nil
		switch n.Kind {
		case yaml.MappingNode:
			var next *yaml.Node
			for i := 0; i+1 < len(n.Content); i += 2 {
				if n.Content[i].Value == seg {
					key, next = n.Content[i], n.Content[i+1]
					break
				}
			}
			n = next
		case yaml.SequenceNode:
			i, err := strconv.Atoi(seg)
			if err != nil || i < 0 || i >= len(n.Content) {
				return nil, nil
			}
			n = n.Content[i]
		default:
			return nil, nil
		}
	}
	return n, key
}

func displayPath(loc []string) string {
	if len(loc) == 0 {
		return "(document)"
	}
	var b strings.Builder
	for _, seg := range loc {
		if _, err := strconv.Atoi(seg); err == nil {
			b.WriteString("[" + seg + "]")
			continue
		}
		if b.Len() > 0 {
			b.WriteByte('.')
		}
		b.WriteString(seg)
	}
	return b.String()
}

// dedupe drops identical problems, which the anyOf fallback can produce when a
// value fails the same way under several parents.
func dedupe(in []Problem) []Problem {
	out := in[:0]
	seen := map[Problem]bool{}
	for _, p := range in {
		if !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	return out
}

// wrongDiscriminator reports whether e was rejected because a `kind` key did
// not match a branch's const -- that is, this is not the branch the author was
// writing. Only content blocks carry such a const, so this is inert for every
// other union in the schema.
func wrongDiscriminator(e *jsonschema.ValidationError) bool {
	if _, ok := e.ErrorKind.(*kind.Const); ok &&
		len(e.InstanceLocation) > 0 &&
		e.InstanceLocation[len(e.InstanceLocation)-1] == "kind" {
		return true
	}
	for _, c := range e.Causes {
		if wrongDiscriminator(c) {
			return true
		}
	}
	return false
}
