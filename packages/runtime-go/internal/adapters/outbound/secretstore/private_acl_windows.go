//go:build windows

package secretstore

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

type privateWindowsACLHeader struct {
	revision byte
	padding  byte
	size     uint16
	aceCount uint16
	padding2 uint16
}

func privateWindowsSecurityAttributes() (windows.SecurityAttributes, error) {
	token, err := windows.OpenCurrentProcessToken()
	if err != nil {
		return windows.SecurityAttributes{}, err
	}
	defer token.Close()
	user, err := token.GetTokenUser()
	if err != nil || user == nil || user.User.Sid == nil || !user.User.Sid.IsValid() {
		return windows.SecurityAttributes{}, errors.New("private Windows owner is unavailable")
	}
	// Protect the DACL against later inheritance changes. Administrators and
	// LocalSystem retain their normal machine recovery access.
	descriptor, err := windows.SecurityDescriptorFromString(
		"O:" + user.User.Sid.String() + "D:P(A;OICI;FA;;;" + user.User.Sid.String() + ")(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)",
	)
	if err != nil {
		return windows.SecurityAttributes{}, err
	}
	return windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), SecurityDescriptor: descriptor}, nil
}

func validatePrivateWindowsSecurity(handle windows.Handle) error {
	return validateWindowsOwnerAndDACL(handle, true)
}

func hardenPrivateWindowsSecurity(handle windows.Handle) error {
	// Existing DPAPI profiles may have inherited read grants from a parent.
	// Admit only the current owner's non-reparse object with no outside writer,
	// then protect its DACL without changing its bytes or master key.
	if err := validateWindowsOwnerAndDACL(handle, false); err != nil {
		return err
	}
	attributes, err := privateWindowsSecurityAttributes()
	if err != nil {
		return err
	}
	dacl, _, err := attributes.SecurityDescriptor.DACL()
	if err != nil || dacl == nil {
		return errors.New("private Windows DACL construction failed")
	}
	if err := windows.SetSecurityInfo(handle, windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION,
		nil, nil, dacl, nil); err != nil {
		return err
	}
	return validatePrivateWindowsSecurity(handle)
}

func validateWindowsOwnerAndDACL(handle windows.Handle, private bool) error {
	descriptor, err := windows.GetSecurityInfo(handle, windows.SE_FILE_OBJECT,
		windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	if err != nil || descriptor == nil {
		return errors.New("private Windows security descriptor is unavailable")
	}
	owner, _, err := descriptor.Owner()
	if err != nil || owner == nil || !owner.IsValid() {
		return errors.New("private Windows owner is invalid")
	}
	token, err := windows.OpenCurrentProcessToken()
	if err != nil {
		return err
	}
	defer token.Close()
	user, err := token.GetTokenUser()
	// A non-private ancestor may be owned by Administrators or LocalSystem
	// (notably the elevated Windows runner's temporary root). Its DACL still
	// must exclude outside writers. The private subtree always requires the
	// current user as owner.
	trustedAncestorOwner := !private && (owner.IsWellKnown(windows.WinBuiltinAdministratorsSid) ||
		owner.IsWellKnown(windows.WinLocalSystemSid))
	if err != nil || user == nil || user.User.Sid == nil ||
		(!owner.Equals(user.User.Sid) && !trustedAncestorOwner) {
		return errors.New("private Windows owner is not the current user")
	}
	dacl, _, err := descriptor.DACL()
	if err != nil || dacl == nil {
		return errors.New("private Windows DACL is unavailable")
	}
	header := (*privateWindowsACLHeader)(unsafe.Pointer(dacl))
	if header.aceCount == 0 {
		return errors.New("private Windows DACL denies the owner")
	}
	ownerAllowed := false
	for index := uint32(0); index < uint32(header.aceCount); index++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(dacl, index, &ace); err != nil || ace == nil {
			return errors.New("private Windows DACL entry is invalid")
		}
		if ace.Header.AceType == windows.ACCESS_DENIED_ACE_TYPE {
			continue
		}
		if ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE {
			return errors.New("private Windows DACL entry type is unsupported")
		}
		sid := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
		trusted := sid != nil && sid.IsValid() &&
			(sid.Equals(owner) || sid.IsWellKnown(windows.WinLocalSystemSid) || sid.IsWellKnown(windows.WinBuiltinAdministratorsSid) || sid.IsWellKnown(windows.WinCreatorOwnerSid))
		const writable = windows.GENERIC_WRITE | windows.GENERIC_ALL | windows.FILE_GENERIC_WRITE |
			windows.FILE_WRITE_DATA | windows.FILE_APPEND_DATA | windows.FILE_WRITE_EA |
			windows.FILE_WRITE_ATTRIBUTES | windows.DELETE | windows.WRITE_DAC |
			windows.WRITE_OWNER | 0x00000040 // FILE_DELETE_CHILD
		if !trusted && (private || uint32(ace.Mask)&writable != 0) {
			return errors.New("private Windows DACL grants access outside the owner and machine authorities")
		}
		ownerAllowed = ownerAllowed || sid.Equals(owner)
	}
	if private && !ownerAllowed {
		return errors.New("private Windows DACL does not admit the owner")
	}
	return nil
}

