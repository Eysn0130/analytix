//go:build windows

package finalauthority

import (
	"errors"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
)

type privateWindowsFileIDInfo struct {
	VolumeSerialNumber uint64
	FileID             [16]byte
}

type privateWindowsObjectIdentity struct {
	id            privateWindowsFileIDInfo
	attributes    uint32
	links         uint32
	sizeHigh      uint32
	sizeLow       uint32
	creationTime  windows.Filetime
	lastWriteTime windows.Filetime
}

type privateWindowsAuthorityACLHeader struct {
	revision byte
	padding1 byte
	size     uint16
	aceCount uint16
	padding2 uint16
}

type privateWindowsAuthorityStreamHeader struct {
	nextOffset     uint32
	nameLength     uint32
	streamSize     int64
	allocationSize int64
}

func privateWindowsValidateAuthorityObject(
	handle windows.Handle,
	directory bool,
	maxBytes uint64,
) (privateWindowsObjectIdentity, error) {
	return privateWindowsValidateAuthorityObjectWithOptions(handle, directory, maxBytes, false, false)
}

func privateWindowsValidateAuthorityObjectWithOptions(
	handle windows.Handle,
	directory bool,
	maxBytes uint64,
	allowEmpty bool,
	allowRecoveryLink bool,
) (privateWindowsObjectIdentity, error) {
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(handle, &info); err != nil ||
		info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 ||
		(directory && info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY == 0) ||
		(!directory && info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY != 0) ||
		(!directory && !allowRecoveryLink && info.NumberOfLinks != 1) ||
		(!directory && allowRecoveryLink && (info.NumberOfLinks == 0 || info.NumberOfLinks > 2)) {
		return privateWindowsObjectIdentity{}, errors.New("private authority Windows object type, reparse state, or links are unsafe")
	}
	size := uint64(info.FileSizeHigh)<<32 | uint64(info.FileSizeLow)
	if !directory && (maxBytes == 0 || !allowEmpty && size == 0 || size > maxBytes) {
		return privateWindowsObjectIdentity{}, errors.New("private authority Windows file size is invalid")
	}
	if err := privateWindowsValidateProtectedDACL(handle); err != nil ||
		privateWindowsValidateStreams(handle, directory) != nil {
		return privateWindowsObjectIdentity{}, errors.New("private authority Windows object ACL or streams are unsafe")
	}
	id, err := privateWindowsHandleFileID(handle, directory)
	if err != nil {
		return privateWindowsObjectIdentity{}, errors.New("private authority Windows FileID is unavailable")
	}
	return privateWindowsObjectIdentity{
		id: id, attributes: info.FileAttributes, links: info.NumberOfLinks,
		sizeHigh: info.FileSizeHigh, sizeLow: info.FileSizeLow,
		creationTime: info.CreationTime, lastWriteTime: info.LastWriteTime,
	}, nil
}

func privateWindowsHandleFileID(handle windows.Handle, directory bool) (privateWindowsFileIDInfo, error) {
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(handle, &info); err != nil ||
		info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 ||
		(directory && info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY == 0) ||
		(!directory && info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY != 0) {
		return privateWindowsFileIDInfo{}, errors.New("private authority Windows handle type is unsafe")
	}
	var id privateWindowsFileIDInfo
	if err := windows.GetFileInformationByHandleEx(
		handle, windows.FileIdInfo, (*byte)(unsafe.Pointer(&id)), uint32(unsafe.Sizeof(id)),
	); err != nil || id.VolumeSerialNumber == 0 || id.FileID == ([16]byte{}) {
		return privateWindowsFileIDInfo{}, errors.New("private authority Windows FileID is unavailable")
	}
	return id, nil
}

func privateWindowsValidateProtectedDACL(handle windows.Handle) error {
	descriptor, err := windows.GetSecurityInfo(
		handle, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION,
	)
	if err != nil {
		return err
	}
	owner, _, err := descriptor.Owner()
	if err != nil || owner == nil || !owner.IsValid() {
		return errors.New("private authority Windows owner is invalid")
	}
	token, err := windows.OpenCurrentProcessToken()
	if err != nil {
		return err
	}
	defer token.Close()
	user, err := token.GetTokenUser()
	if err != nil || user == nil || user.User.Sid == nil || !owner.Equals(user.User.Sid) {
		return errors.New("private authority Windows owner is not the current host user")
	}
	control, _, err := descriptor.Control()
	if err != nil || control&windows.SE_DACL_PROTECTED == 0 {
		return errors.New("private authority Windows DACL is not protected")
	}
	dacl, _, err := descriptor.DACL()
	if err != nil || dacl == nil {
		return errors.New("private authority Windows DACL is invalid")
	}
	header := (*privateWindowsAuthorityACLHeader)(unsafe.Pointer(dacl))
	for index := uint32(0); index < uint32(header.aceCount); index++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(dacl, index, &ace); err != nil || ace == nil ||
			ace.Header.AceFlags&windows.INHERITED_ACE != 0 {
			return errors.New("private authority Windows DACL entry is invalid or inherited")
		}
		if ace.Header.AceType == windows.ACCESS_DENIED_ACE_TYPE {
			continue
		}
		if ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE {
			return errors.New("private authority Windows DACL entry type is unsupported")
		}
		sid := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
		if sid == nil || !sid.IsValid() || !privateWindowsTrustedAuthoritySID(sid, owner) {
			return errors.New("private authority Windows DACL grants access outside host authority")
		}
	}
	return nil
}

func privateWindowsTrustedAuthoritySID(sid, owner *windows.SID) bool {
	return sid.Equals(owner) || sid.IsWellKnown(windows.WinLocalSystemSid) ||
		sid.IsWellKnown(windows.WinBuiltinAdministratorsSid)
}

func privateWindowsValidateStreams(handle windows.Handle, directory bool) error {
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
		if offset > len(buffer)-int(unsafe.Sizeof(privateWindowsAuthorityStreamHeader{})) {
			return errors.New("private authority Windows stream information is truncated")
		}
		header := (*privateWindowsAuthorityStreamHeader)(unsafe.Pointer(&buffer[offset]))
		nameStart := offset + int(unsafe.Sizeof(privateWindowsAuthorityStreamHeader{}))
		nameLength := int(header.nameLength)
		if nameLength <= 0 || nameLength%2 != 0 || nameStart > len(buffer)-nameLength {
			return errors.New("private authority Windows stream name is invalid")
		}
		units := unsafe.Slice((*uint16)(unsafe.Pointer(&buffer[nameStart])), nameLength/2)
		for _, unit := range units {
			if unit == 0 {
				return errors.New("private authority Windows stream name contains NUL")
			}
		}
		if string(utf16.Decode(units)) != "::$DATA" {
			return errors.New("private authority Windows named data stream is forbidden")
		}
		count++
		if count > 1 {
			return errors.New("private authority Windows has multiple data streams")
		}
		if header.nextOffset == 0 {
			break
		}
		next := int(header.nextOffset)
		if next%8 != 0 || next < int(unsafe.Sizeof(privateWindowsAuthorityStreamHeader{}))+nameLength ||
			offset > len(buffer)-next {
			return errors.New("private authority Windows stream offset is invalid")
		}
		offset += next
	}
	if !directory && count != 1 {
		return errors.New("private authority Windows default stream is missing")
	}
	return nil
}

func privateWindowsSameObject(left, right privateWindowsObjectIdentity) bool {
	return left.id == right.id && left.attributes == right.attributes && left.links == right.links &&
		left.sizeHigh == right.sizeHigh && left.sizeLow == right.sizeLow
}
