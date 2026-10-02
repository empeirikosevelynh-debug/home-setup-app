package plan_test

import (
	"golden-gate-setup/internal/domain"
	"golden-gate-setup/internal/plan"
	"golden-gate-setup/internal/testutil"
	"path/filepath"
	"strings"
	"testing"
)

const oldHome = "/Volumes/Backup/Users/you"

func importHost(t *testing.T) (domain.Host, domain.Options) {
	h := testutil.FreshHost(t.TempDir())
	h.FreeBytes = 100e9
	h.Import = []domain.ImportScan{
		{Folder: "Documents", Copy: 120, CopyBytes: 3e9, Differ: 2, DifferBytes: 1e6, Same: 4, Digest: "documents"},
		{Folder: "Pictures", Same: 50, CloudOnly: 3, Special: 1, Digest: "pictures"},
		{Folder: ".ssh", Copy: 3, CopyBytes: 4000, Digest: "ssh"},
	}
	o := plan.DefaultOptions()
	o.RecoveryDate = "2026-10-02"
	o.ImportFrom, o.ImportFolders = oldHome, []string{"Documents", "Pictures"}
	return h, o
}

func steps(p domain.Plan, kind string) []domain.Step {
	var r []domain.Step
	for _, s := range p.Steps {
		if s.Kind == kind {
			r = append(r, s)
		}
	}
	return r
}

func task(p domain.Plan, id string) *domain.ManualTask {
	for i := range p.ManualTasks {
		if p.ManualTasks[i].ID == id {
			return &p.ManualTasks[i]
		}
	}
	return nil
}

func TestImportPlanned(t *testing.T) {
	h, o := importHost(t)
	h.ConflictDates = []string{"2026-09-30", "2026-10-02"}
	p, err := plan.Build(h, o)
	if err != nil || !p.Supported || p.Later != "" {
		t.Fatal(err, p.Problems, p.Later)
	}
	imports := steps(p, "import")
	if len(imports) != 1 || imports[0].ID != "import:Documents" {
		t.Fatalf("only folders with work are imported: %+v", imports)
	}
	s := imports[0]
	if s.Label != "Import Documents from your previous Mac: 120 new files (3.0 GB); 2 existing files to compare" || s.Check.Expected != "documents" {
		t.Fatal(s.Label, s.Check)
	}
	job := *s.Import
	conflicts := filepath.Join(h.Home, "Imported conflicts")
	if job.Source != oldHome+"/Documents" || job.Destination != filepath.Join(h.Home, "Documents") || job.Conflicts != filepath.Join(conflicts, "2026-10-02/Documents") || len(job.Saved) != 1 || job.Saved[0] != filepath.Join(conflicts, "2026-09-30/Documents") {
		t.Fatalf("%+v", job)
	}
	// Packages come first; the rest of setup follows the import.
	if p.Steps[0].Kind != "package" || len(steps(p, "file")) == 0 {
		t.Fatal("import not ordered between packages and configuration")
	}
	for _, id := range []string{"import-conflicts", "import-icloud", "import-special"} {
		if task(p, id) == nil {
			t.Fatal("missing follow-up", id)
		}
	}
}

func TestImportPlansAgainstIncomingFiles(t *testing.T) {
	h, o := importHost(t)
	o.ImportFolders = []string{"Documents", ".ssh"}
	fish, gitconfig := filepath.Join(h.Home, ".config/fish/config.fish"), filepath.Join(h.Home, ".gitconfig")
	h.Files[fish] = domain.FileState{Path: fish, Exists: true, Imported: true, Mode: 0644, SHA256: "old", Contents: []byte("set -g old 1\n")}
	h.Files[gitconfig] = domain.FileState{Path: gitconfig, Exists: true, Imported: true, Mode: 0644, SHA256: "git"}
	p, err := plan.Build(h, o)
	if err != nil || p.Later != "" || len(steps(p, "import")) != 2 {
		t.Fatal(err, p.Later)
	}
	if task(p, "file:"+fish) == nil {
		t.Fatal("the imported Fish configuration is not kept")
	}
	for _, s := range p.Steps {
		if s.ID == "file:"+fish {
			t.Fatal("starter template planned over the imported file")
		}
		if s.Kind == "git" && (len(s.BeforeFiles) == 0 || s.BeforeFiles[0].SHA256 != "git") {
			t.Fatal("Git step not checked against the imported configuration", s.BeforeFiles)
		}
	}
	// An approved replacement of an imported file is checked against it.
	o.FileChoices[fish] = domain.Replace
	if p, err = plan.Build(h, o); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, s := range p.Steps {
		found = found || s.ID == "file:"+fish && s.File.BeforeExists && s.File.BeforeSHA256 == "old"
	}
	if !found {
		t.Fatal("replacement not planned against the imported file")
	}
	// An imported chezmoi source is reviewed before anything is added to it.
	h.ChezmoiIncoming = true
	if p, err = plan.Build(h, o); err != nil || len(steps(p, "chezmoi")) != 0 || task(p, "chezmoi-imported") == nil {
		t.Fatal("adoption planned into an incoming chezmoi source", err)
	}
}

