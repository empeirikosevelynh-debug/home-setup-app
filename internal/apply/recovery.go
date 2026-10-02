package apply

import (
	"context"
	"encoding/json"
	"fmt"
	"golden-gate-setup/internal/command"
	"golden-gate-setup/internal/domain"
	"golden-gate-setup/internal/files"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func PrepareRecovery(ctx context.Context, h domain.Host, dir string, r command.Runner) (string, error) {
	m := files.Manager{Roots: []string{h.Home}}
	if _, e := os.Lstat(dir); e == nil {
		return "", fmt.Errorf("recovery destination already exists: %s; inspect and archive partial records before retrying", dir)
	} else if !os.IsNotExist(e) {
		return "", e
	}
	if e := m.EnsurePrivateDir(dir); e != nil {
		return "", e
	}
	path := filepath.Join(dir, "Homebrew-full.Brewfile")
	if e := ctx.Err(); e != nil {
		return "", e
	}
	if e := r.Run(ctx, domain.Command{Path: h.BrewPath, Args: []string{"bundle", "dump", "--file=" + path}, UnsetEnv: []string{"HOMEBREW_BUNDLE_*"}, Env: []string{"HOMEBREW_NO_AUTO_UPDATE=1", "HOMEBREW_NO_ANALYTICS=1"}}, io.Discard); e != nil {
		return "", e
	}
	state, e := m.Inspect(path)
	if e != nil {
		return "", e
	}
	if !state.Exists {
		return "", fmt.Errorf("inventory was not created")
	}
	if e = m.MakePrivateFile(path); e != nil {
		return "", e
	}
	meta := recoveryMetadata(h)
	if _, e = m.ApplyContext(ctx, domain.FileChange{Path: filepath.Join(dir, "installed-apps.json"), Desired: meta, Mode: 0600, Decision: domain.Create}, dir); e != nil {
		return "", e
	}
	return path, nil
}
func recoveryNotes(p domain.Plan) []byte {
	return []byte("Golden Gate recovery preparation\n\nThese records reinstall packages; they do not back up application data or the home folder.\n\nKeep Applite's native App Migration export here as a separate, clearly dated file. It omits Applite itself. Use the same /opt/homebrew/bin/brew in Cork and Applite; refresh both and avoid overlapping operations. Restore taps before imports and select only missing apps.\n\nReview the full captured Homebrew inventory against ~/.Brewfile. If you want to update that curated file, review the diff and save a backup first, then add that chosen file to chezmoi. Never import the commented starter Brewfile in Applite.\n\n" + manualRecoveryText(p.ManualTasks))
}

func manualRecoveryText(tasks []domain.ManualTask) string {
	var b strings.Builder
	for _, t := range tasks {
		fmt.Fprintf(&b, "%s\n%s\n%s\n\n", t.Title, t.Instructions, t.URL)
	}
	return b.String()
}

func recoveryMetadata(h domain.Host) []byte {
	data, _ := json.MarshalIndent(struct {
		Packages     map[string]domain.InstalledPackage
		Applications []domain.AppBundle
	}{h.Packages, h.Apps}, "", "  ")
	return data
}
