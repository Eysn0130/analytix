//go:build windows

package filestore

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unicode/utf16"
	"unicode/utf8"
	"unsafe"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	"golang.org/x/sys/windows"
)

var caseBindingWindowsReOpenFile = syscall.NewLazyDLL("kernel32.dll").NewProc("ReOpenFile")

const (
	caseBindingFileName = "case-project.json"

	// Retained case-binding handles deliberately do not share write or delete
	// access. This turns an attempted in-place write, rename, or replacement
	// during observation into a failed operation instead of a last-window race.
	caseBindingWindowsShare = windows.FILE_SHARE_READ

	caseBindingWindowsDirectoryAccess = windows.FILE_TRAVERSE |
		windows.FILE_LIST_DIRECTORY |
		windows.FILE_READ_ATTRIBUTES |
		windows.READ_CONTROL |
		windows.SYNCHRONIZE
	caseBindingWindowsFileAccess = windows.FILE_GENERIC_READ |
		windows.READ_CONTROL |
		windows.SYNCHRONIZE
	caseBindingWindowsMaxMetadataEntries = 4096
)

type caseBindingWindowsFileID struct {
	VolumeSerialNumber uint64
	FileID             [16]byte
}

type caseBindingWindowsIdentity struct {
	id            caseBindingWindowsFileID
	attributes    uint32
	links         uint32
	sizeHigh      uint32
	sizeLow       uint32
	creationTime  windows.Filetime
	lastWriteTime windows.Filetime
}

type caseBindingWindowsACLHeader struct {
	revision byte
	padding1 byte
	size     uint16
	aceCount uint16
	padding2 uint16
}

type caseBindingWindowsStreamHeader struct {
	nextOffset     uint32
	nameLength     uint32
	streamSize     int64
	allocationSize int64
}

func secureReadCaseBinding(workspaceRealPath string, maxBytes int, hook caseBindingReadHook) ([]byte, error) {
	workspace, err := openCaseBindingWindowsAbsoluteDirectory(workspaceRealPath)
	if err != nil {
		return nil, classifyCaseBindingWindowsPathError(err, domainsecurity.CaseBindingStateWorkspaceMissing)
	}
	defer windows.CloseHandle(workspace)
	workspaceIdentity, err := validateCaseBindingWindowsObject(workspace, true, 0, false)
	if err != nil {
		return nil, newCaseBindingReadError(domainsecurity.CaseBindingStateInvalid, err)
	}

	metadata, err := openCaseBindingWindowsRelative(
		workspace,
		workspaceHostMetadataDir,
		caseBindingWindowsDirectoryAccess,
		true,
	)
	if err != nil {
		return nil, classifyCaseBindingWindowsPathError(err, domainsecurity.CaseBindingStateMissing)
	}
	defer windows.CloseHandle(metadata)
	metadataIdentity, err := validateCaseBindingWindowsObject(metadata, true, 0, true)
	if err != nil {
		return nil, newCaseBindingReadError(domainsecurity.CaseBindingStateInvalid, err)
	}
	if err := validateCaseBindingWindowsExactName(metadata); err != nil {
		return nil, classifyCaseBindingWindowsPathError(err, domainsecurity.CaseBindingStateMissing)
	}

	file, err := openCaseBindingWindowsRelative(
		metadata,
		caseBindingFileName,
		caseBindingWindowsFileAccess,
		false,
	)
	if err != nil {
		return nil, classifyCaseBindingWindowsPathError(err, domainsecurity.CaseBindingStateMissing)
	}
	fileObject := os.NewFile(uintptr(file), caseBindingFileName)
	if fileObject == nil {
		_ = windows.CloseHandle(file)
		return nil, newCaseBindingReadError(
			domainsecurity.CaseBindingStateUnreadable,
			errors.New("case binding Windows file handle is invalid"),
		)
	}
	defer fileObject.Close()

	body, fileIdentity, err := readCaseBindingWindowsHandle(fileObject, maxBytes)
	if err != nil {
		return nil, err
	}
	if hook != nil {
		hook("after_initial_read")
	}
	if err := validateCaseBindingWindowsPath(
		workspaceRealPath,
		workspace,
		workspaceIdentity,
		metadata,
		metadataIdentity,
		file,
		fileIdentity,
	); err != nil {
		return nil, newCaseBindingReadError(domainsecurity.CaseBindingStateUnstable, err)
	}

	readbackHandle, err := openCaseBindingWindowsRelative(
		metadata,
		caseBindingFileName,
		caseBindingWindowsFileAccess,
		false,
	)
	if err != nil {
		return nil, newCaseBindingReadError(domainsecurity.CaseBindingStateUnstable, err)
	}
	readbackObject := os.NewFile(uintptr(readbackHandle), caseBindingFileName)
	if readbackObject == nil {
		_ = windows.CloseHandle(readbackHandle)
		return nil, newCaseBindingReadError(
			domainsecurity.CaseBindingStateUnstable,
			errors.New("case binding Windows readback handle is invalid"),
		)
	}
	defer readbackObject.Close()
	readback, readbackIdentity, err := readCaseBindingWindowsHandle(readbackObject, maxBytes)
	if err != nil || readbackIdentity != fileIdentity || !bytes.Equal(body, readback) {
		return nil, newCaseBindingReadError(
			domainsecurity.CaseBindingStateUnstable,
			errors.Join(err, errors.New("case binding changed during secure Windows readback")),
		)
	}

	if hook != nil {
		hook("before_final_validation")
	}
	if err := validateCaseBindingWindowsPath(
		workspaceRealPath,
		workspace,
		workspaceIdentity,
		metadata,
		metadataIdentity,
		readbackHandle,
		readbackIdentity,
	); err != nil {
		return nil, newCaseBindingReadError(domainsecurity.CaseBindingStateUnstable, err)
	}
	return body, nil
}

