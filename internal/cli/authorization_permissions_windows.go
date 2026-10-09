//go:build windows

package cli

import (
	"errors"
	"github.com/hidxt/miskoai/internal/privatefs"
	"os"
)

var errAuthorizationPermissions = errors.New("authorization file permissions could not be secured")

func protectAuthorizationFile(file *os.File) error {
	if err := privatefs.ProtectEmpty(file); err != nil {
		return errAuthorizationPermissions
	}
	return nil
}
