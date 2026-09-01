// Package sections
package sections

import (
	"fmt"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

type Content interface {
	Kind() string
	IsEmpty() bool
	Append(other Content) (Content, error)
}

type Prose struct {
	Text []string `yaml:"text"`
}

type Block struct {
	Body Content
}

var _ Content = (*Prose)(nil)

var registry = map[string]func() Content{}

func Register(kind string, factoryFn func() Content) {
	if _, exists := registry[kind]; exists {
		panic(fmt.Sprintf("sections: content kind of %q registered twice", kind))
	}
	registry[kind] = factoryFn
}

func (p *Prose) Kind() string {
	return "prose"
}

func (p *Prose) IsEmpty() bool {
	for _, paragraphs := range p.Text {
		if strings.TrimSpace(paragraphs) != "" {
			return false
		}
	}
	return true
}

// this is needed because Table.Apend and Prose.Append have to lookk
// the same to compiler, and Content is thing that is common to both, but
// we want to use Prose, so we do this for the type assertion

func (p *Prose) Append(other Content) (Content, error) {
	typedValue, isProse := other.(*Prose)
	if !isProse {
		return nil, fmt.Errorf("%w: cannot append %s content to prose", ErrKindMismatch, other.Kind())
	}

	merged := make([]string, 0, len(p.Text)+len(typedValue.Text))
	merged = append(merged, p.Text...)
	merged = append(merged, typedValue.Text...)

	return &Prose{Text: merged}, nil
}

// need sorting, not randomized

func RegisteredKinds() []string {
	kinds := make([]string, 0, len(registry))
	for kind := range registry {
		kinds = append(kinds, kind)
	}
	slices.Sort(kinds)
	return kinds
}

func (b *Block) UnmarshalYAML(node *yaml.Node) error {
	// Shorthand: authors overriding a subsection should not have to spell out
	// a full block for the common case, so a bare sequence or string decodes
	// as prose.
	//
	//	append: ["Do not induce vomiting."]
	//	append: "Do not induce vomiting."
	//
	// Anything richer than paragraphs must use the explicit form, since there
	// is no `kind` here to dispatch on.
	switch node.Kind {
	case yaml.SequenceNode:
		var paragraphs []string
		if err := node.Decode(&paragraphs); err != nil {
			return fmt.Errorf("line %d: a sequence shorthand must be a list of strings: %w",
				node.Line, err)
		}
		b.Body = &Prose{Text: paragraphs}
		return nil
	case yaml.ScalarNode:
		var paragraph string
		if err := node.Decode(&paragraph); err != nil {
			return fmt.Errorf("line %d: a scalar shorthand must be a string: %w", node.Line, err)
		}
		b.Body = &Prose{Text: []string{paragraph}}
		return nil
	}

	var head struct {
		Kind string `yaml:"kind"`
	}
	if err := node.Decode(&head); err != nil {
		return fmt.Errorf("reading content kind: %w", err)
	}

	if head.Kind == "" {
		return fmt.Errorf("line %d: content block has no `kind` field; known kinds: %s",
			node.Line, strings.Join(RegisteredKinds(), ", "))
	}

	factory, ok := registry[head.Kind]
	if !ok {
		return fmt.Errorf("line %d: unknown content kind %q; known kinds: %s",
			node.Line, head.Kind, strings.Join(RegisteredKinds(), ", "))
	}

	body := factory()
	if err := node.Decode(body); err != nil {
		return fmt.Errorf("line %d: decoding %s content: %w", node.Line, head.Kind, err)
	}

	b.Body = body
	return nil
}

func init() {
	Register("prose", func() Content {
		return &Prose{}
	})
}
