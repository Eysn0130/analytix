//go:build windows

package persistencefs

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unsafe"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainstartup "analytix.local/runtime-go/internal/domain/startup"
	"golang.org/x/sys/windows"
)

const windowsSecureShare = windows.FILE_SHARE_READ | windows.FILE_SHARE_WRITE | windows.FILE_SHARE_DELETE

const (
	secureWindowsTraverseDirectoryAccess = windows.FILE_TRAVERSE | windows.FILE_READ_ATTRIBUTES | windows.SYNCHRONIZE
	secureWindowsMutateDirectoryAccess   = windows.FILE_GENERIC_READ | windows.FILE_GENERIC_WRITE | windows.DELETE
)

type windowsFileRenameInformation struct {
	ReplaceIfExists uint32
	RootDirectory   windows.Handle
	FileNameLength  uint32
	FileName        [1]uint16
}

func secureEnsureManagedRoot(authority *RootAuthority, root string) error {
	capability, err := authority.rootCapability(root)
	if err != nil {
		return err
	}
	if capability.RootIdentity == "" {
		handle, identity, err := secureWindowsCreateManagedRootExclusive(capability)
		if err != nil {
			return err
		}
		if err := authority.pinCreatedRoot(root, identity); err != nil {
			_ = windows.CloseHandle(handle)
			return err
		}
		return errors.Join(secureWindowsSyncDirectory(handle), windows.CloseHandle(handle))
	}
	handle, err := secureWindowsOpenAbsoluteDirectory(root, true)
	if err != nil {
		return err
	}
	identity, err := windowsOpenedDirectoryIdentity(handle)
	if err != nil || authority.verifyOpenedRoot(root, identity) != nil {
		_ = windows.CloseHandle(handle)
		return errors.New("semantic startup opened a different managed root")
	}
	return errors.Join(secureWindowsSyncDirectory(handle), windows.CloseHandle(handle))
}

func secureWindowsCreateManagedRootExclusive(capability frozenRootCapability) (windows.Handle, string, error) {
	if capability.RootIdentity != "" || capability.MissingSuffix == "" {
		return 0, "", errors.New("cold Windows persistence root creation capability is invalid")
	}
	current, err := secureWindowsOpenAbsoluteDirectory(capability.Anchor, false)
	if err != nil {
		return 0, "", err
	}
	anchorIdentity, err := windowsOpenedDirectoryIdentity(current)
	if err != nil || anchorIdentity != capability.AnchorIdentity {
		_ = windows.CloseHandle(current)
		return 0, "", errors.New("cold Windows persistence root ancestor identity changed")
	}
	parts, err := secureWindowsRelativeComponents(capability.MissingSuffix)
	if err != nil {
		_ = windows.CloseHandle(current)
		return 0, "", err
	}
	for _, component := range parts {
		next, err := secureWindowsOpenRelative(current, component, windows.FILE_GENERIC_READ|windows.FILE_GENERIC_WRITE|windows.DELETE, windows.FILE_CREATE, true)
		if err != nil {
			_ = windows.CloseHandle(current)
			if err == windows.STATUS_OBJECT_NAME_COLLISION || err == windows.STATUS_OBJECT_NAME_EXISTS {
				return 0, "", errors.New("cold Windows persistence root appeared after authority freeze")
			}
			return 0, "", err
		}
		if err := secureWindowsSyncDirectory(current); err != nil {
			_ = windows.CloseHandle(next)
			_ = windows.CloseHandle(current)
			return 0, "", err
		}
		_ = windows.CloseHandle(current)
		current = next
	}
	identity, err := windowsOpenedDirectoryIdentity(current)
	if err != nil {
		_ = windows.CloseHandle(current)
		return 0, "", err
	}
	return current, identity, nil
}

