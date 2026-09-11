//go:build windows

package persistencefs

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"strings"
	"unsafe"

	domainstartup "analytix.local/runtime-go/internal/domain/startup"
	"golang.org/x/sys/windows"
)

type startupPrivateDirectory struct {
	handle   windows.Handle
	identity string
}

func secureStartupOpenRootDirectory(root startupAuthorityRoot) (*startupPrivateDirectory, error) {
	handle, err := root.open()
	if err != nil {
		return nil, err
	}
	identity, err := windowsOpenedDirectoryIdentity(handle)
	if err != nil {
		_ = windows.CloseHandle(handle)
		return nil, err
	}
	return &startupPrivateDirectory{handle: handle, identity: identity}, nil
}

func secureStartupCreateDirectory(root startupAuthorityRoot, name string) (*startupPrivateDirectory, error) {
	parent, err := root.open()
	if err != nil {
		return nil, err
	}
	defer windows.CloseHandle(parent)
	return secureStartupCreateDirectoryAt(parent, name)
}

func secureStartupCreateDirectoryAt(parent windows.Handle, name string) (*startupPrivateDirectory, error) {
	if !secureWindowsComponent(name) {
		return nil, errors.New("startup private Windows directory name is invalid")
	}
	handle, err := secureWindowsOpenRelative(parent, name, windows.FILE_GENERIC_READ|windows.FILE_GENERIC_WRITE|windows.DELETE, windows.FILE_CREATE, true)
	if err == windows.STATUS_OBJECT_NAME_COLLISION || err == windows.STATUS_OBJECT_NAME_EXISTS {
		return nil, os.ErrExist
	}
	if err != nil {
		return nil, err
	}
	identity, err := windowsOpenedDirectoryIdentity(handle)
	if err != nil {
		_ = windows.CloseHandle(handle)
		return nil, err
	}
	if err := errors.Join(secureWindowsSyncDirectory(handle), secureWindowsSyncDirectory(parent)); err != nil {
		_ = windows.CloseHandle(handle)
		return nil, err
	}
	return &startupPrivateDirectory{handle: handle, identity: identity}, nil
}

func secureStartupOpenDirectory(root startupAuthorityRoot, name, expectedIdentity string) (*startupPrivateDirectory, error) {
	parent, err := root.open()
	if err != nil {
		return nil, err
	}
	defer windows.CloseHandle(parent)
	return secureStartupOpenDirectoryAt(parent, name, expectedIdentity)
}

func secureStartupOpenDirectoryAt(parent windows.Handle, name, expectedIdentity string) (*startupPrivateDirectory, error) {
	if !secureWindowsComponent(name) {
		return nil, errors.New("startup private Windows directory name is invalid")
	}
	handle, err := secureWindowsOpenRelative(parent, name, windows.FILE_GENERIC_READ|windows.FILE_GENERIC_WRITE|windows.DELETE, windows.FILE_OPEN, true)
	if secureWindowsNotFound(err) {
		return nil, os.ErrNotExist
	}
	if err != nil {
		return nil, err
	}
	identity, err := windowsOpenedDirectoryIdentity(handle)
	if err != nil || expectedIdentity != "" && identity != expectedIdentity {
		_ = windows.CloseHandle(handle)
		return nil, errors.New("startup private Windows directory identity changed")
	}
	return &startupPrivateDirectory{handle: handle, identity: identity}, nil
}

func (directory *startupPrivateDirectory) OpenDirectory(name, expectedIdentity string) (*startupPrivateDirectory, error) {
	if directory == nil || directory.handle == 0 {
		return nil, errors.New("startup private Windows directory is unavailable")
	}
	return secureStartupOpenDirectoryAt(directory.handle, name, expectedIdentity)
}

func (directory *startupPrivateDirectory) CreateDirectory(name string) (*startupPrivateDirectory, error) {
	if directory == nil || directory.handle == 0 {
		return nil, errors.New("startup private Windows directory is unavailable")
	}
	return secureStartupCreateDirectoryAt(directory.handle, name)
}

