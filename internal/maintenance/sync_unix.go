//go:build !windows

package maintenance

import (
	"github.com/hidxt/miskoai/internal/privatefs"
	"os"
)

// File sync alone does not persist a newly created directory entry on Linux.
// Check the private directory and opened identity before syncing metadata.
func syncDirectory(dir string) error {
	if privatefs.CheckDir(dir) != nil {
		return errMaintenance
	}
	before, e := os.Lstat(dir)
	if e != nil {
		return errMaintenance
	}
	f, e := os.Open(dir)
	if e != nil {
		return errMaintenance
	}
	actual, e := f.Stat()
	if e != nil || !os.SameFile(before, actual) {
		f.Close()
		return errMaintenance
	}
	se := f.Sync()
	ce := f.Close()
	after, e := os.Lstat(dir)
	if se != nil || ce != nil || e != nil || !os.SameFile(actual, after) || privatefs.CheckDir(dir) != nil {
		return errMaintenance
	}
	return nil
}
