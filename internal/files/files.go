package files

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"golden-gate-setup/internal/domain"
)

type Manager struct {
	Roots        []string
	BeforeRename func() error
	// SkipBackup replaces files without keeping their previous contents. Only
	// the installer's own records use it; user configuration is always backed up.
	SkipBackup bool
}

func digest(data []byte) string { s := sha256.Sum256(data); return hex.EncodeToString(s[:]) }

func matches(s domain.FileState, c domain.FileChange) bool {
	return s.Exists == c.BeforeExists && (!s.Exists || s.SHA256 == c.BeforeSHA256)
}
func (m Manager) Apply(c domain.FileChange, backupDir string) (domain.StepResult, error) {
	return m.ApplyContext(context.Background(), c, backupDir)
}
