package command

import (
	"fmt"
	"github.com/charmbracelet/x/ansi"
	"regexp"
	"strings"
	"sync"
)

// Remove the entire tail: headers and passwords can contain several words.
var credentials = regexp.MustCompile(`(?i)(token|password|passwd|secret|authorization|api[_-]?key)([=: ]+).*`)
var urlCredentials = regexp.MustCompile(`(https?://)[^/@\s]+:[^/@\s]+@`)

// heldLines is how much of the end of a long output is kept once the live
// budget is spent; a failing process prints its cause last.
const heldLines = 20

type diagnostics struct {
	mu        sync.Mutex
	emit      func(string)
	remaining int
	pending   []byte
	dropping  bool
	held      []string
	omitted   int
	last      string
}

func sanitize(s string) string {
	s = ansi.Strip(s)
	s = credentials.ReplaceAllString(s, "$1$2[redacted]")
	s = urlCredentials.ReplaceAllString(s, "$1[redacted]@")
	return strings.Map(func(r rune) rune {
		if r < 32 && r != '\t' {
			return -1
		}
		return r
	}, s)
}

// line streams a completed line while the budget lasts. After that it keeps
// only the most recent lines, which flush shows when the process ends.
func (d *diagnostics) line(raw []byte) {
	s := sanitize(string(raw))
	if strings.TrimSpace(s) == "" {
		return
	}
	d.last = s
	if len(d.held) == 0 && d.remaining >= len(s) {
		d.remaining -= len(s)
		if d.emit != nil {
			d.emit(s)
		}
		return
	}
	d.held = append(d.held, s)
	if len(d.held) > heldLines {
		d.held = d.held[1:]
		d.omitted++
	}
}
func (d *diagnostics) Write(p []byte) (int, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, b := range p {
		if b == '\n' {
			if !d.dropping {
				d.line(d.pending)
			}
			d.pending = d.pending[:0]
			d.dropping = false
			continue
		}
		if d.dropping {
			continue
		}
		if len(d.pending) >= 4096 {
			d.pending = d.pending[:0]
			d.dropping = true
			continue
		}
		d.pending = append(d.pending, b)
	}
	return len(p), nil
}
func (d *diagnostics) flush() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if !d.dropping {
		d.line(d.pending)
	}
	d.pending = nil
	d.dropping = false
	if d.emit != nil {
		if d.omitted > 0 {
			d.emit(fmt.Sprintf("… %d lines omitted …", d.omitted))
		}
		for _, s := range d.held {
			d.emit(s)
		}
	}
	d.held, d.omitted = nil, 0
}

// lastLine is the final non-empty output line, shortened for an error message.
func (d *diagnostics) lastLine() string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return ansi.Truncate(strings.TrimSpace(d.last), 200, "…")
}
