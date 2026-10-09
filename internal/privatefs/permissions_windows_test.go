//go:build windows

package privatefs

import (
	"errors"
	"golang.org/x/sys/windows"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

// This regression uses the existing boundary APIs so RED proves unsafe raw
// spellings are accepted, rather than merely proving a new API is absent.
func TestPrivateRawAliasesRejectedWindows(t *testing.T) {
	for _, spelling := range []struct {
		name, suffix string
		forward      bool
	}{
		{"dot-backslash", ".", false}, {"space-backslash", " ", false},
		{"dot-forward", ".", true}, {"space-forward", " ", true},
	} {
		t.Run(spelling.name, func(t *testing.T) {
			dir := fixtureDir(t)
			path := filepath.Join(dir, "known")
			writeFixture(t, path, "unchanged synthetic marker")
			info, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
			if err != nil {
				t.Fatal(err)
			}
			dirInfo, err := os.Stat(dir)
			if err != nil {
				t.Fatal(err)
			}
			dirSD, err := windows.GetNamedSecurityInfo(dir, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
			if err != nil {
				t.Fatal(err)
			}
			raw := func(p string) string {
				if spelling.forward {
					return strings.ReplaceAll(p, `\`, "/")
				}
				return p
			}
			if err := EnsureDir(raw(dir + spelling.suffix)); !errors.Is(err, ErrUnsafe) {
				t.Errorf("directory alias not safely refused: %v", err)
			}
			if err := CheckFile(raw(path+spelling.suffix), 100); !errors.Is(err, ErrUnsafe) {
				t.Errorf("file alias not safely refused: %v", err)
			}
			if got, err := Read(raw(path+spelling.suffix), 100); !errors.Is(err, ErrUnsafe) || got != nil {
				t.Errorf("read alias returned payload or unsafe error: %q %v", got, err)
			}
			newPath := filepath.Join(dir, "new")
			f, err := Create(raw(newPath + spelling.suffix))
			if f != nil {
				f.Close()
			}
			if !errors.Is(err, ErrUnsafe) {
				t.Errorf("creation alias not safely refused: %v", err)
			}
			if _, err := os.Stat(newPath); !os.IsNotExist(err) {
				t.Errorf("creation alias touched canonical target: %v", err)
			}
			empty := filepath.Join(dir, "empty")
			writeFixture(t, empty, "")
			broadACL(t, empty)
			emptyInfo, err := os.Stat(empty)
			if err != nil {
				t.Fatal(err)
			}
			emptySD, err := windows.GetNamedSecurityInfo(empty, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
			if err != nil {
				t.Fatal(err)
			}
			opened, err := os.OpenFile(raw(empty+spelling.suffix), os.O_WRONLY, 0600)
			if err != nil {
				t.Fatal("alias setup could not open known empty fixture", err)
			}
			if err := ProtectEmpty(opened); !errors.Is(err, ErrUnsafe) {
				t.Errorf("empty alias not safely refused: %v", err)
			}
			opened.Close()
			for _, known := range []struct {
				path    string
				info    os.FileInfo
				acl     string
				content string
			}{
				{path, info, sd.String(), "unchanged synthetic marker"}, {empty, emptyInfo, emptySD.String(), ""},
			} {
				after, err := os.Stat(known.path)
				if err != nil || !os.SameFile(known.info, after) || known.info.Size() != after.Size() || !known.info.ModTime().Equal(after.ModTime()) {
					t.Errorf("known target identity/metadata changed: %v", err)
				}
				actual, err := windows.GetNamedSecurityInfo(known.path, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
				if err != nil || actual.String() != known.acl {
					t.Errorf("known target ACL changed: %v", err)
				}
				data, err := os.ReadFile(known.path)
				if err != nil || string(data) != known.content {
					t.Errorf("known target bytes changed: %v", err)
				}
			}
			after, err := os.Stat(dir)
			if err != nil || !os.SameFile(dirInfo, after) {
				t.Errorf("directory identity changed: %v", err)
			}
			actual, err := windows.GetNamedSecurityInfo(dir, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
			if err != nil || actual.String() != dirSD.String() {
				t.Errorf("directory ACL changed: %v", err)
			}
		})
	}
}

func TestPrivateOrdinaryRelativePathsWindows(t *testing.T) {
	dir := fixtureDir(t)
	path := filepath.Join(dir, "marker")
	writeFixture(t, path, "relative marker")
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	relative, err := filepath.Rel(cwd, dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{`.\` + relative, "./" + strings.ReplaceAll(relative, `\`, "/"), `..\` + filepath.Base(cwd) + `\` + relative} {
		if err := EnsureDir(p); err != nil {
			t.Errorf("ordinary relative directory refused: %v", err)
		}
		if err := CheckFile(p+`\marker`, 100); err != nil {
			t.Errorf("ordinary relative file refused: %v", err)
		}
		if got, err := Read(p+`\marker`, 100); err != nil || string(got) != "relative marker" {
			t.Errorf("ordinary relative read failed: %q %v", got, err)
		}
	}
}

func TestPrivateResolveRawWindowsLexical(t *testing.T) {
	for _, path := range []string{
		`C:\synthetic\known.`, `C:/synthetic/known `,
		`C:\synthetic\alias.\..\leaf`, `C:/synthetic/alias /../leaf`,
		`C:\synthetic\file:stream`, `C:/synthetic/file:stream/../leaf`,
		`C:\synthetic\NUL.txt\..\leaf`, `C:/synthetic/COM1/../leaf`,
		`\\synthetic-server.\share\leaf`, `//synthetic-server/share /leaf`,
	} {
		if got, err := Resolve(path); !errors.Is(err, ErrUnsafe) || got != "" {
			t.Errorf("unsafe raw spelling resolved: %q %v", got, err)
		}
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ raw, want string }{
		{`./synthetic-data`, filepath.Join(cwd, "synthetic-data")},
		{`.\synthetic-data`, filepath.Join(cwd, "synthetic-data")},
		{`..\synthetic-data`, filepath.Join(filepath.Dir(cwd), "synthetic-data")},
		{`../synthetic-data`, filepath.Join(filepath.Dir(cwd), "synthetic-data")},
	} {
		if got, err := Resolve(tc.raw); err != nil || got != tc.want {
			t.Errorf("ordinary dot operator resolution failed: %q %v", got, err)
		}
	}
}
