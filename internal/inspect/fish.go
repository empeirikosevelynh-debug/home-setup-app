package inspect

import (
	"errors"
	"fmt"
	"golden-gate-setup/internal/domain"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// FishUniversalVariables reads fish's universal-variable store directly.
// `fish --no-config` does not load universal variables, so asking fish would
// hide existing Fisher state; reading the store also runs no user configuration.
func FishUniversalVariables(h domain.Host) (map[string][]string, error) {
	config := h.ConfigHome
	if !filepath.IsAbs(config) {
		config = filepath.Join(h.Home, ".config")
	}
	path := filepath.Join(config, "fish/fish_variables")
	st, e := os.Stat(path)
	if errors.Is(e, os.ErrNotExist) {
		return map[string][]string{}, nil
	}
	if e != nil {
		return nil, e
	}
	if !st.Mode().IsRegular() {
		return nil, fmt.Errorf("not a regular file: %s", path)
	}
	f, e := os.Open(path)
	if e != nil {
		return nil, e
	}
	defer f.Close()
	data, e := io.ReadAll(io.LimitReader(f, (1<<20)+1))
	if e != nil {
		return nil, e
	}
	if len(data) > 1<<20 {
		return nil, errors.New("fish universal variables exceed 1 MiB")
	}
	vars := map[string][]string{}
	for _, line := range strings.Split(string(data), "\n") {
		rest, ok := strings.CutPrefix(line, "SETUVAR ")
		if !ok {
			continue
		}
		for strings.HasPrefix(rest, "--") {
			_, rest, _ = strings.Cut(rest, " ")
		}
		name, value, ok := strings.Cut(rest, ":")
		if !ok || name == "" {
			continue
		}
		value = unescapeFish(value)
		if value == "\x1d" {
			vars[name] = []string{}
		} else {
			vars[name] = strings.Split(value, "\x1e")
		}
	}
	return vars, nil
}

// unescapeFish reverses the escaping fish applies to stored values: letters,
// digits, / and _ are literal and everything else is a \x, \u or \U escape.
func unescapeFish(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != '\\' || i+1 == len(s) {
			b.WriteByte(s[i])
			continue
		}
		i++
		switch c := s[i]; c {
		case 'n':
			b.WriteByte('\n')
		case 't':
			b.WriteByte('\t')
		case 'r':
			b.WriteByte('\r')
		case 'e':
			b.WriteByte(0x1b)
		case 'x', 'X', 'u', 'U':
			digits := map[byte]int{'x': 2, 'X': 2, 'u': 4, 'U': 8}[c]
			end := i + 1
			for end < len(s) && end <= i+digits && strings.IndexByte("0123456789abcdefABCDEF", s[end]) >= 0 {
				end++
			}
			n, e := strconv.ParseUint(s[i+1:end], 16, 32)
			if e != nil {
				b.WriteByte(c)
				continue
			}
			b.WriteRune(rune(n))
			i = end - 1
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}
