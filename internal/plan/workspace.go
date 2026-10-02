package plan

import (
	"bytes"
	"fmt"
	"golden-gate-setup/internal/domain"
	"golden-gate-setup/internal/templates"
	"path/filepath"
	"regexp"
	"strings"
)

var projectName = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
var goModule = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._~+-]*(/[A-Za-z0-9][A-Za-z0-9._~+-]*)+$`)
var crystalEntry = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_-]*\.cr$`)

func WorkspaceChanges(h domain.Host, w domain.Workspace) ([]domain.FileChange, error) {
	return workspaceChanges(h, w, nil)
}
func workspaceChanges(h domain.Host, w domain.Workspace, choices map[string]domain.FileDecision) ([]domain.FileChange, error) {
	if !Has(Languages, w.Language) || !filepath.IsAbs(w.Path) || filepath.Clean(w.Path) != w.Path || strings.ContainsAny(w.Path, "\x00\r\n") {
		return nil, fmt.Errorf("choose a clean absolute workspace path and language")
	}
	state := h.Workspaces[w.Path]
	if state.Symlink || state.Exists && !state.Directory {
		return nil, fmt.Errorf("workspace must be a real directory")
	}
	fresh := w.Create && (!state.Exists || state.Empty)
	if !state.Exists && !w.Create {
		return nil, fmt.Errorf("choose project creation for an absent workspace")
	}
	if w.Language == "go" && fresh && !goModule.MatchString(w.Module) {
		return nil, fmt.Errorf("new Go workspace needs its actual module path")
	}
	if w.Language != "go" && !projectName.MatchString(w.Module) {
		return nil, fmt.Errorf("%s workspace needs its actual project/binary name", w.Language)
	}
	if w.Language == "crystal" && !crystalEntry.MatchString(w.EntryPoint) {
		return nil, fmt.Errorf("Crystal entry point must be a safe .cr filename under src/")
	}
	desired := map[string][]byte{}
	for _, name := range []string{"settings.json", "tasks.json"} {
		data, e := templates.Load("workspaces/" + w.Language + "/zed/" + name)
		if e != nil {
			return nil, e
		}
		if w.Language == "crystal" {
			data = bytes.ReplaceAll(data, []byte("my_crystal_app.cr"), []byte(w.EntryPoint))
		}
		if w.Language == "nim" {
			data = bytes.ReplaceAll(data, []byte("my_nim_app"), []byte(w.Module))
		}
		desired[".zed/"+name] = data
	}
	order := []string{}
	if fresh {
		switch w.Language {
		case "go":
			desired["go.mod"] = []byte("module " + w.Module + "\n\ngo 1.26.0\n")
			desired["main.go"] = []byte("package main\n\nimport \"fmt\"\n\nfunc main() { fmt.Println(\"Hello from your Go workspace\") }\n")
			order = append(order, "go.mod", "main.go")
		case "crystal":
			desired["shard.yml"] = []byte("name: " + w.Module + "\nversion: 0.1.0\ntargets:\n  " + w.Module + ":\n    main: src/" + w.EntryPoint + "\n")
			desired["src/"+w.EntryPoint] = []byte("puts \"Hello from " + w.Module + "\"\n")
			order = append(order, "shard.yml", "src/"+w.EntryPoint)
		case "nim":
			manifest := w.Module + ".nimble"
			desired[manifest] = []byte("version = \"0.1.0\"\nauthor = \"\"\ndescription = \"Local application\"\nlicense = \"UNLICENSED\"\nsrcDir = \"src\"\nbin = @[\"" + w.Module + "\"]\nrequires \"nim >= 1.6.0\"\n")
			desired["src/"+w.Module+".nim"] = []byte("echo \"Hello from " + w.Module + "\"\n")
			order = append(order, manifest, "src/"+w.Module+".nim")
		}
	}
	order = append(order, ".zed/settings.json", ".zed/tasks.json")
	var result []domain.FileChange
	for _, rel := range order {
		path := filepath.Join(w.Path, rel)
		before := h.Files[path]
		data := desired[rel]
		if before.Exists {
			if before.Symlink || !before.Mode.IsRegular() || bytes.Equal(before.Contents, data) {
				continue
			}
			if choices[path] != domain.Replace || !strings.HasPrefix(rel, ".zed/") {
				continue
			}
		}
		decision := domain.Create
		if before.Exists {
			decision = domain.Replace
		}
		result = append(result, domain.FileChange{Path: path, BeforeExists: before.Exists, BeforeSHA256: before.SHA256, Mode: 0644, Desired: data, Decision: decision})
	}
	return result, nil
}
