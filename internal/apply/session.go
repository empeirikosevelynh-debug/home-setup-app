package apply

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"golden-gate-setup/internal/domain"
	"golden-gate-setup/internal/files"
	"golden-gate-setup/internal/plan"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

type SessionStore struct {
	Dir  string
	Root string
}

var sessionID = regexp.MustCompile(`^[a-f0-9]{64}$`)

func (s SessionStore) Path(id string) string { return filepath.Join(s.Dir, id+".json") }
func (s SessionStore) manager() (files.Manager, error) {
	if !filepath.IsAbs(s.Dir) || filepath.Clean(s.Dir) != s.Dir {
		return files.Manager{}, errors.New("session directory must be absolute")
	}
	root := s.Root
	if root == "" {
		root = filepath.Dir(s.Dir)
	}
	// A session record only grows; its earlier versions are not worth a backup.
	return files.Manager{Roots: []string{root}, SkipBackup: true}, nil
}
func (s SessionStore) Load(id string) (domain.Session, error) {
	var result domain.Session
	if !sessionID.MatchString(id) {
		return result, errors.New("invalid session ID")
	}
	m, e := s.manager()
	if e != nil {
		return result, e
	}
	state, e := m.Inspect(s.Path(id))
	if e != nil {
		return result, e
	}
	if !state.Exists {
		return result, os.ErrNotExist
	}
	if state.Mode.Perm()&0077 != 0 {
		return result, errors.New("session permissions are not private")
	}
	decoder := json.NewDecoder(bytes.NewReader(state.Contents))
	decoder.DisallowUnknownFields()
	if e = decoder.Decode(&result); e != nil {
		return result, e
	}
	var extra any
	if e = decoder.Decode(&extra); e != io.EOF {
		return result, errors.New("unexpected trailing session data")
	}
	fingerprint, e := plan.Fingerprint(result.Plan)
	if e != nil || result.SchemaVersion != 1 || result.Plan.SchemaVersion != 1 || result.Plan.ID != id || fingerprint != id {
		return domain.Session{}, errors.New("invalid or incompatible session")
	}
	result.Plan.Accepted = false
	return result, nil
}
func (s SessionStore) Latest() (domain.Session, error) {
	m, e := s.manager()
	if e != nil {
		return domain.Session{}, e
	}
	if _, e = m.Inspect(filepath.Join(s.Dir, "probe")); e != nil {
		return domain.Session{}, e
	}
	entries, e := os.ReadDir(s.Dir)
	if e != nil {
		return domain.Session{}, e
	}
	type candidate struct {
		id   string
		time int64
	}
	var candidates []candidate
	for _, f := range entries {
		if f.IsDir() || !strings.HasSuffix(f.Name(), ".json") {
			continue
		}
		id := strings.TrimSuffix(f.Name(), ".json")
		if !sessionID.MatchString(id) {
			continue
		}
		info, e := f.Info()
		if e != nil {
			return domain.Session{}, e
		}
		candidates = append(candidates, candidate{id, info.ModTime().UnixNano()})
	}
	if len(candidates) == 0 {
		return domain.Session{}, os.ErrNotExist
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].time > candidates[j].time })
	session, e := s.Load(candidates[0].id)
	if e != nil {
		return session, fmt.Errorf("%s: %w", s.Path(candidates[0].id), e)
	}
	return session, nil
}
func (s SessionStore) Save(value domain.Session) error {
	if value.SchemaVersion != 1 || value.Plan.SchemaVersion != 1 || !sessionID.MatchString(value.Plan.ID) {
		return errors.New("invalid session schema or ID")
	}
	id, e := plan.Fingerprint(value.Plan)
	if e != nil || id != value.Plan.ID {
		return errors.New("session plan was modified")
	}
	m, e := s.manager()
	if e != nil {
		return e
	}
	path := s.Path(value.Plan.ID)
	before, e := m.Inspect(path)
	if e != nil {
		return e
	}
	data, e := json.MarshalIndent(value, "", "  ")
	if e != nil {
		return e
	}
	if before.Exists && before.Mode.Perm()&0077 != 0 {
		return errors.New("existing session is not private")
	}
	if e = m.EnsurePrivateDir(s.Dir); e != nil {
		return e
	}
	decision := domain.Create
	if before.Exists {
		decision = domain.Replace
	}
	_, e = m.Apply(domain.FileChange{Path: path, BeforeExists: before.Exists, BeforeSHA256: before.SHA256, Desired: data, Mode: 0600, Decision: decision}, filepath.Join(s.Dir, "record-backups"))
	if e != nil {
		return fmt.Errorf("save session: %w", e)
	}
	return nil
}
