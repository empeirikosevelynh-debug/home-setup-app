//go:build !windows

package apply

import (
	"golden-gate-setup/internal/domain"
	"golden-gate-setup/internal/plan"
	"golden-gate-setup/internal/testutil"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResumeRestoresChoicesWithoutApproval(t *testing.T) {
	dir := t.TempDir()
	s := SessionStore{Dir: dir + "/sessions"}
	o := plan.DefaultOptions()
	o.Apps = []string{"zed"}
	p, _ := plan.Build(testutil.FreshHost(dir), o)
	p.Accepted = true
	if e := s.Save(domain.Session{SchemaVersion: 1, Plan: p}); e != nil {
		t.Fatal(e)
	}
	loaded, e := s.Latest()
	if e != nil || loaded.Plan.Accepted || len(loaded.Plan.Options.Apps) != 1 || loaded.Plan.Options.Apps[0] != "zed" {
		t.Fatal(loaded, e)
	}
	st, _ := os.Stat(filepath.Join(s.Dir, p.ID+".json"))
	if st.Mode().Perm() != 0600 {
		t.Fatal("session not private")
	}
}
func TestMalformedSessionRejected(t *testing.T) {
	s := SessionStore{Dir: t.TempDir()}
	id := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	os.WriteFile(filepath.Join(s.Dir, id+".json"), []byte(`{"SchemaVersion":1`), 0600)
	if _, e := s.Load(id); e == nil {
		t.Fatal("truncated session accepted")
	}
	if _, e := s.Load("../escape"); e == nil {
		t.Fatal("invalid ID accepted")
	}
}
func TestRepeatedSavesKeepNoRecordBackups(t *testing.T) {
	dir := t.TempDir()
	s := SessionStore{Dir: dir + "/sessions"}
	p, _ := plan.Build(testutil.FreshHost(dir), plan.DefaultOptions())
	session := domain.Session{SchemaVersion: 1, Plan: p, Status: "running"}
	for i := 0; i < 3; i++ {
		session.Results = append(session.Results, domain.StepResult{ID: p.Steps[i].ID, Status: "verified"})
		if e := s.Save(session); e != nil {
			t.Fatal(e)
		}
	}
	if entries, _ := os.ReadDir(filepath.Join(s.Dir, "record-backups")); len(entries) != 0 {
		t.Fatal("session saves accumulated backups", len(entries))
	}
	if loaded, e := s.Load(p.ID); e != nil || len(loaded.Results) != 3 {
		t.Fatal("latest session not saved", e)
	}
}
func TestUnreadableLatestSessionNamesItsFile(t *testing.T) {
	s := SessionStore{Dir: t.TempDir()}
	id := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	os.WriteFile(filepath.Join(s.Dir, id+".json"), []byte(`{}`), 0644)
	if _, e := s.Latest(); e == nil || !strings.Contains(e.Error(), s.Path(id)) {
		t.Fatal("error does not say which session to remove", e)
	}
}
