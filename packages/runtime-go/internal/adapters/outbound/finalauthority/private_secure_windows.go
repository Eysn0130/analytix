//go:build windows

package finalauthority

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

const privateWindowsShare = windows.FILE_SHARE_READ | windows.FILE_SHARE_WRITE | windows.FILE_SHARE_DELETE

var privateWindowsReOpenFile = syscall.NewLazyDLL("kernel32.dll").NewProc("ReOpenFile")

const (
	privateWindowsTraverseDirectoryAccess = windows.FILE_TRAVERSE | windows.FILE_READ_ATTRIBUTES | windows.SYNCHRONIZE
	privateWindowsObserveDirectoryAccess  = privateWindowsTraverseDirectoryAccess | windows.FILE_LIST_DIRECTORY | windows.READ_CONTROL
	privateWindowsMutateDirectoryAccess   = windows.FILE_GENERIC_READ | windows.FILE_GENERIC_WRITE | windows.DELETE
)

type privateRootAuthority struct {
	path string
	id   privateWindowsFileIDInfo
}

type privateWindowsRenameInformation struct {
	ReplaceIfExists uint32
	RootDirectory   windows.Handle
	FileNameLength  uint32
	FileName        [1]uint16
}

func newPrivateNamedRootAuthority(path, name string, maxBytes int) (privateRootAuthority, error) {
	if !privateWindowsComponent(name) || maxBytes <= 0 {
		return privateRootAuthority{}, errors.New("private named authority root input is invalid")
	}
	authority, err := capturePrivateRootAuthority(path)
	if err != nil {
		return privateRootAuthority{}, err
	}
	if err := privateWindowsRecoverNamedRoot(authority, name, uint64(maxBytes)); err != nil {
		return privateRootAuthority{}, err
	}
	return authority, nil
}

func capturePrivateRootAuthority(path string) (privateRootAuthority, error) {
	absolute, err := filepath.Abs(strings.TrimSpace(path))
	if err != nil || strings.TrimSpace(path) == "" {
		return privateRootAuthority{}, errors.New("private authority root is invalid")
	}
	handle, err := privateWindowsOpenAbsoluteDirectory(filepath.Clean(absolute), true)
	if err != nil {
		return privateRootAuthority{}, err
	}
	identity, err := privateWindowsValidateAuthorityObject(handle, true, 0)
	if err != nil {
		_ = windows.CloseHandle(handle)
		return privateRootAuthority{}, err
	}
	authority := privateRootAuthority{
		path: filepath.Clean(absolute), id: identity.id,
	}
	if err := errors.Join(privateWindowsSyncDirectory(handle), windows.CloseHandle(handle)); err != nil {
		return privateRootAuthority{}, err
	}
	return authority, nil
}

