//go:build windows

package files

import (
	"context"
	"errors"
	"golden-gate-setup/internal/domain"
)

var errImportUnavailable = errors.New("importing a previous home folder is not available on Windows yet")

func ScanImport(domain.ImportJob, func(dst, src string)) (domain.ImportScan, error) {
	return domain.ImportScan{}, errImportUnavailable
}
func (Manager) Import(context.Context, domain.ImportJob, func(string)) (domain.ImportScan, error) {
	return domain.ImportScan{}, errImportUnavailable
}
func (Manager) Imported(context.Context, domain.ImportJob) (bool, error) {
	return false, errImportUnavailable
}
func FreeBytes(string) (int64, error) { return 0, errImportUnavailable }
