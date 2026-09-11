//go:build windows

package finalauthority

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

func secureReadExistingPrivateNamedFile(path, name string, maxBytes int) ([]byte, existingFileAuthorityIdentity, error) {
	if !privateWindowsComponent(name) || maxBytes <= 0 {
		return nil, existingFileAuthorityIdentity{}, errors.New("existing private named authority Windows input is invalid")
	}
	absolute, err := filepath.Abs(strings.TrimSpace(path))
	if err != nil || strings.TrimSpace(path) == "" {
		return nil, existingFileAuthorityIdentity{}, errors.New("existing private authority Windows root is invalid")
	}
	root, err := privateWindowsOpenAbsoluteDirectoryReadOnly(filepath.Clean(absolute))
	if privateWindowsNotFound(err) {
		return nil, existingFileAuthorityIdentity{}, os.ErrNotExist
	}
	if err != nil {
		return nil, existingFileAuthorityIdentity{}, err
	}
	defer windows.CloseHandle(root)
	if err := validateExistingPrivateWindowsAuthority(root); err != nil {
		return nil, existingFileAuthorityIdentity{}, err
	}
	rootID, err := privateWindowsHandleFileID(root, true)
	if err != nil {
		return nil, existingFileAuthorityIdentity{}, errors.New("existing private authority Windows root identity is invalid")
	}
	entries, err := privateCASWindowsReadDirBounded(root, 1)
	if err != nil {
		return nil, existingFileAuthorityIdentity{}, err
	}
	if len(entries) == 0 {
		return nil, existingFileAuthorityIdentity{}, os.ErrNotExist
	}
	if !existingPrivateWindowsInventoryExact(entries, name) {
		return nil, existingFileAuthorityIdentity{}, errors.New("existing private authority Windows root contains recovery residue or unknown state")
	}
	body, identity, err := privateWindowsReadExistingNamedAt(root, name, uint64(maxBytes))
	if err != nil {
		return nil, existingFileAuthorityIdentity{}, err
	}
	readback, current, err := privateWindowsReadExistingNamedAt(root, name, uint64(maxBytes))
	if err != nil || identity != current || !bytes.Equal(body, readback) {
		return nil, existingFileAuthorityIdentity{}, errors.New("existing private authority Windows file changed during read")
	}
	entries, err = privateCASWindowsReadDirBounded(root, 1)
	currentRootID, currentRootErr := privateWindowsHandleFileID(root, true)
	if err != nil || !existingPrivateWindowsInventoryExact(entries, name) || validateExistingPrivateWindowsAuthority(root) != nil ||
		currentRootErr != nil || currentRootID != rootID {
		return nil, existingFileAuthorityIdentity{}, errors.New("existing private authority Windows root changed during read")
	}
	return body, existingFileAuthorityIdentity{
		rootVolume: rootID.VolumeSerialNumber, rootFileID: rootID.FileID,
		fileVolume: identity.id.VolumeSerialNumber, fileFileID: identity.id.FileID,
		fileSize: uint64(identity.sizeHigh)<<32 | uint64(identity.sizeLow),
	}, nil
}

func existingPrivateWindowsInventoryExact(entries []os.DirEntry, name string) bool {
	return len(entries) == 1 && entries[0].Name() == name && !entries[0].IsDir()
}

func privateWindowsOpenAbsoluteDirectoryReadOnly(path string) (windows.Handle, error) {
	path = filepath.Clean(path)
	volume := filepath.VolumeName(path)
	if volume == "" || !filepath.IsAbs(path) {
		return 0, errors.New("existing private authority Windows root is invalid")
	}
	current, err := privateWindowsOpenAbsolute(volume+string(os.PathSeparator), windows.FILE_GENERIC_READ, windows.FILE_OPEN, true)
	if err != nil {
		return 0, err
	}
	remainder := strings.Trim(strings.TrimPrefix(path, volume), `\/`)
	for _, component := range strings.FieldsFunc(remainder, func(char rune) bool { return char == '\\' || char == '/' }) {
		if !privateWindowsComponent(component) {
			_ = windows.CloseHandle(current)
			return 0, errors.New("existing private authority Windows root component is invalid")
		}
		next, openErr := privateWindowsOpenRelative(current, component, windows.FILE_GENERIC_READ, windows.FILE_OPEN, true)
		_ = windows.CloseHandle(current)
		if openErr != nil {
			return 0, openErr
		}
		current = next
	}
	return current, nil
}

type existingPrivateWindowsIdentity struct {
	id       privateWindowsFileIDInfo
	sizeHigh uint32
	sizeLow  uint32
}

