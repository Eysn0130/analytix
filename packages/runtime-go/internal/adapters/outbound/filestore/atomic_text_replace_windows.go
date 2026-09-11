//go:build windows

package filestore

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

const atomicWindowsShare = windows.FILE_SHARE_READ | windows.FILE_SHARE_WRITE | windows.FILE_SHARE_DELETE

const (
	atomicWindowsTraverseDirectoryAccess = windows.FILE_TRAVERSE | windows.FILE_READ_ATTRIBUTES | windows.SYNCHRONIZE
	atomicWindowsMutateDirectoryAccess   = windows.FILE_GENERIC_READ | windows.FILE_GENERIC_WRITE | windows.DELETE
)

type atomicWindowsFileID struct {
	VolumeSerialNumber uint64
	FileID             [16]byte
}

type atomicWindowsState struct {
	state      atomicTextState
	identity   atomicWindowsFileID
	attributes uint32
	size       uint64
}

type atomicWindowsRenameInformation struct {
	ReplaceIfExists uint32
	RootDirectory   windows.Handle
	FileNameLength  uint32
	FileName        [1]uint16
}

func inspectAtomicTextTargetPlatform(path string, missingParentsAreAbsent bool) (atomicTextState, error) {
	parent, base, missing, err := openAtomicWindowsParent(path, false, false)
	if err != nil {
		return atomicTextState{}, err
	}
	if missing {
		if missingParentsAreAbsent {
			return atomicTextState{}, nil
		}
		return atomicTextState{}, os.ErrNotExist
	}
	defer windows.CloseHandle(parent)
	state, err := inspectAtomicWindowsTarget(parent, base, windows.FILE_GENERIC_READ)
	return state.state, err
}

func atomicReplaceTextPlatform(request atomicTextReplaceRequest, hooks *atomicTextTestHooks) error {
	parent, base, missing, err := openAtomicWindowsParent(request.Path, request.CreateParents, true)
	if err != nil {
		return err
	}
	if missing {
		return os.ErrNotExist
	}
	defer windows.CloseHandle(parent)
	initial, err := inspectAtomicWindowsTarget(parent, base, windows.FILE_GENERIC_READ)
	if err != nil {
		return err
	}
	if err := validateAtomicTextBefore(request, initial.state); err != nil {
		return err
	}
	// Windows does not expose a handle-relative conditional exchange primitive
	// equivalent to renameat2(RENAME_EXCHANGE) or renameatx_np(RENAME_SWAP).
	// Replacing or deleting an existing pathname after validation would leave a
	// last-moment identity race, so those mutations are deliberately unavailable
	// until they can be backed by an equally strong kernel primitive.
	if request.ExpectedExists {
		return fmt.Errorf("%w: conditional replacement of an existing Windows target is unavailable", ErrAtomicTextUnsupportedPlatform)
	}
	if hooks != nil && hooks.AfterInitialValidation != nil {
		hooks.AfterInitialValidation()
	}

	tempName, err := atomicTextTempName(request.Path)
	if err != nil {
		return err
	}
	temp, err := atomicWindowsOpenRelative(parent, tempName, windows.FILE_GENERIC_READ|windows.FILE_GENERIC_WRITE|windows.DELETE, windows.FILE_CREATE, false, true)
	if err != nil {
		return fmt.Errorf("create private atomic text temporary file: %w", err)
	}
	renamed := false
	defer func() {
		if !renamed {
			_ = atomicWindowsDeleteHandle(temp)
		}
		_ = windows.CloseHandle(temp)
	}()
	if err := writeAtomicTextContent(func(body []byte) (int, error) {
		var written uint32
		err := windows.WriteFile(temp, body, &written, nil)
		return int(written), err
	}, request.Content, hooks); err != nil {
		return fmt.Errorf("write private atomic text temporary file: %w", err)
	}
	if err := windows.FlushFileBuffers(temp); err != nil {
		return fmt.Errorf("sync private atomic text temporary file: %w", err)
	}
	tempID, err := atomicWindowsHandleFileID(temp, false)
	if err != nil {
		return err
	}

	current, err := inspectAtomicWindowsTarget(parent, base, windows.FILE_GENERIC_READ)
	if err != nil {
		return err
	}
	if err := validateAtomicTextBefore(request, current.state); err != nil {
		return err
	}
	if !sameAtomicWindowsState(initial, current) {
		return fmt.Errorf("%w: target identity changed before replace", ErrAtomicTextBeforeDrift)
	}
	if err := atomicTextBeforeReplace(hooks); err != nil {
		return fmt.Errorf("replace atomic text target: %w", err)
	}
	if err := atomicWindowsRenameNoReplace(temp, parent, base); err != nil {
		return fmt.Errorf("replace atomic text target: %w", err)
	}
	renamed = true
	if err := windows.FlushFileBuffers(temp); err != nil {
		return fmt.Errorf("sync atomic text replacement: %w", err)
	}
	replaced, err := inspectAtomicWindowsTarget(parent, base, windows.FILE_GENERIC_READ)
	if err != nil {
		return fmt.Errorf("verify atomic text replacement: %w", err)
	}
	if !replaced.state.Exists || replaced.identity != tempID || !equalAtomicTextBytes(replaced.state.Content, request.Content) {
		return errors.New("atomic text replacement verification failed")
	}
	if err := windows.FlushFileBuffers(parent); err != nil {
		return fmt.Errorf("sync atomic text parent directory: %w", err)
	}
	return nil
}