func secureManagedTargetMatches(authority *RootAuthority, root, relative string, expected domainstartup.SemanticEntryStateV1) (bool, error) {
	parent, base, err := secureWindowsOpenManagedParent(authority, root, relative)
	if secureWindowsNotFound(err) {
		return expected.Type == domainstartup.ManagedEntryTypeAbsent, nil
	}
	if err != nil {
		return false, err
	}
	defer windows.CloseHandle(parent)
	return secureWindowsTargetMatchesAt(parent, base, expected)
}

func secureManagedCreateDirectory(authority *RootAuthority, root, relative string, _ os.FileMode) error {
	parent, base, err := secureWindowsOpenManagedParent(authority, root, relative)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(parent)
	handle, err := secureWindowsOpenRelative(parent, base, windows.FILE_GENERIC_READ|windows.FILE_GENERIC_WRITE|windows.DELETE, windows.FILE_CREATE, true)
	if err != nil {
		return err
	}
	if err := errors.Join(secureWindowsSyncDirectory(handle), windows.CloseHandle(handle)); err != nil {
		return err
	}
	return secureWindowsSyncDirectory(parent)
}

func secureManagedInstallFile(
	authority *RootAuthority,
	root, relative string,
	body []byte,
	mode os.FileMode,
	operationID string,
	before domainstartup.SemanticEntryStateV1,
	after domainstartup.SemanticEntryStateV1,
) error {
	if !domainsecurity.IsSHA256Hex(operationID) {
		return errors.New("semantic startup install operation identity is invalid")
	}
	parent, base, err := secureWindowsOpenManagedParent(authority, root, relative)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(parent)
	if matches, err := secureWindowsTargetMatchesAt(parent, base, before); err != nil || !matches {
		return errors.New("semantic startup install target changed before write")
	}
	temporary := "." + base + ".startup-" + operationID + ".tmp"
	existing, exists, err := secureWindowsReadRegularAt(parent, temporary)
	if err != nil {
		return err
	}
	if exists && !bytes.Equal(existing, body) {
		if matches, matchErr := secureWindowsTargetMatchesAt(parent, base, before); matchErr != nil || !matches {
			return errors.New("semantic startup install temp conflicts with target state")
		}
		if err := secureWindowsDeleteRelative(parent, temporary, domainstartup.ManagedEntryTypeFile); err != nil {
			return err
		}
		if err := secureWindowsSyncDirectory(parent); err != nil {
			return err
		}
		exists = false
	}
	if !exists {
		if err := secureWindowsWriteExclusiveAt(parent, temporary, body, mode); err != nil {
			return err
		}
		if err := secureWindowsSyncDirectory(parent); err != nil {
			return err
		}
	}
	if matches, err := secureWindowsTargetMatchesAt(parent, base, after); err == nil && matches {
		if err := secureWindowsDeleteRelative(parent, temporary, domainstartup.ManagedEntryTypeFile); err != nil {
			return err
		}
		return secureWindowsSyncDirectory(parent)
	}
	if matches, err := secureWindowsTargetMatchesAt(parent, base, before); err != nil || !matches {
		return errors.New("semantic startup install target changed before commit")
	}
	if err := secureWindowsRenameRelative(parent, temporary, base); err != nil {
		return err
	}
	if err := secureWindowsSyncDirectory(parent); err != nil {
		return err
	}
	matches, err := secureWindowsTargetMatchesAt(parent, base, after)
	if err != nil || !matches {
		return errors.New("semantic startup secure install readback failed")
	}
	return nil
}

func secureManagedSetMode(authority *RootAuthority, root, relative string, before domainstartup.SemanticEntryStateV1, mode os.FileMode) error {
	parent, base, err := secureWindowsOpenManagedParent(authority, root, relative)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(parent)
	directory := before.Type == domainstartup.ManagedEntryTypeDirectory
	handle, err := secureWindowsOpenRelative(parent, base, windows.FILE_GENERIC_READ|windows.FILE_WRITE_ATTRIBUTES, windows.FILE_OPEN, directory)
	if err != nil {
		return err
	}
	file := os.NewFile(uintptr(handle), base)
	if file == nil {
		_ = windows.CloseHandle(handle)
		return errors.New("semantic startup Windows mode handle is invalid")
	}
	defer file.Close()
	if matches, err := secureWindowsOpenedMatches(handle, file, before); err != nil || !matches {
		return errors.New("semantic startup mode target changed before update")
	}
	if err := file.Chmod(mode); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	return secureWindowsSyncDirectory(parent)
}