func privateWindowsReadExistingNamedAt(parent windows.Handle, name string, maxBytes uint64) ([]byte, existingPrivateWindowsIdentity, error) {
	handle, err := privateWindowsOpenRelative(parent, name, windows.FILE_GENERIC_READ, windows.FILE_OPEN, false)
	if privateWindowsNotFound(err) {
		return nil, existingPrivateWindowsIdentity{}, os.ErrNotExist
	}
	if err != nil {
		return nil, existingPrivateWindowsIdentity{}, err
	}
	file := os.NewFile(uintptr(handle), name)
	if file == nil {
		_ = windows.CloseHandle(handle)
		return nil, existingPrivateWindowsIdentity{}, errors.New("existing private authority Windows file handle is invalid")
	}
	defer file.Close()
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(handle, &info); err != nil || info.NumberOfLinks != 1 ||
		info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 || info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY != 0 {
		return nil, existingPrivateWindowsIdentity{}, errors.New("existing private authority Windows file type or links are unsafe")
	}
	size := uint64(info.FileSizeHigh)<<32 | uint64(info.FileSizeLow)
	if size == 0 || size > maxBytes {
		return nil, existingPrivateWindowsIdentity{}, errors.New("existing private authority Windows file size is invalid")
	}
	if err := validateExistingPrivateWindowsAuthority(handle); err != nil {
		return nil, existingPrivateWindowsIdentity{}, err
	}
	identity, err := privateWindowsHandleFileID(handle, false)
	if err != nil {
		return nil, existingPrivateWindowsIdentity{}, err
	}
	body, err := io.ReadAll(io.LimitReader(file, int64(maxBytes)+1))
	if err != nil || uint64(len(body)) != size {
		return nil, existingPrivateWindowsIdentity{}, errors.New("existing private authority Windows exact read failed")
	}
	return body, existingPrivateWindowsIdentity{
		id: identity, sizeHigh: info.FileSizeHigh, sizeLow: info.FileSizeLow,
	}, nil
}

type existingPrivateWindowsACLHeader struct {
	revision byte
	padding1 byte
	size     uint16
	aceCount uint16
	padding2 uint16
}

func validateExistingPrivateWindowsAuthority(handle windows.Handle) error {
	descriptor, err := windows.GetSecurityInfo(handle, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return err
	}
	owner, _, err := descriptor.Owner()
	if err != nil || owner == nil || !owner.IsValid() {
		return errors.New("existing private authority Windows owner is invalid")
	}
	token, err := windows.OpenCurrentProcessToken()
	if err != nil {
		return err
	}
	defer token.Close()
	user, err := token.GetTokenUser()
	if err != nil {
		return err
	}
	if user == nil || user.User.Sid == nil || !owner.Equals(user.User.Sid) {
		return errors.New("existing private authority Windows owner is not the current host user")
	}
	dacl, _, err := descriptor.DACL()
	if err != nil || dacl == nil {
		return errors.New("existing private authority Windows DACL is invalid")
	}
	header := (*existingPrivateWindowsACLHeader)(unsafe.Pointer(dacl))
	const writableMask = uint32(windows.GENERIC_WRITE | windows.GENERIC_ALL |
		windows.FILE_WRITE_DATA | windows.FILE_APPEND_DATA | windows.FILE_WRITE_EA | windows.FILE_WRITE_ATTRIBUTES |
		windows.DELETE | windows.WRITE_DAC | windows.WRITE_OWNER | 0x00000040)
	for index := uint32(0); index < uint32(header.aceCount); index++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(dacl, index, &ace); err != nil || ace == nil {
			return errors.New("existing private authority Windows DACL entry is invalid")
		}
		if ace.Header.AceType == windows.ACCESS_DENIED_ACE_TYPE {
			continue
		}
		if ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE {
			return errors.New("existing private authority Windows DACL entry type is unsupported")
		}
		if uint32(ace.Mask)&writableMask == 0 {
			continue
		}
		sid := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
		if sid == nil || !sid.IsValid() || !existingPrivateWindowsTrustedWriter(sid, owner) {
			return errors.New("existing private authority Windows DACL grants write access outside host authority")
		}
	}
	return nil
}

func existingPrivateWindowsTrustedWriter(sid, owner *windows.SID) bool {
	return sid.Equals(owner) || sid.IsWellKnown(windows.WinCreatorOwnerSid) ||
		sid.IsWellKnown(windows.WinLocalSystemSid) || sid.IsWellKnown(windows.WinBuiltinAdministratorsSid)
}
