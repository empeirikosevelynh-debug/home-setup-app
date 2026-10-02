package apply

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"golden-gate-setup/internal/command"
	"golden-gate-setup/internal/domain"
	"golden-gate-setup/internal/plan"
	"io"
	"os"
	"path/filepath"
)

func tool(h domain.Host, name string) string {
	if h.Tools[name] != "" {
		return h.Tools[name]
	}
	return filepath.Join("/opt/homebrew/bin", name)
}
func goBin(ctx context.Context, h domain.Host, r command.Runner) (string, error) {
	var b bytes.Buffer
	if e := r.Run(ctx, domain.Command{Path: tool(h, "go"), Args: []string{"env", "-json", "GOBIN", "GOPATH"}, Env: []string{"GOTOOLCHAIN=local"}}, &b); e != nil {
		return "", e
	}
	var data struct{ GOBIN, GOPATH string }
	if e := json.Unmarshal(b.Bytes(), &data); e != nil {
		return "", e
	}
	path := data.GOBIN
	if path == "" {
		paths := filepath.SplitList(data.GOPATH)
		if len(paths) == 0 || paths[0] == "" {
			return "", fmt.Errorf("Go returned no binary directory")
		}
		path = filepath.Join(paths[0], "bin")
	}
	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("Go binary directory is not absolute")
	}
	return path, nil
}
func nimBin(h domain.Host) string {
	base := os.Getenv("NIMBLE_DIR")
	if base == "" {
		base = filepath.Join(h.Home, ".nimble")
	}
	return filepath.Join(base, "bin")
}
func executable(path string) bool {
	st, e := os.Stat(path)
	return e == nil && st.Mode().IsRegular() && st.Mode().Perm()&0111 != 0
}
func languagesPresent(ctx context.Context, h domain.Host, selected []string, r command.Runner) (bool, error) {
	for _, lang := range selected {
		switch lang {
		case "go":
			if h.Tools["gopls"] == "" {
				bin, e := goBin(ctx, h, r)
				if e != nil {
					return false, e
				}
				if !executable(filepath.Join(bin, "gopls")) {
					return false, nil
				}
			}
		case "crystal":
			for _, n := range []string{"crystal", "crystalline", "ameba"} {
				if h.Tools[n] == "" {
					return false, nil
				}
			}
		case "nim":
			if h.Tools["nimpretty"] == "" {
				return false, nil
			}
			if h.Tools["nimlangserver"] == "" && !executable(filepath.Join(nimBin(h), "nimlangserver")) {
				return false, nil
			}
		}
	}
	return true, nil
}
func InstallLanguageTools(ctx context.Context, h domain.Host, selected []string, r command.Runner) error {
	for _, lang := range selected {
		if !plan.Has(plan.Languages, lang) {
			return fmt.Errorf("unknown language %s", lang)
		}
		switch lang {
		case "go":
			if h.Tools["gopls"] != "" {
				continue
			}
			bin, e := goBin(ctx, h, r)
			if e != nil {
				return e
			}
			if executable(filepath.Join(bin, "gopls")) {
				continue
			}
			if e = r.Run(ctx, domain.Command{Path: tool(h, "go"), Stream: true, Args: []string{"install", "golang.org/x/tools/gopls@v0.23.0"}, Env: []string{"GOTOOLCHAIN=local", "GOWORK=off", "GOFLAGS="}}, io.Discard); e != nil {
				return e
			}
		case "nim":
			if h.Tools["nimlangserver"] != "" || executable(filepath.Join(nimBin(h), "nimlangserver")) {
				continue
			}
			if e := r.Run(ctx, domain.Command{Path: tool(h, "nimble"), Dir: h.Home, Args: []string{"install", "-g", "nimlangserver@1.14.0"}, Interactive: true}, io.Discard); e != nil {
				return e
			}
		}
	}
	return nil
}