func secureWritePrivateNamedFileExclusive(authority privateRootAuthority, name string, body []byte, maxBytes int) error {
	if !privateWindowsComponent(name) || len(body) == 0 || maxBytes <= 0 || len(body) > maxBytes {
		return errors.New("private named authority Windows write input is invalid")
	}
	root, err := authority.open()
	if err != nil {
		return err
	}
	defer windows.CloseHandle(root)
	suffix := make([]byte, 12)
	if _, err := rand.Read(suffix); err != nil {
		return err
	}
	temporary := "." + name + "-" + hex.EncodeToString(suffix) + ".tmp"
	handle, err := privateWindowsOpenRelative(root, temporary, windows.FILE_GENERIC_READ|windows.FILE_GENERIC_WRITE|windows.DELETE, windows.FILE_CREATE, false)
	if err != nil {
		return err
	}
	file := os.NewFile(uintptr(handle), temporary)
	if file == nil {
		_ = windows.CloseHandle(handle)
		return errors.New("private named authority Windows temp handle is invalid")
	}
	tempExists := true
	defer func() {
		_ = file.Close()
		if tempExists {
			_ = privateWindowsDeleteRelative(root, temporary, false)
		}
	}()
	for written := 0; written < len(body); {
		count, writeErr := file.Write(body[written:])
		if writeErr != nil {
			return writeErr
		}
		if count <= 0 {
			return errors.New("private named authority Windows write was incomplete")
		}
		written += count
	}
	if err := file.Sync(); err != nil {
		return err
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return err
	}
	staged, err := io.ReadAll(io.LimitReader(file, int64(maxBytes)+1))
	if err != nil || !bytes.Equal(staged, body) {
		return errors.New("private named authority Windows staged write verification failed")
	}
	if err := privateWindowsRenameRelativeNoReplace(handle, root, name); err != nil {
		if err == windows.STATUS_OBJECT_NAME_COLLISION || err == windows.STATUS_OBJECT_NAME_EXISTS {
			return os.ErrExist
		}
		return err
	}
	tempExists = false
	if err := file.Sync(); err != nil {
		return err
	}
	if err := privateWindowsSyncDirectory(root); err != nil {
		return err
	}
	written, err := privateWindowsReadNamedAt(root, name, uint64(maxBytes))
	if err != nil || !bytes.Equal(written, body) {
		return errors.New("private named authority Windows committed write verification failed")
	}
	return nil
}

func secureReadPrivateNamedFile(authority privateRootAuthority, name string, maxBytes int) ([]byte, error) {
	if !privateWindowsComponent(name) || maxBytes <= 0 {
		return nil, errors.New("private named authority Windows read input is invalid")
	}
	root, err := authority.open()
	if err != nil {
		return nil, err
	}
	defer windows.CloseHandle(root)
	return privateWindowsReadNamedAt(root, name, uint64(maxBytes))
}

func privateWindowsRecoverNamedRoot(authority privateRootAuthority, name string, maxBytes uint64) error {
	root, err := authority.open()
	if err != nil {
		return err
	}
	defer windows.CloseHandle(root)
	if err := privateWindowsValidateNamedRoot(root, name, maxBytes); err != nil {
		return err
	}
	changed := false
	for batch := 0; batch < maxPrivateCASRecoveryBatches; batch++ {
		temps := make([]string, 0, privateCASScanPageEntries)
		targetSeen := false
		walkErr := privateCASWindowsWalkDir(root, func(entry os.DirEntry) error {
			if entry.Name() == name {
				if entry.IsDir() || targetSeen {
					return errors.New("private named authority Windows target is ambiguous")
				}
				targetSeen = true
				return nil
			}
			if entry.IsDir() || !privateNamedWriteTempName(entry.Name(), name) {
				return errors.New("private named authority Windows root contains unknown residue")
			}
			if len(temps) == cap(temps) {
				return errPrivateNamedRecoveryBatchFull
			}
			temps = append(temps, entry.Name())
			return nil
		})
		if walkErr != nil && !errors.Is(walkErr, errPrivateNamedRecoveryBatchFull) {
			return walkErr
		}
		if len(temps) == 0 {
			if walkErr != nil {
				return walkErr
			}
			if targetSeen {
				if _, err := privateWindowsReadNamedAt(root, name, maxBytes); err != nil {
					return err
				}
			}
			if changed {
				return privateWindowsSyncDirectory(root)
			}
			return nil
		}
		for _, temporary := range temps {
			info, err := privateWindowsInfoAt(root, temporary, false)
			size := uint64(info.FileSizeHigh)<<32 | uint64(info.FileSizeLow)
			if err != nil || info.NumberOfLinks != 1 || info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 || size > maxBytes {
				return errors.New("private named authority Windows temp is unsafe")
			}
			if err := privateWindowsDeleteRelative(root, temporary, false); err != nil {
				return err
			}
			changed = true
		}
		if err := privateWindowsSyncDirectory(root); err != nil {
			return err
		}
	}
	return errors.New("private named authority Windows recovery did not reach a fixed point")
}