func secureManagedRemove(authority *RootAuthority, root, relative string, before domainstartup.SemanticEntryStateV1) error {
	parent, base, err := secureWindowsOpenManagedParent(authority, root, relative)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(parent)
	if matches, err := secureWindowsTargetMatchesAt(parent, base, before); err != nil || !matches {
		return errors.New("semantic startup remove target changed before delete")
	}
	if err := secureWindowsDeleteRelative(parent, base, before.Type); err != nil {
		return err
	}
	return secureWindowsSyncDirectory(parent)
}

func secureWindowsOpenManagedParent(authority *RootAuthority, root, relative string) (windows.Handle, string, error) {
	if err := authority.validateRoot(root); err != nil {
		return 0, "", err
	}
	parts, err := secureWindowsRelativeComponents(relative)
	if err != nil {
		return 0, "", err
	}
	current, err := secureWindowsOpenAbsoluteDirectory(root, false)
	if err != nil {
		return 0, "", err
	}
	identity, err := windowsOpenedDirectoryIdentity(current)
	if err != nil || authority.verifyOpenedRoot(root, identity) != nil {
		_ = windows.CloseHandle(current)
		return 0, "", errors.New("semantic startup opened a different managed root")
	}
	for _, component := range parts[:len(parts)-1] {
		next, openErr := secureWindowsOpenRelative(current, component, windows.FILE_GENERIC_READ|windows.FILE_GENERIC_WRITE, windows.FILE_OPEN, true)
		if openErr != nil {
			_ = windows.CloseHandle(current)
			return 0, "", openErr
		}
		_ = windows.CloseHandle(current)
		current = next
	}
	return current, parts[len(parts)-1], nil
}

func windowsOpenedDirectoryIdentity(handle windows.Handle) (string, error) {
	return windowsOpenedObjectIdentity(handle, true)
}

func secureWindowsOpenAbsoluteDirectory(path string, create bool) (windows.Handle, error) {
	path = filepath.Clean(path)
	volume := filepath.VolumeName(path)
	if volume == "" || !filepath.IsAbs(path) {
		return 0, errors.New("semantic startup secure Windows root is invalid")
	}
	volumeRoot := volume + string(os.PathSeparator)
	current, err := secureWindowsOpenAbsolute(volumeRoot, secureWindowsTraverseDirectoryAccess, windows.FILE_OPEN, true)
	if err != nil {
		return 0, err
	}
	remainder := strings.Trim(strings.TrimPrefix(path, volume), `\/`)
	components := strings.FieldsFunc(remainder, func(char rune) bool { return char == '\\' || char == '/' })
	if len(components) == 0 {
		_ = windows.CloseHandle(current)
		return secureWindowsOpenAbsolute(volumeRoot, secureWindowsMutateDirectoryAccess, windows.FILE_OPEN, true)
	}
	currentPath := volumeRoot
	for index, component := range components {
		if !secureWindowsComponent(component) {
			_ = windows.CloseHandle(current)
			return 0, errors.New("semantic startup secure Windows root component is invalid")
		}
		access := uint32(secureWindowsTraverseDirectoryAccess)
		if index == len(components)-1 {
			access = secureWindowsMutateDirectoryAccess
		}
		next, openErr := secureWindowsOpenRelative(current, component, access, windows.FILE_OPEN, true)
		created := false
		if secureWindowsNotFound(openErr) && create {
			mutationParent, mutationErr := secureWindowsReopenSameDirectoryForMutation(currentPath, current)
			if mutationErr != nil {
				_ = windows.CloseHandle(current)
				return 0, mutationErr
			}
			next, openErr = secureWindowsOpenRelative(
				mutationParent, component, secureWindowsMutateDirectoryAccess, windows.FILE_CREATE, true,
			)
			created = openErr == nil
			if created {
				openErr = errors.Join(secureWindowsSyncDirectory(next), secureWindowsSyncDirectory(mutationParent))
			}
			_ = windows.CloseHandle(mutationParent)
		}
		if openErr != nil {
			if next != 0 {
				_ = windows.CloseHandle(next)
			}
			_ = windows.CloseHandle(current)
			return 0, openErr
		}
		_ = windows.CloseHandle(current)
		current = next
		currentPath = filepath.Join(currentPath, component)
	}
	return current, nil
}

