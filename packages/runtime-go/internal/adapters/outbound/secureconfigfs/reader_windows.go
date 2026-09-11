//go:build windows

package secureconfigfs

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
	"unsafe"

	"golang.org/x/sys/windows"
)

const windowsShareAll = windows.FILE_SHARE_READ | windows.FILE_SHARE_WRITE | windows.FILE_SHARE_DELETE

type windowsFileIDInfo struct {
	VolumeSerialNumber uint64
	FileID             [16]byte
}

type windowsIdentity struct {
	id            windowsFileIDInfo
	attributes    uint32
	links         uint32
	sizeHigh      uint32
	sizeLow       uint32
	creationTime  windows.Filetime
	lastWriteTime windows.Filetime
}

type windowsACLHeader struct {
	revision byte
	padding1 byte
	size     uint16
	aceCount uint16
	padding2 uint16
}

type windowsStreamHeader struct {
	nextOffset     uint32
	nameLength     uint32
	streamSize     int64
	allocationSize int64
}

func readBundle(input normalizedBundle) (map[string][]byte, error) {
	failed := true
	var bodies map[string][]byte
	defer func() {
		if failed {
			wipeSensitiveWindowsBundle(input.files, bodies)
		}
	}()
	if !windowsBundleNamesSafe(input) {
		return nil, errors.New("secure configuration Windows inventory name is invalid")
	}
	root, err := windowsOpenAbsoluteDirectory(input.root)
	if err != nil {
		return nil, err
	}
	defer windows.CloseHandle(root)
	rootIdentity, err := windowsValidateObject(root, true, 0)
	if err != nil {
		return nil, err
	}
	if err := windowsValidateInventory(root, input.expectedNames); err != nil {
		return nil, err
	}
	bodies = make(map[string][]byte, len(input.files))
	identities := make(map[string]windowsIdentity, len(input.files))
	var totalBytes int64
	for _, target := range input.files {
		body, identity, readErr := windowsReadNamed(root, target.Name, target.MaxBytes)
		if readErr != nil || int64(len(body)) > input.maxTotalBytes-totalBytes {
			return nil, errors.New("secure configuration Windows bundle read failed")
		}
		totalBytes += int64(len(body))
		bodies[target.Name] = body
		identities[target.Name] = identity
	}
	for _, target := range input.files {
		body, identity, readErr := windowsReadNamed(root, target.Name, target.MaxBytes)
		equal := readErr == nil && identity == identities[target.Name] && bytes.Equal(body, bodies[target.Name])
		if target.Sensitive {
			clear(body)
		}
		if !equal {
			return nil, errors.New("secure configuration Windows bundle changed during read")
		}
	}
	currentRoot, err := windowsValidateObject(root, true, 0)
	if err != nil || currentRoot != rootIdentity || windowsValidateInventory(root, input.expectedNames) != nil {
		return nil, errors.New("secure configuration Windows root changed during read")
	}
	reopened, err := windowsOpenAbsoluteDirectory(input.root)
	if err != nil {
		return nil, errors.New("secure configuration Windows root changed during read")
	}
	defer windows.CloseHandle(reopened)
	reopenedIdentity, err := windowsValidateObject(reopened, true, 0)
	if err != nil || reopenedIdentity != rootIdentity || windowsValidateInventory(reopened, input.expectedNames) != nil {
		return nil, errors.New("secure configuration Windows root changed during read")
	}
	for _, target := range input.files {
		readback, identity, readErr := windowsReadNamed(reopened, target.Name, target.MaxBytes)
		if target.Sensitive {
			clear(readback)
		}
		if readErr != nil || identity != identities[target.Name] {
			return nil, errors.New("secure configuration Windows file name changed during read")
		}
	}
	failed = false
	return bodies, nil
}

func wipeSensitiveWindowsBundle(files []BundleFile, bodies map[string][]byte) {
	for _, file := range files {
		if file.Sensitive {
			clear(bodies[file.Name])
		}
	}
}

func windowsBundleNamesSafe(input normalizedBundle) bool {
	seen := make(map[string]struct{}, len(input.expectedNames))
	for _, name := range input.expectedNames {
		if !windowsSafeInventoryName(name) {
			return false
		}
		folded := strings.ToUpper(name)
		if _, duplicate := seen[folded]; duplicate {
			return false
		}
		seen[folded] = struct{}{}
	}
	for _, file := range input.files {
		if !windowsSafeInventoryName(file.Name) {
			return false
		}
	}
	return true
}