func openAtomicWindowsParent(path string, createParents bool, mutate bool) (windows.Handle, string, bool, error) {
	clean := filepath.Clean(path)
	volume := filepath.VolumeName(clean)
	if volume == "" || !filepath.IsAbs(clean) {
		return 0, "", false, fmt.Errorf("%w: Windows target must be absolute", ErrAtomicTextUnsafePath)
	}
	base := filepath.Base(clean)
	if !atomicWindowsComponent(base) {
		return 0, "", false, fmt.Errorf("%w: Windows target name is invalid", ErrAtomicTextUnsafePath)
	}
	volumeRoot := volume + string(os.PathSeparator)
	current, err := atomicWindowsOpenAbsolute(volumeRoot, atomicWindowsTraverseDirectoryAccess, windows.FILE_OPEN, true)
	if err != nil {
		return 0, "", false, err
	}
	components := strings.FieldsFunc(strings.Trim(strings.TrimPrefix(filepath.Dir(clean), volume), `\/`), func(char rune) bool {
		return char == '\\' || char == '/'
	})
	if len(components) == 0 {
		if !mutate {
			return current, base, false, nil
		}
		mutation, reopenErr := atomicWindowsReopenSameDirectoryForMutation(volumeRoot, current)
		_ = windows.CloseHandle(current)
		return mutation, base, false, reopenErr
	}
	currentPath := volumeRoot
	for index, component := range components {
		if !atomicWindowsComponent(component) {
			_ = windows.CloseHandle(current)
			return 0, "", false, fmt.Errorf("%w: Windows parent component is invalid", ErrAtomicTextUnsafePath)
		}
		access := uint32(atomicWindowsTraverseDirectoryAccess)
		if mutate && index == len(components)-1 {
			access = atomicWindowsMutateDirectoryAccess
		}
		next, openErr := atomicWindowsOpenRelative(current, component, access, windows.FILE_OPEN, true, false)
		if atomicWindowsNotFound(openErr) && createParents {
			mutationParent, mutationErr := atomicWindowsReopenSameDirectoryForMutation(currentPath, current)
			if mutationErr != nil {
				_ = windows.CloseHandle(current)
				return 0, "", false, mutationErr
			}
			createAccess := access
			if createAccess == atomicWindowsTraverseDirectoryAccess {
				createAccess = atomicWindowsMutateDirectoryAccess
			}
			next, openErr = atomicWindowsOpenRelative(mutationParent, component, createAccess, windows.FILE_CREATE, true, false)
			if atomicWindowsCollision(openErr) {
				openErr = fmt.Errorf("%w: Windows parent creation collided", ErrAtomicTextBeforeDrift)
			}
			if openErr == nil {
				openErr = errors.Join(windows.FlushFileBuffers(next), windows.FlushFileBuffers(mutationParent))
			}
			_ = windows.CloseHandle(mutationParent)
		}
		if atomicWindowsNotFound(openErr) {
			_ = windows.CloseHandle(current)
			return 0, base, true, nil
		}
		if openErr != nil {
			if next != 0 {
				_ = windows.CloseHandle(next)
			}
			_ = windows.CloseHandle(current)
			return 0, "", false, fmt.Errorf("%w: open Windows parent component without reparse traversal: %v", ErrAtomicTextUnsafePath, openErr)
		}
		_ = windows.CloseHandle(current)
		current = next
		currentPath = filepath.Join(currentPath, component)
	}
	return current, base, false, nil
}

