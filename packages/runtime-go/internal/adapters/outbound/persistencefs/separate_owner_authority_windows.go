//go:build windows

package persistencefs

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
	"unsafe"

	"golang.org/x/sys/windows"
)

type separateOwnerWindowsACLHeader struct {
	revision byte
	padding1 byte
	size     uint16
	aceCount uint16
	padding2 uint16
}

type separateOwnerWindowsStreamHeader struct {
	nextOffset     uint32
	nameLength     uint32
	streamSize     int64
	allocationSize int64
}

func platformSeparateOwnerDirectoryBinding(path string, requirePrivateRoot bool) (string, string, error) {
	handle, err := separateOwnerWindowsOpenAbsoluteRead(path)
	if err != nil {
		return "", "", err
	}
	defer windows.CloseHandle(handle)
	identity, err := windowsOpenedObjectIdentity(handle, true)
	if err != nil {
		return "", "", errors.New("separate-owner Windows directory identity is unavailable")
	}
	securityDigest, err := separateOwnerWindowsSecurityDigest(handle, true, requirePrivateRoot, false)
	if err != nil {
		return "", "", err
	}
	return identity, securityDigest, nil
}

func separateOwnerWindowsOpenAbsoluteRead(path string) (windows.Handle, error) {
	path = filepath.Clean(path)
	volume := filepath.VolumeName(path)
	if volume == "" || !filepath.IsAbs(path) {
		return 0, errors.New("separate-owner Windows path is invalid")
	}
	access := uint32(windows.FILE_TRAVERSE | windows.FILE_READ_ATTRIBUTES | windows.READ_CONTROL | windows.SYNCHRONIZE)
	volumeRoot := volume + string(os.PathSeparator)
	if err := separateOwnerWindowsValidateVolume(volumeRoot); err != nil {
		return 0, err
	}
	current, err := secureWindowsOpenAbsolute(volumeRoot, access, windows.FILE_OPEN, true)
	if err != nil {
		return 0, err
	}
	remainder := strings.Trim(strings.TrimPrefix(path, volume), `\/`)
	components := strings.FieldsFunc(remainder, func(char rune) bool { return char == '\\' || char == '/' })
	for _, component := range components {
		if !separateOwnerWindowsComponent(component) {
			_ = windows.CloseHandle(current)
			return 0, errors.New("separate-owner Windows path component is invalid")
		}
		next, openErr := secureWindowsOpenRelative(current, component, access, windows.FILE_OPEN, true)
		if openErr == nil {
			openErr = separateOwnerWindowsVerifyOpenedComponent(next, component)
		}
		_ = windows.CloseHandle(current)
		if openErr != nil {
			if next != 0 {
				_ = windows.CloseHandle(next)
			}
			return 0, openErr
		}
		current = next
	}
	return current, nil
}

func separateOwnerWindowsSecurityDigest(
	handle windows.Handle,
	directory bool,
	requireCurrentOwner bool,
	requireProtected bool,
) (string, error) {
	if requireProtected && !requireCurrentOwner {
		return "", errors.New("separate-owner Windows private object must be owned by the current host SID")
	}
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(handle, &info); err != nil ||
		info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 ||
		(directory && info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY == 0) ||
		(!directory && info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY != 0) ||
		(!directory && info.NumberOfLinks != 1) {
		return "", errors.New("separate-owner Windows object metadata is unsafe")
	}
	if err := separateOwnerWindowsValidateStreams(handle, directory); err != nil {
		return "", err
	}
	descriptor, err := windows.GetSecurityInfo(
		handle,
		windows.SE_FILE_OBJECT,
		windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION,
	)
	if err != nil || descriptor == nil || !descriptor.IsValid() {
		return "", errors.New("separate-owner Windows security descriptor is invalid")
	}
	owner, _, err := descriptor.Owner()
	if err != nil || owner == nil || !owner.IsValid() {
		return "", errors.New("separate-owner Windows owner SID is invalid")
	}
	if requireCurrentOwner {
		token, tokenErr := windows.OpenCurrentProcessToken()
		if tokenErr != nil {
			return "", tokenErr
		}
		user, userErr := token.GetTokenUser()
		_ = token.Close()
		if userErr != nil || user == nil || user.User.Sid == nil || !owner.Equals(user.User.Sid) {
			return "", errors.New("separate-owner Windows object is not owned by the current host SID")
		}
		if err := separateOwnerWindowsRejectUntrustedWriters(descriptor, owner); err != nil {
			return "", err
		}
	}
	control, _, err := descriptor.Control()
	if err != nil || requireProtected && control&windows.SE_DACL_PROTECTED == 0 {
		return "", errors.New("separate-owner Windows private DACL is not protected")
	}
	if requireProtected {
		if err := separateOwnerWindowsValidatePrivateDACL(descriptor, owner); err != nil {
			return "", err
		}
	}
	sddl := descriptor.String()
	if sddl == "" {
		return "", errors.New("separate-owner Windows security descriptor is not canonical")
	}
	body, _ := json.Marshal(struct {
		SDDL       string `json:"sddl"`
		Attributes uint32 `json:"attributes"`
	}{sddl, info.FileAttributes})
	return separateOwnerSHA256(body), nil
}