func windowsOpenAbsoluteDirectory(path string) (windows.Handle, error) {
	if filepath.Clean(path) != path || !filepath.IsAbs(path) || strings.HasPrefix(path, `\\`) {
		return 0, errors.New("secure configuration Windows root is invalid")
	}
	volume := filepath.VolumeName(path)
	if len(volume) != 2 || volume[1] != ':' || !windowsASCIILetter(volume[0]) {
		return 0, errors.New("secure configuration Windows volume is invalid")
	}
	volumeRoot := volume + `\`
	volumeRootPointer, err := windows.UTF16PtrFromString(volumeRoot)
	if err != nil || windows.GetDriveType(volumeRootPointer) != windows.DRIVE_FIXED {
		return 0, errors.New("secure configuration Windows root is not on a fixed local drive")
	}
	current, err := windowsOpenAbsolute(volumeRoot, windows.FILE_GENERIC_READ, true)
	if err != nil {
		return 0, err
	}
	remainder := strings.Trim(strings.TrimPrefix(path, volume), `\/`)
	for _, component := range strings.FieldsFunc(remainder, func(value rune) bool { return value == '\\' || value == '/' }) {
		if !windowsSafeComponent(component) {
			_ = windows.CloseHandle(current)
			return 0, errors.New("secure configuration Windows root component is invalid")
		}
		next, openErr := windowsOpenRelative(current, component, windows.FILE_GENERIC_READ, true)
		_ = windows.CloseHandle(current)
		if openErr != nil {
			return 0, openErr
		}
		current = next
	}
	return current, nil
}

func windowsOpenAbsolute(path string, access uint32, directory bool) (windows.Handle, error) {
	name, err := windows.NewNTUnicodeString(`\??\` + path)
	if err != nil {
		return 0, err
	}
	attributes := &windows.OBJECT_ATTRIBUTES{ObjectName: name, Attributes: windows.OBJ_CASE_INSENSITIVE | windows.OBJ_DONT_REPARSE}
	attributes.Length = uint32(unsafe.Sizeof(*attributes))
	return windowsNTCreate(attributes, access, directory)
}

func windowsOpenRelative(parent windows.Handle, name string, access uint32, directory bool) (windows.Handle, error) {
	if !windowsSafeComponent(name) {
		return 0, errors.New("secure configuration Windows relative name is invalid")
	}
	objectName, err := windows.NewNTUnicodeString(name)
	if err != nil {
		return 0, err
	}
	attributes := &windows.OBJECT_ATTRIBUTES{
		RootDirectory: parent, ObjectName: objectName,
		Attributes: windows.OBJ_CASE_INSENSITIVE | windows.OBJ_DONT_REPARSE,
	}
	attributes.Length = uint32(unsafe.Sizeof(*attributes))
	return windowsNTCreate(attributes, access, directory)
}

func windowsNTCreate(attributes *windows.OBJECT_ATTRIBUTES, access uint32, directory bool) (windows.Handle, error) {
	options := uint32(windows.FILE_OPEN_REPARSE_POINT | windows.FILE_SYNCHRONOUS_IO_NONALERT)
	fileAttributes := uint32(windows.FILE_ATTRIBUTE_NORMAL)
	if directory {
		options |= windows.FILE_DIRECTORY_FILE
		fileAttributes = windows.FILE_ATTRIBUTE_DIRECTORY
	} else {
		options |= windows.FILE_NON_DIRECTORY_FILE
	}
	var handle windows.Handle
	var status windows.IO_STATUS_BLOCK
	allocation := int64(0)
	if err := windows.NtCreateFile(
		&handle, access, attributes, &status, &allocation, fileAttributes,
		windowsShareAll, windows.FILE_OPEN, options, 0, 0,
	); err != nil {
		return 0, err
	}
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(handle, &info); err != nil ||
		info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 ||
		(directory && info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY == 0) ||
		(!directory && info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY != 0) {
		_ = windows.CloseHandle(handle)
		return 0, errors.New("secure configuration Windows handle crossed a reparse boundary")
	}
	return handle, nil
}

func windowsValidateObject(handle windows.Handle, directory bool, maxBytes int64) (windowsIdentity, error) {
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(handle, &info); err != nil ||
		info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 ||
		(directory && info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY == 0) ||
		(!directory && info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY != 0) ||
		(!directory && info.NumberOfLinks != 1) {
		return windowsIdentity{}, errors.New("secure configuration Windows object type, reparse state, or links are unsafe")
	}
	size := uint64(info.FileSizeHigh)<<32 | uint64(info.FileSizeLow)
	if !directory && (maxBytes <= 0 || size == 0 || size > uint64(maxBytes)) {
		return windowsIdentity{}, errors.New("secure configuration Windows file size is invalid")
	}
	if err := windowsValidatePrivateDACL(handle); err != nil || windowsValidateStreams(handle, directory) != nil {
		return windowsIdentity{}, errors.New("secure configuration Windows object ACL or streams are unsafe")
	}
	var id windowsFileIDInfo
	if err := windows.GetFileInformationByHandleEx(
		handle, windows.FileIdInfo, (*byte)(unsafe.Pointer(&id)), uint32(unsafe.Sizeof(id)),
	); err != nil || id.VolumeSerialNumber == 0 || id.FileID == ([16]byte{}) {
		return windowsIdentity{}, errors.New("secure configuration Windows FileID is unavailable")
	}
	return windowsIdentity{
		id: id, attributes: info.FileAttributes, links: info.NumberOfLinks,
		sizeHigh: info.FileSizeHigh, sizeLow: info.FileSizeLow,
		creationTime: info.CreationTime, lastWriteTime: info.LastWriteTime,
	}, nil
}

func windowsReadNamed(parent windows.Handle, name string, maxBytes int64) ([]byte, windowsIdentity, error) {
	handle, err := windowsOpenRelative(parent, name, windows.FILE_GENERIC_READ, false)
	if err != nil {
		return nil, windowsIdentity{}, err
	}
	file := os.NewFile(uintptr(handle), name)
	if file == nil {
		_ = windows.CloseHandle(handle)
		return nil, windowsIdentity{}, errors.New("secure configuration Windows file handle is invalid")
	}
	defer file.Close()
	identity, err := windowsValidateObject(handle, false, maxBytes)
	if err != nil {
		return nil, windowsIdentity{}, err
	}
	body, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
	if err != nil || uint64(len(body)) != uint64(identity.sizeHigh)<<32|uint64(identity.sizeLow) {
		return nil, windowsIdentity{}, errors.New("secure configuration Windows exact read failed")
	}
	current, err := windowsValidateObject(handle, false, maxBytes)
	if err != nil || current != identity {
		return nil, windowsIdentity{}, errors.New("secure configuration Windows file changed during read")
	}
	return body, identity, nil
}

func windowsValidateInventory(root windows.Handle, expected []string) error {
	process := windows.CurrentProcess()
	var duplicate windows.Handle
	if err := windows.DuplicateHandle(process, root, process, &duplicate, 0, false, windows.DUPLICATE_SAME_ACCESS); err != nil {
		return err
	}
	directory := os.NewFile(uintptr(duplicate), "secure-config-windows-root")
	if directory == nil {
		_ = windows.CloseHandle(duplicate)
		return errors.New("secure configuration Windows directory handle is invalid")
	}
	entries, readErr := directory.ReadDir(len(expected) + 1)
	closeErr := directory.Close()
	if readErr != nil && !errors.Is(readErr, io.EOF) || closeErr != nil {
		return errors.Join(readErr, closeErr)
	}
	if len(entries) != len(expected) {
		return errors.New("secure configuration Windows root contains unknown or missing files")
	}
	names := make([]string, 0, len(entries))
	seen := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		folded := strings.ToUpper(name)
		if entry.IsDir() || !windowsSafeInventoryName(name) {
			return errors.New("secure configuration Windows inventory entry is unsafe")
		}
		if _, duplicate := seen[folded]; duplicate {
			return errors.New("secure configuration Windows inventory aliases by case")
		}
		seen[folded] = struct{}{}
		names = append(names, name)
	}
	sort.Strings(names)
	for index := range names {
		if names[index] != expected[index] {
			return errors.New("secure configuration Windows inventory casing or name differs")
		}
	}
	return nil
}

func windowsValidatePrivateDACL(handle windows.Handle) error {
	descriptor, err := windows.GetSecurityInfo(
		handle, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION,
	)
	if err != nil {
		return err
	}
	owner, _, err := descriptor.Owner()
	if err != nil || owner == nil || !owner.IsValid() {
		return errors.New("secure configuration Windows owner is invalid")
	}
	token, err := windows.OpenCurrentProcessToken()
	if err != nil {
		return err
	}
	defer token.Close()
	user, err := token.GetTokenUser()
	if err != nil || user == nil || user.User.Sid == nil || !owner.Equals(user.User.Sid) {
		return errors.New("secure configuration Windows owner is not the current host user")
	}
	dacl, _, err := descriptor.DACL()
	if err != nil || dacl == nil {
		return errors.New("secure configuration Windows DACL is invalid")
	}
	header := (*windowsACLHeader)(unsafe.Pointer(dacl))
	for index := uint32(0); index < uint32(header.aceCount); index++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(dacl, index, &ace); err != nil || ace == nil {
			return errors.New("secure configuration Windows DACL entry is invalid")
		}
		if ace.Header.AceType == windows.ACCESS_DENIED_ACE_TYPE {
			continue
		}
		if ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE {
			return errors.New("secure configuration Windows DACL entry type is unsupported")
		}
		if ace.Mask == 0 {
			continue
		}
		sid := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
		if sid == nil || !sid.IsValid() || !windowsTrustedAuthoritySID(sid, owner) {
			return errors.New("secure configuration Windows DACL grants access outside host authority")
		}
	}
	return nil
}

func windowsTrustedAuthoritySID(sid, owner *windows.SID) bool {
	return sid.Equals(owner) || sid.IsWellKnown(windows.WinLocalSystemSid) || sid.IsWellKnown(windows.WinBuiltinAdministratorsSid)
}

func windowsValidateStreams(handle windows.Handle, directory bool) error {
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
		if offset > len(buffer)-int(unsafe.Sizeof(windowsStreamHeader{})) {
			return errors.New("secure configuration Windows stream information is truncated")
		}
		header := (*windowsStreamHeader)(unsafe.Pointer(&buffer[offset]))
		nameStart := offset + int(unsafe.Sizeof(windowsStreamHeader{}))
		nameLength := int(header.nameLength)
		if nameLength <= 0 || nameLength%2 != 0 || nameStart > len(buffer)-nameLength {
			return errors.New("secure configuration Windows stream name is invalid")
		}
		units := unsafe.Slice((*uint16)(unsafe.Pointer(&buffer[nameStart])), nameLength/2)
		for _, unit := range units {
			if unit == 0 {
				return errors.New("secure configuration Windows stream name contains NUL")
			}
		}
		if string(utf16.Decode(units)) != "::$DATA" {
			return errors.New("secure configuration Windows named data stream is forbidden")
		}
		count++
		if count > 1 {
			return errors.New("secure configuration Windows has multiple data streams")
		}
		if header.nextOffset == 0 {
			break
		}
		next := int(header.nextOffset)
		if next%8 != 0 || next < int(unsafe.Sizeof(windowsStreamHeader{}))+nameLength || offset > len(buffer)-next {
			return errors.New("secure configuration Windows stream offset is invalid")
		}
		offset += next
	}
	if !directory && count != 1 {
		return errors.New("secure configuration Windows default stream is missing")
	}
	return nil
}

func windowsSafeInventoryName(name string) bool {
	if !windowsSafeComponent(name) || name != strings.ToLower(name) || len(name) > 128 {
		return false
	}
	for _, value := range []byte(name) {
		if value >= utf8.RuneSelf || !(value >= 'a' && value <= 'z' || value >= '0' && value <= '9' || value == '.' || value == '_' || value == '-') {
			return false
		}
	}
	return true
}

func windowsSafeComponent(component string) bool {
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
	base = strings.ToUpper(base)
	switch base {
	case "CON", "PRN", "AUX", "NUL",
		"COM1", "COM2", "COM3", "COM4", "COM5", "COM6", "COM7", "COM8", "COM9",
		"LPT1", "LPT2", "LPT3", "LPT4", "LPT5", "LPT6", "LPT7", "LPT8", "LPT9",
		"COM¹", "COM²", "COM³", "LPT¹", "LPT²", "LPT³":
		return false
	default:
		return true
	}
}

func windowsASCIILetter(value byte) bool {
	return value >= 'a' && value <= 'z' || value >= 'A' && value <= 'Z'
}