func inspectAtomicWindowsTarget(parent windows.Handle, name string, access uint32) (atomicWindowsState, error) {
	handle, err := atomicWindowsOpenRelative(parent, name, access, windows.FILE_OPEN, false, false)
	if atomicWindowsNotFound(err) {
		return atomicWindowsState{}, nil
	}
	if err != nil {
		return atomicWindowsState{}, fmt.Errorf("%w: open Windows target without reparse traversal: %v", ErrAtomicTextUnsafePath, err)
	}
	file := os.NewFile(uintptr(handle), name)
	if file == nil {
		_ = windows.CloseHandle(handle)
		return atomicWindowsState{}, errors.New("atomic text Windows target handle is invalid")
	}
	defer file.Close()
	before, err := atomicWindowsObjectState(handle, false)
	if err != nil {
		return atomicWindowsState{}, err
	}
	content, readErr := io.ReadAll(file)
	if readErr != nil {
		return atomicWindowsState{}, readErr
	}
	after, err := atomicWindowsObjectState(handle, false)
	if err != nil {
		return atomicWindowsState{}, err
	}
	if before.identity != after.identity || before.attributes != after.attributes || uint64(len(content)) != after.size {
		return atomicWindowsState{}, fmt.Errorf("%w: Windows target changed while being read", ErrAtomicTextBeforeDrift)
	}
	after.state.Content = content
	return after, nil
}

func atomicWindowsObjectState(handle windows.Handle, directory bool) (atomicWindowsState, error) {
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(handle, &info); err != nil ||
		info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 ||
		(directory && info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY == 0) ||
		(!directory && info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY != 0) {
		return atomicWindowsState{}, fmt.Errorf("%w: Windows object type or reparse state is unsafe", ErrAtomicTextUnsafePath)
	}
	identity, err := atomicWindowsHandleFileID(handle, directory)
	if err != nil {
		return atomicWindowsState{}, err
	}
	size := uint64(info.FileSizeHigh)<<32 | uint64(info.FileSizeLow)
	mode := os.FileMode(0o644)
	if info.FileAttributes&windows.FILE_ATTRIBUTE_READONLY != 0 {
		mode = 0o444
	}
	return atomicWindowsState{
		state:      atomicTextState{Exists: true, Mode: mode},
		identity:   identity,
		attributes: info.FileAttributes,
		size:       size,
	}, nil
}

func atomicWindowsHandleFileID(handle windows.Handle, directory bool) (atomicWindowsFileID, error) {
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(handle, &info); err != nil ||
		info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 ||
		(directory && info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY == 0) ||
		(!directory && info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY != 0) {
		return atomicWindowsFileID{}, fmt.Errorf("%w: Windows handle type is unsafe", ErrAtomicTextUnsafePath)
	}
	var identity atomicWindowsFileID
	if err := windows.GetFileInformationByHandleEx(handle, windows.FileIdInfo, (*byte)(unsafe.Pointer(&identity)), uint32(unsafe.Sizeof(identity))); err != nil ||
		identity.VolumeSerialNumber == 0 || identity.FileID == ([16]byte{}) {
		return atomicWindowsFileID{}, errors.New("atomic text Windows FileID is unavailable")
	}
	return identity, nil
}

