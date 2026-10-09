//go:build windows

package maintenance

import "github.com/hidxt/miskoai/internal/privatefs"

// Windows development verifies private metadata and normal restart behavior.
// It does not establish the Linux directory-fsync power-crash guarantee.
func syncDirectory(dir string) error {
	if privatefs.CheckDir(dir) != nil {
		return errMaintenance
	}
	return nil
}
