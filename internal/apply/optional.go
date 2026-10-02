package apply

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"golden-gate-setup/internal/command"
	"golden-gate-setup/internal/domain"
	"golden-gate-setup/internal/files"
	"golden-gate-setup/internal/plan"
	"io"
	"path/filepath"
	"strings"
)

func OptionalHandlers(r command.Runner, m files.Manager) map[string]Handler {
	return map[string]Handler{
		"language-tools": {Apply: func(c context.Context, h domain.Host, p domain.Plan, s domain.Step, d string) (domain.StepResult, error) {
			return domain.StepResult{ID: s.ID}, InstallLanguageTools(c, h, p.Options.Languages, r)
		}, Verify: func(c context.Context, h domain.Host, p domain.Plan, s domain.Step, d string) (bool, error) {
			return languagesPresent(c, h, p.Options.Languages, r)
		}},
		"workspace-init": {Apply: func(c context.Context, h domain.Host, p domain.Plan, s domain.Step, d string) (domain.StepResult, error) {
			if s.Command == nil || s.File == nil {
				return domain.StepResult{}, fmt.Errorf("workspace initialization has no reviewed arguments")
			}
			state := h.Workspaces[s.Command.Dir]
			if state.Symlink || state.Exists && !state.Empty {
				return domain.StepResult{}, fmt.Errorf("workspace became occupied; review it again")
			}
			if e := m.EnsurePrivateDir(s.Command.Dir); e != nil {
				return domain.StepResult{}, e
			}
			cmd := *s.Command
			cmd.Path = tool(h, "go")
			cmd.Env = []string{"GOTOOLCHAIN=local", "GOWORK=off", "GOFLAGS="}
			if e := r.Run(c, cmd, nilWriter{}); e != nil {
				return domain.StepResult{}, e
			}
			cmd.Args = []string{"mod", "edit", "-go=1.26.0"}
			return domain.StepResult{ID: s.ID}, r.Run(c, cmd, nilWriter{})
		}, Verify: func(c context.Context, h domain.Host, p domain.Plan, s domain.Step, d string) (bool, error) {
			f, e := m.Inspect(s.Check.Target)
			return e == nil && f.Exists && f.SHA256 == s.Check.Expected, e
		}},
		"tap": {Apply: func(c context.Context, h domain.Host, p domain.Plan, s domain.Step, d string) (domain.StepResult, error) {
			return domain.StepResult{ID: s.ID}, r.Run(c, domain.Command{Path: h.BrewPath, Stream: true, Args: []string{"tap", s.Check.Target}, Env: []string{"HOMEBREW_NO_AUTO_UPDATE=1", "HOMEBREW_NO_ANALYTICS=1"}}, io.Discard)
		}, Verify: func(c context.Context, h domain.Host, p domain.Plan, s domain.Step, d string) (bool, error) {
			return plan.Has(h.Taps, s.Check.Target), nil
		}},
		"import": {Apply: func(c context.Context, h domain.Host, p domain.Plan, s domain.Step, d string) (domain.StepResult, error) {
			if s.Import == nil {
				return domain.StepResult{}, fmt.Errorf("import step has no reviewed folder")
			}
			var report func(string)
			if reporter, ok := r.(interface{ Report(string) }); ok {
				report = reporter.Report
			}
			done, e := m.Import(c, *s.Import, report)
			return domain.StepResult{ID: s.ID, Message: importMessage(done, *s.Import)}, e
		}, Verify: func(c context.Context, h domain.Host, p domain.Plan, s domain.Step, d string) (bool, error) {
			if s.Import == nil {
				return false, fmt.Errorf("import step has no reviewed folder")
			}
			return m.Imported(c, *s.Import)
		}},
		"chezmoi-init": {Apply: func(c context.Context, h domain.Host, p domain.Plan, s domain.Step, d string) (domain.StepResult, error) {
			if s.Command == nil {
				return domain.StepResult{}, fmt.Errorf("clone step has no reviewed repository")
			}
			cmd := *s.Command
			cmd.Path, cmd.Interactive = tool(h, "chezmoi"), true
			return domain.StepResult{ID: s.ID}, r.Run(c, cmd, io.Discard)
		}, Verify: func(c context.Context, h domain.Host, p domain.Plan, s domain.Step, d string) (bool, error) {
			return h.DotfilesState == "cloned" || h.DotfilesState == "waiting", nil
		}},
		"recovery": {Apply: func(c context.Context, h domain.Host, p domain.Plan, s domain.Step, d string) (domain.StepResult, error) {
			dir := recoveryDir(h, p)
			var inventory string
			var e error
			if p.Options.CaptureInventory {
				inventory, e = PrepareRecovery(c, h, dir, r)
			} else {
				e = m.EnsurePrivateDir(dir)
			}
			if e != nil {
				return domain.StepResult{}, e
			}
			notes := filepath.Join(dir, "Recovery-notes.txt")
			_, e = m.ApplyContext(c, domain.FileChange{Path: notes, Desired: recoveryNotes(p), Mode: 0600, Decision: domain.Create}, d)
			message := "Recovery records: " + dir
			if inventory != "" {
				message += " · inventory: " + inventory
			}
			return domain.StepResult{ID: s.ID, Message: message}, e
		}, Verify: func(c context.Context, h domain.Host, p domain.Plan, s domain.Step, d string) (bool, error) {
			dir := recoveryDir(h, p)
			notes, e := m.Inspect(filepath.Join(dir, "Recovery-notes.txt"))
			if e != nil {
				return false, e
			}
			sum := sha256.Sum256(recoveryNotes(p))
			if !notes.Exists || notes.SHA256 != hex.EncodeToString(sum[:]) {
				return false, nil
			}
			if p.Options.CaptureInventory {
				meta, e := m.Inspect(filepath.Join(dir, "installed-apps.json"))
				if e != nil {
					return false, e
				}
				sum := sha256.Sum256(recoveryMetadata(h))
				if !meta.Exists || !files.IsPrivate(meta.Mode) || meta.SHA256 != hex.EncodeToString(sum[:]) {
					return false, nil
				}
				return inventorySatisfied(m, h, filepath.Join(dir, "Homebrew-full.Brewfile"))
			}
			return true, nil
		}},
	}
}