func (directory *startupPrivateDirectory) Close() error {
	if directory == nil || directory.handle == 0 {
		return nil
	}
	handle := directory.handle
	directory.handle = 0
	return windows.CloseHandle(handle)
}

func (directory *startupPrivateDirectory) Identity() string {
	if directory == nil {
		return ""
	}
	return directory.identity
}

func (directory *startupPrivateDirectory) ReadFile(name string, maxBytes int64, allowEmpty bool) ([]byte, string, error) {
	if directory == nil || directory.handle == 0 {
		return nil, "", errors.New("startup private Windows directory is unavailable")
	}
	body, identity, err := secureStartupAuthorityReadAt(directory.handle, name, maxBytes)
	if allowEmpty && err != nil {
		handle, openErr := secureWindowsOpenRelative(directory.handle, name, windows.FILE_GENERIC_READ, windows.FILE_OPEN, false)
		if openErr != nil {
			return nil, "", err
		}
		file := os.NewFile(uintptr(handle), name)
		if file == nil {
			_ = windows.CloseHandle(handle)
			return nil, "", errors.New("startup private Windows empty handle is invalid")
		}
		defer file.Close()
		var info windows.ByHandleFileInformation
		if windows.GetFileInformationByHandle(handle, &info) != nil || info.NumberOfLinks != 1 ||
			info.FileAttributes&(windows.FILE_ATTRIBUTE_REPARSE_POINT|windows.FILE_ATTRIBUTE_DIRECTORY) != 0 || info.FileSizeHigh != 0 || info.FileSizeLow != 0 {
			return nil, "", errors.New("startup private Windows empty file is unsafe")
		}
		identity, err = windowsOpenedObjectIdentity(handle, false)
		if err != nil {
			return nil, "", errors.New("startup private Windows empty file identity is unavailable")
		}
		return []byte{}, identity, nil
	}
	return body, identity, err
}

func (directory *startupPrivateDirectory) WriteExclusive(name string, body []byte, maxBytes int64) error {
	if directory == nil || directory.handle == 0 {
		return errors.New("startup private Windows directory is unavailable")
	}
	temporary, err := startupPrivateTempName(name)
	if err != nil {
		return err
	}
	return secureStartupWindowsWrite(directory.handle, name, temporary, body, maxBytes, false, nil)
}

func (directory *startupPrivateDirectory) WriteReplace(name string, body []byte, maxBytes int64, fault func(string) error) error {
	if directory == nil || directory.handle == 0 {
		return errors.New("startup private Windows directory is unavailable")
	}
	return secureStartupWindowsWrite(directory.handle, name, "", body, maxBytes, true, fault)
}

func (directory *startupPrivateDirectory) writeExclusiveWithTemporaryName(
	name string,
	temporary string,
	body []byte,
	maxBytes int64,
) error {
	if directory == nil || directory.handle == 0 {
		return errors.New("startup private Windows directory is unavailable")
	}
	return secureStartupWindowsWrite(directory.handle, name, temporary, body, maxBytes, false, nil)
}