func secureWindowsReopenSameDirectoryForMutation(path string, expected windows.Handle) (windows.Handle, error) {
	expectedID, err := windowsOpenedDirectoryFileID(expected)
	if err != nil {
		return 0, err
	}
	mutation, err := secureWindowsOpenAbsolute(path, secureWindowsMutateDirectoryAccess, windows.FILE_OPEN, true)
	if err != nil {
		return 0, err
	}
	currentID, err := windowsOpenedDirectoryFileID(mutation)
	if err != nil || currentID != expectedID {
		_ = windows.CloseHandle(mutation)
		return 0, errors.New("semantic startup Windows mutation parent identity changed")
	}
	return mutation, nil
}

func secureWindowsOpenAbsolute(path string, access, disposition uint32, directory bool) (windows.Handle, error) {
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
	return secureWindowsNtCreate(attributes, access, disposition, directory)
}

func secureWindowsOpenRelative(parent windows.Handle, name string, access, disposition uint32, directory bool) (windows.Handle, error) {
	return secureWindowsOpenRelativeWithShare(parent, name, access, disposition, directory, windowsSecureShare)
}

func secureWindowsOpenRelativeWithShare(
	parent windows.Handle,
	name string,
	access uint32,
	disposition uint32,
	directory bool,
	share uint32,
) (windows.Handle, error) {
	if !secureWindowsComponent(name) {
		return 0, errors.New("semantic startup secure Windows path component is invalid")
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
	return secureWindowsNtCreateWithShare(attributes, access, disposition, directory, share)
}

func secureWindowsReopenDirectory(parent windows.Handle) (windows.Handle, error) {
	objectName, err := windows.NewNTUnicodeString(".")
	if err != nil {
		return 0, err
	}
	attributes := &windows.OBJECT_ATTRIBUTES{
		RootDirectory: parent, ObjectName: objectName,
		Attributes: windows.OBJ_CASE_INSENSITIVE | windows.OBJ_DONT_REPARSE,
	}
	attributes.Length = uint32(unsafe.Sizeof(*attributes))
	return secureWindowsNtCreate(attributes, windows.FILE_GENERIC_READ, windows.FILE_OPEN, true)
}

func secureWindowsNtCreate(attributes *windows.OBJECT_ATTRIBUTES, access, disposition uint32, directory bool) (windows.Handle, error) {
	return secureWindowsNtCreateWithShare(attributes, access, disposition, directory, windowsSecureShare)
}

func secureWindowsNtCreateWithShare(
	attributes *windows.OBJECT_ATTRIBUTES,
	access uint32,
	disposition uint32,
	directory bool,
	share uint32,
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
	err := windows.NtCreateFile(&handle, access, attributes, &status, &allocation, fileAttributes, share, disposition, options, 0, 0)
	if err != nil {
		return 0, err
	}
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(handle, &info); err != nil || info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 ||
		(directory && info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY == 0) || (!directory && info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY != 0) {
		_ = windows.CloseHandle(handle)
		return 0, errors.New("semantic startup Windows handle crossed a reparse boundary")
	}
	return handle, nil
}

func secureWindowsTargetMatchesAt(parent windows.Handle, base string, expected domainstartup.SemanticEntryStateV1) (bool, error) {
	directory := expected.Type == domainstartup.ManagedEntryTypeDirectory
	if expected.Type == domainstartup.ManagedEntryTypeAbsent {
		for _, asDirectory := range []bool{false, true} {
			handle, err := secureWindowsOpenRelative(parent, base, windows.FILE_GENERIC_READ, windows.FILE_OPEN, asDirectory)
			if err == nil {
				_ = windows.CloseHandle(handle)
				return false, nil
			}
			if !secureWindowsNotFound(err) && err != windows.STATUS_FILE_IS_A_DIRECTORY && err != windows.STATUS_NOT_A_DIRECTORY {
				return false, err
			}
		}
		return true, nil
	}
	handle, err := secureWindowsOpenRelative(parent, base, windows.FILE_GENERIC_READ, windows.FILE_OPEN, directory)
	if secureWindowsNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	file := os.NewFile(uintptr(handle), base)
	if file == nil {
		_ = windows.CloseHandle(handle)
		return false, errors.New("semantic startup Windows target handle is invalid")
	}
	defer file.Close()
	return secureWindowsOpenedMatches(handle, file, expected)
}

func secureWindowsOpenedMatches(handle windows.Handle, file *os.File, expected domainstartup.SemanticEntryStateV1) (bool, error) {
	info, err := file.Stat()
	if err != nil {
		return false, err
	}
	var handleInfo windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(handle, &handleInfo); err != nil || handleInfo.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		return false, errors.New("semantic startup Windows target is unsafe")
	}
	if expected.Type == domainstartup.ManagedEntryTypeFile && handleInfo.NumberOfLinks != 1 {
		return false, errors.New("semantic startup Windows target is not single-link authority")
	}
	if expected.Type == domainstartup.ManagedEntryTypeAbsent || uint32(info.Mode()) != expected.Mode {
		return false, nil
	}
	if expected.Type == domainstartup.ManagedEntryTypeDirectory {
		return info.IsDir(), nil
	}
	if expected.Type != domainstartup.ManagedEntryTypeFile || !info.Mode().IsRegular() || info.Size() != expected.Size {
		return false, nil
	}
	hasher := sha256.New()
	written, copyErr := io.Copy(hasher, file)
	if copyErr != nil || written != expected.Size {
		return false, copyErr
	}
	return hex.EncodeToString(hasher.Sum(nil)) == expected.SHA256, nil
}

