package inspect

import (
	"bytes"
	"context"
	"fmt"
	"golden-gate-setup/internal/command"
	"golden-gate-setup/internal/domain"
	"golden-gate-setup/internal/files"
	"golden-gate-setup/internal/plan"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

type Inspector struct {
	Home     string
	Runner   command.Runner
	Platform func(context.Context) (domain.Host, error)
	Lookup   func(string) (string, error)
	AppsDirs []string
	Getenv   func(string) string
}

func (i Inspector) getenv(name string) string {
	if i.Getenv != nil {
		return i.Getenv(name)
	}
	return os.Getenv(name)
}

func (i Inspector) capture(ctx context.Context, path string, args ...string) (string, error) {
	out, err := i.captureRaw(ctx, path, args...)
	return strings.TrimSpace(string(out)), err
}

// captureRaw runs a read-only command and returns its output as it is.
func (i Inspector) captureRaw(ctx context.Context, path string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	runner := i.Runner
	if runner == nil {
		runner = command.ProcessRunner{}
	}
	var buf boundedBuffer
	err := runner.Run(ctx, domain.Command{Path: path, Args: args, Env: []string{"HOMEBREW_NO_AUTO_UPDATE=1", "HOMEBREW_NO_ANALYTICS=1", "GIT_OPTIONAL_LOCKS=0"}}, &buf)
	return buf.Bytes(), err
}

type boundedBuffer struct{ bytes.Buffer }

func (b *boundedBuffer) Write(p []byte) (int, error) {
	if b.Len()+len(p) > 16*1024*1024 {
		return 0, fmt.Errorf("inspection output exceeds limit")
	}
	return b.Buffer.Write(p)
}

var _ io.Writer = (*boundedBuffer)(nil)

func (i Inspector) lookup(name string) (string, error) {
	if i.Lookup != nil {
		return i.Lookup(name)
	}
	p, e := exec.LookPath(name)
	if e == nil || filepath.IsAbs(name) {
		return p, e
	}
	for _, dir := range fallbackDirs() {
		if p, err := exec.LookPath(filepath.Join(dir, name)); err == nil {
			return p, nil
		}
	}
	return "", e
}
func (i Inspector) platform(ctx context.Context) (domain.Host, error) {
	if i.Platform != nil {
		return i.Platform(ctx)
	}
	h := domain.Host{OS: runtime.GOOS, Arch: runtime.GOARCH}
	if h.OS == "windows" {
		readWindows(&h)
		return h, nil
	}
	if h.OS != "darwin" {
		return h, nil
	}
	v, e := i.capture(ctx, "/usr/bin/sw_vers", "-productVersion")
	if e != nil {
		return h, e
	}
	h.Version = v
	translated, e := i.capture(ctx, "/usr/sbin/sysctl", "-in", "sysctl.proc_translated")
	if e != nil && command.ExitCode(e) != 1 {
		return h, e
	}
	h.Rosetta = translated == "1" || h.Arch != "arm64"
	return h, nil
}
func (i Inspector) Read(ctx context.Context, o domain.Options) (domain.Host, error) {
	h, err := i.platform(ctx)
	if err != nil {
		return h, err
	}
	h.Home = i.Home
	if h.Home == "" {
		h.Home, err = os.UserHomeDir()
		if err != nil {
			return h, err
		}
	}
	if !filepath.IsAbs(h.Home) {
		return h, fmt.Errorf("home directory must be absolute")
	}
	h.Home = filepath.Clean(h.Home)
	h.ConfigHome = i.getenv("XDG_CONFIG_HOME")
	h.BrewPath, h.BrewPrefix = "", ""
	h.Packages = map[string]domain.InstalledPackage{}
	h.Files = map[string]domain.FileState{}
	h.Workspaces = map[string]domain.WorkspaceState{}
	h.Tools = map[string]string{}
	h.LazyGitDir = filepath.Join(h.Home, "Library/Application Support/lazygit")
	if h.OS == "windows" {
		h.AppData = i.getenv("APPDATA")
		if !filepath.IsAbs(h.AppData) {
			h.AppData = filepath.Join(h.Home, "AppData", "Roaming")
		}
		h.LazyGitDir = filepath.Join(h.AppData, "lazygit")
		if err = i.readChocolatey(ctx, &h, i.getenv("ChocolateyInstall")); err != nil {
			return h, err
		}
	} else if p, e := i.lookup("/opt/homebrew/bin/brew"); e == nil {
		h.BrewPath = p
		prefix, e := i.capture(ctx, p, "--prefix")
		if e != nil {
			return h, e
		}
		h.BrewPrefix = prefix
		raw, e := i.capture(ctx, p, "info", "--json=v2", "--installed")
		if e != nil {
			return h, e
		}
		h.Packages, e = readPackages([]byte(raw))
		if e != nil {
			return h, e
		}
	}
	if h.OS != "windows" {
		if err = i.readPrevious(ctx, &h, o); err != nil {
			return h, err
		}
	}
	for _, name := range []string{"fish", "starship", "zoxide", "chezmoi", "gh", "fzf", "fd", "bat", "eza", "rg", "delta", "lazygit", "git", "zed", "go", "gopls", "golangci-lint", "dlv", "crystal", "crystalline", "ameba", "nim", "nimble", "nimpretty", "nimlangserver"} {
		if p, e := i.lookup(name); e == nil {
			h.Tools[name] = p
		}
	}
	if p := h.Tools["lazygit"]; p != "" {
		dir, e := i.capture(ctx, p, "--print-config-dir")
		if e != nil {
			return h, e
		}
		if !filepath.IsAbs(dir) {
			return h, fmt.Errorf("lazygit returned an invalid configuration path")
		}
		h.LazyGitDir = filepath.Clean(dir)
	}
	if h.OS == "windows" {
		h.Apps = windowsApps(uninstallEntries(), h.Packages)
	} else if h.Apps, err = i.readApps(h); err != nil {
		return h, err
	}
	for _, w := range o.Workspaces {
		if w.Path == "" || !plan.Has(o.Languages, w.Language) {
			continue
		}
		if !filepath.IsAbs(w.Path) || filepath.Clean(w.Path) != w.Path {
			return h, fmt.Errorf("workspace path must be clean and absolute")
		}
		if _, e := (files.Manager{Roots: []string{w.Path}}).Inspect(filepath.Join(w.Path, ".root-check")); e != nil {
			return h, e
		}
		state := domain.WorkspaceState{}
		st, e := os.Lstat(w.Path)
		if e == nil {
			state.Exists = true
			state.Directory = st.IsDir()
			state.Symlink = st.Mode()&os.ModeSymlink != 0
			if state.Directory && !state.Symlink {
				entries, e := os.ReadDir(w.Path)
				if e != nil {
					return h, e
				}
				state.Empty = len(entries) == 0
			}
		} else if !os.IsNotExist(e) {
			return h, e
		}
		h.Workspaces[w.Path] = state
	}
	for _, path := range Paths(h, o) {
		root := h.Home
		for _, w := range o.Workspaces {
			if w.Path != "" && plan.Has(o.Languages, w.Language) {
				rel, e := filepath.Rel(w.Path, path)
				if e == nil && rel != ".." && !strings.HasPrefix(rel, "../") {
					root = w.Path
				}
			}
		}
		state, e := ReadFileState(root, path)
		if e != nil {
			return h, e
		}
		h.Files[path] = state
	}
	if h.OS == "windows" {
		return i.finish(ctx, h, o)
	}
	for _, sub := range []string{"functions", "conf.d", "completions"} {
		entries, e := os.ReadDir(filepath.Join(h.Home, ".config/fish", sub))
		if e != nil && !os.IsNotExist(e) {
			return h, e
		}
		if len(entries) > 0 {
			h.FishPluginConflict = true
		}
	}
	universal, err := FishUniversalVariables(h)
	if err != nil {
		return h, err
	}
	h.FishInstalledPlugins = universal["_fisher_plugins"]
	for _, key := range []string{"_fisher_list", "fisher_path"} {
		if _, ok := universal[key]; ok {
			h.FishLegacy = true
		}
	}
	return i.finish(ctx, h, o)
}

// finish reads state shared by both platforms after the platform's own,
// then what macOS brings over: the dotfiles repository before the previous
// home folder, whose import leaves the repository's files alone.
func (i Inspector) finish(ctx context.Context, h domain.Host, o domain.Options) (domain.Host, error) {
	if p := h.Tools["chezmoi"]; p != "" {
		if err := i.readChezmoi(ctx, &h, p); err != nil {
			return h, err
		}
	}
	if h.OS == "windows" {
		return h, nil
	}
	if err := i.readDotfiles(ctx, &h, o); err != nil {
		return h, err
	}
	return h, readImport(&h, o)
}
func (i Inspector) readApps(h domain.Host) ([]domain.AppBundle, error) {
	dirs := i.AppsDirs
	if dirs == nil {
		dirs = []string{"/Applications", filepath.Join(h.Home, "Applications")}
	}
	apps := []domain.AppBundle{}
	for _, dir := range dirs {
		entries, e := os.ReadDir(dir)
		if os.IsNotExist(e) {
			continue
		}
		if e != nil {
			return nil, e
		}
		for _, entry := range entries {
			if !strings.HasSuffix(entry.Name(), ".app") {
				continue
			}
			a := domain.AppBundle{Name: strings.TrimSuffix(entry.Name(), ".app"), Path: filepath.Join(dir, entry.Name())}
			_, e = os.Stat(filepath.Join(a.Path, "Contents/_MASReceipt/receipt"))
			a.StoreReceipt = e == nil
			for token, name := range plan.AppNames {
				if strings.EqualFold(a.Name, name) {
					_, a.Registered = h.Packages["cask:"+token]
				}
			}
			apps = append(apps, a)
		}
	}
	return apps, nil
}