func secureStartupWindowsWrite(
	parent windows.Handle,
	name string,
	temporary string,
	body []byte,
	maxBytes int64,
	replace bool,
	fault func(string) error,
) error {
	if !secureWindowsComponent(name) || maxBytes <= 0 || int64(len(body)) > maxBytes || replace && len(body) == 0 {
		return errors.New("startup private Windows write input is invalid")
	}
	previousIdentity := ""
	previousExists := false
	var err error
	if replace {
		_, previousIdentity, err = secureStartupAuthorityReadAt(parent, name, maxBytes)
		previousExists = err == nil
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return errors.New("startup private Windows replacement target is unsafe")
		}
	}
	if replace {
		temporary, err = startupPrivateRandomName(semanticJournalTempPrefix)
		if err == nil {
			temporary += semanticJournalTempSuffix
		}
	} else if !secureWindowsComponent(temporary) || temporary == name {
		return errors.New("startup private Windows exclusive write input is invalid")
	}
	if err != nil {
		return err
	}
	handle, err := secureWindowsOpenRelative(parent, temporary, windows.FILE_GENERIC_READ|windows.FILE_GENERIC_WRITE|windows.DELETE, windows.FILE_CREATE, false)
	if err != nil {
		return err
	}
	file := os.NewFile(uintptr(handle), temporary)
	if file == nil {
		_ = windows.CloseHandle(handle)
		return errors.New("startup private Windows temp handle is invalid")
	}
	preserve := false
	defer func() {
		_ = file.Close()
		if !preserve {
			_ = secureWindowsDeleteRelative(parent, temporary, domainstartup.ManagedEntryTypeFile)
		}
	}()
	if err := startupPrivateWindowsWriteAndVerify(file, body, maxBytes); err != nil {
		return err
	}
	if fault != nil {
		if err := fault("after_journal_temp_sync"); err != nil {
			preserve = true
			return err
		}
	}
	if replace {
		_, currentIdentity, currentErr := secureStartupAuthorityReadAt(parent, name, maxBytes)
		if previousExists {
			if currentErr != nil || currentIdentity != previousIdentity {
				return errors.New("startup private Windows replacement target changed before commit")
			}
			if err := startupWindowsRenameReplace(handle, parent, name); err != nil {
				return err
			}
		} else {
			if !errors.Is(currentErr, os.ErrNotExist) {
				return errors.New("startup private Windows replacement target appeared before commit")
			}
			if err := startupWindowsRenameNoReplace(handle, parent, name); err != nil {
				return err
			}
		}
	} else if err := startupWindowsRenameNoReplace(handle, parent, name); err != nil {
		if err == windows.STATUS_OBJECT_NAME_COLLISION || err == windows.STATUS_OBJECT_NAME_EXISTS {
			return os.ErrExist
		}
		return err
	}
	preserve = true
	if fault != nil {
		if err := fault("after_journal_replace"); err != nil {
			return err
		}
	}
	if err := errors.Join(file.Sync(), secureWindowsSyncDirectory(parent)); err != nil {
		return err
	}
	borrowed := &startupPrivateDirectory{handle: parent}
	written, _, err := borrowed.ReadFile(name, maxBytes, true)
	if err != nil || !bytes.Equal(written, body) {
		return errors.New("startup private Windows write readback failed")
	}
	return nil
}

func (directory *startupPrivateDirectory) readStartupExclusiveWriteResidue(name string, maxBytes int64) ([]byte, string, error) {
	if directory == nil || directory.handle == 0 {
		return nil, "", errors.New("startup private Windows directory is unavailable")
	}
	return secureStartupAuthorityReadAt(directory.handle, name, maxBytes)
}

func (directory *startupPrivateDirectory) removeStartupExclusiveWriteResidue(name, expectedIdentity string) error {
	if directory == nil || directory.handle == 0 || !secureWindowsComponent(name) || expectedIdentity == "" {
		return errors.New("startup private Windows exclusive-write residue deletion input is invalid")
	}
	handle, err := secureWindowsOpenRelativeWithShare(
		directory.handle, name, windows.FILE_GENERIC_READ|windows.DELETE|windows.FILE_READ_ATTRIBUTES,
		windows.FILE_OPEN, false, 0,
	)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(handle)
	var info windows.ByHandleFileInformation
	identity, identityErr := windowsOpenedObjectIdentity(handle, false)
	if windows.GetFileInformationByHandle(handle, &info) != nil || identityErr != nil || identity != expectedIdentity ||
		info.NumberOfLinks != 1 || info.FileAttributes&(windows.FILE_ATTRIBUTE_REPARSE_POINT|windows.FILE_ATTRIBUTE_DIRECTORY) != 0 {
		return errors.New("startup private Windows exclusive-write residue identity changed")
	}
	flags := uint32(windows.FILE_DISPOSITION_DELETE | windows.FILE_DISPOSITION_POSIX_SEMANTICS | windows.FILE_DISPOSITION_IGNORE_READONLY_ATTRIBUTE)
	buffer := (*[4]byte)(unsafe.Pointer(&flags))
	var status windows.IO_STATUS_BLOCK
	return windows.NtSetInformationFile(handle, &status, &buffer[0], uint32(len(buffer)), windows.FileDispositionInformationEx)
}

