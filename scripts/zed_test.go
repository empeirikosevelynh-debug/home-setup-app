package scripts

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Zed loads .zed/settings.json, tasks.json and debug.json at any depth, so
// only the repository root may hold them. Workspace templates live in zed/.
func TestOnlyRootZedConfig(t *testing.T) {
	root, e := filepath.Abs("..")
	if e != nil {
		t.Fatal(e)
	}
	e = filepath.WalkDir(root, func(path string, d fs.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if d.IsDir() && filepath.Dir(path) == root && (d.Name() == ".git" || d.Name() == "bin" || d.Name() == "dist") {
			return filepath.SkipDir
		}
		dir := filepath.Dir(path)
		if !d.IsDir() && filepath.Base(dir) == ".zed" && filepath.Dir(dir) != root {
			switch d.Name() {
			case "settings.json", "tasks.json", "debug.json":
				rel, _ := filepath.Rel(root, path)
				t.Errorf("Zed would load %s as live project configuration", rel)
			}
		}
		return nil
	})
	if e != nil {
		t.Fatal(e)
	}
}

func TestZedProjectConfig(t *testing.T) {
	for _, name := range []string{"settings.json", "tasks.json", "debug.json"} {
		data, e := os.ReadFile(filepath.Join("..", ".zed", name))
		if e != nil {
			t.Fatal(e)
		}
		var value any
		if e = json.Unmarshal(data, &value); e != nil {
			t.Fatalf(".zed/%s: %v", name, e)
		}
	}
	data, _ := os.ReadFile(filepath.Join("..", ".zed", "tasks.json"))
	var tasks []struct{ Label, Command string }
	if e := json.Unmarshal(data, &tasks); e != nil || len(tasks) == 0 {
		t.Fatal("no Zed tasks", e)
	}
	seen := map[string]bool{}
	for _, task := range tasks {
		if task.Label == "" || task.Command == "" || seen[task.Label] {
			t.Errorf("task needs a unique label and a command: %+v", task)
		}
		seen[task.Label] = true
		// Contributors never test by applying setup to their own account.
		for _, part := range strings.Split(task.Command, "&&") {
			fields := strings.Fields(part)
			runs := len(fields) > 0 && strings.HasSuffix(fields[0], "bin/golden-setup") ||
				len(fields) > 2 && fields[0] == "go" && fields[1] == "run" && strings.Contains(part, "./cmd/golden-setup")
			if runs && !strings.Contains(part, "--plan") {
				t.Errorf("%q runs the real wizard", task.Label)
			}
		}
	}
}