var errPrivateNamedRecoveryBatchFull = errors.New("private named authority recovery batch is full")

func privateWindowsValidateNamedRoot(root windows.Handle, name string, maxBytes uint64) error {
	targetSeen := false
	tempCount := 0
	err := privateCASWindowsWalkDir(root, func(entry os.DirEntry) error {
		if entry.Name() == name {
			if entry.IsDir() || targetSeen {
				return errors.New("private named authority Windows target is ambiguous")
			}
			targetSeen = true
			return nil
		}
		if entry.IsDir() || !privateNamedWriteTempName(entry.Name(), name) {
			return errors.New("private named authority Windows root contains unknown residue")
		}
		if tempCount == privateCASScanPageEntries*maxPrivateCASRecoveryBatches {
			return errors.New("private named authority Windows recovery entry bound exceeded")
		}
		info, err := privateWindowsInfoAt(root, entry.Name(), false)
		size := uint64(info.FileSizeHigh)<<32 | uint64(info.FileSizeLow)
		if err != nil || info.NumberOfLinks != 1 || size > maxBytes {
			return errors.New("private named authority Windows temp is unsafe")
		}
		tempCount++
		return nil
	})
	if err != nil {
		return err
	}
	if targetSeen {
		if _, err := privateWindowsReadNamedAt(root, name, maxBytes); err != nil {
			return err
		}
	}
	return nil
}

func privateWindowsReadNamedAt(parent windows.Handle, name string, maxBytes uint64) ([]byte, error) {
	handle, err := privateWindowsOpenRelative(parent, name, windows.FILE_GENERIC_READ, windows.FILE_OPEN, false)
	if privateWindowsNotFound(err) {
		return nil, os.ErrNotExist
	}
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(handle), name)
	if file == nil {
		_ = windows.CloseHandle(handle)
		return nil, errors.New("private named authority Windows file handle is invalid")
	}
	defer file.Close()
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(handle, &info); err != nil || info.NumberOfLinks != 1 || info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		return nil, errors.New("private named authority Windows file is unsafe")
	}
	size := uint64(info.FileSizeHigh)<<32 | uint64(info.FileSizeLow)
	if size == 0 || size > maxBytes {
		return nil, errors.New("private named authority Windows file size is invalid")
	}
	body, err := io.ReadAll(io.LimitReader(file, int64(maxBytes)+1))
	if err != nil || uint64(len(body)) != size {
		return nil, errors.New("private named authority Windows file read failed")
	}
	return body, nil
}

func secureWritePrivateFileExclusive(authority privateRootAuthority, digest string, body []byte) error {
	if !validPrivateDigest(digest) || len(body) == 0 || len(body) > maxPrivateAcceptedFinalBytes {
		return errors.New("private authority secure write input is invalid")
	}
	root, err := authority.open()
	if err != nil {
		return err
	}
	defer windows.CloseHandle(root)
	shard, err := privateWindowsOpenShard(root, digest[:2], true)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(shard)
	name := digest + ".json"
	suffix := make([]byte, 12)
	if _, err := rand.Read(suffix); err != nil {
		return err
	}
	temporary := "." + name + "-" + hex.EncodeToString(suffix) + ".tmp"
	handle, err := privateWindowsOpenRelative(shard, temporary, windows.FILE_GENERIC_READ|windows.FILE_GENERIC_WRITE|windows.DELETE, windows.FILE_CREATE, false)
	if err != nil {
		return err
	}
	file := os.NewFile(uintptr(handle), temporary)
	if file == nil {
		_ = windows.CloseHandle(handle)
		return errors.New("private authority Windows temp handle is invalid")
	}
	tempExists := true
	defer func() {
		_ = file.Close()
		if tempExists {
			_ = privateWindowsDeleteRelative(shard, temporary, false)
		}
	}()
	for written := 0; written < len(body); {
		count, writeErr := file.Write(body[written:])
		if writeErr != nil {
			return writeErr
		}
		if count <= 0 {
			return errors.New("private authority Windows write was incomplete")
		}
		written += count
	}
	if err := file.Sync(); err != nil {
		return err
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return err
	}
	staged, err := io.ReadAll(io.LimitReader(file, maxPrivateAcceptedFinalBytes+1))
	if err != nil || !bytes.Equal(staged, body) {
		return errors.New("private authority Windows staged write verification failed")
	}
	if err := privateWindowsRenameRelativeNoReplace(handle, shard, name); err != nil {
		if err == windows.STATUS_OBJECT_NAME_COLLISION || err == windows.STATUS_OBJECT_NAME_EXISTS {
			return os.ErrExist
		}
		return err
	}
	tempExists = false
	if err := file.Sync(); err != nil {
		return err
	}
	if err := privateWindowsSyncDirectory(shard); err != nil {
		return err
	}
	written, err := privateWindowsReadAt(shard, name)
	if err != nil || !bytes.Equal(written, body) {
		return errors.New("private authority Windows committed write verification failed")
	}
	return nil
}