func secureWindowsReadRegularAt(parent windows.Handle, base string) ([]byte, bool, error) {
	handle, err := secureWindowsOpenRelative(parent, base, windows.FILE_GENERIC_READ, windows.FILE_OPEN, false)
	if secureWindowsNotFound(err) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	file := os.NewFile(uintptr(handle), base)
	if file == nil {
		_ = windows.CloseHandle(handle)
		return nil, false, errors.New("semantic startup Windows temp handle is invalid")
	}
	defer file.Close()
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(handle, &info); err != nil || info.NumberOfLinks != 1 || info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		return nil, false, errors.New("semantic startup Windows temp is unsafe")
	}
	body, err := io.ReadAll(io.LimitReader(file, 64*1024*1024+1))
	if err != nil || len(body) > 64*1024*1024 {
		return nil, false, errors.New("semantic startup Windows temp is unreadable")
	}
	return body, true, nil
}

func secureWindowsWriteExclusiveAt(parent windows.Handle, base string, body []byte, mode os.FileMode) error {
	handle, err := secureWindowsOpenRelative(parent, base, windows.FILE_GENERIC_READ|windows.FILE_GENERIC_WRITE|windows.DELETE, windows.FILE_CREATE, false)
	if err != nil {
		return err
	}
	file := os.NewFile(uintptr(handle), base)
	if file == nil {
		_ = windows.CloseHandle(handle)
		return errors.New("semantic startup Windows write handle is invalid")
	}
	defer file.Close()
	for written := 0; written < len(body); {
		count, writeErr := file.Write(body[written:])
		if writeErr != nil {
			return writeErr
		}
		if count <= 0 {
			return errors.New("semantic startup Windows write was incomplete")
		}
		written += count
	}
	if err := file.Chmod(mode); err != nil {
		return err
	}
	return file.Sync()
}