func separateOwnerWindowsProjectedPrivateSecurityDigest(handle windows.Handle, directory bool) (string, error) {
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(handle, &info); err != nil ||
		info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 ||
		(directory && info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY == 0) ||
		(!directory && info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY != 0) ||
		(!directory && info.NumberOfLinks != 1) {
		return "", errors.New("separate-owner Windows projected object metadata is unsafe")
	}
	if err := separateOwnerWindowsValidateStreams(handle, directory); err != nil {
		return "", err
	}
	descriptor, err := separateOwnerWindowsPrivateDescriptor()
	if err != nil || descriptor == nil || !descriptor.IsValid() {
		return "", errors.Join(errors.New("separate-owner Windows projected private descriptor is invalid"), err)
	}
	sddl := descriptor.String()
	if sddl == "" {
		return "", errors.New("separate-owner Windows projected private descriptor is not canonical")
	}
	body, _ := json.Marshal(struct {
		SDDL       string `json:"sddl"`
		Attributes uint32 `json:"attributes"`
	}{sddl, info.FileAttributes})
	return separateOwnerSHA256(body), nil
}

func separateOwnerWindowsValidatePrivateDACL(descriptor *windows.SECURITY_DESCRIPTOR, owner *windows.SID) error {
	dacl, _, err := descriptor.DACL()
	if err != nil || dacl == nil {
		return errors.New("separate-owner Windows private DACL is invalid")
	}
	header := (*separateOwnerWindowsACLHeader)(unsafe.Pointer(dacl))
	for index := uint32(0); index < uint32(header.aceCount); index++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(dacl, index, &ace); err != nil || ace == nil ||
			ace.Header.AceFlags&windows.INHERITED_ACE != 0 {
			return errors.New("separate-owner Windows private DACL contains an invalid or inherited ACE")
		}
		if ace.Header.AceType == windows.ACCESS_DENIED_ACE_TYPE {
			continue
		}
		if ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE {
			return errors.New("separate-owner Windows private DACL ACE type is unsupported")
		}
		sid := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
		if ace.Mask != 0 && (sid == nil || !sid.IsValid() || !separateOwnerWindowsTrustedSID(sid, owner)) {
			return errors.New("separate-owner Windows private DACL grants access outside host authority")
		}
	}
	return nil
}

func separateOwnerWindowsRejectUntrustedWriters(descriptor *windows.SECURITY_DESCRIPTOR, owner *windows.SID) error {
	dacl, _, err := descriptor.DACL()
	if err != nil || dacl == nil {
		return errors.New("separate-owner Windows DACL is invalid")
	}
	header := (*separateOwnerWindowsACLHeader)(unsafe.Pointer(dacl))
	writeMask := windows.ACCESS_MASK(windows.GENERIC_WRITE | windows.GENERIC_ALL | windows.WRITE_DAC | windows.WRITE_OWNER |
		windows.DELETE | windows.FILE_WRITE_DATA | windows.FILE_APPEND_DATA | windows.FILE_WRITE_EA |
		windows.FILE_WRITE_ATTRIBUTES | 0x00000040)
	for index := uint32(0); index < uint32(header.aceCount); index++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(dacl, index, &ace); err != nil || ace == nil {
			return errors.New("separate-owner Windows DACL entry is invalid")
		}
		if ace.Header.AceType == windows.ACCESS_DENIED_ACE_TYPE || ace.Header.AceFlags&windows.INHERIT_ONLY_ACE != 0 || ace.Mask&writeMask == 0 {
			continue
		}
		if ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE {
			return errors.New("separate-owner Windows writable DACL entry type is unsupported")
		}
		sid := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
		if sid == nil || !sid.IsValid() || !separateOwnerWindowsTrustedSID(sid, owner) {
			return errors.New("separate-owner Windows DACL grants write access outside host authority")
		}
	}
	return nil
}

