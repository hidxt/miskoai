//go:build windows

package privatefs

import (
	"golang.org/x/sys/windows"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"unsafe"
)

func broadACL(t *testing.T, path string) {
	t.Helper()
	user, e := windows.GetCurrentProcessToken().GetTokenUser()
	if e != nil {
		t.Fatal(e)
	}
	sd, e := windows.SecurityDescriptorFromString("D:P(A;OICI;FA;;;" + user.User.Sid.String() + ")(A;OICI;FR;;;WD)")
	if e != nil {
		t.Fatal(e)
	}
	acl, _, e := sd.DACL()
	if e != nil {
		t.Fatal(e)
	}
	if e = windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, acl, nil); e != nil {
		t.Fatal(e)
	}
}
func TestPrivateBroadACLRejectedWindows(t *testing.T) {
	dir := fixtureDir(t)
	path := filepath.Join(dir, "synthetic")
	writeFixture(t, path, "unchanged")
	broadACL(t, path)
	before, e := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if e != nil {
		t.Fatal(e)
	}
	if b, e := Read(path, 100); e == nil || b != nil {
		t.Fatal("broad DACL payload read")
	}
	after, e := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if e != nil {
		t.Fatal(e)
	}
	if before.String() != after.String() {
		t.Fatal("file DACL repaired")
	}
	b, e := os.ReadFile(path)
	if e != nil || string(b) != "unchanged" {
		t.Fatal("payload changed", e)
	}
	parentMarker := filepath.Join(dir, "private-parent-marker")
	writeFixture(t, parentMarker, "private child")
	broadACL(t, dir)
	before, e = windows.GetNamedSecurityInfo(dir, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if e != nil {
		t.Fatal(e)
	}
	if e = EnsureDir(dir); e == nil {
		t.Fatal("broad directory accepted")
	}
	after, e = windows.GetNamedSecurityInfo(dir, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if e != nil {
		t.Fatal(e)
	}
	if before.String() != after.String() {
		t.Fatal("directory repaired")
	}
	if _, e = Read(parentMarker, 100); e == nil {
		t.Fatal("broad parent accepted")
	}
}
func TestPrivateCreationACLAndInheritedSidecarWindows(t *testing.T) {
	dir := fixtureDir(t)
	sd, e := windows.GetNamedSecurityInfo(dir, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	if e != nil {
		t.Fatal(e)
	}
	ctl, _, e := sd.Control()
	if e != nil || ctl&windows.SE_DACL_PROTECTED == 0 {
		t.Fatal("unprotected directory")
	}
	acl, _, e := sd.DACL()
	if e != nil || acl == nil || acl.AceCount != 1 {
		t.Fatal("directory must have one ACE")
	}
	var ace *windows.ACCESS_ALLOWED_ACE
	if e = windows.GetAce(acl, 0, &ace); e != nil {
		t.Fatal(e)
	}
	if ace.Header.AceFlags&(windows.OBJECT_INHERIT_ACE|windows.CONTAINER_INHERIT_ACE) != windows.OBJECT_INHERIT_ACE|windows.CONTAINER_INHERIT_ACE {
		t.Fatal("sidecar inheritance absent")
	}
	owner, _, e := sd.Owner()
	if e != nil || !windows.EqualSid((*windows.SID)(unsafe.Pointer(&ace.SidStart)), owner) {
		t.Fatal("wrong principal")
	}
	path := filepath.Join(dir, "synthetic-sidecar")
	if e = os.WriteFile(path, []byte("sidecar"), 0600); e != nil {
		t.Fatal(e)
	}
	if b, e := Read(path, 7); e != nil || string(b) != "sidecar" {
		t.Fatal("owner inherited sidecar rejected", e)
	}
	path = filepath.Join(dir, "explicit")
	file, e := Create(path)
	if e != nil {
		t.Fatal(e)
	}
	defer file.Close()
	sd, e = windows.GetSecurityInfo(windows.Handle(file.Fd()), windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if e != nil {
		t.Fatal(e)
	}
	ctl, _, e = sd.Control()
	if e != nil || ctl&windows.SE_DACL_PROTECTED == 0 {
		t.Fatal("new file DACL not protected")
	}
	if _, e = file.WriteString("fixture"); e != nil {
		t.Fatal(e)
	}
}

func TestPrivateJunctionTraversalWindows(t *testing.T) {
	target := fixtureDir(t)
	path := filepath.Join(target, "synthetic")
	writeFixture(t, path, "unchanged")
	link := filepath.Join(t.TempDir(), "junction")
	if out, err := exec.Command("cmd.exe", "/c", "mklink", "/J", link, target).CombinedOutput(); err != nil {
		t.Skipf("host denies synthetic junction setup: %v %s", err, out)
	}
	defer os.Remove(link)
	if err := CheckDir(link); err == nil {
		t.Fatal("junction directory accepted")
	}
	if err := EnsureDir(filepath.Join(link, "new")); err == nil {
		t.Fatal("created through junction")
	}
	if _, err := Read(filepath.Join(link, "synthetic"), 100); err == nil {
		t.Fatal("junction payload read")
	}
	if _, err := os.Stat(filepath.Join(target, "new")); !os.IsNotExist(err) {
		t.Fatal("target changed")
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != "unchanged" {
		t.Fatal("target payload changed", err)
	}
}
