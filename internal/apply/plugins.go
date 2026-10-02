package apply

import (
	"bytes"
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"errors"
	"fmt"
	"golden-gate-setup/internal/command"
	"golden-gate-setup/internal/domain"
	"golden-gate-setup/internal/files"
	"golden-gate-setup/internal/plan"
	"io"
	"path/filepath"
	"strings"
)

var ErrDeferred = errors.New("existing state requires individual manual review")

//go:embed assets/fisher.fish assets/FISHER-LICENSE.md
var fisherAssets embed.FS

const FisherRevision = "a04308be92daa6cfecdbb0ca58b1e8508664cff2"
const FisherSHA256 = "0fb6c81ae3003e95b5671766fa6c25c3597066e29965b7772f6c1b007387356d"
const FisherStateQuery = "for key in _fisher_list fisher_path _fisher_plugins; if set -qU $key; printf '%s\\n' $key; end; end"

func InstallPlugins(ctx context.Context, h domain.Host, selected []string, r command.Runner) error {
	if len(selected) == 0 {
		return nil
	}
	for _, p := range selected {
		if !plan.Has(append(append([]string{}, plan.StartingPlugins...), plan.ExtraPlugins...), p) {
			return fmt.Errorf("unknown plugin %s", p)
		}
	}
	if plan.PluginsSatisfied(h, selected) {
		return nil
	}
	manifest := filepath.Join(h.Home, ".config/fish/fish_plugins")
	if h.FishPluginConflict || h.FishLegacy || h.Files[manifest].Exists {
		return ErrDeferred
	}
	tool := h.Tools["fish"]
	if tool == "" {
		return errors.New("Fish unavailable")
	}
	var state bytes.Buffer
	if e := r.Run(ctx, domain.Command{Path: tool, Args: []string{"--no-config", "--command", FisherStateQuery}}, &state); e != nil {
		return e
	}
	if strings.TrimSpace(state.String()) != "" {
		return ErrDeferred
	}
	data, e := fisherAssets.ReadFile("assets/fisher.fish")
	if e != nil {
		return e
	}
	sum := sha256.Sum256(data)
	if hex.EncodeToString(sum[:]) != FisherSHA256 {
		return errors.New("bundled Fisher checksum mismatch")
	}
	path := filepath.Join(h.Home, ".config/fish/functions/fisher.fish")
	before := h.Files[path]
	if before.Exists {
		return ErrDeferred
	}
	m := files.Manager{Roots: []string{h.Home}}
	if _, e = m.ApplyContext(ctx, domain.FileChange{Path: path, Desired: data, Mode: 0644, Decision: domain.Create}, filepath.Join(h.Home, "Library/Application Support/Golden Gate Setup/configuration-backups")); e != nil {
		return e
	}
	args := []string{"--no-config", "--command", "source $argv[1]; and fisher install $argv[2..-1]", "--", path}
	args = append(args, selected...)
	return r.Run(ctx, domain.Command{Path: tool, Args: args, Env: []string{"XDG_CONFIG_HOME=" + filepath.Join(h.Home, ".config")}, Interactive: true}, io.Discard)
}