func separateOwnerWindowsTrustedSID(sid, owner *windows.SID) bool {
	return sid.Equals(owner) || sid.IsWellKnown(windows.WinLocalSystemSid) ||
		sid.IsWellKnown(windows.WinBuiltinAdministratorsSid)
}

func separateOwnerWindowsValidateStreams(handle windows.Handle, directory bool) error {
	buffer := make([]byte, 4096)
	err := windows.GetFileInformationByHandleEx(handle, windows.FileStreamInfo, &buffer[0], uint32(len(buffer)))
	if errors.Is(err, windows.ERROR_HANDLE_EOF) && directory {
		return nil
	}
	if err != nil {
		return err
	}
	offset := 0
	count := 0
	for {
		if offset > len(buffer)-int(unsafe.Sizeof(separateOwnerWindowsStreamHeader{})) {
			return errors.New("separate-owner Windows stream information is truncated")
		}
		header := (*separateOwnerWindowsStreamHeader)(unsafe.Pointer(&buffer[offset]))
		nameStart := offset + int(unsafe.Sizeof(separateOwnerWindowsStreamHeader{}))
		nameLength := int(header.nameLength)
		if nameLength <= 0 || nameLength%2 != 0 || nameStart > len(buffer)-nameLength {
			return errors.New("separate-owner Windows stream name is invalid")
		}
		units := unsafe.Slice((*uint16)(unsafe.Pointer(&buffer[nameStart])), nameLength/2)
		for _, unit := range units {
			if unit == 0 {
				return errors.New("separate-owner Windows stream name contains NUL")
			}
		}
		if string(utf16.Decode(units)) != "::$DATA" {
			return errors.New("separate-owner Windows named data stream is forbidden")
		}
		count++
		if count > 1 {
			return errors.New("separate-owner Windows has multiple data streams")
		}
		if header.nextOffset == 0 {
			break
		}
		next := int(header.nextOffset)
		if next%8 != 0 || next < int(unsafe.Sizeof(separateOwnerWindowsStreamHeader{}))+nameLength || offset > len(buffer)-next {
			return errors.New("separate-owner Windows stream offset is invalid")
		}
		offset += next
	}
	if !directory && count != 1 {
		return errors.New("separate-owner Windows default stream is missing")
	}
	return nil
}

func separateOwnerWindowsPrivateDescriptor() (*windows.SECURITY_DESCRIPTOR, error) {
	token, err := windows.OpenCurrentProcessToken()
	if err != nil {
		return nil, err
	}
	defer token.Close()
	user, err := token.GetTokenUser()
	if err != nil || user == nil || user.User.Sid == nil || !user.User.Sid.IsValid() {
		return nil, errors.New("separate-owner Windows current host SID is invalid")
	}
	sid := user.User.Sid.String()
	return windows.SecurityDescriptorFromString(
		"O:" + sid + "D:P(A;;FA;;;" + sid + ")(A;;FA;;;SY)(A;;FA;;;BA)",
	)
}

func separateOwnerWindowsValidateVolume(volumeRoot string) error {
	pointer, err := windows.UTF16PtrFromString(volumeRoot)
	if err != nil || windows.GetDriveType(pointer) != windows.DRIVE_FIXED {
		return errors.New("separate-owner Windows root is not on a fixed local volume")
	}
	var flags uint32
	if err := windows.GetVolumeInformation(pointer, nil, 0, nil, nil, &flags, nil, 0); err != nil ||
		flags&windows.FILE_PERSISTENT_ACLS == 0 || flags&windows.FILE_NAMED_STREAMS == 0 ||
		flags&windows.FILE_READ_ONLY_VOLUME != 0 {
		return errors.New("separate-owner Windows volume lacks required ACL or stream semantics")
	}
	return nil
}

func separateOwnerWindowsComponent(component string) bool {
	if component == "" || component == "." || component == ".." || !utf8.ValidString(component) ||
		strings.ContainsAny(component, `\/:<>"|?*`) || strings.TrimRight(component, ". ") != component {
		return false
	}
	for _, value := range component {
		if value < 0x20 {
			return false
		}
	}
	base := component
	if index := strings.IndexByte(base, '.'); index >= 0 {
		base = base[:index]
	}
	switch strings.ToUpper(base) {
	case "CON", "PRN", "AUX", "NUL",
		"COM1", "COM2", "COM3", "COM4", "COM5", "COM6", "COM7", "COM8", "COM9",
		"LPT1", "LPT2", "LPT3", "LPT4", "LPT5", "LPT6", "LPT7", "LPT8", "LPT9",
		"COM¹", "COM²", "COM³", "LPT¹", "LPT²", "LPT³":
		return false
	default:
		return true
	}
}