func TestImportProblems(t *testing.T) {
	h, o := importHost(t)
	h.FreeBytes = 1e9
	p, err := plan.Build(h, o)
	if err != nil || p.Supported || !strings.Contains(strings.Join(p.Problems, "\n"), "Importing needs 3.0 GB but only 1.0 GB is free") {
		t.Fatal(err, p.Problems)
	}
	h, o = importHost(t)
	h.ImportProblems = []string{"Setup cannot read /Volumes/Backup/Users/you/Documents."}
	h.Import = h.Import[1:]
	if p, err = plan.Build(h, o); err != nil || p.Supported || p.Problems[len(p.Problems)-1] != h.ImportProblems[0] {
		t.Fatal(err, p.Problems)
	}
}

func TestImportChoicesChecked(t *testing.T) {
	for name, edit := range map[string]func(*domain.Host, *domain.Options){
		"inside home":       func(h *domain.Host, o *domain.Options) { o.ImportFrom = filepath.Join(h.Home, "old") },
		"containing home":   func(h *domain.Host, o *domain.Options) { o.ImportFrom = filepath.Dir(h.Home) },
		"relative":          func(h *domain.Host, o *domain.Options) { o.ImportFrom = "Users/you" },
		"Library":           func(h *domain.Host, o *domain.Options) { o.ImportFolders = []string{"Library"} },
		"nested":            func(h *domain.Host, o *domain.Options) { o.ImportFolders = []string{"Documents/Work"} },
		"parent":            func(h *domain.Host, o *domain.Options) { o.ImportFolders = []string{".."} },
		"control character": func(h *domain.Host, o *domain.Options) { o.ImportFolders = []string{"Docs\x1b[2J"} },
		"twice":             func(h *domain.Host, o *domain.Options) { o.ImportFolders = []string{"Documents", "Documents"} },
		"not inspected":     func(h *domain.Host, o *domain.Options) { o.ImportFolders = []string{"Music"} },
		"no source":         func(h *domain.Host, o *domain.Options) { o.ImportFrom = "" },
		"conflicts":         func(h *domain.Host, o *domain.Options) { o.ImportFolders = []string{"Imported conflicts"} },
	} {
		h, o := importHost(t)
		edit(&h, &o)
		if _, err := plan.Build(h, o); err == nil {
			t.Fatal(name, "accepted")
		}
	}
}

func TestImportJobTopFiles(t *testing.T) {
	h, o := importHost(t)
	job := plan.ImportJob(h, o, ".")
	if job.Source != oldHome || job.Destination != h.Home || !job.TopFilesOnly || filepath.Base(job.Conflicts) != "Top of home folder" {
		t.Fatalf("%+v", job)
	}
	if plan.SizeText(999) != "999 bytes" || plan.SizeText(1500) != "1.5 KB" || plan.SizeText(2e12) != "2.0 TB" {
		t.Fatal("size text")
	}
}

func TestWindowsRefusesImport(t *testing.T) {
	o := plan.DefaultOptionsFor("windows")
	o.ImportFrom, o.ImportFolders = `D:\Users\you`, []string{"Documents"}
	if _, err := plan.Build(testutil.FreshWindowsHost(`C:\Users\you`), o); err == nil || !strings.Contains(err.Error(), "not available on Windows") {
		t.Fatal(err)
	}
}

func TestImportedDotfilesAddedToChezmoi(t *testing.T) {
	h, o := importHost(t)
	zshrc, other := filepath.Join(h.Home, ".zshrc"), filepath.Join(h.Home, ".ssh/id_ed25519")
	h.ChezmoiCandidates = []string{zshrc}
	o.AdoptChezmoi = false
	o.ChezmoiAdd = []string{zshrc, other}
	p, err := plan.Build(h, o)
	if err != nil {
		t.Fatal(err)
	}
	adopt := steps(p, "chezmoi")
	if len(adopt) != 1 || adopt[0].Check.Expected != zshrc || !strings.Contains(adopt[0].Label, "with 1 imported dotfiles") || task(p, "dotfiles-commit") == nil {
		t.Fatalf("only offered files are added: %+v", adopt)
	}
	o.ChezmoiAdd = nil
	if p, err = plan.Build(h, o); err != nil || len(steps(p, "chezmoi")) != 0 {
		t.Fatal("adoption planned with nothing chosen", err)
	}
}
