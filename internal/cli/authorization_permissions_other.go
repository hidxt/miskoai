//go:build !windows

package cli

import (
	"github.com/hidxt/miskoai/internal/privatefs"
	"os"
)

func protectAuthorizationFile(file *os.File) error { return privatefs.ProtectEmpty(file) }
