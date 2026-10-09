package privatefs

import (
	"errors"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func fixtureDir(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "private")
	if err := EnsureDir(path); err != nil {
		t.Fatal(err)
	}
	return path
}
func writeFixture(t *testing.T, path string, content string) {
	t.Helper()
	f, err := Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.WriteString(content); err != nil {
		t.Fatal(err)
	}
	if err = f.Close(); err != nil {
		t.Fatal(err)
	}
}
func TestPrivateFileRejectedBeforeRead(t *testing.T) {
	dir := fixtureDir(t)
	path := filepath.Join(dir, "synthetic-marker")
	writeFixture(t, path, "synthetic marker")
	if got, err := Read(path, 3); err == nil || got != nil {
		t.Fatal("oversize payload returned", string(got), err)
	}
	if got, err := Read(path, 15); err == nil || got != nil {
		t.Fatal("exact max+1 payload returned", err)
	}
	if err := CheckFile(path, -1); err == nil {
		t.Fatal("negative limit accepted")
	}
	if runtime.GOOS != "windows" {
		if err := os.Chmod(path, 0644); err != nil {
			t.Fatal(err)
		}
		if got, err := Read(path, 100); err == nil || got != nil {
			t.Fatal("broad file read", err)
		}
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != "synthetic marker" {
		t.Fatal("target changed", err)
	}
}

func TestPrivateMetadataMaximumLimit(t *testing.T) {
	path := filepath.Join(fixtureDir(t), "synthetic")
	writeFixture(t, path, "small fixture")
	if err := CheckFile(path, math.MaxInt64); err != nil {
		t.Fatal("metadata check must not introduce a new payload cap", err)
	}
	if _, err := Read(path, math.MaxInt64); err == nil {
		t.Fatal("overflowing max+1 read bound accepted")
	}
}

func TestPrivateReadLimitOverflow(t *testing.T) {
	path := filepath.Join(fixtureDir(t), "nonempty")
	writeFixture(t, path, "nonempty synthetic fixture")
	if got, err := Read(path, math.MaxInt64); err == nil || got != nil {
		t.Fatal("overflowing limit returned a successful or partial payload", string(got), err)
	}
}
func TestPrivateDirNeverRepairsExisting(t *testing.T) {
	broad := t.TempDir()
	if runtime.GOOS == "windows" {
		t.Skip("explicit broad DACL tested in Windows test")
	}
	if err := os.Chmod(broad, 0755); err != nil {
		t.Fatal(err)
	}
	if err := EnsureDir(broad); err == nil {
		t.Fatal("broad directory accepted")
	}
	info, err := os.Stat(broad)
	if err != nil || info.Mode().Perm() != 0755 {
		t.Fatal("existing permissions changed", err)
	}
}
func TestPrivateCreateExclusive(t *testing.T) {
	dir := fixtureDir(t)
	path := filepath.Join(dir, "synthetic")
	writeFixture(t, path, "original")
	if f, err := Create(path); err == nil {
		f.Close()
		t.Fatal("existing file overwritten")
	}
	got, err := Read(path, 8)
	if err != nil || string(got) != "original" {
		t.Fatal("original not retained", err)
	}
	if f, err := Create(filepath.Join(t.TempDir(), "unsafe")); err == nil {
		f.Close()
		t.Fatal("unsafe parent accepted")
	}
}
func TestPrivateSymlinkTraversal(t *testing.T) {
	target := fixtureDir(t)
	writeFixture(t, filepath.Join(target, "synthetic"), "unchanged")
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(target, link); err != nil {
		if runtime.GOOS == "windows" {
			t.Skip("host lacks symlink privilege", err)
		}
		t.Fatal(err)
	}
	if err := EnsureDir(link); err == nil {
		t.Fatal("linked directory accepted")
	}
	if _, err := Read(filepath.Join(link, "synthetic"), 20); err == nil {
		t.Fatal("linked ancestor traversed")
	}
	if err := EnsureDir(filepath.Join(link, "new")); err == nil {
		t.Fatal("created through link")
	}
	empty := filepath.Join(target, "empty")
	f, err := Create(empty)
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	fileLink := filepath.Join(target, "file-link")
	if err := os.Symlink(empty, fileLink); err != nil {
		t.Fatal(err)
	}
	if _, err := Read(fileLink, 10); err == nil {
		t.Fatal("symlink file read")
	}
	f, err = os.OpenFile(fileLink, os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if err := ProtectEmpty(f); err == nil {
		f.Close()
		t.Fatal("symlink opened empty file protected")
	}
	f.Close()
	if _, err := os.Stat(filepath.Join(target, "new")); !os.IsNotExist(err) {
		t.Fatal("target changed")
	}
	got, err := os.ReadFile(filepath.Join(target, "synthetic"))
	if err != nil || string(got) != "unchanged" {
		t.Fatal("target changed", err)
	}
}
func TestPrivateSafeErrorsAndDedicatedDirectory(t *testing.T) {
	cwd, _ := os.Getwd()
	for _, path := range []string{"", ".", cwd, filepath.VolumeName(cwd) + string(os.PathSeparator), filepath.Join(t.TempDir(), "attacker-marker", "missing")} {
		if err := EnsureDir(path); err == nil {
			t.Fatal("unsafe directory accepted")
		} else if strings.Contains(err.Error(), "attacker-marker") {
			t.Fatal("path leaked")
		}
	}
	if err := ProtectEmpty(nil); err == nil {
		t.Fatal("nil file accepted")
	}
	dir := fixtureDir(t)
	openedDir, err := os.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := ProtectEmpty(openedDir); err == nil {
		openedDir.Close()
		t.Fatal("directory protected as file")
	}
	openedDir.Close()
	path := filepath.Join(dir, "nonempty")
	writeFixture(t, path, "synthetic")
	f, err := os.OpenFile(path, os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if err := ProtectEmpty(f); err == nil {
		t.Fatal("populated file repaired")
	}
	f.Close()
	if err := ProtectEmpty(f); err == nil {
		t.Fatal("closed file accepted")
	}
}
func TestPrivateHardlinkAndEmptyProtection(t *testing.T) {
	dir := fixtureDir(t)
	path := filepath.Join(dir, "empty")
	f, err := Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "second")
	if err := os.Link(path, link); err != nil {
		if runtime.GOOS == "windows" && os.IsPermission(err) {
			t.Skip("host denies synthetic hardlink creation", err)
		}
		t.Fatal(err)
	}
	f, err = os.OpenFile(path, os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := ProtectEmpty(f); err == nil {
		t.Fatal("multiple-link empty object protected")
	}
	if _, err := Read(path, 1); err == nil {
		t.Fatal("multiple-link file read")
	}
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	if err := ProtectEmpty(f); err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("x"); err != nil {
		t.Fatal(err)
	}
}

func TestPrivateResolveIsLexicalOnly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing-parent", "leaf")
	got, err := Resolve(path)
	if err != nil || got != path {
		t.Fatal("lexically valid missing path refused or changed", got, err)
	}
	if _, err := os.Stat(filepath.Dir(path)); !os.IsNotExist(err) {
		t.Fatal("resolution created a directory", err)
	}
	if got, err := Resolve(""); !errors.Is(err, ErrUnsafe) || got != "" {
		t.Fatal("empty path resolved", got, err)
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if got, err := Resolve("."); err != nil || got != cwd {
		t.Fatal("resolution should not impose dedicated directory/privacy policy", got, err)
	}
}