func secureWindowsRenameRelative(parent windows.Handle, source, target string) error {
	handle, err := secureWindowsOpenRelative(parent, source, windows.FILE_GENERIC_READ|windows.FILE_GENERIC_WRITE|windows.DELETE, windows.FILE_OPEN, false)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(handle)
	targetName, err := windows.UTF16FromString(target)
	if err != nil {
		return err
	}
	nameLength := (len(targetName) - 1) * 2
	dummy := windowsFileRenameInformation{}
	buffer := make([]byte, int(unsafe.Offsetof(dummy.FileName))+nameLength)
	info := (*windowsFileRenameInformation)(unsafe.Pointer(&buffer[0]))
	info.ReplaceIfExists = windows.FILE_RENAME_REPLACE_IF_EXISTS | windows.FILE_RENAME_POSIX_SEMANTICS
	info.RootDirectory = parent
	info.FileNameLength = uint32(nameLength)
	copy((*[windows.MAX_LONG_PATH]uint16)(unsafe.Pointer(&info.FileName[0]))[:nameLength/2:nameLength/2], targetName[:len(targetName)-1])
	var status windows.IO_STATUS_BLOCK
	return windows.NtSetInformationFile(handle, &status, &buffer[0], uint32(len(buffer)), windows.FileRenameInformation)
}

func secureWindowsDeleteRelative(parent windows.Handle, base, entryType string) error {
	directory := entryType == domainstartup.ManagedEntryTypeDirectory
	handle, err := secureWindowsOpenRelative(parent, base, windows.DELETE|windows.FILE_READ_ATTRIBUTES, windows.FILE_OPEN, directory)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(handle)
	flags := uint32(windows.FILE_DISPOSITION_DELETE | windows.FILE_DISPOSITION_POSIX_SEMANTICS | windows.FILE_DISPOSITION_IGNORE_READONLY_ATTRIBUTE)
	buffer := (*[4]byte)(unsafe.Pointer(&flags))
	var status windows.IO_STATUS_BLOCK
	return windows.NtSetInformationFile(handle, &status, &buffer[0], uint32(len(buffer)), windows.FileDispositionInformationEx)
}

func secureWindowsSyncDirectory(handle windows.Handle) error {
	return windows.FlushFileBuffers(handle)
}

func secureWindowsRelativeComponents(relative string) ([]string, error) {
	relative = filepath.Clean(relative)
	if relative == "" || relative == "." || filepath.IsAbs(relative) || relative == ".." || strings.HasPrefix(relative, `..\`) {
		return nil, errors.New("semantic startup secure Windows relative path is invalid")
	}
	parts := strings.FieldsFunc(relative, func(char rune) bool { return char == '\\' || char == '/' })
	if len(parts) == 0 {
		return nil, errors.New("semantic startup secure Windows relative path is invalid")
	}
	for _, component := range parts {
		if !secureWindowsComponent(component) {
			return nil, errors.New("semantic startup secure Windows path component is invalid")
		}
	}
	return parts, nil
}

func secureWindowsComponent(component string) bool {
	if component == "" || component == "." || component == ".." || strings.ContainsAny(component, `:<>"|?*`) ||
		strings.TrimRight(component, ". ") != component {
		return false
	}
	for _, char := range component {
		if char < 0x20 {
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

func secureWindowsNotFound(err error) bool {
	return err == windows.STATUS_OBJECT_NAME_NOT_FOUND || err == windows.STATUS_OBJECT_PATH_NOT_FOUND || errors.Is(err, os.ErrNotExist)
}
