// Package testutil contains isolated external-system doubles. Production code
// must not import it; the terminal fixture has a separate executable.
package sandbox

import (
	"context"
	"encoding/json"
	"fmt"
	"golden-gate-setup/internal/command"
	"golden-gate-setup/internal/domain"
	"golden-gate-setup/internal/inspect"
	"golden-gate-setup/internal/plan"
	"golden-gate-setup/internal/testutil"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

type Sandbox struct {
	Home        string
	FailToken   string
	mu          sync.Mutex
	packages    map[string]domain.InstalledPackage
	plugins     []string
	managed     map[string]bool
	sourceDirty bool
	calls       []domain.Command
}

func NewSandbox(home string) *Sandbox {
	return &Sandbox{Home: home, packages: map[string]domain.InstalledPackage{}, managed: map[string]bool{}}
}
func (s *Sandbox) Lookup(name string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if name == "/opt/homebrew/bin/brew" {
		return name, nil
	}
	if name == "git" {
		return exec.LookPath("git")
	}
	if name == "gopls" || name == "nimlangserver" {
		base := "go"
		if name == "nimlangserver" {
			base = ".nimble"
		}
		path := filepath.Join(s.Home, base, "bin", name)
		if st, e := os.Stat(path); e == nil && st.Mode().Perm()&0111 != 0 {
			return path, nil
		}
		return "", fs.ErrNotExist
	}
	pkg := name
	switch name {
	case "rg":
		pkg = "ripgrep"
	case "delta":
		pkg = "git-delta"
	case "dlv":
		pkg = "delve"
	case "nimble", "nimpretty":
		pkg = "nim"
	}
	if _, ok := s.packages["formula:"+pkg]; ok {
		return "/fake/" + name, nil
	}
	if name == "zed" {
		if _, ok := s.packages["cask:zed"]; ok {
			return "/fake/zed", nil
		}
	}
	return "", fs.ErrNotExist
}
func (s *Sandbox) Inspect(ctx context.Context, o domain.Options) (domain.Host, error) {
	for _, w := range o.Workspaces {
		if w.Path != "" {
			rel, e := filepath.Rel(s.Home, w.Path)
			if e != nil || rel == ".." || strings.HasPrefix(rel, "../") {
				return domain.Host{}, fmt.Errorf("fixture workspaces must stay in its temporary home")
			}
		}
	}
	return (inspect.Inspector{Home: s.Home, Runner: s, Lookup: s.Lookup, Platform: func(context.Context) (domain.Host, error) { return testutil.FreshHost(s.Home), nil }, AppsDirs: []string{filepath.Join(s.Home, "Applications")}}).Read(ctx, o)
}
func (s *Sandbox) Run(ctx context.Context, c domain.Command, w io.Writer) error {
	if e := ctx.Err(); e != nil {
		return e
	}
	s.mu.Lock()
	s.calls = append(s.calls, c)
	s.mu.Unlock()
	if w == nil {
		w = io.Discard
	}
	if strings.HasSuffix(c.Path, "/git") && len(c.Args) > 0 && c.Args[0] == "config" {
		for i, a := range c.Args {
			if a == "--file" && i+1 < len(c.Args) {
				rel, e := filepath.Rel(s.Home, c.Args[i+1])
				if e != nil || rel == ".." || strings.HasPrefix(rel, "../") {
					return fmt.Errorf("Git fixture escaped its home")
				}
			}
		}
		return (command.ProcessRunner{}).Run(ctx, c, w)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	switch {
	case c.Path == "/opt/homebrew/bin/brew":
		if len(c.Args) == 0 {
			return fmt.Errorf("empty brew call")
		}
		switch c.Args[0] {
		case "--prefix":
			io.WriteString(w, "/opt/homebrew\n")
		case "info":
			formulae := []map[string]any{}
			casks := []map[string]any{}
			keys := []string{}
			for k := range s.packages {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, key := range keys {
				kind, token, _ := strings.Cut(key, ":")
				v := s.packages[key]
				if kind == "formula" {
					formulae = append(formulae, map[string]any{"name": token, "full_name": token, "linked_keg": v.Version, "installed": []map[string]any{{"version": v.Version, "installed_on_request": true}}})
				} else {
					casks = append(casks, map[string]any{"token": token, "tap": "homebrew/cask", "installed": v.Version})
				}
			}
			return json.NewEncoder(w).Encode(map[string]any{"formulae": formulae, "casks": casks})
		case "install":
			if len(c.Args) != 3 {
				return fmt.Errorf("unexpected install arguments")
			}
			kind := strings.TrimPrefix(c.Args[1], "--")
			token := c.Args[2]
			if token == s.FailToken {
				return fmt.Errorf("fixture package installation failed")
			}
			yes := true
			s.packages[kind+":"+token] = domain.InstalledPackage{Version: "99.0.0", OnRequest: &yes}
			if kind == "cask" {
				return os.MkdirAll(filepath.Join(s.Home, "Applications", plan.AppNames[token]+".app"), 0700)
			}
		case "bundle":
			var path string
			for _, a := range c.Args {
				if strings.HasPrefix(a, "--file=") {
					path = strings.TrimPrefix(a, "--file=")
				}
			}
			if path == "" {
				return fmt.Errorf("dump needs a path")
			}
			var lines []string
			for key := range s.packages {
				kind, token, _ := strings.Cut(key, ":")
				if kind == "formula" {
					kind = "brew"
				}
				lines = append(lines, fmt.Sprintf("%s %q", kind, token))
			}
			sort.Strings(lines)
			return os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0600)
		default:
			return fmt.Errorf("unexpected brew command")
		}
	case c.Path == "/fake/lazygit":
		io.WriteString(w, filepath.Join(s.Home, "Library/Application Support/lazygit")+"\n")
	case c.Path == "/fake/fish":
		script := strings.Join(c.Args, " ")
		if strings.Contains(script, "fisher install") {
			at := -1
			for i, a := range c.Args {
				if a == "--" {
					at = i + 2
					break
				}
			}
			if at < 0 || at >= len(c.Args) {
				return fmt.Errorf("missing Fisher selection")
			}
			s.plugins = append([]string(nil), c.Args[at:]...)
			return os.WriteFile(filepath.Join(s.Home, ".config/fish/fish_plugins"), []byte(strings.Join(s.plugins, "\n")+"\n"), 0644)
		}
		if strings.Contains(script, "$_fisher_plugins") {
			io.WriteString(w, strings.Join(s.plugins, "\n"))
		}
	case c.Path == "/fake/chezmoi":
		if len(c.Args) == 0 {
			return fmt.Errorf("empty chezmoi call")
		}
		src := filepath.Join(s.Home, ".local/share/chezmoi")
		switch c.Args[0] {
		case "source-path":
			io.WriteString(w, src+"\n")
		case "init":
			return os.MkdirAll(filepath.Join(src, ".git"), 0700)
		case "managed":
			var paths []string
			for p := range s.managed {
				paths = append(paths, p)
			}
			sort.Strings(paths)
			io.WriteString(w, strings.Join(paths, "\x00"))
		case "add":
			path := c.Args[len(c.Args)-1]
			s.managed[path] = true
			s.sourceDirty = true
		case "status":
			if s.sourceDirty {
				io.WriteString(w, " M chosen settings\n")
			}
		default:
			return fmt.Errorf("unexpected chezmoi call")
		}
	case strings.HasSuffix(c.Path, "/git") && len(c.Args) > 0 && c.Args[0] == "-C":
		if s.sourceDirty {
			io.WriteString(w, " M selected files\n")
		}
	case c.Path == "/fake/go":
		switch c.Args[0] {
		case "env":
			return json.NewEncoder(w).Encode(map[string]string{"GOBIN": "", "GOPATH": filepath.Join(s.Home, "go")})
		case "install":
			path := filepath.Join(s.Home, "go/bin/gopls")
			os.MkdirAll(filepath.Dir(path), 0700)
			return os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0755)
		case "mod":
			if c.Args[1] == "init" {
				return os.WriteFile(filepath.Join(c.Dir, "go.mod"), []byte("module "+c.Args[2]+"\n\ngo 1.26.0\n"), 0644)
			}
			if c.Args[1] == "edit" {
				return nil
			}
		default:
			return fmt.Errorf("unexpected Go call")
		}
	case c.Path == "/fake/nimble":
		path := filepath.Join(s.Home, ".nimble/bin/nimlangserver")
		os.MkdirAll(filepath.Dir(path), 0700)
		return os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0755)
	default:
		return fmt.Errorf("fixture refuses unknown command %s", c.Path)
	}
	return nil
}
func (s *Sandbox) Migrated() {
	os.MkdirAll(filepath.Join(s.Home, "Applications/Zed.app/Contents/_MASReceipt"), 0700)
	os.WriteFile(filepath.Join(s.Home, "Applications/Zed.app/Contents/_MASReceipt/receipt"), []byte("receipt"), 0600)
	os.MkdirAll(filepath.Join(s.Home, ".config/zed"), 0700)
	os.WriteFile(filepath.Join(s.Home, ".config/zed/settings.json"), []byte("// migrated comments\n{\"theme\":\"custom\"}\n"), 0600)
	os.MkdirAll(filepath.Join(s.Home, ".config/fish/functions"), 0700)
	os.WriteFile(filepath.Join(s.Home, ".config/fish/functions/custom.fish"), []byte("function custom; echo retained; end\n"), 0600)
}
func (s *Sandbox) Calls() []domain.Command {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]domain.Command(nil), s.calls...)
}
func (s *Sandbox) ResetCalls() { s.mu.Lock(); defer s.mu.Unlock(); s.calls = nil }
func (s *Sandbox) ConfigPaths() []string {
	h := testutil.FreshHost(s.Home)
	return []string{h.Home + "/.config/fish/config.fish", h.Home + "/.config/starship.toml", h.Home + "/.config/zed/settings.json", h.Home + "/.gitconfig", h.LazyGitDir + "/config.yml"}
}
