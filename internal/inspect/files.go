package inspect

import (
	"golden-gate-setup/internal/domain"
	"golden-gate-setup/internal/files"
	"golden-gate-setup/internal/plan"
	"os"
	"path/filepath"
	"strings"
)

func Paths(h domain.Host, o domain.Options) []string {
	if h.OS == "windows" {
		return []string{filepath.Join(h.Home, ".config", "starship.toml"), plan.ZedSettingsPath(h), filepath.Join(h.LazyGitDir, "config.yml"), filepath.Join(h.Home, ".gitconfig"), filepath.Join(h.Home, ".config", "git", "config")}
	}
	paths := []string{filepath.Join(h.Home, ".config/fish/config.fish"), filepath.Join(h.Home, ".config/fish/fish_plugins"), filepath.Join(h.Home, ".config/fish/functions/fisher.fish"), filepath.Join(h.Home, ".config/starship.toml"), filepath.Join(h.Home, ".config/zed/settings.json"), filepath.Join(h.LazyGitDir, "config.yml"), filepath.Join(h.Home, ".gitconfig"), filepath.Join(h.Home, ".config/git/config"), filepath.Join(h.Home, ".Brewfile"), filepath.Join(h.Home, ".kopiaignore")}
	for _, w := range o.Workspaces {
		if filepath.IsAbs(w.Path) {
			for _, file := range []string{".zed/settings.json", ".zed/tasks.json", "go.mod", "main.go", "shard.yml", "src/" + w.EntryPoint} {
				paths = append(paths, filepath.Join(w.Path, file))
			}
		}
	}
	return paths
}
func ReadFileState(root, path string) (domain.FileState, error) {
	m := files.Manager{Roots: []string{root}}
	s, e := m.Inspect(path)
	if e == nil {
		return s, nil
	}
	rel, re := filepath.Rel(root, path)
	outside := re != nil || rel == ".." || strings.HasPrefix(rel, "../")
	if outside || files.IsRedirect(e) {
		s = domain.FileState{Path: path, Symlink: true}
		st, err := os.Lstat(path)
		if err == nil {
			s.Exists = true
			s.Mode = st.Mode()
		} else if !os.IsNotExist(err) {
			return s, err
		}
		return s, nil
	}
	return s, e
}
