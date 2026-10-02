package scripts

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

type fixture struct {
	root, bin, log string
	env            []string
}

func bootFixture(t *testing.T) fixture {
	t.Helper()
	f := fixture{root: filepath.Join(t.TempDir(), "project space 日本語"), bin: t.TempDir(), log: filepath.Join(t.TempDir(), "calls")}
	os.MkdirAll(filepath.Join(f.root, "scripts"), 0700)
	data, e := os.ReadFile("bootstrap.zsh")
	if e != nil {
		t.Fatal(e)
	}
	os.WriteFile(filepath.Join(f.root, "scripts/bootstrap.zsh"), data, 0600)
	clt := t.TempDir()
	tools := map[string]string{
		"uname": "printf 'arm64\\n'", "sw_vers": "printf '27.0.1\\n'", "sysctl": "printf '0\\n'", "xcode-select": `if [ "$1" = -p ]; then [ "$GG_CLT" = missing ] && exit 1; printf '%s\n' "$GG_CLT"; else printf 'clt-install\n' >> "$GG_LOG"; fi`, "clang": "printf 'clang version fake\\n'", "git": "printf 'git version 2.50\\n'",
		"brew": `if [ "$1" = --prefix ]; then printf '%s\n' "$GG_PREFIX"; elif [ "$1" = list ]; then exit 1; else printf 'brew %s\n' "$*" >> "$GG_LOG"; fi`,
		"go":   `if [ "$1" = version ]; then printf 'go version go%s darwin/arm64\n' "$GG_GO"; else printf 'go %s\n' "$*" >> "$GG_LOG"; mkdir -p bin; printf '#!/bin/sh\nprintf "wizard:%%s\\n" "$@" >> "$GG_LOG"\n' > bin/golden-setup; chmod +x bin/golden-setup; fi`,
		"curl": `printf 'curl\n' >> "$GG_LOG"; exit 1`}
	for name, body := range tools {
		os.WriteFile(filepath.Join(f.bin, name), []byte("#!/bin/sh\n"+body+"\n"), 0700)
	}
	f.env = append(os.Environ(), "PATH="+f.bin+":/usr/bin:/bin:/usr/sbin:/sbin", "GG_LOG="+f.log, "GG_PREFIX=/opt/homebrew", "GG_GO=1.27.1", "GG_CLT="+clt)
	return f
}
func (f fixture) run(input string, args ...string) (string, error) {
	cmd := exec.Command("/bin/zsh", append([]string{filepath.Join(f.root, "scripts/bootstrap.zsh")}, args...)...)
	cmd.Env = f.env
	cmd.Stdin = strings.NewReader(input)
	b, e := cmd.CombinedOutput()
	return string(b), e
}
func (f fixture) calls() string { b, _ := os.ReadFile(f.log); return string(b) }
func TestBootstrapPreviewNoMutation(t *testing.T) {
	f := bootFixture(t)
	f.env = append(f.env, "GG_CLT=missing", "GG_GO=1.25.0")
	out, e := f.run("", "--plan")
	if e != nil || f.calls() != "" {
		t.Fatal(out, e, f.calls())
	}
	if _, e := os.Stat(filepath.Join(f.root, "bin")); !os.IsNotExist(e) {
		t.Fatal("preview wrote output")
	}
}
func TestBootstrapDirectoryWithSpaces(t *testing.T) {
	f := bootFixture(t)
	out, e := f.run("", "--build-only")
	if e != nil {
		t.Fatal(out, e)
	}
	if _, e := os.Stat(filepath.Join(f.root, "bin/golden-setup")); e != nil {
		t.Fatal(e)
	}
}
func TestBootstrapMissingCLTPauses(t *testing.T) {
	f := bootFixture(t)
	f.env = append(f.env, "GG_CLT=missing")
	out, e := f.run("yes\n", "--build-only")
	if e == nil || !strings.Contains(f.calls(), "clt-install") || strings.Contains(f.calls(), "go build") || !strings.Contains(strings.ToLower(out), "run again") {
		t.Fatal(out, e, f.calls())
	}
}
func TestBootstrapRequiresNativePrefix(t *testing.T) {
	f := bootFixture(t)
	f.env = append(f.env, "GG_PREFIX=/usr/local")
	out, e := f.run("yes\n", "--build-only")
	if e == nil || f.calls() != "" {
		t.Fatal(out, e, f.calls())
	}
}
func TestBootstrapGoFloor(t *testing.T) {
	f := bootFixture(t)
	f.env = append(f.env, "GG_GO=1.25.0")
	out, e := f.run("no\n", "--build-only")
	if e == nil || strings.Contains(f.calls(), "go build") {
		t.Fatal(out, e, f.calls())
	}
}
func TestBootstrapPassesArguments(t *testing.T) {
	f := bootFixture(t)
	out, e := f.run("", "--", "--accessible", "argument with spaces; $(literal)")
	if e != nil {
		t.Fatal(out, e)
	}
	if !strings.Contains(f.calls(), "wizard:--accessible\nwizard:argument with spaces; $(literal)\n") {
		t.Fatal("arguments changed", f.calls())
	}
}
