// Package privatefs checks dedicated private directories and owner-only files
// before payload access. It does not defend against a malicious process running
// as the same OS user, nor revoke handles acquired before ordinary file checks.
package privatefs

import (
	"errors"
	"io"
	"math"
	"os"
	"path/filepath"
)

var (
	ErrUnsafe = errors.New("private filesystem permissions or object are unsafe")
	ErrCreate = errors.New("private filesystem object exists or cannot be created")
	ErrRead   = errors.New("private filesystem read failed or exceeded limit")
)

func dedicated(path string) (string, error) {
	if path == "" {
		return "", ErrUnsafe
	}
	abs, err := filepath.Abs(path)
	if err != nil || !validPath(abs) {
		return "", ErrUnsafe
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", ErrUnsafe
	}
	if abs == filepath.VolumeName(abs)+string(os.PathSeparator) || samePath(abs, cwd) {
		return "", ErrUnsafe
	}
	return abs, nil
}

// ancestors checks traversal without requiring unrelated ancestor permissions.
func ancestors(path string) error {
	for p := path; ; p = filepath.Dir(p) {
		info, err := os.Lstat(p)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || !plainPath(p) {
			return ErrUnsafe
		}
		next := filepath.Dir(p)
		if next == p {
			return nil
		}
	}
}

// EnsureDir creates only a missing final directory beneath existing real
// ancestors. An existing dedicated directory must already be private.
func EnsureDir(path string) error {
	abs, err := dedicated(path)
	if err != nil {
		return err
	}
	if err = ancestors(filepath.Dir(abs)); err != nil {
		return err
	}
	if _, err = os.Lstat(abs); err == nil {
		return CheckDir(abs)
	} else if !errors.Is(err, os.ErrNotExist) {
		return ErrUnsafe
	}
	if err = makePrivateDir(abs); err != nil {
		return ErrCreate
	}
	return CheckDir(abs)
}

// CheckDir verifies an existing dedicated directory without modifying it.
func CheckDir(path string) error {
	abs, err := dedicated(path)
	if err != nil {
		return err
	}
	if err = ancestors(abs); err != nil {
		return err
	}
	f, err := openObject(abs, true)
	if err != nil {
		return ErrUnsafe
	}
	defer f.Close()
	before, err := os.Lstat(abs)
	if err != nil {
		return ErrUnsafe
	}
	actual, err := f.Stat()
	if err != nil || !actual.IsDir() || !os.SameFile(before, actual) {
		return ErrUnsafe
	}
	if err = checkPermissions(f, true); err != nil {
		return ErrUnsafe
	}
	after, err := os.Lstat(abs)
	if err != nil || !os.SameFile(after, actual) {
		return ErrUnsafe
	}
	return nil
}

func checkedOpen(path string, maxBytes int64) (*os.File, error) {
	if maxBytes < 0 {
		return nil, ErrUnsafe
	}
	abs, err := filepath.Abs(path)
	if err != nil || !validPath(abs) {
		return nil, ErrUnsafe
	}
	if err = CheckDir(filepath.Dir(abs)); err != nil {
		return nil, err
	}
	before, err := os.Lstat(abs)
	if err != nil || !before.Mode().IsRegular() || before.Size() < 0 || before.Size() > maxBytes || !plainPath(abs) {
		return nil, ErrUnsafe
	}
	f, err := openObject(abs, false)
	if err != nil {
		return nil, ErrUnsafe
	}
	fail := func() (*os.File, error) { f.Close(); return nil, ErrUnsafe }
	actual, err := f.Stat()
	if err != nil || !sameMetadata(before, actual) || checkPermissions(f, false) != nil {
		return fail()
	}
	after, err := os.Lstat(abs)
	if err != nil || !sameMetadata(after, actual) || CheckDir(filepath.Dir(abs)) != nil {
		return fail()
	}
	return f, nil
}
func sameMetadata(a, b os.FileInfo) bool {
	return os.SameFile(a, b) && a.Mode() == b.Mode() && a.Size() == b.Size() && a.ModTime().Equal(b.ModTime())
}

// CheckFile verifies a regular owner-only file and its dedicated parent without
// reading payload bytes. The bounded size and opened object must agree.
func CheckFile(path string, maxBytes int64) error {
	f, err := checkedOpen(path, maxBytes)
	if err != nil {
		return err
	}
	if f.Close() != nil {
		return ErrUnsafe
	}
	return nil
}

// Create exclusively creates an owner-only file in an already private parent.
// Protection is supplied at creation, before the caller can write payloads.
func Create(path string) (*os.File, error) {
	abs, err := filepath.Abs(path)
	if err != nil || !validPath(abs) {
		return nil, ErrUnsafe
	}
	if err = CheckDir(filepath.Dir(abs)); err != nil {
		return nil, err
	}
	f, err := createPrivateFile(abs)
	if err != nil {
		return nil, ErrCreate
	}
	if checkPermissions(f, false) != nil || CheckDir(filepath.Dir(abs)) != nil {
		f.Close()
		return nil, ErrUnsafe
	}
	info, err := os.Lstat(abs)
	actual, e := f.Stat()
	if err != nil || e != nil || !sameMetadata(info, actual) || !plainPath(abs) {
		f.Close()
		return nil, ErrUnsafe
	}
	return f, nil
}

// Read reads only through the verified handle, caps reads at maxBytes+1 and
// rejects changed identity, metadata or permissions without returning payload.
func Read(path string, maxBytes int64) ([]byte, error) {
	if maxBytes == math.MaxInt64 {
		return nil, ErrUnsafe
	}
	f, err := checkedOpen(path, maxBytes)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	before, err := f.Stat()
	if err != nil {
		return nil, ErrRead
	}
	data, err := io.ReadAll(io.LimitReader(f, maxBytes+1))
	if err != nil || int64(len(data)) > maxBytes || int64(len(data)) != before.Size() {
		return nil, ErrRead
	}
	after, err := f.Stat()
	named, e := os.Lstat(path)
	if err != nil || e != nil || !sameMetadata(before, after) || !sameMetadata(after, named) || checkPermissions(f, false) != nil || CheckDir(filepath.Dir(path)) != nil {
		return nil, ErrUnsafe
	}
	return data, nil
}

// ProtectEmpty is the compatibility boundary for an already-open empty file.
// It never repairs populated private data. Prefer Create for ordinary creation.
func ProtectEmpty(f *os.File) error {
	if f == nil {
		return ErrUnsafe
	}
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() != 0 {
		return ErrUnsafe
	}
	named, err := os.Lstat(f.Name())
	if err != nil || !sameMetadata(named, info) || !plainPath(f.Name()) || ancestors(filepath.Dir(f.Name())) != nil {
		return ErrUnsafe
	}
	return protectEmpty(f)
}
