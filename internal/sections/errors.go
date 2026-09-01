package sections

import "errors"

// Sentinel errors for the content package.
//
// A sentinel is a package-level error VALUE that callers can test for with
// errors.Is, instead of matching on message text (which breaks the moment you
// reword a message). Create one whenever a caller might reasonably want to
// react differently to this failure -- Phase 8's validator will want to report
// a ragged row differently from a kind mismatch.
//
// The convention is `Err` + a short noun phrase, and the message reads as a
// fragment, because it is almost always wrapped with more context:
//
//	fmt.Errorf("%w: cannot append %s to table", ErrKindMismatch, other.Kind())
//
// errors.Is then sees THROUGH that %w wrapping down to the sentinel itself.
var (
	// ErrKindMismatch means two content blocks of different kinds were combined.
	ErrKindMismatch = errors.New("content kind mismatch")

	// ErrHeaderMismatch means two tables with incompatible column headers were
	// appended.
	ErrHeaderMismatch = errors.New("table header mismatch")

	// ErrRaggedRow means a table row has a different number of cells than the
	// table has columns.
	ErrRaggedRow = errors.New("table row width mismatch")
)
