//go:build !windows

package privatefs

import (
	"golang.org/x/sys/unix"
	"os"
	"syscall"
)

func validPath(string) bool            { return true }
func samePath(a, b string) bool        { return a == b }
func plainPath(string) bool            { return true }
func makePrivateDir(path string) error { return os.Mkdir(path, 0700) }
func createPrivateFile(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
}
func openObject(path string, dir bool) (*os.File, error) {
	flags := unix.O_RDONLY | unix.O_NOFOLLOW | unix.O_CLOEXEC
	if dir {
		flags |= unix.O_DIRECTORY
	}
	fd, err := unix.Open(path, flags, 0)
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(fd), path), nil
}
func checkPermissions(f *os.File, dir bool) error {
	info, err := f.Stat()
	if err != nil || info.Mode().Perm()&0077 != 0 {
		return ErrUnsafe
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat.Uid != uint32(os.Geteuid()) || (!dir && stat.Nlink != 1) {
		return ErrUnsafe
	}
	if dir && !info.IsDir() || !dir && !info.Mode().IsRegular() {
		return ErrUnsafe
	}
	return nil
}
func protectEmpty(f *os.File) error {
	info, err := f.Stat()
	if err != nil {
		return ErrUnsafe
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat.Nlink != 1 || stat.Uid != uint32(os.Geteuid()) {
		return ErrUnsafe
	}
	if err = f.Chmod(0600); err != nil {
		return ErrUnsafe
	}
	return checkPermissions(f, false)
}
