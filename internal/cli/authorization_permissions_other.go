//go:build !windows

package cli

import "os"

// Authorization files on non-Windows targets are created with mode 0600.
func protectAuthorizationFile(file *os.File) error { return nil }