type nilWriter struct{}

func (nilWriter) Write(p []byte) (int, error) { return len(p), nil }
func recoveryDir(h domain.Host, p domain.Plan) string {
	return filepath.Join(h.Home, "Golden Gate Recovery", p.Options.RecoveryDate+"-"+p.ID[:12])
}
func inventorySatisfied(m files.Manager, h domain.Host, path string) (bool, error) {
	f, e := m.Inspect(path)
	if e != nil || !f.Exists {
		return false, e
	}
	found := map[string]bool{}
	entries, _ := plan.ParseBrewfile(f.Contents)
	for _, key := range entries {
		found[key] = true
	}
	for key, v := range h.Packages {
		if strings.HasPrefix(key, "formula:") && v.OnRequest != nil && !*v.OnRequest {
			continue
		}
		if !found[key] {
			return false, nil
		}
	}
	return len(f.Contents) > 0, nil
}

// importMessage sums up what an import did.
func importMessage(done domain.ImportScan, job domain.ImportJob) string {
	count := func(n int, noun string) string {
		if n == 1 {
			return "1 " + noun
		}
		return fmt.Sprintf("%d %ss", n, noun)
	}
	parts := []string{fmt.Sprintf("Copied %s (%s)", count(done.Copy, "new file"), plan.SizeText(done.CopyBytes))}
	if done.Same > 0 {
		parts = append(parts, fmt.Sprintf("%d already here", done.Same))
	}
	if done.Differ > 0 {
		parts = append(parts, count(done.Differ, "differing file")+" saved in "+job.Conflicts)
	}
	if done.Aside > 0 {
		parts = append(parts, fmt.Sprintf("%d saved earlier", done.Aside))
	}
	if done.CloudOnly > 0 {
		parts = append(parts, count(done.CloudOnly, "iCloud-only file")+" not copied")
	}
	if done.Special > 0 {
		parts = append(parts, count(done.Special, "special file")+" skipped")
	}
	return strings.Join(parts, " · ")
}

// sourceContents reads a file the plan restores from the dotfiles source,
// refusing one that changed since it was reviewed.
func sourceContents(s domain.FileSource) ([]byte, error) {
	f, err := (files.Manager{Roots: []string{s.Root}}).Inspect(s.Path)
	if err != nil {
		return nil, err
	}
	if !f.Exists || f.SHA256 != s.SHA256 {
		return nil, fmt.Errorf("your dotfiles changed since preview: %s", s.Path)
	}
	return f.Contents, nil
}