func openPrivateWindowsDirectory(path string) (windows.Handle, error) {
	return openWindowsDirectory(path, true)
}

func privateWindowsDirectoryName(path string) bool {
	switch filepath.Base(path) {
	case "private", "provider-secrets", "provider-secrets-v2", "master-key":
		return true
	default:
		return false
	}
}

// ValidateWindowsDevelopmentAuthorityDirectory checks the existing shared
// source-development authority before Core opens it. Existing directory ACLs
// may be narrowed, but the directory and its credential bytes are retained.
func ValidateWindowsDevelopmentAuthorityDirectory(path string) error {
	if err := validatePrivateWindowsPath(path); err != nil {
		return err
	}
	handle, err := openPrivateWindowsDirectory(path)
	if err != nil {
		return err
	}
	return windows.CloseHandle(handle)
}

func openWindowsDirectory(path string, private bool) (windows.Handle, error) {
	if private {
		if err := validatePrivateWindowsExactName(path); err != nil {
			return 0, err
		}
	}
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, err
	}
	access := uint32(windows.FILE_READ_ATTRIBUTES | windows.READ_CONTROL)
	if private {
		access |= windows.WRITE_DAC
	}
	handle, err := windows.CreateFile(name, access,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil,
		windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		return 0, err
	}
	var information windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(handle, &information); err != nil ||
		information.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY == 0 ||
		information.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		_ = windows.CloseHandle(handle)
		return 0, errors.New("private Windows directory is invalid or a reparse point")
	}
	if err := validateWindowsOwnerAndDACL(handle, private); err != nil {
		if !private || hardenPrivateWindowsSecurity(handle) != nil {
			_ = windows.CloseHandle(handle)
			return 0, err
		}
	}
	return handle, nil
}

func createPrivateWindowsFile(path string) (*os.File, error) {
	attributes, err := privateWindowsSecurityAttributes()
	if err != nil {
		return nil, err
	}
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	handle, err := windows.CreateFile(name, windows.GENERIC_READ|windows.GENERIC_WRITE|windows.DELETE,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_DELETE, &attributes, windows.CREATE_NEW,
		windows.FILE_ATTRIBUTE_NORMAL|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(handle), path)
	if file == nil {
		_ = windows.CloseHandle(handle)
		return nil, errors.New("private Windows file open failed")
	}
	if err := validatePrivateWindowsSecurity(handle); err != nil {
		_ = file.Close()
		return nil, err
	}
	return file, nil
}

func createPrivateWindowsTempFile(directory, base string) (*os.File, error) {
	for attempt := 0; attempt < 8; attempt++ {
		var suffix [16]byte
		if _, err := rand.Read(suffix[:]); err != nil {
			return nil, err
		}
		file, err := createPrivateWindowsFile(filepath.Join(directory, "."+base+".tmp-"+hex.EncodeToString(suffix[:])))
		if errors.Is(err, windows.ERROR_FILE_EXISTS) || errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
			continue
		}
		return file, err
	}
	return nil, errors.New("private Windows temporary name collision")
}

func validatePrivateWindowsPath(path string) error {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path || strings.HasPrefix(path, `\\`) ||
		strings.Contains(path[len(filepath.VolumeName(path)):], ":") {
		return errors.New("private Windows path is not canonical")
	}
	for directory := filepath.Dir(path); directory != filepath.Dir(directory); directory = filepath.Dir(directory) {
		name, err := windows.UTF16PtrFromString(directory)
		if err != nil {
			return err
		}
		attributes, err := windows.GetFileAttributes(name)
		if err == nil && attributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
			return errors.New("private Windows path traverses a reparse point")
		}
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}

func validatePrivateWindowsExactName(path string) error {
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		return err
	}
	base := filepath.Base(path)
	for _, entry := range entries {
		if strings.EqualFold(entry.Name(), base) {
			if entry.Name() == base {
				return nil
			}
			return errors.New("private Windows path has a case alias")
		}
	}
	return os.ErrNotExist
}