func (directory *startupPrivateDirectory) syncStartupExclusiveWriteResidues() error {
	if directory == nil || directory.handle == 0 {
		return errors.New("startup private Windows directory is unavailable")
	}
	return secureWindowsSyncDirectory(directory.handle)
}

func startupWindowsRenameReplace(handle, parent windows.Handle, target string) error {
	return startupWindowsRename(handle, parent, target, true)
}

func (directory *startupPrivateDirectory) Entries() ([]os.DirEntry, error) {
	return directory.ReadEntriesBounded(maxStartupPrivateEntries)
}

func (directory *startupPrivateDirectory) ReadEntriesBounded(limit int) ([]os.DirEntry, error) {
	return directory.ReadEntriesBoundedContext(context.Background(), limit)
}

func (directory *startupPrivateDirectory) ReadEntriesBoundedContext(ctx context.Context, limit int) ([]os.DirEntry, error) {
	if err := contextError(ctx); err != nil {
		return nil, err
	}
	if directory == nil || directory.handle == 0 {
		return nil, errors.New("startup private Windows directory is unavailable")
	}
	if limit < 0 || limit > maxStartupPrivateEntries {
		return nil, errors.New("startup private Windows directory entry limit is invalid")
	}
	duplicate, err := secureWindowsReopenDirectory(directory.handle)
	if err != nil {
		return nil, err
	}
	identity, err := windowsOpenedDirectoryIdentity(duplicate)
	if err != nil || identity != directory.identity {
		_ = windows.CloseHandle(duplicate)
		return nil, errors.New("startup private Windows directory changed before inventory")
	}
	file := os.NewFile(uintptr(duplicate), "startup-private-directory")
	if file == nil {
		_ = windows.CloseHandle(duplicate)
		return nil, errors.New("startup private Windows duplicate is invalid")
	}
	entries := make([]os.DirEntry, 0, min(limit, startupPrivateEntryPage))
	for {
		if err := contextError(ctx); err != nil {
			_ = file.Close()
			return nil, err
		}
		page, readErr := file.ReadDir(startupPrivateEntryPage)
		if len(entries)+len(page) > limit {
			_ = file.Close()
			return nil, StartupResourceLimitError{Code: "private_directory_entries", Limit: int64(limit)}
		}
		entries = append(entries, page...)
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			_ = file.Close()
			return nil, readErr
		}
	}
	closeErr := file.Close()
	if closeErr != nil {
		return nil, closeErr
	}
	if err := contextError(ctx); err != nil {
		return nil, err
	}
	current, err := secureWindowsReopenDirectory(directory.handle)
	if err != nil {
		return nil, errors.New("startup private Windows directory changed after inventory")
	}
	currentIdentity, identityErr := windowsOpenedDirectoryIdentity(current)
	closeErr = windows.CloseHandle(current)
	if identityErr != nil || closeErr != nil || currentIdentity != directory.identity {
		return nil, errors.New("startup private Windows directory changed after inventory")
	}
	return entries, nil
}

func (directory *startupPrivateDirectory) PreflightTree() error {
	return directory.PreflightTreeContext(context.Background())
}

func (directory *startupPrivateDirectory) PreflightTreeContext(ctx context.Context) error {
	if directory == nil || directory.handle == 0 {
		return errors.New("startup private Windows directory is unavailable")
	}
	count := 0
	return preflightWindowsPrivateTree(ctx, directory.handle, 0, &count)
}

func secureStartupRetireDirectory(root startupAuthorityRoot, name, expectedIdentity, prefix string) (string, *startupPrivateDirectory, error) {
	if !secureWindowsComponent(name) || !strings.HasPrefix(prefix, ".retired-") {
		return "", nil, errors.New("startup private Windows retirement input is invalid")
	}
	parent, err := root.open()
	if err != nil {
		return "", nil, err
	}
	defer windows.CloseHandle(parent)
	return secureStartupRetireDirectoryAt(parent, name, expectedIdentity, prefix)
}

