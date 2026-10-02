package plan

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"golden-gate-setup/internal/domain"
	"golden-gate-setup/internal/templates"
	"path/filepath"
	"strings"
	"time"
)

func Build(h domain.Host, o domain.Options) (domain.Plan, error) {
	hostJSON, err := json.Marshal(h)
	if err != nil {
		return domain.Plan{}, err
	}
	hostSum := sha256.Sum256(hostJSON)
	p := domain.Plan{InspectionID: hex.EncodeToString(hostSum[:]), SchemaVersion: 1, Supported: true, Options: o, Steps: []domain.Step{}, ManualTasks: []domain.ManualTask{}, Problems: []string{}}
	if h.OS == "windows" {
		return buildWindows(h, o, p)
	}
	for _, choice := range []struct {
		values, allowed []string
		name            string
	}{{o.Apps, Apps, "app"}, {o.Plugins, append(append([]string{}, StartingPlugins...), ExtraPlugins...), "plugin"}, {o.Languages, Languages, "language"}} {
		for _, value := range choice.values {
			if !Has(choice.allowed, value) {
				return p, fmt.Errorf("unknown %s selection: %s", choice.name, value)
			}
		}
	}
	if (o.CaptureInventory || o.PrepareRecovery) && o.RecoveryDate != "" {
		if _, e := time.Parse("2006-01-02", o.RecoveryDate); e != nil {
			return p, fmt.Errorf("invalid recovery date")
		}
	}
	if h.OS != "darwin" || h.Arch != "arm64" || strings.Split(h.Version, ".")[0] != "27" || h.Rosetta {
		p.Supported = false
		p.Problems = append(p.Problems, "This setup requires native Apple-silicon macOS 27.")
	}
	if h.BrewPath != "/opt/homebrew/bin/brew" || h.BrewPrefix != "/opt/homebrew" {
		p.Supported = false
		p.Problems = append(p.Problems, "Run the starter to prepare native Homebrew at /opt/homebrew before applying this plan.")
	}
	if !filepath.IsAbs(h.Home) {
		return p, fmt.Errorf("home directory must be absolute")
	}
	if h.ConfigHome != "" && h.ConfigHome != filepath.Join(h.Home, ".config") {
		p.Supported = false
		p.Problems = append(p.Problems, "Custom XDG_CONFIG_HOME is preserved. Merge these standard-location templates manually, or run from your standard configuration environment.")
	}
	add := func(s domain.Step) {
		if len(p.Steps) > 0 {
			s.DependsOn = []string{p.Steps[len(p.Steps)-1].ID}
		}
		p.Steps = append(p.Steps, s)
	}
	handled := map[string]bool{}
	pkg := func(kind, token, minimum string) {
		key := kind + ":" + token
		handled[key] = true
		if installed, ok := h.Packages[key]; ok {
			if minimum != "" && !AtLeastVersion(installed.Version, minimum) {
				p.ManualTasks = append(p.ManualTasks, domain.ManualTask{ID: "update:" + key, Title: "Review the required update for " + token, Instructions: "Selected features require " + token + " " + minimum + " or later. Update it deliberately in Cork or Homebrew, then inspect again.", Required: true})
				p.Supported = false
				p.Problems = append(p.Problems, token+" needs a reviewed minimum-version update.")
			}
			return
		}
		for existing := range h.Packages {
			if strings.HasPrefix(existing, kind+":") && strings.HasSuffix(existing, "/"+token) {
				p.Supported = false
				p.ManualTasks = append(p.ManualTasks, domain.ManualTask{ID: "custom-package:" + key, Title: "Preserve custom " + token, Instructions: "Reconcile " + existing + " with the selected standard package before setup. The custom registration will not be replaced.", Required: true})
				p.Problems = append(p.Problems, "A custom tap supplies "+token+"; review its compatibility first.")
				return
			}
		}
		if kind == "cask" {
			for _, app := range h.Apps {
				if strings.EqualFold(app.Name, AppNames[token]) {
					p.ManualTasks = append(p.ManualTasks, domain.ManualTask{ID: "vendor:" + token, Title: "Keep the existing " + app.Name + " installation", Instructions: "This app is already present outside shared Homebrew. Retain its distribution channel; review any adoption or replacement individually.", URL: "https://docs.brew.sh/Manpage#install-options-formulacask-", Required: false})
					return
				}
			}
		}
		q := domain.Package{Kind: kind, Token: token, MinVersion: minimum}
		add(domain.Step{ID: "package:" + key, Kind: "package", Label: "Install " + token, Package: &q, Check: domain.Check{Kind: "package", Target: key, Expected: minimum}})
	}
	for _, token := range Core {
		minimum := ""
		if token == "fish" && len(o.Plugins) > 0 {
			minimum = "4.0.0"
		}
		if token == "lazygit" {
			minimum = "0.65.1"
		}
		pkg("formula", token, minimum)
	}
	for _, token := range Apps {
		if Has(o.Apps, token) {
			pkg("cask", token, "")
		}
	}
	for _, lang := range Languages {
		if Has(o.Languages, lang) {
			for _, token := range LanguagePackages[lang] {
				minimum := ""
				if token == "go" {
					minimum = "1.26.0"
				}
				pkg("formula", token, minimum)
			}
		}
	}
	if err := addPrevious(&p, h, o, handled, pkg, add); err != nil {
		return p, err
	}
	cloning, err := addClone(&p, h, o, add)
	if err != nil {
		return p, err
	}
	if cloning {
		p.Later = "Restoring your dotfiles and the rest of setup are planned in a second review, once the repository is cloned."
		p.ID, err = Fingerprint(p)
		return p, err
	}
	if err := addImport(&p, h, o, add); err != nil {
		return p, err
	}
	restored := addDotfiles(&p, h, o, add)
	config := []configFile{{"fish/config.fish", filepath.Join(h.Home, ".config/fish/config.fish")}, {"starship.toml", filepath.Join(h.Home, ".config/starship.toml")}, {"zed/settings.example.json", filepath.Join(h.Home, ".config/zed/settings.json")}, {"lazygit/config.yml", filepath.Join(h.LazyGitDir, "config.yml")}}
	if o.PrepareRecovery && Has(o.Apps, "kopiaui") {
		config = append(config, configFile{"recovery/kopiaignore", filepath.Join(h.Home, ".kopiaignore")})
	}
	for _, c := range config {
		if restored[c.path] {
			continue
		}
		if err := addConfig(&p, h, o, c, add); err != nil {
			return p, err
		}
	}
	workspacePaths := map[string]bool{}
	for _, w := range o.Workspaces {
		if w.Path == "" || !Has(o.Languages, w.Language) {
			continue
		}
		if workspacePaths[w.Path] {
			return p, fmt.Errorf("choose a separate workspace path for each selected language")
		}
		workspacePaths[w.Path] = true
		changes, e := workspaceChanges(h, w, o.FileChoices)
		if e != nil {
			return p, e
		}
		for _, change := range changes {
			sum := sha256.Sum256(change.Desired)
			step := domain.Step{ID: "file:" + change.Path, Label: "Configure " + change.Path, Kind: "file", File: &change, Check: domain.Check{Kind: "file", Target: change.Path, Expected: hex.EncodeToString(sum[:])}}
			if w.Language == "go" && filepath.Base(change.Path) == "go.mod" {
				step.Kind = "workspace-init"
				step.Command = &domain.Command{Dir: w.Path, Args: []string{"mod", "init", w.Module}}
			}
			add(step)
		}
		if h.Workspaces[w.Path].Exists && !h.Workspaces[w.Path].Empty {
			p.ManualTasks = append(p.ManualTasks, domain.ManualTask{ID: "existing-workspace:" + w.Path, Title: "Keep the existing project at " + w.Path, Instructions: "Source and manifests are preserved. Review any preserved .zed files separately. If a previous starter run stopped here, finish missing source files after inspecting the project.", Required: false})
		}
	}
	if o.ConfigureGit {
		if restored[filepath.Join(h.Home, ".gitconfig")] || restored[filepath.Join(h.Home, ".config/git/config")] {
			p.ManualTasks = append(p.ManualTasks, domain.ManualTask{ID: "git-dotfiles", Title: "Set Git's editor and viewer in your dotfiles", Instructions: "Your dotfiles repository manages Git's configuration, so setup leaves it as restored. To use Zed and delta, add these settings there: core.editor = zed --wait, core.pager = delta, interactive.diffFilter = delta --color-only, delta.navigate = true.", Required: false})
		} else {
			add(gitStep(h))
		}
	}
	if len(o.Plugins) > 0 {
		if PluginsSatisfied(h, o.Plugins) {
		} else if h.FishPluginConflict || h.FishLegacy || h.Files[filepath.Join(h.Home, ".config/fish/fish_plugins")].Exists || restored[filepath.Join(h.Home, ".config/fish/fish_plugins")] {
			p.ManualTasks = append(p.ManualTasks, domain.ManualTask{ID: "plugins-conflict", Title: "Preserve existing Fish plugin files", Instructions: "Review the selected plugins and existing functions before adding them. No existing plugin or function is removed.", Required: false})
		} else {
			add(domain.Step{ID: "plugins", Label: "Install selected Fish plugins", Kind: "plugins", Check: domain.Check{Kind: "plugins", Expected: strings.Join(o.Plugins, "\n")}})
		}
	}
	if len(o.Languages) > 0 {
		add(languageStep(o))
	}
	if added := chezmoiAdds(h, o); o.AdoptChezmoi || len(added) > 0 {
		addChezmoi(&p, h, added, add)
	}
	if o.CaptureInventory || o.PrepareRecovery {
		add(domain.Step{ID: "recovery", Label: "Prepare dated recovery records", Kind: "recovery", Check: domain.Check{Kind: "recovery"}})
	}
	p.ManualTasks = append(p.ManualTasks, FinishTasks(o)...)
	p.ID, err = Fingerprint(p)
	if err != nil {
		return p, err
	}
	return p, nil
}

