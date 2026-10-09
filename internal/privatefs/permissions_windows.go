//go:build windows

package privatefs

import (
	"golang.org/x/sys/windows"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"unsafe"
)

const privateFileAccess windows.ACCESS_MASK = windows.STANDARD_RIGHTS_REQUIRED | windows.SYNCHRONIZE | 0x1ff

var privateReOpenFile = windows.NewLazySystemDLL("kernel32.dll").NewProc("ReOpenFile")

func validPath(path string) bool {
	rest := strings.TrimPrefix(path, filepath.VolumeName(path))
	if strings.Contains(rest, ":") {
		return false
	}
	for _, part := range strings.Split(rest, string(os.PathSeparator)) {
		if part == "" {
			continue
		}
		if strings.TrimRight(part, " .") != part {
			return false
		}
		stem := strings.ToUpper(strings.SplitN(part, ".", 2)[0])
		if stem == "CON" || stem == "PRN" || stem == "AUX" || stem == "NUL" || stem == "CONIN$" || stem == "CONOUT$" {
			return false
		}
		if len(stem) == 4 && (strings.HasPrefix(stem, "COM") || strings.HasPrefix(stem, "LPT")) && stem[3] >= '1' && stem[3] <= '9' {
			return false
		}
	}
	return true
}
func samePath(a, b string) bool { return strings.EqualFold(a, b) }
func plainPath(path string) bool {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return false
	}
	attrs, err := windows.GetFileAttributes(p)
	return err == nil && attrs&windows.FILE_ATTRIBUTE_REPARSE_POINT == 0
}
func creationSecurity(dir bool) (*windows.SECURITY_DESCRIPTOR, error) {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil || user.User.Sid == nil {
		return nil, ErrUnsafe
	}
	sid := user.User.Sid.String()
	inherit := ""
	if dir {
		inherit = "OICI"
	}
	sd, err := windows.SecurityDescriptorFromString("O:" + sid + "D:P(A;" + inherit + ";FA;;;" + sid + ")")
	if err != nil {
		return nil, ErrUnsafe
	}
	return sd, nil
}
func makePrivateDir(path string) error {
	sd, err := creationSecurity(true)
	if err != nil {
		return err
	}
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	sa := windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), SecurityDescriptor: sd}
	err = windows.CreateDirectory(p, &sa)
	runtime.KeepAlive(sd)
	return err
}
func createPrivateFile(path string) (*os.File, error) {
	sd, err := creationSecurity(false)
	if err != nil {
		return nil, err
	}
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	sa := windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), SecurityDescriptor: sd}
	h, err := windows.CreateFile(p, windows.GENERIC_WRITE|windows.READ_CONTROL, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, &sa, windows.CREATE_NEW, windows.FILE_ATTRIBUTE_NORMAL|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	runtime.KeepAlive(sd)
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(h), path), nil
}
func openObject(path string, dir bool) (*os.File, error) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	access := uint32(windows.GENERIC_READ | windows.READ_CONTROL)
	flags := uint32(windows.FILE_FLAG_OPEN_REPARSE_POINT)
	if dir {
		access = windows.READ_CONTROL
		flags |= windows.FILE_FLAG_BACKUP_SEMANTICS
	}
	h, err := windows.CreateFile(p, access, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil, windows.OPEN_EXISTING, flags, 0)
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(h), path), nil
}
func checkPermissions(f *os.File, dir bool) error {
	h := windows.Handle(f.Fd())
	var info windows.ByHandleFileInformation
	if windows.GetFileInformationByHandle(h, &info) != nil || info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		return ErrUnsafe
	}
	isDir := info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY != 0
	if isDir != dir || !dir && info.NumberOfLinks != 1 {
		return ErrUnsafe
	}
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil || user.User.Sid == nil {
		return ErrUnsafe
	}
	sd, err := windows.GetSecurityInfo(h, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	if err != nil || sd == nil {
		return ErrUnsafe
	}
	owner, _, err := sd.Owner()
	if err != nil || owner == nil || !windows.EqualSid(owner, user.User.Sid) {
		return ErrUnsafe
	}
	control, _, err := sd.Control()
	if err != nil || dir && control&windows.SE_DACL_PROTECTED == 0 {
		return ErrUnsafe
	}
	acl, _, err := sd.DACL()
	if err != nil || acl == nil || acl.AceCount == 0 {
		return ErrUnsafe
	}
	inheritable := false
	for i := uint16(0); i < acl.AceCount; i++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if windows.GetAce(acl, uint32(i), &ace) != nil || ace == nil || ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE {
			return ErrUnsafe
		}
		if !windows.EqualSid((*windows.SID)(unsafe.Pointer(&ace.SidStart)), user.User.Sid) {
			return ErrUnsafe
		}
		if ace.Header.AceFlags&windows.INHERIT_ONLY_ACE != 0 {
			return ErrUnsafe
		}
		if dir && ace.Header.AceFlags&(windows.OBJECT_INHERIT_ACE|windows.CONTAINER_INHERIT_ACE) == windows.OBJECT_INHERIT_ACE|windows.CONTAINER_INHERIT_ACE && ace.Header.AceFlags&windows.NO_PROPAGATE_INHERIT_ACE == 0 {
			inheritable = true
		}
	}
	runtime.KeepAlive(sd)
	if dir && !inheritable {
		return ErrUnsafe
	}
	return nil
}
func protectEmpty(file *os.File) error {
	if file == nil {
		return ErrUnsafe
	}
	original := windows.Handle(file.Fd())
	if original == windows.InvalidHandle {
		return ErrUnsafe
	}
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(original, &info); err != nil || info.FileAttributes&(windows.FILE_ATTRIBUTE_DIRECTORY|windows.FILE_ATTRIBUTE_REPARSE_POINT) != 0 || info.NumberOfLinks != 1 || info.FileSizeHigh != 0 || info.FileSizeLow != 0 {
		return ErrUnsafe
	}
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil || user.User.Sid == nil {
		return ErrUnsafe
	}
	var pinner runtime.Pinner
	pinner.Pin(user.User.Sid)
	defer pinner.Unpin()
	dacl, err := windows.ACLFromEntries([]windows.EXPLICIT_ACCESS{{AccessPermissions: privateFileAccess, AccessMode: windows.SET_ACCESS, Inheritance: windows.NO_INHERITANCE, Trustee: windows.TRUSTEE{TrusteeForm: windows.TRUSTEE_IS_SID, TrusteeType: windows.TRUSTEE_IS_USER, TrusteeValue: windows.TrusteeValueFromSID(user.User.Sid)}}}, nil)
	if err != nil || dacl == nil {
		return ErrUnsafe
	}
	if err := privateReOpenFile.Find(); err != nil {
		return ErrUnsafe
	}
	// The original caller writes through an O_WRONLY handle. Deny read sharing
	// while changing security: an already-open reader must make this reopen
	// fail, because changing a DACL does not revoke existing handle rights.
	// Include FILE_WRITE_DATA so this reopen participates in Windows' data
	// sharing checks; security-only handle requests do not exclude readers.
	opened, _, _ := privateReOpenFile.Call(uintptr(original), uintptr(windows.WRITE_DAC|windows.WRITE_OWNER|windows.READ_CONTROL|windows.FILE_WRITE_DATA), uintptr(windows.FILE_SHARE_WRITE), uintptr(windows.FILE_FLAG_OPEN_REPARSE_POINT))
	handle := windows.Handle(opened)
	if handle == windows.InvalidHandle {
		return ErrUnsafe
	}
	defer windows.CloseHandle(handle)
	if err := windows.SetSecurityInfo(handle, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, user.User.Sid, nil, dacl, nil); err != nil {
		return ErrUnsafe
	}
	// Read back on the original handle that the caller will write to. Never
	// permit token persistence if the filesystem did not apply the intended ACL.
	sd, err := windows.GetSecurityInfo(original, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	if err != nil || sd == nil {
		return ErrUnsafe
	}
	owner, _, err := sd.Owner()
	if err != nil || owner == nil || !windows.EqualSid(owner, user.User.Sid) {
		return ErrUnsafe
	}
	control, _, err := sd.Control()
	if err != nil || control&windows.SE_DACL_PROTECTED == 0 {
		return ErrUnsafe
	}
	actual, _, err := sd.DACL()
	if err != nil || actual == nil || actual.AceCount != 1 {
		return ErrUnsafe
	}
	var ace *windows.ACCESS_ALLOWED_ACE
	if err := windows.GetAce(actual, 0, &ace); err != nil || ace == nil || ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE || ace.Header.AceFlags != 0 || ace.Mask != privateFileAccess {
		return ErrUnsafe
	}
	if !windows.EqualSid((*windows.SID)(unsafe.Pointer(&ace.SidStart)), user.User.Sid) {
		return ErrUnsafe
	}
	runtime.KeepAlive(sd)
	runtime.KeepAlive(file)
	return nil
}
