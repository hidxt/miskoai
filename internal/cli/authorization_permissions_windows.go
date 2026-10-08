//go:build windows

package cli

import (
	"errors"
	"os"
	"runtime"
	"unsafe"

	"golang.org/x/sys/windows"
)

var errAuthorizationPermissions = errors.New("authorization file permissions could not be secured")

// ReOpenFile operates on the existing object, avoiding a path-based reopen race.
// x/sys/windows v0.48.0 does not expose this kernel32 entry point.
var authorizationReOpenFile = windows.NewLazySystemDLL("kernel32.dll").NewProc("ReOpenFile")

const authorizationFileAccess windows.ACCESS_MASK = windows.STANDARD_RIGHTS_REQUIRED | windows.SYNCHRONIZE | 0x1ff

// protectAuthorizationFile must succeed before writing any credential bytes.
// It replaces all inherited/explicit grants with a protected current-user DACL
// on the open object, sets that user's ownership, then verifies the result.
// Windows administrators may still take ownership using their OS privileges.
func protectAuthorizationFile(file *os.File) error {
	if file == nil {
		return errAuthorizationPermissions
	}
	original := windows.Handle(file.Fd())
	if original == windows.InvalidHandle {
		return errAuthorizationPermissions
	}
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(original, &info); err != nil || info.FileAttributes&(windows.FILE_ATTRIBUTE_DIRECTORY|windows.FILE_ATTRIBUTE_REPARSE_POINT) != 0 || info.NumberOfLinks != 1 || info.FileSizeHigh != 0 || info.FileSizeLow != 0 {
		return errAuthorizationPermissions
	}
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil || user.User.Sid == nil {
		return errAuthorizationPermissions
	}
	var pinner runtime.Pinner
	pinner.Pin(user.User.Sid)
	defer pinner.Unpin()
	dacl, err := windows.ACLFromEntries([]windows.EXPLICIT_ACCESS{{AccessPermissions: authorizationFileAccess, AccessMode: windows.SET_ACCESS, Inheritance: windows.NO_INHERITANCE, Trustee: windows.TRUSTEE{TrusteeForm: windows.TRUSTEE_IS_SID, TrusteeType: windows.TRUSTEE_IS_USER, TrusteeValue: windows.TrusteeValueFromSID(user.User.Sid)}}}, nil)
	if err != nil || dacl == nil {
		return errAuthorizationPermissions
	}
	if err := authorizationReOpenFile.Find(); err != nil {
		return errAuthorizationPermissions
	}
	// The original caller writes through an O_WRONLY handle. Deny read sharing
	// while changing security: an already-open reader must make this reopen
	// fail, because changing a DACL does not revoke existing handle rights.
	// Include FILE_WRITE_DATA so this reopen participates in Windows' data
	// sharing checks; security-only handle requests do not exclude readers.
	opened, _, _ := authorizationReOpenFile.Call(uintptr(original), uintptr(windows.WRITE_DAC|windows.WRITE_OWNER|windows.READ_CONTROL|windows.FILE_WRITE_DATA), uintptr(windows.FILE_SHARE_WRITE), uintptr(windows.FILE_FLAG_OPEN_REPARSE_POINT))
	handle := windows.Handle(opened)
	if handle == windows.InvalidHandle {
		return errAuthorizationPermissions
	}
	defer windows.CloseHandle(handle)
	if err := windows.SetSecurityInfo(handle, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, user.User.Sid, nil, dacl, nil); err != nil {
		return errAuthorizationPermissions
	}
	// Read back on the original handle that the caller will write to. Never
	// permit token persistence if the filesystem did not apply the intended ACL.
	sd, err := windows.GetSecurityInfo(original, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	if err != nil || sd == nil {
		return errAuthorizationPermissions
	}
	owner, _, err := sd.Owner()
	if err != nil || owner == nil || !windows.EqualSid(owner, user.User.Sid) {
		return errAuthorizationPermissions
	}
	control, _, err := sd.Control()
	if err != nil || control&windows.SE_DACL_PROTECTED == 0 {
		return errAuthorizationPermissions
	}
	actual, _, err := sd.DACL()
	if err != nil || actual == nil || actual.AceCount != 1 {
		return errAuthorizationPermissions
	}
	var ace *windows.ACCESS_ALLOWED_ACE
	if err := windows.GetAce(actual, 0, &ace); err != nil || ace == nil || ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE || ace.Header.AceFlags != 0 || ace.Mask != authorizationFileAccess {
		return errAuthorizationPermissions
	}
	if !windows.EqualSid((*windows.SID)(unsafe.Pointer(&ace.SidStart)), user.User.Sid) {
		return errAuthorizationPermissions
	}
	runtime.KeepAlive(sd)
	runtime.KeepAlive(file)
	return nil
}