// addPrevious plans the entries chosen from the previous Mac's app list:
// taps first, then missing formulae and casks. Installed entries are left
// as they are, and entries the catalog already handled are not repeated.
func addPrevious(p *domain.Plan, h domain.Host, o domain.Options, handled map[string]bool, pkg func(kind, token, minimum string), add func(domain.Step)) error {
	for _, key := range o.PreviousPackages {
		if !Has(h.PreviousEntries, key) {
			return fmt.Errorf("%s is not in the previous app list", key)
		}
	}
	planned := func(id string) bool {
		for _, s := range p.Steps {
			if s.ID == id {
				return true
			}
		}
		return false
	}
	for _, key := range h.PreviousEntries {
		if !Has(o.PreviousPackages, key) || handled[key] {
			continue
		}
		kind, name, _ := strings.Cut(key, ":")
		if kind == "tap" {
			if !Has(h.Taps, name) && !planned("tap:"+name) {
				add(domain.Step{ID: "tap:" + name, Label: "Tap " + name, Kind: "tap", Check: domain.Check{Kind: "tap", Target: name}})
			}
			continue
		}
		pkg(kind, name, "")
	}
	if o.PreviousBrewfile != "" && len(h.PreviousOther) > 0 {
		p.ManualTasks = append(p.ManualTasks, domain.ManualTask{ID: "previous-other", Title: "Reinstall the rest of your previous app list", Instructions: "These entries are not installed automatically; install the ones you still want yourself (App Store apps from the App Store): " + strings.Join(h.PreviousOther, "; "), Required: false})
	}
	return nil
}

