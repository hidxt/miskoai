//go:build windows

package cli

import (
	"os"
	"path/filepath"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

func TestAuthorizationFileProtectedOwnerOnlyWindows(t *testing.T) {
	path := filepath.Join(t.TempDir(), "synthetic-authorization.json")
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if err := protectAuthorizationFile(file); err != nil {
		t.Fatal(err)
	}
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatal(err)
	}
	sd, err := windows.GetSecurityInfo(windows.Handle(file.Fd()), windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatal(err)
	}
	owner, _, err := sd.Owner()
	if err != nil || owner == nil || !windows.EqualSid(owner, user.User.Sid) {
		t.Fatal("owner must be current process user")
	}
	control, _, err := sd.Control()
	if err != nil || control&windows.SE_DACL_PROTECTED == 0 {
		t.Fatal("DACL must be protected against inherited access")
	}
	dacl, _, err := sd.DACL()
	if err != nil || dacl == nil || dacl.AceCount != 1 {
		t.Fatal("DACL must contain exactly one owner allow entry")
	}
	for i := uint16(0); i < dacl.AceCount; i++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(dacl, uint32(i), &ace); err != nil {
			t.Fatal(err)
		}
		if ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE || ace.Header.AceFlags&windows.INHERITED_ACE != 0 {
			t.Fatal("only explicit owner allow access is permitted")
		}
		if !windows.EqualSid((*windows.SID)(unsafe.Pointer(&ace.SidStart)), user.User.Sid) {
			t.Fatal("allow entry grants another principal")
		}
		if ace.Mask&windows.ACCESS_MASK(windows.GENERIC_READ|windows.GENERIC_WRITE) != 0 {
			t.Fatal("file rights must be concrete, not unmapped generic rights")
		}
		if ace.Mask&windows.ACCESS_MASK(windows.READ_CONTROL|windows.WRITE_DAC|windows.FILE_READ_DATA|windows.FILE_WRITE_DATA) != windows.ACCESS_MASK(windows.READ_CONTROL|windows.WRITE_DAC|windows.FILE_READ_DATA|windows.FILE_WRITE_DATA) {
			t.Fatal("owner missing required file access")
		}
	}
	if _, err := file.WriteString("synthetic fixture only"); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.ReadFile(path); err != nil {
		t.Fatal("owner cannot read after protection")
	}
}

func TestAuthorizationFileProtectionFailsClosedWindows(t *testing.T) {
	if err := protectAuthorizationFile(nil); err == nil {
		t.Fatal("nil file accepted")
	}
	file, err := os.CreateTemp(t.TempDir(), "synthetic-closed-")
	if err != nil {
		t.Fatal(err)
	}
	file.Close()
	if err := protectAuthorizationFile(file); err == nil {
		t.Fatal("closed handle accepted")
	}
	dir, err := os.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer dir.Close()
	if err := protectAuthorizationFile(dir); err == nil {
		t.Fatal("directory accepted")
	}
}

func TestAuthorizationProtectionRejectsPreexistingReaderWindows(t *testing.T) {
	path := filepath.Join(t.TempDir(), "synthetic-preopened.json")
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	reader, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	if err := protectAuthorizationFile(file); err == nil {
		t.Fatal("preexisting reader retains access after ACL change; protection must fail closed")
	}
}

func TestAuthorizationProtectionRejectsExistingContentWindows(t *testing.T) {
	path := filepath.Join(t.TempDir(), "synthetic-content.json")
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if _, err := file.WriteString("synthetic"); err != nil {
		t.Fatal(err)
	}
	if err := protectAuthorizationFile(file); err == nil {
		t.Fatal("already populated file accepted")
	}
}
