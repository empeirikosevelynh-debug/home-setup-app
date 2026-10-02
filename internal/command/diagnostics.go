package command

import (
	"github.com/charmbracelet/x/ansi"
	"regexp"
	"strings"
	"sync"
)

// Remove the entire tail: headers and passwords can contain several words.
var credentials = regexp.MustCompile(`(?i)(token|password|passwd|secret|authorization|api[_-]?key)([=: ]+).*`)
var urlCredentials = regexp.MustCompile(`(https?://)[^/@\s]+:[^/@\s]+@`)

type diagnostics struct {
	mu        sync.Mutex
	emit      func(string)
	remaining int
	pending   string
	dropping  bool
}

func (d *diagnostics) emitLine(s string) {
	if d.emit == nil {
		return
	}
	s = ansi.Strip(s)
	s = credentials.ReplaceAllString(s, "$1$2[redacted]")
	s = urlCredentials.ReplaceAllString(s, "$1[redacted]@")
	s = strings.Map(func(r rune) rune {
		if r < 32 && r != '\t' {
			return -1
		}
		return r
	}, s)
	if s != "" {
		d.emit(s)
	}
}
func (d *diagnostics) Write(p []byte) (int, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	n := len(p)
	for _, b := range p {
		if d.remaining <= 0 {
			break
		}
		d.remaining--
		if b == '\n' {
			if !d.dropping {
				d.emitLine(d.pending)
			}
			d.pending = ""
			d.dropping = false
			continue
		}
		if d.dropping {
			continue
		}
		if len(d.pending) >= 4096 {
			d.pending = ""
			d.dropping = true
			continue
		}
		d.pending += string(b)
	}
	return n, nil
}
func (d *diagnostics) flush() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if !d.dropping {
		d.emitLine(d.pending)
	}
	d.pending = ""
}
