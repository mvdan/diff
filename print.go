package diff

import (
	"fmt"
	"io"
)

// TODO: add diff writing that uses < and > (don't know what that is called)
// TODO: add side by side diffs
// TODO: add html diffs (?)
// TODO: add intraline highlighting?
// TODO: a way to specify alternative colors, like a ColorScheme write option

// A WriteOpt is used to provide options when writing a diff.
type WriteOpt interface {
	isWriteOpt()
}

// Names provides the before/after names for writing a diff.
// They are traditionally filenames.
func Names(a, b string) WriteOpt {
	return names{a, b}
}

type names struct {
	a, b string
}

func (names) isWriteOpt() {}

// TerminalColor specifies that a diff intended for a terminal should be written
// using red and green colors.
//
// Do not use TerminalColor if TERM=dumb is set in the environment.
func TerminalColor() WriteOpt {
	return colorOpt(true)
}

type colorOpt bool

func (colorOpt) isWriteOpt() {}

const (
	ansiFgRed   = "\u001b[31m"
	ansiFgGreen = "\u001b[32m"
	ansiReset   = "\u001b[0m"
)

// WriteUnified writes e to w using unified diff format.
// ab writes the individual elements. Opts are optional write arguments.
// WriteUnified returns the number of bytes written and the first error (if any) encountered.
func (e EditScript) WriteUnified(w io.Writer, ab WriterTo, opts ...WriteOpt) (int, error) {
	// read opts
	nameA := "a"
	nameB := "b"
	color := false
	for _, opt := range opts {
		switch opt := opt.(type) {
		case names:
			nameA = opt.a
			nameB = opt.b
		case colorOpt:
			// TODO: color "---" and "@@" lines too?
			color = true
		// TODO: add date/time/timezone WriteOpts
		default:
			panic(fmt.Sprintf("unrecognized WriteOpt type %T", opt))
		}
	}

	ew := newErrWriter(w)
	// TODO: Wrap w in a bufio.Writer? And then use w.WriteByte below instead of w.Write.
	// Maybe bufio.Writer is enough and we should entirely ditch newErrWriter.

	// per-file header
	fmt.Fprintf(ew, "--- %s\n", nameA)
	fmt.Fprintf(ew, "+++ %s\n", nameB)

	needsColorReset := false

	for i := 0; i < len(e.segs); {
		// Peek into the future to learn the line ranges for this chunk of output.
		// A chunk of output ends when there's a discontiguity in the edit script.
		var ar, br lineRange
		var started [2]bool
		var j int
		for j = i; j < len(e.segs); j++ {
			curr := e.segs[j]
			switch curr.op() {
			case del, eq:
				if !started[0] {
					ar.first = curr.FromA
					started[0] = true
				}
				ar.last = curr.ToA
			}
			switch curr.op() {
			case ins, eq:
				if !started[1] {
					br.first = curr.FromB
					started[1] = true
				}
				br.last = curr.ToB
			}
			if j+1 >= len(e.segs) {
				// end of script
				break
			}
			if next := e.segs[j+1]; curr.ToA != next.FromA || curr.ToB != next.FromB {
				// discontiguous edit script
				break
			}
		}

		// Print chunk header.
		// TODO: add per-chunk context, like what function we're in
		// But how do we get this? need to add PairWriter methods?
		// Maybe it should be stored in the EditScript,
		// and we can have EditScript methods to populate it somehow?
		fmt.Fprintf(ew, "@@ -%s +%s @@\n", ar, br)

		// Print prefixed lines.
		for k := i; k <= j; k++ {
			seg := e.segs[k]
			switch seg.op() {
			case eq:
				if needsColorReset {
					ew.WriteString(ansiReset)
				}
				for m := seg.FromA; m < seg.ToA; m++ {
					// " a[m]\n"
					ew.WriteByte(' ')
					ab.WriteATo(ew, m)
					ew.WriteByte('\n')
				}
			case del:
				if color {
					ew.WriteString(ansiFgRed)
					needsColorReset = true
				}
				for m := seg.FromA; m < seg.ToA; m++ {
					// "-a[m]\n"
					ew.WriteByte('-')
					ab.WriteATo(ew, m)
					ew.WriteByte('\n')
				}
			case ins:
				if color {
					ew.WriteString(ansiFgGreen)
					needsColorReset = true
				}
				for m := seg.FromB; m < seg.ToB; m++ {
					// "+b[m]\n"
					ew.WriteByte('+')
					ab.WriteBTo(ew, m)
					ew.WriteByte('\n')
				}
			}
		}

		// Advance to next chunk.
		i = j + 1

		// TODO: break if error detected?
	}

	// Always finish the output with no color, to prevent "leaking" the
	// color into any output that follows a diff.
	if needsColorReset {
		ew.WriteString(ansiReset)
	}

	// TODO:
	// If the last line of a file doesn't end in a newline character,
	// it is displayed with a newline character,
	// and the following line in the chunk has the literal text (starting in the first column):
	// '\ No newline at end of file'

	return ew.wrote, ew.Error()
}

type lineRange struct {
	first, last int
}

func (r lineRange) String() string {
	len := r.last - r.first
	r.first++ // 1-based index, safe to modify r directly because it is a value
	if len <= 0 {
		r.first-- // for no obvious reason, empty ranges are "before" the range
	}
	return fmt.Sprintf("%d,%d", r.first, len)
}

func (r lineRange) GoString() string {
	return fmt.Sprintf("(%d, %d)", r.first, r.last)
}

func newErrWriter(w io.Writer) *errwriter {
	return &errwriter{w: w}
}

type errwriter struct {
	w         io.Writer
	err       error
	wrote     int
	attempted int
}

func (w *errwriter) Write(b []byte) (int, error) {
	w.attempted += len(b)
	if w.err != nil {
		return 0, w.err // TODO: use something like errors.Wrap(w.err)?
	}
	n, err := w.w.Write(b)
	if err != nil {
		w.err = err
	}
	w.wrote += n
	return n, err
}

func (w *errwriter) WriteString(s string) {
	// TODO: use w.w's WriteString method, if it exists
	w.Write([]byte(s))
}

func (w *errwriter) WriteByte(b byte) {
	// TODO: use w.w's WriteByte method, if it exists
	w.Write([]byte{b})
}

func (w *errwriter) Error() error { return w.err }