type configFile struct{ asset, path string }

// addConfig plans one supplied configuration file: create it, replace it
// after an approved review, or preserve it with a manual task.
func addConfig(p *domain.Plan, h domain.Host, o domain.Options, c configFile, add func(domain.Step)) error {
	data, err := templates.Load(c.asset)
	if err != nil {
		return err
	}
	before := h.Files[c.path]
	decision := domain.Create
	if before.Exists {
		if !before.Symlink && bytes.Equal(before.Contents, data) {
			return nil
		}
		decision = domain.Preserve
		if value, ok := o.FileChoices[c.path]; ok {
			decision = value
		}
		if before.Symlink || !before.Mode.IsRegular() && before.Mode != 0 || decision != domain.Replace {
			p.ManualTasks = append(p.ManualTasks, domain.ManualTask{ID: "file:" + c.path, Title: "Preserve existing " + filepath.Base(c.path), Instructions: "Review and merge the supplied configuration with " + c.path + ". Comments and unrelated settings remain unchanged.", Required: false})
			return nil
		}
	}
	if !within(h.Home, c.path) {
		p.ManualTasks = append(p.ManualTasks, domain.ManualTask{ID: "file:" + c.path, Title: "Review custom configuration location", Instructions: c.path + " is outside the home directory and is preserved.", Required: false})
		return nil
	}
	hash := sha256.Sum256(data)
	change := domain.FileChange{Path: c.path, BeforeSHA256: before.SHA256, BeforeExists: before.Exists, Mode: 0644, Desired: data, Decision: decision}
	add(domain.Step{ID: "file:" + c.path, Label: "Configure " + filepath.Base(c.path), Kind: "file", File: &change, Check: domain.Check{Kind: "file", Target: c.path, Expected: hex.EncodeToString(hash[:])}})
	return nil
}
func gitStep(h domain.Host) domain.Step {
	return domain.Step{ID: "git", Label: "Set Git’s Zed editor and delta viewer", Kind: "git", BeforeFiles: []domain.FileState{fileState(h, filepath.Join(h.Home, ".gitconfig")), fileState(h, filepath.Join(h.Home, ".config/git/config"))}, Check: domain.Check{Kind: "git"}}
}
func languageStep(o domain.Options) domain.Step {
	label := "Install selected language servers"
	if Has(o.Languages, "go") {
		label += " (gopls 0.23.0)"
	}
	if Has(o.Languages, "nim") {
		label += " (nimlangserver 1.14.0; Nimble may request Nim 2.0.8)"
	}
	return domain.Step{ID: "language-tools", Label: label, Kind: "language-tools", Check: domain.Check{Kind: "language-tools"}}
}