func (directory *startupPrivateDirectory) RetireDirectory(name, expectedIdentity, prefix string) (string, *startupPrivateDirectory, error) {
	if directory == nil || directory.handle == 0 {
		return "", nil, errors.New("startup private Windows directory is unavailable")
	}
	return secureStartupRetireDirectoryAt(directory.handle, name, expectedIdentity, prefix)
}

func secureStartupRetireDirectoryAt(parent windows.Handle, name, expectedIdentity, prefix string) (string, *startupPrivateDirectory, error) {
	current, err := secureStartupOpenDirectoryAt(parent, name, expectedIdentity)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil, nil
	}
	if err != nil {
		return "", nil, err
	}
	retired, err := startupPrivateRandomName(prefix)
	if err != nil {
		_ = current.Close()
		return "", nil, err
	}
	if err := startupWindowsRenameNoReplace(current.handle, parent, retired); err != nil {
		_ = current.Close()
		return "", nil, err
	}
	if err := secureWindowsSyncDirectory(parent); err != nil {
		_ = current.Close()
		return "", nil, err
	}
	return retired, current, nil
}

func secureStartupRemoveRetiredDirectory(root startupAuthorityRoot, name string, directory *startupPrivateDirectory) error {
	return secureStartupRemoveRetiredDirectoryContext(context.Background(), root, name, directory)
}

func secureStartupRemoveRetiredDirectoryContext(ctx context.Context, root startupAuthorityRoot, name string, directory *startupPrivateDirectory) error {
	if directory == nil || directory.handle == 0 || !strings.HasPrefix(name, ".retired-") {
		return errors.New("startup retired Windows directory input is invalid")
	}
	parent, err := root.open()
	if err != nil {
		return err
	}
	defer windows.CloseHandle(parent)
	return secureStartupRemoveRetiredDirectoryAtContext(ctx, parent, name, directory)
}

func (directory *startupPrivateDirectory) RemoveRetiredDirectory(name string, retired *startupPrivateDirectory) error {
	return directory.RemoveRetiredDirectoryContext(context.Background(), name, retired)
}

func (directory *startupPrivateDirectory) RemoveRetiredDirectoryContext(ctx context.Context, name string, retired *startupPrivateDirectory) error {
	if directory == nil || directory.handle == 0 {
		return errors.New("startup private Windows directory is unavailable")
	}
	return secureStartupRemoveRetiredDirectoryAtContext(ctx, directory.handle, name, retired)
}

func secureStartupRemoveRetiredDirectoryAt(parent windows.Handle, name string, directory *startupPrivateDirectory) error {
	return secureStartupRemoveRetiredDirectoryAtContext(context.Background(), parent, name, directory)
}

func secureStartupRemoveRetiredDirectoryAtContext(ctx context.Context, parent windows.Handle, name string, directory *startupPrivateDirectory) error {
	if err := contextError(ctx); err != nil {
		return err
	}
	opened, err := secureStartupOpenDirectoryAt(parent, name, directory.identity)
	if err != nil {
		return err
	}
	defer opened.Close()
	if err := secureRemoveWindowsDirectoryContents(ctx, opened.handle); err != nil {
		return err
	}
	if err := secureWindowsDeleteRelative(parent, name, domainstartup.ManagedEntryTypeDirectory); err != nil {
		return err
	}
	return secureWindowsSyncDirectory(parent)
}

func startupPrivateWindowsWriteAndVerify(file *os.File, body []byte, maxBytes int64) error {
	for written := 0; written < len(body); {
		count, err := file.Write(body[written:])
		if err != nil {
			return err
		}
		if count <= 0 {
			return errors.New("startup private Windows write was incomplete")
		}
		written += count
	}
	if err := file.Sync(); err != nil {
		return err
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return err
	}
	written, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
	if err != nil || !bytes.Equal(written, body) {
		return errors.New("startup private Windows staged write verification failed")
	}
	return nil
}

func startupPrivateTempName(name string) (string, error) {
	random, err := startupPrivateRandomName("")
	if err != nil {
		return "", err
	}
	return "." + name + "-" + random + ".tmp", nil
}

func startupPrivateRandomName(prefix string) (string, error) {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return prefix + hex.EncodeToString(value), nil
}