func openCaseBindingWindowsAbsoluteDirectory(path string) (windows.Handle, error) {
	path = filepath.Clean(path)
	if !filepath.IsAbs(path) || strings.HasPrefix(path, `\\`) {
		return 0, errors.New("case binding Windows workspace must be an absolute local path")
	}
	volume := filepath.VolumeName(path)
	if len(volume) != 2 || volume[1] != ':' || !caseBindingWindowsASCIILetter(volume[0]) {
		return 0, errors.New("case binding Windows workspace volume is invalid")
	}
	volumeRoot := volume + `\`
	volumeRootPointer, err := windows.UTF16PtrFromString(volumeRoot)
	if err != nil || windows.GetDriveType(volumeRootPointer) != windows.DRIVE_FIXED {
		return 0, errors.New("case binding Windows workspace is not on a fixed local drive")
	}
	current, err := openCaseBindingWindowsAbsolute(volumeRoot, caseBindingWindowsDirectoryAccess, true)
	if err != nil {
		return 0, err
	}
	remainder := strings.Trim(strings.TrimPrefix(path, volume), `\/`)
	for _, component := range strings.FieldsFunc(remainder, func(value rune) bool {
		return value == '\\' || value == '/'
	}) {
		if !caseBindingWindowsSafeComponent(component) {
			_ = windows.CloseHandle(current)
			return 0, errors.New("case binding Windows workspace component is invalid")
		}
		next, openErr := openCaseBindingWindowsRelative(
			current,
			component,
			caseBindingWindowsDirectoryAccess,
			true,
		)
		_ = windows.CloseHandle(current)
		if openErr != nil {
			return 0, openErr
		}
		current = next
	}
	return current, nil
}

func openCaseBindingWindowsAbsolute(path string, access uint32, directory bool) (windows.Handle, error) {
	name, err := windows.NewNTUnicodeString(`\??\` + path)
	if err != nil {
		return 0, err
	}
	attributes := &windows.OBJECT_ATTRIBUTES{
		ObjectName: name,
		Attributes: windows.OBJ_CASE_INSENSITIVE | windows.OBJ_DONT_REPARSE,
	}
	attributes.Length = uint32(unsafe.Sizeof(*attributes))
	return openCaseBindingWindowsObject(attributes, access, directory)
}

func openCaseBindingWindowsRelative(
	parent windows.Handle,
	name string,
	access uint32,
	directory bool,
) (windows.Handle, error) {
	if !caseBindingWindowsSafeComponent(name) {
		return 0, errors.New("case binding Windows relative name is invalid")
	}
	objectName, err := windows.NewNTUnicodeString(name)
	if err != nil {
		return 0, err
	}
	attributes := &windows.OBJECT_ATTRIBUTES{
		RootDirectory: parent,
		ObjectName:    objectName,
		Attributes:    windows.OBJ_CASE_INSENSITIVE | windows.OBJ_DONT_REPARSE,
	}
	attributes.Length = uint32(unsafe.Sizeof(*attributes))
	return openCaseBindingWindowsObject(attributes, access, directory)
}

func openCaseBindingWindowsObject(
	attributes *windows.OBJECT_ATTRIBUTES,
	access uint32,
	directory bool,
) (windows.Handle, error) {
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
		&handle,
		access,
		attributes,
		&status,
		&allocation,
		fileAttributes,
		caseBindingWindowsShare,
		windows.FILE_OPEN,
		options,
		0,
		0,
	); err != nil {
		return 0, err
	}
	if _, err := validateCaseBindingWindowsObject(handle, directory, 0, false); err != nil {
		_ = windows.CloseHandle(handle)
		return 0, err
	}
	return handle, nil
}

func readCaseBindingWindowsHandle(file *os.File, maxBytes int) ([]byte, caseBindingWindowsIdentity, error) {
	handle := windows.Handle(file.Fd())
	before, err := validateCaseBindingWindowsObject(handle, false, int64(maxBytes), true)
	if err != nil {
		return nil, caseBindingWindowsIdentity{}, newCaseBindingReadError(
			domainsecurity.CaseBindingStateInvalid,
			err,
		)
	}
	body, readErr := io.ReadAll(io.LimitReader(file, int64(maxBytes)+1))
	after, validationErr := validateCaseBindingWindowsObject(handle, false, int64(maxBytes), true)
	if readErr != nil {
		return nil, caseBindingWindowsIdentity{}, newCaseBindingReadError(
			domainsecurity.CaseBindingStateUnreadable,
			readErr,
		)
	}
	size := uint64(after.sizeHigh)<<32 | uint64(after.sizeLow)
	if validationErr != nil || before != after || len(body) == 0 ||
		len(body) > maxBytes || uint64(len(body)) != size {
		return nil, caseBindingWindowsIdentity{}, newCaseBindingReadError(
			domainsecurity.CaseBindingStateUnstable,
			errors.Join(validationErr, errors.New("case binding Windows file changed while being read")),
		)
	}
	return body, after, nil
}

func validateCaseBindingWindowsPath(
	workspacePath string,
	workspace windows.Handle,
	workspaceIdentity caseBindingWindowsIdentity,
	metadata windows.Handle,
	metadataIdentity caseBindingWindowsIdentity,
	file windows.Handle,
	fileIdentity caseBindingWindowsIdentity,
) error {
	currentWorkspace, err := validateCaseBindingWindowsObject(workspace, true, 0, false)
	if err != nil || currentWorkspace != workspaceIdentity {
		return errors.New("case binding Windows workspace changed during read")
	}
	currentMetadata, err := validateCaseBindingWindowsObject(metadata, true, 0, true)
	if err != nil || currentMetadata != metadataIdentity {
		return errors.New("case binding Windows metadata authority changed during read")
	}
	currentFile, err := validateCaseBindingWindowsObject(file, false, int64(maxCaseBindingBytes), true)
	if err != nil || currentFile != fileIdentity {
		return errors.New("case binding Windows file authority changed during read")
	}
	if err := validateCaseBindingWindowsExactName(metadata); err != nil {
		return err
	}
	reopened, err := openCaseBindingWindowsAbsoluteDirectory(workspacePath)
	if err != nil {
		return errors.New("case binding Windows workspace path changed during read")
	}
	defer windows.CloseHandle(reopened)
	reopenedIdentity, err := validateCaseBindingWindowsObject(reopened, true, 0, false)
	if err != nil || reopenedIdentity != workspaceIdentity {
		return errors.New("case binding Windows workspace path changed during read")
	}
	reopenedMetadata, err := openCaseBindingWindowsRelative(
		reopened,
		workspaceHostMetadataDir,
		caseBindingWindowsDirectoryAccess,
		true,
	)
	if err != nil {
		return errors.New("case binding Windows metadata path changed during read")
	}
	defer windows.CloseHandle(reopenedMetadata)
	reopenedMetadataIdentity, err := validateCaseBindingWindowsObject(reopenedMetadata, true, 0, true)
	if err != nil || reopenedMetadataIdentity != metadataIdentity {
		return errors.New("case binding Windows metadata path changed during read")
	}
	reopenedFile, err := openCaseBindingWindowsRelative(
		reopenedMetadata,
		caseBindingFileName,
		caseBindingWindowsFileAccess,
		false,
	)
	if err != nil {
		return errors.New("case binding Windows file path changed during read")
	}
	defer windows.CloseHandle(reopenedFile)
	reopenedFileIdentity, err := validateCaseBindingWindowsObject(
		reopenedFile,
		false,
		int64(maxCaseBindingBytes),
		true,
	)
	if err != nil || reopenedFileIdentity != fileIdentity {
		return errors.New("case binding Windows file path changed during read")
	}
	return validateCaseBindingWindowsExactName(reopenedMetadata)
}

func validateCaseBindingWindowsObject(
	handle windows.Handle,
	directory bool,
	maxBytes int64,
	private bool,
) (caseBindingWindowsIdentity, error) {
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(handle, &info); err != nil ||
		info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 ||
		(directory && info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY == 0) ||
		(!directory && info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY != 0) ||
		(!directory && info.NumberOfLinks != 1) {
		return caseBindingWindowsIdentity{}, errors.New(
			"case binding Windows object type, reparse state, or single-link authority is unsafe",
		)
	}
	size := uint64(info.FileSizeHigh)<<32 | uint64(info.FileSizeLow)
	if !directory && maxBytes > 0 && (size == 0 || size > uint64(maxBytes)) {
		return caseBindingWindowsIdentity{}, errors.New("case binding Windows file size is invalid")
	}
	if private {
		if err := validateCaseBindingWindowsHandleOwner(handle); err != nil {
			return caseBindingWindowsIdentity{}, err
		}
		if err := validateCaseBindingWindowsStreams(handle, directory); err != nil {
			return caseBindingWindowsIdentity{}, err
		}
	}
	var id caseBindingWindowsFileID
	if err := windows.GetFileInformationByHandleEx(
		handle,
		windows.FileIdInfo,
		(*byte)(unsafe.Pointer(&id)),
		uint32(unsafe.Sizeof(id)),
	); err != nil || id.VolumeSerialNumber == 0 || id.FileID == ([16]byte{}) {
		return caseBindingWindowsIdentity{}, errors.New("case binding Windows FileID is unavailable")
	}
	return caseBindingWindowsIdentity{
		id:            id,
		attributes:    info.FileAttributes,
		links:         info.NumberOfLinks,
		sizeHigh:      info.FileSizeHigh,
		sizeLow:       info.FileSizeLow,
		creationTime:  info.CreationTime,
		lastWriteTime: info.LastWriteTime,
	}, nil
}

func validateCaseBindingWindowsExactName(metadata windows.Handle) error {
	expected, err := validateCaseBindingWindowsObject(metadata, true, 0, true)
	if err != nil {
		return err
	}
	reopened, err := reopenCaseBindingWindowsDirectory(metadata)
	if err != nil {
		return err
	}
	current, err := validateCaseBindingWindowsObject(reopened, true, 0, true)
	if err != nil || current != expected {
		_ = windows.CloseHandle(reopened)
		return errors.New("case binding Windows metadata changed before inventory")
	}
	directory := os.NewFile(uintptr(reopened), "case-binding-metadata")
	if directory == nil {
		_ = windows.CloseHandle(reopened)
		return errors.New("case binding Windows metadata enumeration handle is invalid")
	}
	defer directory.Close()

	matches := 0
	entriesRead := 0
	for {
		entries, err := directory.ReadDir(256)
		entriesRead += len(entries)
		if entriesRead > caseBindingWindowsMaxMetadataEntries {
			return errors.New("case binding Windows metadata inventory is too large")
		}
		for _, entry := range entries {
			if strings.EqualFold(entry.Name(), caseBindingFileName) {
				if entry.Name() != caseBindingFileName || entry.IsDir() ||
					entry.Type()&os.ModeSymlink != 0 {
					return errors.New("case binding Windows file name aliases by case or type")
				}
				matches++
			}
		}
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
	}
	if matches != 1 {
		if matches == 0 {
			return os.ErrNotExist
		}
		return errors.New("case binding Windows file name is ambiguous")
	}
	return nil
}

func reopenCaseBindingWindowsDirectory(handle windows.Handle) (windows.Handle, error) {
	reopened, _, callErr := caseBindingWindowsReOpenFile.Call(
		uintptr(handle),
		uintptr(caseBindingWindowsDirectoryAccess),
		uintptr(caseBindingWindowsShare),
		uintptr(windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT),
	)
	result := windows.Handle(reopened)
	if result == windows.InvalidHandle {
		if callErr != nil && callErr != syscall.Errno(0) {
			return 0, callErr
		}
		return 0, errors.New("case binding Windows metadata handle could not be reopened")
	}
	return result, nil
}

func validateCaseBindingWindowsHandleOwner(handle windows.Handle) error {
	descriptor, err := windows.GetSecurityInfo(
		handle,
		windows.SE_FILE_OBJECT,
		windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION,
	)
	if err != nil {
		return newCaseBindingReadError(domainsecurity.CaseBindingStateUnreadable, err)
	}
	owner, _, err := descriptor.Owner()
	if err != nil || owner == nil || !owner.IsValid() {
		return newCaseBindingReadError(
			domainsecurity.CaseBindingStateInvalid,
			errors.New("case binding Windows owner is invalid"),
		)
	}
	token, err := windows.OpenCurrentProcessToken()
	if err != nil {
		return newCaseBindingReadError(domainsecurity.CaseBindingStateUnreadable, err)
	}
	defer token.Close()
	user, err := token.GetTokenUser()
	if err != nil {
		return newCaseBindingReadError(domainsecurity.CaseBindingStateUnreadable, err)
	}
	if user == nil || user.User.Sid == nil || !owner.Equals(user.User.Sid) {
		return newCaseBindingReadError(
			domainsecurity.CaseBindingStateInvalid,
			errors.New("case binding Windows owner is not the current host user"),
		)
	}
	if err := validateCaseBindingWindowsDACL(descriptor, owner); err != nil {
		return err
	}
	return nil
}

func validateCaseBindingWindowsDACL(
	descriptor *windows.SECURITY_DESCRIPTOR,
	owner *windows.SID,
) error {
	dacl, _, err := descriptor.DACL()
	if err != nil || dacl == nil {
		return newCaseBindingReadError(
			domainsecurity.CaseBindingStateInvalid,
			errors.New("case binding Windows DACL is invalid"),
		)
	}
	header := (*caseBindingWindowsACLHeader)(unsafe.Pointer(dacl))
	const writableMask = uint32(
		windows.GENERIC_WRITE |
			windows.GENERIC_ALL |
			windows.FILE_GENERIC_WRITE |
			windows.FILE_WRITE_DATA |
			windows.FILE_APPEND_DATA |
			windows.FILE_WRITE_EA |
			windows.FILE_WRITE_ATTRIBUTES |
			windows.DELETE |
			windows.WRITE_DAC |
			windows.WRITE_OWNER |
			0x00000040, // FILE_DELETE_CHILD
	)
	for index := uint32(0); index < uint32(header.aceCount); index++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(dacl, index, &ace); err != nil || ace == nil {
			return newCaseBindingReadError(
				domainsecurity.CaseBindingStateInvalid,
				errors.New("case binding Windows DACL entry is invalid"),
			)
		}
		if ace.Header.AceType == windows.ACCESS_DENIED_ACE_TYPE {
			continue
		}
		if ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE {
			return newCaseBindingReadError(
				domainsecurity.CaseBindingStateInvalid,
				errors.New("case binding Windows DACL entry type is unsupported"),
			)
		}
		if uint32(ace.Mask)&writableMask == 0 {
			continue
		}
		sid := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
		if sid == nil || !sid.IsValid() || !caseBindingWindowsTrustedWriter(sid, owner) {
			return newCaseBindingReadError(
				domainsecurity.CaseBindingStateInvalid,
				errors.New("case binding Windows DACL grants write access outside host authority"),
			)
		}
	}
	return nil
}

func caseBindingWindowsTrustedWriter(sid *windows.SID, owner *windows.SID) bool {
	return sid.Equals(owner) ||
		sid.IsWellKnown(windows.WinCreatorOwnerSid) ||
		sid.IsWellKnown(windows.WinLocalSystemSid) ||
		sid.IsWellKnown(windows.WinBuiltinAdministratorsSid)
}

func validateCaseBindingWindowsStreams(handle windows.Handle, directory bool) error {
	buffer := make([]byte, 4096)
	err := windows.GetFileInformationByHandleEx(
		handle,
		windows.FileStreamInfo,
		&buffer[0],
		uint32(len(buffer)),
	)
	if errors.Is(err, windows.ERROR_HANDLE_EOF) && directory {
		return nil
	}
	if err != nil {
		return newCaseBindingReadError(domainsecurity.CaseBindingStateInvalid, err)
	}
	offset := 0
	count := 0
	for {
		if offset > len(buffer)-int(unsafe.Sizeof(caseBindingWindowsStreamHeader{})) {
			return newCaseBindingReadError(
				domainsecurity.CaseBindingStateInvalid,
				errors.New("case binding Windows stream information is truncated"),
			)
		}
		header := (*caseBindingWindowsStreamHeader)(unsafe.Pointer(&buffer[offset]))
		nameStart := offset + int(unsafe.Sizeof(caseBindingWindowsStreamHeader{}))
		nameLength := int(header.nameLength)
		if nameLength <= 0 || nameLength%2 != 0 || nameStart > len(buffer)-nameLength {
			return newCaseBindingReadError(
				domainsecurity.CaseBindingStateInvalid,
				errors.New("case binding Windows stream name is invalid"),
			)
		}
		units := unsafe.Slice((*uint16)(unsafe.Pointer(&buffer[nameStart])), nameLength/2)
		for _, unit := range units {
			if unit == 0 {
				return newCaseBindingReadError(
					domainsecurity.CaseBindingStateInvalid,
					errors.New("case binding Windows stream name contains NUL"),
				)
			}
		}
		if string(utf16.Decode(units)) != "::$DATA" {
			return newCaseBindingReadError(
				domainsecurity.CaseBindingStateInvalid,
				errors.New("case binding Windows named data stream is forbidden"),
			)
		}
		count++
		if count > 1 {
			return newCaseBindingReadError(
				domainsecurity.CaseBindingStateInvalid,
				errors.New("case binding Windows has multiple data streams"),
			)
		}
		if header.nextOffset == 0 {
			break
		}
		next := int(header.nextOffset)
		if next%8 != 0 ||
			next < int(unsafe.Sizeof(caseBindingWindowsStreamHeader{}))+nameLength ||
			offset > len(buffer)-next {
			return newCaseBindingReadError(
				domainsecurity.CaseBindingStateInvalid,
				errors.New("case binding Windows stream offset is invalid"),
			)
		}
		offset += next
	}
	if !directory && count != 1 {
		return newCaseBindingReadError(
			domainsecurity.CaseBindingStateInvalid,
			errors.New("case binding Windows default stream is missing"),
		)
	}
	return nil
}

func caseBindingWindowsSafeComponent(component string) bool {
	if component == "" ||
		component == "." ||
		component == ".." ||
		!utf8.ValidString(component) ||
		strings.ContainsAny(component, `\/:<>\"|?*`) ||
		strings.TrimRight(component, ". ") != component {
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

func caseBindingWindowsASCIILetter(value byte) bool {
	return value >= 'A' && value <= 'Z' || value >= 'a' && value <= 'z'
}

func classifyCaseBindingWindowsPathError(err error, missingState string) error {
	var classified *caseBindingReadError
	if errors.As(err, &classified) {
		return err
	}
	switch {
	case errors.Is(err, os.ErrPermission),
		errors.Is(err, windows.ERROR_ACCESS_DENIED),
		errors.Is(err, windows.ERROR_PRIVILEGE_NOT_HELD),
		errors.Is(err, windows.ERROR_SHARING_VIOLATION),
		errors.Is(err, windows.ERROR_LOCK_VIOLATION):
		return newCaseBindingReadError(domainsecurity.CaseBindingStateUnreadable, err)
	case errors.Is(err, os.ErrNotExist),
		errors.Is(err, windows.ERROR_FILE_NOT_FOUND),
		errors.Is(err, windows.ERROR_PATH_NOT_FOUND):
		return newCaseBindingReadError(missingState, err)
	default:
		return newCaseBindingReadError(domainsecurity.CaseBindingStateInvalid, err)
	}
}