// chezmoiAdds keeps the chosen imported dotfiles that inspection offered:
// a file that now holds a key or a secret is never added.
func chezmoiAdds(h domain.Host, o domain.Options) []string {
	var added []string
	for _, path := range h.ChezmoiCandidates {
		if Has(o.ChezmoiAdd, path) {
			added = append(added, path)
		}
	}
	return added
}

// addChezmoi plans adopting setup's chosen configuration, and the imported
// dotfiles chosen for chezmoi, into chezmoi's source.
func addChezmoi(p *domain.Plan, h domain.Host, added []string, add func(domain.Step)) {
	if h.ChezmoiIncoming {
		p.ManualTasks = append(p.ManualTasks, domain.ManualTask{ID: "chezmoi-imported", Title: "Review your imported chezmoi source", Instructions: "The import brings your previous chezmoi source, so nothing is added to it now. Review it with chezmoi status, then run setup again to adopt files.", Required: false})
	} else if h.ChezmoiDir != "" && !within(h.Home, h.ChezmoiDir) {
		p.ManualTasks = append(p.ManualTasks, domain.ManualTask{ID: "chezmoi-location", Title: "Adopt files in your custom chezmoi source", Instructions: "The source outside your home is preserved; review and add selected files manually.", Required: true})
	} else if h.ChezmoiDirty {
		p.ManualTasks = append(p.ManualTasks, domain.ManualTask{ID: "chezmoi-conflict", Title: "Preserve pending chezmoi source edits", Instructions: "Reconcile source edits before adopting the new live settings.", Required: false})
	} else {
		label := "Adopt selected configuration into chezmoi"
		if len(added) > 0 {
			label += fmt.Sprintf(", with %d imported dotfiles", len(added))
			p.ManualTasks = append(p.ManualTasks, domain.ManualTask{ID: "dotfiles-commit", Title: "Commit and push the dotfiles you added", Instructions: "Setup adds " + homeList(h.Home, added) + " to chezmoi's source without committing. Review them with chezmoi git status, then commit and push them yourself; a public repository makes them public.", Required: false})
		}
		add(domain.Step{ID: "chezmoi", Label: label, Kind: "chezmoi", Check: domain.Check{Kind: "chezmoi", Expected: strings.Join(added, "\n")}})
	}
}
func within(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func Fingerprint(p domain.Plan) (string, error) {
	p.ID = ""
	p.Accepted = false
	raw, e := json.Marshal(p)
	if e != nil {
		return "", e
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

func PluginsSatisfied(h domain.Host, selected []string) bool {
	for _, v := range selected {
		found := false
		for _, installed := range h.FishInstalledPlugins {
			if strings.EqualFold(installed, v) {
				found = true
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func fileState(h domain.Host, path string) domain.FileState {
	f := h.Files[path]
	f.Path = path
	return f
}