func atomicWindowsOpenAbsolute(path string, access, disposition uint32, directory bool) (windows.Handle, error) {
	ntPath := `\??\` + path
	if strings.HasPrefix(path, `\\`) {
		ntPath = `\??\UNC\` + strings.TrimPrefix(path, `\\`)
	}
	name, err := windows.NewNTUnicodeString(ntPath)
	if err != nil {
		return 0, err
	}
	attributes := &windows.OBJECT_ATTRIBUTES{ObjectName: name, Attributes: windows.OBJ_CASE_INSENSITIVE | windows.OBJ_DONT_REPARSE}
	attributes.Length = uint32(unsafe.Sizeof(*attributes))
	return atomicWindowsNtCreate(attributes, access, disposition, directory)
}

func atomicWindowsOpenRelative(parent windows.Handle, name string, access, disposition uint32, directory bool, private bool) (windows.Handle, error) {
	if !atomicWindowsComponent(name) {
		return 0, fmt.Errorf("%w: Windows path component is invalid", ErrAtomicTextUnsafePath)
	}
	objectName, err := windows.NewNTUnicodeString(name)
	if err != nil {
		return 0, err
	}
	attributes := &windows.OBJECT_ATTRIBUTES{
		RootDirectory: parent, ObjectName: objectName,
		Attributes: windows.OBJ_CASE_INSENSITIVE | windows.OBJ_DONT_REPARSE,
	}
	if private && disposition == windows.FILE_CREATE {
		descriptor, err := atomicWindowsPrivateDescriptor()
		if err != nil {
			return 0, err
		}
		attributes.SecurityDescriptor = descriptor
	}
	attributes.Length = uint32(unsafe.Sizeof(*attributes))
	return atomicWindowsNtCreate(attributes, access, disposition, directory)
}

func atomicWindowsNtCreate(attributes *windows.OBJECT_ATTRIBUTES, access, disposition uint32, directory bool) (windows.Handle, error) {
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
	if err := windows.NtCreateFile(&handle, access, attributes, &status, &allocation, fileAttributes, atomicWindowsShare, disposition, options, 0, 0); err != nil {
		return 0, err
	}
	if _, err := atomicWindowsObjectState(handle, directory); err != nil {
		_ = windows.CloseHandle(handle)
		return 0, err
	}
	return handle, nil
}

func atomicWindowsReopenSameDirectoryForMutation(path string, expected windows.Handle) (windows.Handle, error) {
	expectedID, err := atomicWindowsHandleFileID(expected, true)
	if err != nil {
		return 0, err
	}
	mutation, err := atomicWindowsOpenAbsolute(path, atomicWindowsMutateDirectoryAccess, windows.FILE_OPEN, true)
	if err != nil {
		return 0, err
	}
	currentID, err := atomicWindowsHandleFileID(mutation, true)
	if err != nil || currentID != expectedID {
		_ = windows.CloseHandle(mutation)
		return 0, fmt.Errorf("%w: Windows mutation parent identity changed", ErrAtomicTextBeforeDrift)
	}
	return mutation, nil
}

func atomicWindowsPrivateDescriptor() (*windows.SECURITY_DESCRIPTOR, error) {
	token, err := windows.OpenCurrentProcessToken()
	if err != nil {
		return nil, err
	}
	defer token.Close()
	user, err := token.GetTokenUser()
	if err != nil || user == nil || user.User.Sid == nil || !user.User.Sid.IsValid() {
		return nil, errors.New("atomic text Windows current user SID is invalid")
	}
	sid := user.User.Sid.String()
	return windows.SecurityDescriptorFromString("O:" + sid + "D:P(A;;FA;;;" + sid + ")(A;;FA;;;SY)(A;;FA;;;BA)")
}

func atomicWindowsRenameNoReplace(handle windows.Handle, parent windows.Handle, target string) error {
	targetName, err := windows.UTF16FromString(target)
	if err != nil {
		return err
	}
	nameLength := (len(targetName) - 1) * 2
	dummy := atomicWindowsRenameInformation{}
	buffer := make([]byte, int(unsafe.Offsetof(dummy.FileName))+nameLength)
	info := (*atomicWindowsRenameInformation)(unsafe.Pointer(&buffer[0]))
	info.ReplaceIfExists = windows.FILE_RENAME_POSIX_SEMANTICS | windows.FILE_RENAME_IGNORE_READONLY_ATTRIBUTE
	info.RootDirectory = parent
	info.FileNameLength = uint32(nameLength)
	copy((*[windows.MAX_LONG_PATH]uint16)(unsafe.Pointer(&info.FileName[0]))[:nameLength/2:nameLength/2], targetName[:len(targetName)-1])
	var status windows.IO_STATUS_BLOCK
	return windows.NtSetInformationFile(handle, &status, &buffer[0], uint32(len(buffer)), windows.FileRenameInformation)
}

func atomicWindowsDeleteHandle(handle windows.Handle) error {
	flags := uint32(windows.FILE_DISPOSITION_DELETE | windows.FILE_DISPOSITION_POSIX_SEMANTICS | windows.FILE_DISPOSITION_IGNORE_READONLY_ATTRIBUTE)
	buffer := (*[4]byte)(unsafe.Pointer(&flags))
	var status windows.IO_STATUS_BLOCK
	return windows.NtSetInformationFile(handle, &status, &buffer[0], uint32(len(buffer)), windows.FileDispositionInformationEx)
}

func sameAtomicWindowsState(left, right atomicWindowsState) bool {
	if left.state.Exists != right.state.Exists {
		return false
	}
	return !left.state.Exists || left.identity == right.identity
}

func atomicWindowsComponent(component string) bool {
	if component == "" || component == "." || component == ".." || strings.ContainsAny(component, `:<>"|?*`) || strings.TrimRight(component, ". ") != component {
		return false
	}
	for _, character := range component {
		if character < 0x20 {
			return false
		}
	}
	base := strings.ToUpper(strings.TrimSuffix(component, filepath.Ext(component)))
	switch base {
	case "CON", "PRN", "AUX", "NUL", "COM1", "COM2", "COM3", "COM4", "COM5", "COM6", "COM7", "COM8", "COM9",
		"LPT1", "LPT2", "LPT3", "LPT4", "LPT5", "LPT6", "LPT7", "LPT8", "LPT9":
		return false
	default:
		return true
	}
}

func atomicWindowsNotFound(err error) bool {
	return err == windows.STATUS_OBJECT_NAME_NOT_FOUND || err == windows.STATUS_OBJECT_PATH_NOT_FOUND || err == windows.STATUS_NO_SUCH_FILE || errors.Is(err, os.ErrNotExist)
}

func atomicWindowsCollision(err error) bool {
	return err == windows.STATUS_OBJECT_NAME_COLLISION || err == windows.STATUS_OBJECT_NAME_EXISTS || errors.Is(err, os.ErrExist)
}
