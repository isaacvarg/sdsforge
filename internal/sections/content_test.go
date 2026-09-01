package sections

// Test files live in the same directory as the code and end in _test.go.
// Declaring `package sections` (not sections_test) means these tests can see
// unexported identifiers like `registry`.

import (
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// ---------------------------------------------------------------------------
// A test double.
//
// Several tests below need "some Content that is NOT prose". Table doesn't
// exist until Phase 2, so we define a minimal stand-in here. Because Go
// interfaces are structural, anything with these three methods satisfies
// Content -- no declaration required.
//
// Types defined in a _test.go file exist only during testing; they are not
// compiled into the real binary.
// ---------------------------------------------------------------------------

type stubContent struct{}

func (s *stubContent) Kind() string                    { return "stub" }
func (s *stubContent) IsEmpty() bool                   { return true }
func (s *stubContent) Append(Content) (Content, error) { return s, nil }

// Compile-time proof that both types satisfy Content. These cost nothing at
// runtime; they turn "does not implement" into an error on THIS line rather
// than somewhere confusing downstream. Consider adding the *Prose one to
// content.go itself.
// (the *Prose assertion now lives in content.go, next to the type)
var _ Content = (*stubContent)(nil)

// ---------------------------------------------------------------------------
// Prose.Kind
// ---------------------------------------------------------------------------

func TestProseKind(t *testing.T) {
	got := (&Prose{}).Kind()
	// Convention: report as `got, want`. %q quotes strings so an empty or
	// space-padded value is visible in the output.
	if got != "prose" {
		t.Errorf("Kind() = %q, want %q", got, "prose")
	}
}

// ---------------------------------------------------------------------------
// Prose.IsEmpty -- the table-driven idiom.
//
// `tests` is a slice of an ANONYMOUS struct type: defined inline because it is
// only used here. Adding a case is adding one line, which is the whole appeal.
// ---------------------------------------------------------------------------

func TestProseIsEmpty(t *testing.T) {
	tests := []struct {
		name string
		text []string
		want bool
	}{
		{"nil slice", nil, true},
		{"empty slice", []string{}, true},
		{"single empty string", []string{""}, true},
		{"whitespace only", []string{"   ", "\t\n"}, true},
		{"real content", []string{"Move victim to fresh air."}, false},
		{"blank then real", []string{"", "Give oxygen."}, false},
	}

	for _, tt := range tests {
		// t.Run creates a SUBTEST. It reports as
		// TestProseIsEmpty/whitespace_only and fails independently of its
		// siblings. Run `go test -v` to see them listed.
		t.Run(tt.name, func(t *testing.T) {
			p := &Prose{Text: tt.text}
			if got := p.IsEmpty(); got != tt.want {
				t.Errorf("IsEmpty() = %v, want %v (text=%q)", got, tt.want, tt.text)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Prose.Append -- happy path.
// ---------------------------------------------------------------------------

func TestProseAppend(t *testing.T) {
	base := &Prose{Text: []string{"first"}}
	extra := &Prose{Text: []string{"second", "third"}}

	result, err := base.Append(extra)
	// t.Fatalf (not Errorf) because every line below would panic on a nil
	// result. Fatalf stops THIS test only; other tests still run.
	if err != nil {
		t.Fatalf("Append() unexpected error: %v", err)
	}

	// Append returns the Content interface, so assert back to the concrete
	// type to inspect it. The two-value form never panics.
	got, ok := result.(*Prose)
	if !ok {
		t.Fatalf("Append() returned %T, want *Prose", result)
	}

	want := []string{"first", "second", "third"}
	// []string cannot be compared with ==. slices.Equal is the modern stdlib
	// answer; ignore older advice pointing at reflect.DeepEqual.
	if !slices.Equal(got.Text, want) {
		t.Errorf("Append() = %q, want %q", got.Text, want)
	}
}

// ---------------------------------------------------------------------------
// Prose.Append -- purity.
//
// This is the test that catches the slice-aliasing bug. If Append were written
// the tempting way:
//
//	return &Prose{Text: append(p.Text, op.Text...)}
//
// then when p.Text has spare CAPACITY, append writes into p's existing backing
// array instead of allocating. Two appends from the same base would then share
// memory and the second would silently corrupt the first.
//
// Your implementation allocates a fresh slice, so this passes.
// ---------------------------------------------------------------------------

func TestProseAppendDoesNotAlias(t *testing.T) {
	// make(len=1, cap=8): length 1, but room for 7 more -- exactly the
	// condition under which a naive append reuses the array.
	base := &Prose{Text: make([]string, 1, 8)}
	base.Text[0] = "shared"

	first, err := base.Append(&Prose{Text: []string{"A"}})
	if err != nil {
		t.Fatalf("first Append() error: %v", err)
	}
	second, err := base.Append(&Prose{Text: []string{"B"}})
	if err != nil {
		t.Fatalf("second Append() error: %v", err)
	}

	if got := first.(*Prose).Text; !slices.Equal(got, []string{"shared", "A"}) {
		t.Errorf("first result was mutated by the second Append: got %q", got)
	}
	if got := second.(*Prose).Text; !slices.Equal(got, []string{"shared", "B"}) {
		t.Errorf("second result = %q, want [shared B]", got)
	}
	// The receiver itself must be untouched: variants are loaded once and
	// reused across many documents.
	if !slices.Equal(base.Text, []string{"shared"}) {
		t.Errorf("Append mutated its receiver: base.Text = %q", base.Text)
	}
}

// ---------------------------------------------------------------------------
// Prose.Append -- kind mismatch is user error, so it returns an error rather
// than panicking.
// ---------------------------------------------------------------------------

func TestProseAppendKindMismatch(t *testing.T) {
	_, err := (&Prose{Text: []string{"a"}}).Append(&stubContent{})
	if err == nil {
		t.Fatal("Append(stub) = nil error, want an error")
	}
	// Assert the message is actually useful. Phase 8 surfaces these straight
	// to the person editing the YAML, so they must name the offending kind.
	if !strings.Contains(err.Error(), "stub") {
		t.Errorf("error %q should mention the offending kind %q", err, "stub")
	}
}

// ---------------------------------------------------------------------------
// The registry.
//
// init() has already run by the time any test body executes, so "prose" must
// be present without any setup here.
// ---------------------------------------------------------------------------

func TestProseIsRegistered(t *testing.T) {
	factory, ok := registry["prose"]
	if !ok {
		t.Fatalf("registry has no %q kind; keys present: %v", "prose", RegisteredKinds())
	}

	// The registry stores a FACTORY, not a value, so each call must hand back
	// a distinct instance -- otherwise every block in a document would decode
	// into the same struct.
	a, b := factory(), factory()
	if _, isProse := a.(*Prose); !isProse {
		t.Fatalf("factory returned %T, want *Prose", a)
	}
	if a == b {
		t.Error("factory returned the same instance twice; it must construct a new value per call")
	}
}

func TestRegisterRejectsDuplicates(t *testing.T) {
	// Register panics on a duplicate: it runs at init time, so a collision is
	// a bug in the binary, not bad user input. recover() catches a panic and
	// must be called from a DEFERRED function -- defer runs on the way out of
	// the enclosing function, including during a panic.
	defer func() {
		if r := recover(); r == nil {
			t.Error("Register(duplicate) did not panic")
		}
	}()

	// "prose" was claimed by init(). This must panic before it mutates the
	// map, so the registry stays clean for other tests.
	Register("prose", func() Content { return &Prose{} })

	t.Fatal("unreachable: Register should have panicked")
}

// ---------------------------------------------------------------------------
// Block.UnmarshalYAML
//
// >>> THESE TWO FAIL RIGHT NOW, ON PURPOSE. <<<
//
// content.go has no UnmarshalYAML yet, so yaml.v3 tries to decode a mapping
// straight into the Content INTERFACE field and cannot: it has no idea which
// concrete type to build. That is precisely the problem the two-pass decode
// solves. Make these pass and Phase 1 is done.
// ---------------------------------------------------------------------------

func TestBlockUnmarshalProse(t *testing.T) {
	// A raw string literal (backticks) avoids escaping every quote and \n.
	// Keep the YAML flush-left: raw strings preserve your Go indentation, and
	// YAML is whitespace-sensitive.
	src := `
kind: prose
text:
  - "Move victim to fresh air."
  - "If breathing is difficult, give oxygen."
`

	var blk Block
	if err := yaml.Unmarshal([]byte(src), &blk); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	// %T prints the concrete type inside the interface -- the right verb for
	// "I got the wrong thing out of an interface" messages.
	p, ok := blk.Body.(*Prose)
	if !ok {
		t.Fatalf("Body is %T, want *Prose", blk.Body)
	}

	want := []string{
		"Move victim to fresh air.",
		"If breathing is difficult, give oxygen.",
	}
	if !slices.Equal(p.Text, want) {
		t.Errorf("Text = %q, want %q", p.Text, want)
	}
}

func TestBlockUnmarshalErrors(t *testing.T) {
	tests := []struct {
		name string
		src  string
	}{
		{"unknown kind", "kind: definitely_not_a_kind\ntext: []\n"},
		{"missing kind", "text:\n  - hello\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var blk Block
			err := yaml.Unmarshal([]byte(tt.src), &blk)
			if err == nil {
				t.Fatalf("Unmarshal(%q) = nil error, want an error", tt.src)
			}
			// The message must name the bad kind AND list the valid ones --
			// you will be reading this error a lot while authoring YAML.
			t.Logf("error message was: %v", err) // -v shows this; not a failure
		})
	}
}