func secureReadPrivateFile(authority privateRootAuthority, digest string) ([]byte, error) {
	if !validPrivateDigest(digest) {
		return nil, errors.New("private authority content address is invalid")
	}
	root, err := authority.open()
	if err != nil {
		return nil, err
	}
	defer windows.CloseHandle(root)
	shard, err := privateWindowsOpenShard(root, digest[:2], false)
	if err != nil {
		return nil, err
	}
	defer windows.CloseHandle(shard)
	return privateWindowsReadAt(shard, digest+".json")
}

func secureListPrivateFiles(ctx context.Context, authority privateRootAuthority) ([]securePrivateFile, error) {
	files := make([]securePrivateFile, 0)
	var aggregateBytes uint64
	err := secureVisitPrivateFiles(ctx, authority, func(file securePrivateFile) error {
		if len(files) == maxSecurePrivateCASListRecords || uint64(len(file.Body)) > maxSecurePrivateCASListAggregateBytes-aggregateBytes {
			return ErrSecurePrivateCASMaterializationLimit
		}
		aggregateBytes += uint64(len(file.Body))
		files = append(files, file)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(files, func(left, right int) bool { return files[left].Digest < files[right].Digest })
	return files, nil
}

func secureVisitPrivateFiles(ctx context.Context, authority privateRootAuthority, visit func(securePrivateFile) error) error {
	if visit == nil {
		return errors.New("private authority visitor is required")
	}
	root, err := authority.open()
	if err != nil {
		return err
	}
	defer windows.CloseHandle(root)
	entries, err := privateCASWindowsReadDirBounded(root, maxSecurePrivateCASShards)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if err := contextErr(ctx); err != nil {
			return err
		}
		if !entry.IsDir() || !validPrivateShard(entry.Name()) {
			return errors.New("private authority inventory contains a non-canonical shard")
		}
		shard, err := privateWindowsOpenShard(root, entry.Name(), false)
		if err != nil {
			return err
		}
		var records uint64
		walkErr := privateCASWindowsWalkDir(shard, func(child os.DirEntry) error {
			if err := contextErr(ctx); err != nil {
				return err
			}
			digest := strings.TrimSuffix(child.Name(), ".json")
			if child.IsDir() || filepath.Ext(child.Name()) != ".json" || !validPrivateDigest(digest) || digest[:2] != entry.Name() {
				return errors.New("private authority inventory contains a non-canonical record")
			}
			body, err := privateWindowsReadAt(shard, child.Name())
			if err != nil {
				return err
			}
			records++
			return visit(securePrivateFile{Digest: digest, Body: body})
		})
		if walkErr != nil {
			_ = windows.CloseHandle(shard)
			return walkErr
		}
		if records == 0 {
			_ = windows.CloseHandle(shard)
			return errors.New("private authority inventory contains an empty shard")
		}
		if err := windows.CloseHandle(shard); err != nil {
			return err
		}
	}
	return nil
}

func (authority privateRootAuthority) open() (windows.Handle, error) {
	handle, err := privateWindowsOpenAbsoluteDirectory(authority.path, false)
	if err != nil {
		return 0, err
	}
	identity, err := privateWindowsValidateAuthorityObject(handle, true, 0)
	if err != nil || identity.id != authority.id {
		_ = windows.CloseHandle(handle)
		return 0, errors.New("private authority root identity changed")
	}
	return handle, nil
}

func privateWindowsOpenAbsoluteDirectory(path string, create bool) (windows.Handle, error) {
	path = filepath.Clean(path)
	volume := filepath.VolumeName(path)
	if volume == "" || !filepath.IsAbs(path) {
		return 0, errors.New("private authority Windows root is invalid")
	}
	volumeRoot := volume + string(os.PathSeparator)
	current, err := privateWindowsOpenAbsolute(volumeRoot, privateWindowsTraverseDirectoryAccess, windows.FILE_OPEN, true)
	if err != nil {
		return 0, err
	}
	remainder := strings.Trim(strings.TrimPrefix(path, volume), `\/`)
	components := strings.FieldsFunc(remainder, func(char rune) bool { return char == '\\' || char == '/' })
	if len(components) == 0 {
		_ = windows.CloseHandle(current)
		return privateWindowsOpenAbsolute(volumeRoot, privateWindowsMutateDirectoryAccess, windows.FILE_OPEN, true)
	}
	currentPath := volumeRoot
	for index, component := range components {
		if !privateWindowsComponent(component) {
			_ = windows.CloseHandle(current)
			return 0, errors.New("private authority Windows root component is invalid")
		}
		access := uint32(privateWindowsTraverseDirectoryAccess)
		if index == len(components)-1 {
			access = privateWindowsMutateDirectoryAccess
		}
		next, openErr := privateWindowsOpenRelative(current, component, access, windows.FILE_OPEN, true)
		if privateWindowsNotFound(openErr) && create {
			mutationParent, mutationErr := privateWindowsReopenSameDirectoryForMutation(currentPath, current)
			if mutationErr != nil {
				_ = windows.CloseHandle(current)
				return 0, mutationErr
			}
			next, openErr = privateWindowsOpenRelative(
				mutationParent, component, privateWindowsMutateDirectoryAccess, windows.FILE_CREATE, true,
			)
			if privateWindowsCollision(openErr) {
				openErr = errors.New("private authority Windows root creation collided")
			}
			if openErr == nil {
				openErr = errors.Join(privateWindowsSyncDirectory(next), privateWindowsSyncDirectory(mutationParent))
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

func privateWindowsReopenSameDirectoryForMutation(path string, expected windows.Handle) (windows.Handle, error) {
	expectedID, err := privateWindowsHandleFileID(expected, true)
	if err != nil {
		return 0, err
	}
	mutation, err := privateWindowsOpenAbsolute(path, privateWindowsMutateDirectoryAccess, windows.FILE_OPEN, true)
	if err != nil {
		return 0, err
	}
	current, err := privateWindowsValidateAuthorityObject(mutation, true, 0)
	if err != nil || current.id != expectedID {
		_ = windows.CloseHandle(mutation)
		return 0, errors.New("private authority Windows mutation parent identity changed")
	}
	return mutation, nil
}

func privateWindowsOpenAbsolute(path string, access, disposition uint32, directory bool) (windows.Handle, error) {
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
	return privateWindowsNtCreate(attributes, access, disposition, directory)
}

func privateWindowsOpenRelative(parent windows.Handle, name string, access, disposition uint32, directory bool) (windows.Handle, error) {
	if !privateWindowsComponent(name) {
		return 0, errors.New("private authority Windows path component is invalid")
	}
	objectName, err := windows.NewNTUnicodeString(name)
	if err != nil {
		return 0, err
	}
	attributes := &windows.OBJECT_ATTRIBUTES{RootDirectory: parent, ObjectName: objectName, Attributes: windows.OBJ_CASE_INSENSITIVE | windows.OBJ_DONT_REPARSE}
	if disposition == windows.FILE_CREATE {
		descriptor, err := privateWindowsAuthoritySecurityDescriptor()
		if err != nil {
			return 0, err
		}
		attributes.SecurityDescriptor = descriptor
	}
	attributes.Length = uint32(unsafe.Sizeof(*attributes))
	return privateWindowsNtCreate(attributes, access, disposition, directory)
}

func privateWindowsAuthoritySecurityDescriptor() (*windows.SECURITY_DESCRIPTOR, error) {
	token, err := windows.OpenCurrentProcessToken()
	if err != nil {
		return nil, err
	}
	defer token.Close()
	user, err := token.GetTokenUser()
	if err != nil || user == nil || user.User.Sid == nil || !user.User.Sid.IsValid() {
		return nil, errors.New("private authority Windows current user SID is invalid")
	}
	sid := user.User.Sid.String()
	return windows.SecurityDescriptorFromString(
		"O:" + sid + "D:P(A;;FA;;;" + sid + ")(A;;FA;;;SY)(A;;FA;;;BA)",
	)
}

func privateWindowsNtCreate(attributes *windows.OBJECT_ATTRIBUTES, access, disposition uint32, directory bool) (windows.Handle, error) {
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
	err := windows.NtCreateFile(&handle, access, attributes, &status, &allocation, fileAttributes, privateWindowsShare, disposition, options, 0, 0)
	if err != nil {
		return 0, err
	}
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(handle, &info); err != nil || info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 ||
		(directory && info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY == 0) || (!directory && info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY != 0) {
		_ = windows.CloseHandle(handle)
		return 0, errors.New("private authority Windows handle crossed a reparse boundary")
	}
	return handle, nil
}

func privateWindowsReopenFile(
	handle windows.Handle,
	access uint32,
	share uint32,
	flags uint32,
) (windows.Handle, error) {
	reopened, _, callErr := privateWindowsReOpenFile.Call(
		uintptr(handle),
		uintptr(access),
		uintptr(share),
		uintptr(flags),
	)
	result := windows.Handle(reopened)
	if result == windows.InvalidHandle {
		if callErr != nil && callErr != syscall.Errno(0) {
			return 0, callErr
		}
		return 0, errors.New("private authority Windows handle could not be reopened")
	}
	return result, nil
}

func privateWindowsOpenShard(root windows.Handle, shard string, create bool) (windows.Handle, error) {
	if !validPrivateShard(shard) {
		return 0, errors.New("private authority Windows shard is invalid")
	}
	rootIdentity, err := privateWindowsValidateAuthorityObject(root, true, 0)
	if err != nil {
		return 0, err
	}
	handle, present, err := privateCASWindowsOpenExactBoundDirectory(
		root,
		shard,
		rootIdentity.id.VolumeSerialNumber,
	)
	if err == nil && !present && create {
		handle, err = privateCASWindowsCreateBoundDirectory(
			root,
			shard,
			rootIdentity.id.VolumeSerialNumber,
		)
		present = err == nil
	}
	if err == nil && !present {
		return 0, os.ErrNotExist
	}
	if err != nil {
		return 0, err
	}
	if _, err := privateWindowsValidateAuthorityObject(handle, true, 0); err != nil {
		_ = windows.CloseHandle(handle)
		return 0, err
	}
	return handle, nil
}

// privateWindowsOpenOrCreateRelativeDirectory retries FILE_OPEN after a
// concurrent FILE_CREATE winner. The retry remains relative to the same
// already-authorized parent handle, then verifies that the returned FileID is
// still the identity installed at that name. beforeCreate is test-only
// orchestration input used to deterministically exercise the collision cut.
func privateWindowsOpenOrCreateRelativeDirectory(parent windows.Handle, name string, openAccess, createAccess uint32, beforeCreate func()) (windows.Handle, bool, error) {
	handle, err := privateWindowsOpenRelative(parent, name, openAccess, windows.FILE_OPEN, true)
	if err == nil {
		if err := privateWindowsVerifyRelativeDirectoryIdentity(parent, name, handle); err != nil {
			_ = windows.CloseHandle(handle)
			return 0, false, err
		}
		return handle, false, nil
	}
	if !privateWindowsNotFound(err) {
		return 0, false, err
	}
	if beforeCreate != nil {
		beforeCreate()
	}
	handle, err = privateWindowsOpenRelative(parent, name, createAccess, windows.FILE_CREATE, true)
	created := err == nil
	if privateWindowsCollision(err) {
		handle, err = privateWindowsOpenRelative(parent, name, openAccess, windows.FILE_OPEN, true)
		created = false
	}
	if err != nil {
		return 0, false, err
	}
	if created {
		if err := privateWindowsSyncDirectory(parent); err != nil {
			_ = windows.CloseHandle(handle)
			return 0, false, err
		}
	}
	if err := privateWindowsVerifyRelativeDirectoryIdentity(parent, name, handle); err != nil {
		_ = windows.CloseHandle(handle)
		return 0, false, err
	}
	return handle, created, nil
}

func privateWindowsVerifyRelativeDirectoryIdentity(parent windows.Handle, name string, expected windows.Handle) error {
	expectedIdentity, err := privateWindowsValidateAuthorityObject(expected, true, 0)
	if err != nil {
		return errors.New("private authority Windows directory identity is unsafe")
	}
	reopened, err := privateWindowsOpenRelative(parent, name, windows.FILE_GENERIC_READ, windows.FILE_OPEN, true)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(reopened)
	currentIdentity, err := privateWindowsValidateAuthorityObject(reopened, true, 0)
	if err != nil || currentIdentity.id != expectedIdentity.id {
		return errors.New("private authority Windows directory name changed during open")
	}
	return nil
}

func privateWindowsReadAt(parent windows.Handle, name string) ([]byte, error) {
	handle, err := privateWindowsOpenRelative(parent, name, windows.FILE_GENERIC_READ, windows.FILE_OPEN, false)
	if privateWindowsNotFound(err) {
		return nil, os.ErrNotExist
	}
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(handle), name)
	if file == nil {
		_ = windows.CloseHandle(handle)
		return nil, errors.New("private authority Windows file handle is invalid")
	}
	defer file.Close()
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(handle, &info); err != nil || info.NumberOfLinks != 1 || info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 || info.FileSizeHigh != 0 || info.FileSizeLow == 0 || info.FileSizeLow > maxPrivateAcceptedFinalBytes {
		return nil, errors.New("private authority Windows file is unsafe")
	}
	body, err := io.ReadAll(io.LimitReader(file, maxPrivateAcceptedFinalBytes+1))
	if err != nil || len(body) != int(info.FileSizeLow) {
		return nil, errors.New("private authority Windows file read failed")
	}
	return body, nil
}

func privateWindowsInfoAt(parent windows.Handle, name string, directory bool) (windows.ByHandleFileInformation, error) {
	handle, err := privateWindowsOpenRelative(parent, name, windows.FILE_GENERIC_READ, windows.FILE_OPEN, directory)
	if err != nil {
		return windows.ByHandleFileInformation{}, err
	}
	defer windows.CloseHandle(handle)
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(handle, &info); err != nil ||
		info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 ||
		(directory && info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY == 0) ||
		(!directory && info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY != 0) ||
		privateWindowsValidateProtectedDACL(handle) != nil || privateWindowsValidateStreams(handle, directory) != nil {
		return windows.ByHandleFileInformation{}, errors.New("private authority Windows object metadata or ACL is unsafe")
	}
	var id privateWindowsFileIDInfo
	if err := windows.GetFileInformationByHandleEx(
		handle, windows.FileIdInfo, (*byte)(unsafe.Pointer(&id)), uint32(unsafe.Sizeof(id)),
	); err != nil || id.VolumeSerialNumber == 0 || id.FileID == ([16]byte{}) {
		return windows.ByHandleFileInformation{}, errors.New("private authority Windows object FileID is unavailable")
	}
	return info, nil
}

func privateWindowsReadDir(parent windows.Handle) ([]os.DirEntry, error) {
	handle, err := privateWindowsReopenFile(
		parent,
		privateWindowsObserveDirectoryAccess,
		privateWindowsShare,
		windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT,
	)
	if err != nil {
		return nil, err
	}
	directory := os.NewFile(uintptr(handle), "private-authority-directory")
	if directory == nil {
		_ = windows.CloseHandle(handle)
		return nil, errors.New("private authority Windows directory handle is invalid")
	}
	entries, readErr := directory.ReadDir(-1)
	closeErr := directory.Close()
	return entries, errors.Join(readErr, closeErr)
}

func privateWindowsRenameRelativeNoReplace(handle windows.Handle, parent windows.Handle, target string) error {
	targetName, err := windows.UTF16FromString(target)
	if err != nil {
		return err
	}
	nameLength := (len(targetName) - 1) * 2
	dummy := privateWindowsRenameInformation{}
	buffer := make([]byte, int(unsafe.Offsetof(dummy.FileName))+nameLength)
	info := (*privateWindowsRenameInformation)(unsafe.Pointer(&buffer[0]))
	info.RootDirectory = parent
	info.FileNameLength = uint32(nameLength)
	copy((*[windows.MAX_LONG_PATH]uint16)(unsafe.Pointer(&info.FileName[0]))[:nameLength/2:nameLength/2], targetName[:len(targetName)-1])
	var status windows.IO_STATUS_BLOCK
	return windows.NtSetInformationFile(handle, &status, &buffer[0], uint32(len(buffer)), windows.FileRenameInformation)
}

func privateWindowsDeleteRelative(parent windows.Handle, name string, directory bool) error {
	handle, err := privateWindowsOpenRelative(parent, name, windows.DELETE|windows.FILE_READ_ATTRIBUTES, windows.FILE_OPEN, directory)
	if privateWindowsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	defer windows.CloseHandle(handle)
	return privateWindowsDeleteHandle(handle)
}

func privateWindowsDeleteHandle(handle windows.Handle) error {
	flags := uint32(windows.FILE_DISPOSITION_DELETE | windows.FILE_DISPOSITION_POSIX_SEMANTICS | windows.FILE_DISPOSITION_IGNORE_READONLY_ATTRIBUTE)
	buffer := (*[4]byte)(unsafe.Pointer(&flags))
	var status windows.IO_STATUS_BLOCK
	return windows.NtSetInformationFile(handle, &status, &buffer[0], uint32(len(buffer)), windows.FileDispositionInformationEx)
}

func privateWindowsSyncDirectory(handle windows.Handle) error {
	return windows.FlushFileBuffers(handle)
}

func privateWindowsComponent(component string) bool {
	if component == "" || component == "." || component == ".." || strings.ContainsAny(component, `:<>"|?*`) || strings.TrimRight(component, ". ") != component {
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

func privateWindowsNotFound(err error) bool {
	return err == windows.STATUS_OBJECT_NAME_NOT_FOUND || err == windows.STATUS_OBJECT_PATH_NOT_FOUND || errors.Is(err, os.ErrNotExist)
}

func privateWindowsCollision(err error) bool {
	return err == windows.STATUS_OBJECT_NAME_COLLISION || err == windows.STATUS_OBJECT_NAME_EXISTS || errors.Is(err, os.ErrExist)
}
