//go:build windows

package persistencefs

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"math"
	"os"
	"runtime"
	"sort"
	"strings"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	separateOwnerWindowsShareAll   = windows.FILE_SHARE_READ | windows.FILE_SHARE_WRITE | windows.FILE_SHARE_DELETE
	separateOwnerWindowsShareGuard = windows.FILE_SHARE_READ | windows.FILE_SHARE_WRITE
	separateOwnerWindowsShareRead  = windows.FILE_SHARE_READ

	// FILE_WRITE_DATA and FILE_APPEND_DATA are the directory-specific
	// FILE_ADD_FILE and FILE_ADD_SUBDIRECTORY rights. x/sys/windows v0.30.0
	// does not export the directory aliases. 0x40 is FILE_DELETE_CHILD.
	separateOwnerWindowsRootAccess = windows.FILE_GENERIC_READ | windows.FILE_GENERIC_WRITE |
		windows.FILE_TRAVERSE | 0x00000040
	separateOwnerWindowsDirectoryAccess = separateOwnerWindowsRootAccess | windows.DELETE
)

type separateOwnerWindowsRenameInformation struct {
	ReplaceIfExists uint32
	RootDirectory   windows.Handle
	FileNameLength  uint32
	FileName        [1]uint16
}

type separateOwnerWindowsFileMetadata struct {
	identity       string
	securityDigest string
	size           int64
	modified       int64
}

func (metadata separateOwnerWindowsFileMetadata) state(digest string) SeparateOwnerFileState {
	return SeparateOwnerFileState{
		Identity:         metadata.identity,
		SecurityDigest:   metadata.securityDigest,
		Size:             metadata.size,
		ModifiedUnixNano: metadata.modified,
		SHA256:           digest,
	}
}

func platformOpenSeparateOwnerRootDirectory(
	authority *SeparateOwnerRootAuthority,
	validate func() error,
) (*startupPrivateDirectory, SeparateOwnerDirectoryState, error) {
	binding, ok := authority.rootBinding()
	if !ok {
		return nil, SeparateOwnerDirectoryState{}, errors.New("separate-owner Windows root binding is unavailable")
	}
	opened, err := authority.openPinnedRoot()
	if err != nil {
		return nil, SeparateOwnerDirectoryState{}, err
	}
	state, err := separateOwnerWindowsDirectoryState(opened.handle, false)
	if err != nil || state.Identity != binding.Identity || state.SecurityDigest != binding.SecurityDigest || validate() != nil {
		_ = opened.Close()
		return nil, SeparateOwnerDirectoryState{}, errors.New("separate-owner Windows root changed while opening")
	}
	return opened, state, nil
}

func platformValidateSeparateOwnerOpenedDirectory(
	directory *startupPrivateDirectory,
	expected SeparateOwnerDirectoryState,
	requireProtected bool,
) error {
	if directory == nil || directory.handle == 0 {
		return errors.New("separate-owner Windows directory is unavailable")
	}
	current, err := separateOwnerWindowsDirectoryState(directory.handle, requireProtected)
	if err != nil || current != expected {
		return errors.New("separate-owner Windows directory identity or permissions changed")
	}
	return nil
}

func platformValidateSeparateOwnerChildDirectory(
	parent *startupPrivateDirectory,
	name string,
	expectedIdentity string,
) error {
	if parent == nil || parent.handle == 0 || !separateOwnerWindowsComponent(name) || expectedIdentity == "" {
		return errors.New("separate-owner Windows child directory binding is invalid")
	}
	return separateOwnerWindowsVerifyNameMapping(parent.handle, name, expectedIdentity, true)
}

func separateOwnerWindowsReopenRootForAccess(parent windows.Handle) (windows.Handle, error) {
	name, err := windows.NewNTUnicodeString(".")
	if err != nil {
		return 0, err
	}
	attributes := &windows.OBJECT_ATTRIBUTES{
		RootDirectory: parent,
		ObjectName:    name,
		Attributes:    windows.OBJ_CASE_INSENSITIVE | windows.OBJ_DONT_REPARSE,
	}
	attributes.Length = uint32(unsafe.Sizeof(*attributes))
	return separateOwnerWindowsNTCreate(
		attributes,
		separateOwnerWindowsRootAccess,
		windows.FILE_OPEN,
		true,
		separateOwnerWindowsShareGuard,
		false,
	)
}

func separateOwnerWindowsDirectoryState(
	handle windows.Handle,
	requireProtected bool,
) (SeparateOwnerDirectoryState, error) {
	identity, err := windowsOpenedObjectIdentity(handle, true)
	if err != nil {
		return SeparateOwnerDirectoryState{}, errors.New("separate-owner Windows directory identity is unavailable")
	}
	securityDigest, err := separateOwnerWindowsSecurityDigest(handle, true, true, requireProtected)
	if err != nil {
		return SeparateOwnerDirectoryState{}, err
	}
	protected, err := separateOwnerWindowsDACLProtected(handle)
	if err != nil || requireProtected && !protected {
		return SeparateOwnerDirectoryState{}, errors.New("separate-owner Windows directory DACL state is unavailable")
	}
	return SeparateOwnerDirectoryState{
		Identity: identity, SecurityDigest: securityDigest, Protected: protected,
	}, nil
}

func separateOwnerWindowsDACLProtected(handle windows.Handle) (bool, error) {
	descriptor, err := windows.GetSecurityInfo(handle, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil || descriptor == nil || !descriptor.IsValid() {
		return false, errors.New("separate-owner Windows DACL is unavailable")
	}
	control, _, err := descriptor.Control()
	if err != nil {
		return false, err
	}
	return control&windows.SE_DACL_PROTECTED != 0, nil
}

func (directory *SeparateOwnerDirectory) OpenDirectory(
	ctx context.Context,
	name string,
	requireProtected bool,
) (*SeparateOwnerDirectory, bool, error) {
	if err := contextSeparateOwnerError(ctx); err != nil {
		return nil, false, err
	}
	if err := directory.Validate(); err != nil || !separateOwnerWindowsComponent(name) {
		return nil, false, errors.New("separate-owner Windows directory open authority is invalid")
	}
	handle, err := separateOwnerWindowsOpenRelative(
		directory.directory.handle,
		name,
		separateOwnerWindowsDirectoryAccess,
		windows.FILE_OPEN,
		true,
		separateOwnerWindowsShareGuard,
		false,
	)
	if separateOwnerWindowsNotFound(err) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	state, stateErr := separateOwnerWindowsDirectoryState(handle, requireProtected)
	nameErr := separateOwnerWindowsVerifyOpenedComponent(handle, name)
	mappingErr := separateOwnerWindowsVerifyNameMapping(directory.directory.handle, name, state.Identity, true)
	parentErr := directory.Validate()
	contextErr := contextSeparateOwnerError(ctx)
	if stateErr != nil || nameErr != nil || mappingErr != nil || parentErr != nil || contextErr != nil {
		closeErr := windows.CloseHandle(handle)
		return nil, false, errors.Join(
			errors.New("separate-owner Windows directory changed while opening"),
			stateErr, nameErr, mappingErr, parentErr, contextErr, closeErr,
		)
	}
	return &SeparateOwnerDirectory{
		directory: &startupPrivateDirectory{handle: handle, identity: state.Identity},
		state:     state, requireProtected: requireProtected, validateLease: directory.validateLease,
		parent: directory, name: name,
	}, true, nil
}

func (directory *SeparateOwnerDirectory) CreatePrivateDirectory(
	ctx context.Context,
	name string,
) (*SeparateOwnerDirectory, error) {
	if err := contextSeparateOwnerError(ctx); err != nil {
		return nil, err
	}
	if err := directory.Validate(); err != nil || !separateOwnerWindowsComponent(name) {
		return nil, errors.New("separate-owner Windows directory creation authority is invalid")
	}
	handle, err := separateOwnerWindowsOpenRelative(
		directory.directory.handle,
		name,
		separateOwnerWindowsDirectoryAccess,
		windows.FILE_CREATE,
		true,
		separateOwnerWindowsShareGuard,
		true,
	)
	if separateOwnerWindowsCollision(err) {
		return nil, os.ErrExist
	}
	if err != nil {
		return nil, err
	}
	state, stateErr := separateOwnerWindowsDirectoryState(handle, true)
	nameErr := separateOwnerWindowsVerifyOpenedComponent(handle, name)
	mappingErr := separateOwnerWindowsVerifyNameMapping(directory.directory.handle, name, state.Identity, true)
	syncErr := errors.Join(
		secureWindowsSyncDirectory(handle),
		secureWindowsSyncDirectory(directory.directory.handle),
	)
	parentErr := directory.Validate()
	contextErr := contextSeparateOwnerError(ctx)
	if stateErr != nil || nameErr != nil || mappingErr != nil || syncErr != nil || parentErr != nil || contextErr != nil {
		cleanupErr := separateOwnerWindowsDiscardCreated(
			directory.directory.handle, handle, name, true,
		)
		return nil, errors.Join(
			errors.New("separate-owner Windows private directory creation failed verification"),
			stateErr, nameErr, mappingErr, syncErr, parentErr, contextErr, cleanupErr,
		)
	}
	return &SeparateOwnerDirectory{
		directory: &startupPrivateDirectory{handle: handle, identity: state.Identity},
		state:     state, requireProtected: true, validateLease: directory.validateLease,
		parent: directory, name: name,
	}, nil
}

func (directory *SeparateOwnerDirectory) CaptureFile(
	ctx context.Context,
	name string,
	maxBytes int64,
	allowEmpty bool,
	requireProtected bool,
) (SeparateOwnerFileState, []byte, bool, error) {
	file, state, body, present, err := directory.separateOwnerWindowsCaptureGuard(
		ctx, name, maxBytes, allowEmpty, requireProtected, false, true,
	)
	if err != nil || !present {
		return SeparateOwnerFileState{}, nil, present, err
	}
	closeErr := file.Close()
	validateErr := directory.Validate()
	contextErr := contextSeparateOwnerError(ctx)
	if closeErr != nil || validateErr != nil || contextErr != nil {
		clear(body)
		return SeparateOwnerFileState{}, nil, false, errors.Join(
			errors.New("separate-owner Windows file changed after capture"),
			closeErr, validateErr, contextErr,
		)
	}
	return state, body, true, nil
}

func (directory *SeparateOwnerDirectory) ProjectProtectedFileStateExact(
	ctx context.Context,
	name string,
	expected SeparateOwnerFileState,
) (result SeparateOwnerFileState, resultErr error) {
	if err := contextSeparateOwnerError(ctx); err != nil {
		return SeparateOwnerFileState{}, err
	}
	if err := directory.Validate(); err != nil || !separateOwnerWindowsComponent(name) || !expected.Valid() {
		return SeparateOwnerFileState{}, errors.New("separate-owner Windows file protection projection authority is invalid")
	}
	maxBytes := expected.Size
	if maxBytes == 0 {
		maxBytes = 1
	}
	file, current, _, present, err := directory.separateOwnerWindowsCaptureGuard(
		ctx, name, maxBytes, expected.Size == 0, false, false, false,
	)
	if err != nil || !present || current != expected {
		if file != nil {
			_ = file.Close()
		}
		return SeparateOwnerFileState{}, errors.Join(errors.New("separate-owner Windows file changed before protection projection"), err)
	}
	defer func() { resultErr = errors.Join(resultErr, file.Close()) }()
	handle := windows.Handle(file.Fd())
	securityDigest, err := separateOwnerWindowsProjectedPrivateSecurityDigest(handle, false)
	if err != nil {
		return SeparateOwnerFileState{}, err
	}
	projected := expected
	projected.SecurityDigest = securityDigest
	after, err := separateOwnerWindowsFileMetadataForHandle(handle, maxBytes, expected.Size == 0, false)
	afterState := after.state(expected.SHA256)
	if err != nil || afterState != expected || directory.Validate() != nil ||
		separateOwnerWindowsVerifyNameMapping(directory.directory.handle, name, expected.Identity, false) != nil {
		return SeparateOwnerFileState{}, errors.New("separate-owner Windows file changed after protection projection")
	}
	return projected, nil
}

// ProtectFileExact narrows one already-verified owner file to a protected DACL
// before it can enter a private migration journal. NTFS rename preserves a
// file DACL, so this step is mandatory rather than an attribute of the move.
func (directory *SeparateOwnerDirectory) ProtectFileExact(
	ctx context.Context,
	name string,
	expected SeparateOwnerFileState,
) (result SeparateOwnerFileState, resultErr error) {
	if err := contextSeparateOwnerError(ctx); err != nil {
		return SeparateOwnerFileState{}, err
	}
	if err := directory.Validate(); err != nil || !separateOwnerWindowsComponent(name) || !expected.Valid() {
		return SeparateOwnerFileState{}, errors.New("separate-owner Windows file protection authority is invalid")
	}
	maxBytes := expected.Size
	if maxBytes == 0 {
		maxBytes = 1
	}
	current, _, present, err := directory.CaptureFile(ctx, name, maxBytes, expected.Size == 0, false)
	if err != nil || !present || current != expected {
		return SeparateOwnerFileState{}, errors.Join(errors.New("separate-owner Windows file changed before protection"), err)
	}
	handle, err := separateOwnerWindowsOpenRelative(
		directory.directory.handle,
		name,
		windows.FILE_GENERIC_READ|windows.WRITE_DAC,
		windows.FILE_OPEN,
		false,
		separateOwnerWindowsShareRead,
		false,
	)
	if err != nil {
		return SeparateOwnerFileState{}, err
	}
	file := os.NewFile(uintptr(handle), name)
	if file == nil {
		_ = windows.CloseHandle(handle)
		return SeparateOwnerFileState{}, errors.New("separate-owner Windows protection handle is invalid")
	}
	defer func() { resultErr = errors.Join(resultErr, file.Close()) }()
	before, err := separateOwnerWindowsFileMetadataForHandle(handle, maxBytes, expected.Size == 0, false)
	beforeState := before.state(expected.SHA256)
	if err != nil || beforeState != expected {
		return SeparateOwnerFileState{}, errors.Join(errors.New("separate-owner Windows file changed while opening for protection"), err)
	}
	if err := errors.Join(
		separateOwnerWindowsVerifyOpenedComponent(handle, name),
		separateOwnerWindowsVerifyNameMapping(directory.directory.handle, name, expected.Identity, false),
		directory.Validate(),
		contextSeparateOwnerError(ctx),
	); err != nil {
		return SeparateOwnerFileState{}, err
	}
	descriptor, err := separateOwnerWindowsPrivateDescriptor()
	if err != nil {
		return SeparateOwnerFileState{}, err
	}
	dacl, _, err := descriptor.DACL()
	if err != nil || dacl == nil {
		return SeparateOwnerFileState{}, errors.Join(errors.New("separate-owner Windows private DACL is unavailable"), err)
	}
	err = windows.SetSecurityInfo(
		handle,
		windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION,
		nil,
		nil,
		dacl,
		nil,
	)
	runtime.KeepAlive(descriptor)
	if err != nil {
		return SeparateOwnerFileState{}, err
	}
	after, stateErr := separateOwnerWindowsFileMetadataForHandle(handle, maxBytes, expected.Size == 0, true)
	afterState := after.state(expected.SHA256)
	if stateErr != nil || afterState.Identity != expected.Identity || afterState.Size != expected.Size ||
		afterState.ModifiedUnixNano != expected.ModifiedUnixNano {
		return SeparateOwnerFileState{}, errors.Join(errors.New("separate-owner Windows protected file state changed"), stateErr)
	}
	if err := errors.Join(
		separateOwnerWindowsVerifyNameMapping(directory.directory.handle, name, expected.Identity, false),
		secureWindowsSyncDirectory(directory.directory.handle),
		directory.Validate(),
		contextSeparateOwnerError(ctx),
	); err != nil {
		return SeparateOwnerFileState{}, err
	}
	protected, _, present, err := directory.CaptureFile(ctx, name, maxBytes, expected.Size == 0, true)
	if err != nil || !present || protected != afterState || protected.SHA256 != expected.SHA256 {
		return SeparateOwnerFileState{}, errors.Join(errors.New("separate-owner Windows protected file readback failed"), err)
	}
	return protected, nil
}

func (directory *SeparateOwnerDirectory) separateOwnerWindowsCaptureGuard(
	ctx context.Context,
	name string,
	maxBytes int64,
	allowEmpty bool,
	requireProtected bool,
	deleteAccess bool,
	captureBody bool,
) (*os.File, SeparateOwnerFileState, []byte, bool, error) {
	if err := contextSeparateOwnerError(ctx); err != nil {
		return nil, SeparateOwnerFileState{}, nil, false, err
	}
	if err := directory.Validate(); err != nil || !separateOwnerWindowsComponent(name) || maxBytes <= 0 {
		return nil, SeparateOwnerFileState{}, nil, false, errors.New("separate-owner Windows file capture authority is invalid")
	}
	access := uint32(windows.FILE_GENERIC_READ)
	if deleteAccess {
		access |= windows.DELETE
	}
	handle, err := separateOwnerWindowsOpenRelative(
		directory.directory.handle,
		name,
		access,
		windows.FILE_OPEN,
		false,
		separateOwnerWindowsShareRead,
		false,
	)
	if separateOwnerWindowsNotFound(err) {
		return nil, SeparateOwnerFileState{}, nil, false, nil
	}
	if err != nil {
		return nil, SeparateOwnerFileState{}, nil, false, err
	}
	file := os.NewFile(uintptr(handle), name)
	if file == nil {
		_ = windows.CloseHandle(handle)
		return nil, SeparateOwnerFileState{}, nil, false, errors.New("separate-owner Windows file handle is invalid")
	}
	fail := func(cause error, body []byte) (*os.File, SeparateOwnerFileState, []byte, bool, error) {
		clear(body)
		return nil, SeparateOwnerFileState{}, nil, false, errors.Join(cause, file.Close())
	}
	before, err := separateOwnerWindowsFileMetadataForHandle(handle, maxBytes, allowEmpty, requireProtected)
	if err != nil {
		return fail(err, nil)
	}
	if err := separateOwnerWindowsVerifyOpenedComponent(handle, name); err != nil {
		return fail(err, nil)
	}
	hasher := sha256.New()
	var body bytes.Buffer
	writer := io.Writer(hasher)
	if captureBody {
		capacity := before.size
		if capacity > 64*1024 {
			capacity = 64 * 1024
		}
		body.Grow(int(capacity))
		writer = io.MultiWriter(hasher, &body)
	}
	limit := maxBytes
	if limit < math.MaxInt64 {
		limit++
	}
	written, readErr := io.Copy(writer, &separateOwnerWindowsContextReader{
		ctx: ctx, reader: io.LimitReader(file, limit),
	})
	if readErr != nil || written != before.size {
		return fail(errors.New("separate-owner Windows file changed while reading"), body.Bytes())
	}
	after, stateErr := separateOwnerWindowsFileMetadataForHandle(handle, maxBytes, allowEmpty, requireProtected)
	digest := hex.EncodeToString(hasher.Sum(nil))
	state := after.state(digest)
	mappingErr := separateOwnerWindowsVerifyNameMapping(
		directory.directory.handle, name, after.identity, false,
	)
	validateErr := directory.Validate()
	contextErr := contextSeparateOwnerError(ctx)
	if stateErr != nil || before != after || !state.Valid() || mappingErr != nil || validateErr != nil || contextErr != nil {
		return fail(errors.Join(
			errors.New("separate-owner Windows file identity changed while reading"),
			stateErr, mappingErr, validateErr, contextErr,
		), body.Bytes())
	}
	return file, state, body.Bytes(), true, nil
}

func separateOwnerWindowsFileMetadataForHandle(
	handle windows.Handle,
	maxBytes int64,
	allowEmpty bool,
	requireProtected bool,
) (separateOwnerWindowsFileMetadata, error) {
	if maxBytes <= 0 {
		return separateOwnerWindowsFileMetadata{}, errors.New("separate-owner Windows file size authority is invalid")
	}
	securityDigest, err := separateOwnerWindowsSecurityDigest(handle, false, true, requireProtected)
	if err != nil {
		return separateOwnerWindowsFileMetadata{}, err
	}
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(handle, &info); err != nil {
		return separateOwnerWindowsFileMetadata{}, err
	}
	size := uint64(info.FileSizeHigh)<<32 | uint64(info.FileSizeLow)
	if size > math.MaxInt64 || int64(size) > maxBytes || !allowEmpty && size == 0 {
		return separateOwnerWindowsFileMetadata{}, errors.New("separate-owner Windows file size is invalid")
	}
	identity, err := windowsOpenedObjectIdentity(handle, false)
	if err != nil {
		return separateOwnerWindowsFileMetadata{}, err
	}
	return separateOwnerWindowsFileMetadata{
		identity: identity, securityDigest: securityDigest, size: int64(size),
		modified: info.LastWriteTime.Nanoseconds(),
	}, nil
}

func (directory *SeparateOwnerDirectory) WriteExclusiveAtomic(
	ctx context.Context,
	name string,
	body []byte,
	maxBytes int64,
	fault SeparateOwnerWriteFault,
) (resultErr error) {
	if err := contextSeparateOwnerError(ctx); err != nil {
		return err
	}
	if err := directory.Validate(); err != nil || !directory.requireProtected ||
		!separateOwnerWindowsComponent(name) || len(body) == 0 || maxBytes <= 0 || int64(len(body)) > maxBytes {
		return errors.New("separate-owner Windows atomic write authority is invalid")
	}
	temporary, err := startupPrivateTempName(name)
	if err != nil || !separateOwnerWindowsComponent(temporary) {
		return errors.Join(errors.New("separate-owner Windows atomic temp name is invalid"), err)
	}
	handle, err := separateOwnerWindowsOpenRelative(
		directory.directory.handle,
		temporary,
		windows.FILE_GENERIC_READ|windows.FILE_GENERIC_WRITE|windows.DELETE,
		windows.FILE_CREATE,
		false,
		separateOwnerWindowsShareRead,
		true,
	)
	if err != nil {
		return err
	}
	file := os.NewFile(uintptr(handle), temporary)
	if file == nil {
		_ = windows.CloseHandle(handle)
		return errors.New("separate-owner Windows atomic temp handle is invalid")
	}
	published := false
	preserve := false
	defer func() {
		if !published && !preserve {
			deleteErr := separateOwnerWindowsDeleteHandle(handle)
			closeErr := file.Close()
			absenceErr := separateOwnerWindowsRequireNameAbsent(directory.directory.handle, temporary, false)
			syncErr := secureWindowsSyncDirectory(directory.directory.handle)
			resultErr = errors.Join(resultErr, deleteErr, closeErr, absenceErr, syncErr)
			return
		}
		resultErr = errors.Join(resultErr, file.Close())
	}()
	if _, err := separateOwnerWindowsFileMetadataForHandle(handle, maxBytes, true, true); err != nil {
		return err
	}
	if err := startupPrivateWindowsWriteAndVerify(file, body, maxBytes); err != nil {
		return err
	}
	expected, err := separateOwnerWindowsFileMetadataForHandle(handle, maxBytes, false, true)
	if err != nil {
		return err
	}
	expectedState := expected.state(separateOwnerSHA256(body))
	if !expectedState.Valid() {
		return errors.New("separate-owner Windows atomic temp state is invalid")
	}
	if err := errors.Join(
		separateOwnerWindowsVerifyOpenedComponent(handle, temporary),
		separateOwnerWindowsVerifyNameMapping(directory.directory.handle, temporary, expected.identity, false),
		secureWindowsSyncDirectory(directory.directory.handle),
	); err != nil {
		return err
	}
	if fault != nil {
		if err := fault("after_temp_sync"); err != nil {
			preserve = true
			return err
		}
	}
	if err := errors.Join(directory.Validate(), contextSeparateOwnerError(ctx)); err != nil {
		return err
	}
	if err := separateOwnerWindowsRenameNoReplace(handle, directory.directory.handle, name); err != nil {
		if separateOwnerWindowsCollision(err) {
			return os.ErrExist
		}
		return err
	}
	published = true
	rollback := func(cause error) error {
		rollbackErr := separateOwnerWindowsRenameNoReplace(handle, directory.directory.handle, temporary)
		if rollbackErr == nil {
			published = false
			rollbackErr = errors.Join(
				separateOwnerWindowsVerifyNameMapping(directory.directory.handle, temporary, expected.identity, false),
				separateOwnerWindowsRequireNameAbsent(directory.directory.handle, name, false),
				secureWindowsSyncDirectory(directory.directory.handle),
			)
		}
		return errors.Join(cause, rollbackErr)
	}
	if err := errors.Join(
		separateOwnerWindowsVerifyNameMapping(directory.directory.handle, name, expected.identity, false),
		separateOwnerWindowsRequireNameAbsent(directory.directory.handle, temporary, false),
	); err != nil {
		return rollback(err)
	}
	if fault != nil {
		if err := fault("after_publish"); err != nil {
			return err
		}
	}
	if err := errors.Join(file.Sync(), secureWindowsSyncDirectory(directory.directory.handle)); err != nil {
		return err
	}
	if fault != nil {
		if err := fault("after_directory_sync"); err != nil {
			return err
		}
	}
	current, stateErr := separateOwnerWindowsFileMetadataForHandle(handle, maxBytes, false, true)
	currentState := current.state(expectedState.SHA256)
	if stateErr != nil || currentState != expectedState {
		return rollback(errors.Join(errors.New("separate-owner Windows atomic write readback changed"), stateErr))
	}
	if err := errors.Join(
		directory.Validate(),
		separateOwnerWindowsVerifyNameMapping(directory.directory.handle, name, expected.identity, false),
		contextSeparateOwnerError(ctx),
	); err != nil {
		return rollback(err)
	}
	return nil
}

func (directory *SeparateOwnerDirectory) MoveFileNoReplace(
	ctx context.Context,
	name string,
	destination *SeparateOwnerDirectory,
	destinationName string,
	expected SeparateOwnerFileState,
	requireProtected bool,
) (resultErr error) {
	if err := contextSeparateOwnerError(ctx); err != nil {
		return err
	}
	if err := directory.Validate(); err != nil || destination == nil || destination.Validate() != nil ||
		!separateOwnerWindowsComponent(name) || !separateOwnerWindowsComponent(destinationName) || !expected.Valid() ||
		destination.requireProtected && !requireProtected {
		return errors.New("separate-owner Windows move authority is invalid")
	}
	maxBytes := expected.Size
	if maxBytes == 0 {
		maxBytes = 1
	}
	file, current, _, present, err := directory.separateOwnerWindowsCaptureGuard(
		ctx, name, maxBytes, expected.Size == 0, requireProtected, true, false,
	)
	if err != nil || !present || current != expected {
		if file != nil {
			_ = file.Close()
		}
		return errors.Join(errors.New("separate-owner Windows move source changed"), err)
	}
	defer func() { resultErr = errors.Join(resultErr, file.Close()) }()
	handle := windows.Handle(file.Fd())
	sourceID, err := windowsOpenedObjectFileID(handle, false)
	if err != nil {
		return err
	}
	destinationID, err := windowsOpenedObjectFileID(destination.directory.handle, true)
	if err != nil || sourceID.VolumeSerialNumber != destinationID.VolumeSerialNumber {
		return errors.Join(errors.New("separate-owner Windows move crosses a volume boundary"), err)
	}
	if err := errors.Join(directory.Validate(), destination.Validate(), contextSeparateOwnerError(ctx)); err != nil {
		return err
	}
	if err := separateOwnerWindowsRenameNoReplace(handle, destination.directory.handle, destinationName); err != nil {
		if separateOwnerWindowsCollision(err) {
			return os.ErrExist
		}
		return err
	}
	moved := true
	rollback := func(cause error) error {
		if !moved {
			return cause
		}
		rollbackErr := separateOwnerWindowsRenameNoReplace(handle, directory.directory.handle, name)
		if rollbackErr == nil {
			moved = false
			rollbackErr = errors.Join(
				separateOwnerWindowsVerifyNameMapping(directory.directory.handle, name, expected.Identity, false),
				separateOwnerWindowsRequireNameAbsent(destination.directory.handle, destinationName, false),
				secureWindowsSyncDirectory(directory.directory.handle),
				secureWindowsSyncDirectory(destination.directory.handle),
			)
		}
		return errors.Join(cause, rollbackErr)
	}
	currentMetadata, stateErr := separateOwnerWindowsFileMetadataForHandle(
		handle, maxBytes, expected.Size == 0, requireProtected,
	)
	currentState := currentMetadata.state(expected.SHA256)
	if stateErr != nil || currentState != expected {
		return rollback(errors.Join(errors.New("separate-owner Windows moved object state changed"), stateErr))
	}
	if err := errors.Join(
		separateOwnerWindowsVerifyNameMapping(destination.directory.handle, destinationName, expected.Identity, false),
		separateOwnerWindowsRequireNameAbsent(directory.directory.handle, name, false),
		directory.Validate(),
		destination.Validate(),
		contextSeparateOwnerError(ctx),
	); err != nil {
		return rollback(err)
	}
	if err := errors.Join(
		secureWindowsSyncDirectory(directory.directory.handle),
		secureWindowsSyncDirectory(destination.directory.handle),
	); err != nil {
		return rollback(err)
	}
	return nil
}

func (directory *SeparateOwnerDirectory) RemoveFileExact(
	ctx context.Context,
	name string,
	expected SeparateOwnerFileState,
	requireProtected bool,
) (resultErr error) {
	return directory.removeFileExact(ctx, name, expected, requireProtected, nil)
}

func (directory *SeparateOwnerDirectory) removeFileExact(
	ctx context.Context,
	name string,
	expected SeparateOwnerFileState,
	requireProtected bool,
	fault SeparateOwnerWriteFault,
) (resultErr error) {
	if err := contextSeparateOwnerError(ctx); err != nil {
		return err
	}
	if err := directory.Validate(); err != nil || !separateOwnerWindowsComponent(name) || !expected.Valid() {
		return errors.New("separate-owner Windows remove authority is invalid")
	}
	maxBytes := expected.Size
	if maxBytes == 0 {
		maxBytes = 1
	}
	file, current, _, present, err := directory.separateOwnerWindowsCaptureGuard(
		ctx, name, maxBytes, expected.Size == 0, requireProtected, true, false,
	)
	if err != nil || !present || current != expected {
		if file != nil {
			_ = file.Close()
		}
		return errors.Join(errors.New("separate-owner Windows remove target changed"), err)
	}
	handle := windows.Handle(file.Fd())
	if err := errors.Join(directory.Validate(), contextSeparateOwnerError(ctx)); err != nil {
		return errors.Join(err, file.Close())
	}
	if err := callSeparateOwnerFault(fault, "before_unlink"); err != nil {
		return errors.Join(err, file.Close())
	}
	deleteErr := separateOwnerWindowsDeleteHandle(handle)
	absenceErr := separateOwnerWindowsRequireNameAbsent(directory.directory.handle, name, false)
	syncErr := secureWindowsSyncDirectory(directory.directory.handle)
	faultErr := callSeparateOwnerFault(fault, "after_unlink")
	var info windows.ByHandleFileInformation
	infoErr := windows.GetFileInformationByHandle(handle, &info)
	identity, identityErr := windowsOpenedObjectIdentity(handle, false)
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		infoErr = errors.Join(infoErr, err)
	}
	hasher := sha256.New()
	limit := maxBytes
	if limit < math.MaxInt64 {
		limit++
	}
	written, hashErr := io.Copy(hasher, &separateOwnerWindowsContextReader{
		ctx: ctx, reader: io.LimitReader(file, limit),
	})
	size := uint64(info.FileSizeHigh)<<32 | uint64(info.FileSizeLow)
	postDeleteErr := error(nil)
	if infoErr != nil || identityErr != nil || info.NumberOfLinks != 0 || identity != expected.Identity ||
		size > math.MaxInt64 || int64(size) != expected.Size || info.LastWriteTime.Nanoseconds() != expected.ModifiedUnixNano ||
		hashErr != nil || written != expected.Size || hex.EncodeToString(hasher.Sum(nil)) != expected.SHA256 {
		postDeleteErr = errors.Join(
			errors.New("separate-owner Windows removed object retained a link or changed"),
			infoErr, identityErr, hashErr,
		)
	}
	closeErr := file.Close()
	validateErr := directory.Validate()
	contextErr := contextSeparateOwnerError(ctx)
	return errors.Join(deleteErr, absenceErr, syncErr, faultErr, postDeleteErr, closeErr, validateErr, contextErr)
}

func (directory *SeparateOwnerDirectory) RemoveEmptyDirectoryExact(
	ctx context.Context,
	name string,
	expected SeparateOwnerDirectoryState,
) error {
	if err := contextSeparateOwnerError(ctx); err != nil {
		return err
	}
	if err := directory.Validate(); err != nil || !separateOwnerWindowsComponent(name) || !expected.Valid() {
		return errors.New("separate-owner Windows directory removal authority is invalid")
	}
	handle, err := separateOwnerWindowsOpenRelative(
		directory.directory.handle,
		name,
		separateOwnerWindowsDirectoryAccess,
		windows.FILE_OPEN,
		true,
		separateOwnerWindowsShareGuard,
		false,
	)
	if err != nil {
		return err
	}
	closed := false
	closeHandle := func() error {
		if closed {
			return nil
		}
		closed = true
		return windows.CloseHandle(handle)
	}
	defer func() { _ = closeHandle() }()
	state, stateErr := separateOwnerWindowsDirectoryState(handle, expected.Protected)
	nameErr := separateOwnerWindowsVerifyOpenedComponent(handle, name)
	mappingErr := separateOwnerWindowsVerifyNameMapping(directory.directory.handle, name, state.Identity, true)
	if stateErr != nil || state != expected || nameErr != nil || mappingErr != nil {
		return errors.Join(errors.New("separate-owner Windows directory removal target changed"), stateErr, nameErr, mappingErr)
	}
	opened := &startupPrivateDirectory{handle: handle, identity: state.Identity}
	for attempt := 0; attempt < 2; attempt++ {
		entries, readErr := opened.ReadEntriesBoundedContext(ctx, 1)
		if readErr != nil || len(entries) != 0 {
			return errors.Join(errors.New("separate-owner Windows directory is not empty"), readErr)
		}
	}
	current, stateErr := separateOwnerWindowsDirectoryState(handle, expected.Protected)
	parentErr := directory.Validate()
	contextErr := contextSeparateOwnerError(ctx)
	if stateErr != nil || current != expected || parentErr != nil || contextErr != nil {
		return errors.Join(
			errors.New("separate-owner Windows directory changed before removal"),
			stateErr, parentErr, contextErr,
		)
	}
	deleteErr := separateOwnerWindowsDeleteHandle(handle)
	closeErr := closeHandle()
	absenceErr := separateOwnerWindowsRequireNameAbsent(directory.directory.handle, name, true)
	syncErr := secureWindowsSyncDirectory(directory.directory.handle)
	validateErr := directory.Validate()
	contextErr = contextSeparateOwnerError(ctx)
	return errors.Join(deleteErr, closeErr, absenceErr, syncErr, validateErr, contextErr)
}

func (directory *SeparateOwnerDirectory) Sync() error {
	if err := directory.Validate(); err != nil {
		return err
	}
	return secureWindowsSyncDirectory(directory.directory.handle)
}

func (directory *SeparateOwnerDirectory) RemoveKnownTempFiles(
	ctx context.Context,
	targets map[string]SeparateOwnerFileState,
) error {
	names := make([]string, 0, len(targets))
	for name := range targets {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if !separateOwnerWindowsComponent(name) {
			return errors.New("separate-owner Windows temp cleanup name is invalid")
		}
		if err := directory.RemoveFileExact(ctx, name, targets[name], true); err != nil {
			return err
		}
	}
	return nil
}

func separateOwnerWindowsOpenRelative(
	parent windows.Handle,
	name string,
	access uint32,
	disposition uint32,
	directory bool,
	share uint32,
	privateCreate bool,
) (windows.Handle, error) {
	if !separateOwnerWindowsComponent(name) {
		return 0, errors.New("separate-owner Windows path component is invalid")
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
	return separateOwnerWindowsNTCreate(attributes, access, disposition, directory, share, privateCreate)
}

func separateOwnerWindowsNTCreate(
	attributes *windows.OBJECT_ATTRIBUTES,
	access uint32,
	disposition uint32,
	directory bool,
	share uint32,
	privateCreate bool,
) (windows.Handle, error) {
	if attributes == nil || privateCreate && disposition != windows.FILE_CREATE {
		return 0, errors.New("separate-owner Windows native open authority is invalid")
	}
	var descriptor *windows.SECURITY_DESCRIPTOR
	var err error
	if privateCreate {
		descriptor, err = separateOwnerWindowsPrivateDescriptor()
		if err != nil {
			return 0, err
		}
		attributes.SecurityDescriptor = descriptor
	}
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
	err = windows.NtCreateFile(
		&handle, access, attributes, &status, &allocation, fileAttributes,
		share, disposition, options, 0, 0,
	)
	runtime.KeepAlive(descriptor)
	if err != nil {
		return 0, err
	}
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(handle, &info); err != nil ||
		info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 ||
		directory && info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY == 0 ||
		!directory && info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY != 0 {
		_ = windows.CloseHandle(handle)
		return 0, errors.New("separate-owner Windows handle crossed a reparse boundary")
	}
	return handle, nil
}

func separateOwnerWindowsRenameNoReplace(
	handle windows.Handle,
	parent windows.Handle,
	target string,
) error {
	if !separateOwnerWindowsComponent(target) {
		return errors.New("separate-owner Windows rename target is invalid")
	}
	targetName, err := windows.UTF16FromString(target)
	if err != nil {
		return err
	}
	nameLength := (len(targetName) - 1) * 2
	dummy := separateOwnerWindowsRenameInformation{}
	buffer := make([]byte, int(unsafe.Offsetof(dummy.FileName))+nameLength)
	info := (*separateOwnerWindowsRenameInformation)(unsafe.Pointer(&buffer[0]))
	info.RootDirectory = parent
	info.FileNameLength = uint32(nameLength)
	destination := unsafe.Slice((*uint16)(unsafe.Pointer(&info.FileName[0])), nameLength/2)
	copy(destination, targetName[:len(targetName)-1])
	var status windows.IO_STATUS_BLOCK
	return windows.NtSetInformationFile(
		handle, &status, &buffer[0], uint32(len(buffer)), windows.FileRenameInformation,
	)
}

func separateOwnerWindowsDeleteHandle(handle windows.Handle) error {
	flags := uint32(
		windows.FILE_DISPOSITION_DELETE |
			windows.FILE_DISPOSITION_POSIX_SEMANTICS |
			windows.FILE_DISPOSITION_IGNORE_READONLY_ATTRIBUTE,
	)
	buffer := (*[4]byte)(unsafe.Pointer(&flags))
	var status windows.IO_STATUS_BLOCK
	return windows.NtSetInformationFile(
		handle, &status, &buffer[0], uint32(len(buffer)), windows.FileDispositionInformationEx,
	)
}

func separateOwnerWindowsVerifyNameMapping(
	parent windows.Handle,
	name string,
	expectedIdentity string,
	directory bool,
) error {
	if expectedIdentity == "" {
		return errors.New("separate-owner Windows expected identity is unavailable")
	}
	handle, err := separateOwnerWindowsOpenRelative(
		parent,
		name,
		windows.FILE_READ_ATTRIBUTES|windows.READ_CONTROL|windows.SYNCHRONIZE,
		windows.FILE_OPEN,
		directory,
		separateOwnerWindowsShareAll,
		false,
	)
	if err != nil {
		return err
	}
	identity, identityErr := windowsOpenedObjectIdentity(handle, directory)
	nameErr := separateOwnerWindowsVerifyOpenedComponent(handle, name)
	closeErr := windows.CloseHandle(handle)
	if identityErr != nil || identity != expectedIdentity || nameErr != nil || closeErr != nil {
		return errors.Join(
			errors.New("separate-owner Windows name no longer maps to the expected object"),
			identityErr, nameErr, closeErr,
		)
	}
	return nil
}

func separateOwnerWindowsVerifyOpenedComponent(handle windows.Handle, expected string) error {
	buffer := make([]byte, 4+2*windows.MAX_LONG_PATH)
	if err := windows.GetFileInformationByHandleEx(
		handle, windows.FileNameInfo, &buffer[0], uint32(len(buffer)),
	); err != nil {
		return err
	}
	nameLength := int(*(*uint32)(unsafe.Pointer(&buffer[0])))
	if nameLength <= 0 || nameLength%2 != 0 || nameLength > len(buffer)-4 {
		return errors.New("separate-owner Windows opened name is invalid")
	}
	units := unsafe.Slice((*uint16)(unsafe.Pointer(&buffer[4])), nameLength/2)
	for _, unit := range units {
		if unit == 0 {
			return errors.New("separate-owner Windows opened name contains NUL")
		}
	}
	name := string(utf16.Decode(units))
	if index := strings.LastIndexAny(name, `\/`); index >= 0 {
		name = name[index+1:]
	}
	if name != expected {
		return errors.New("separate-owner Windows opened name casing or component changed")
	}
	return nil
}

func separateOwnerWindowsRequireNameAbsent(
	parent windows.Handle,
	name string,
	directory bool,
) error {
	handle, err := separateOwnerWindowsOpenRelative(
		parent,
		name,
		windows.FILE_READ_ATTRIBUTES|windows.SYNCHRONIZE,
		windows.FILE_OPEN,
		directory,
		separateOwnerWindowsShareAll,
		false,
	)
	if separateOwnerWindowsNotFound(err) {
		return nil
	}
	if err != nil {
		return errors.Join(errors.New("separate-owner Windows name absence is ambiguous"), err)
	}
	closeErr := windows.CloseHandle(handle)
	return errors.Join(errors.New("separate-owner Windows name is still present"), closeErr)
}

func separateOwnerWindowsDiscardCreated(
	parent windows.Handle,
	handle windows.Handle,
	name string,
	directory bool,
) error {
	deleteErr := separateOwnerWindowsDeleteHandle(handle)
	closeErr := windows.CloseHandle(handle)
	absenceErr := separateOwnerWindowsRequireNameAbsent(parent, name, directory)
	syncErr := secureWindowsSyncDirectory(parent)
	return errors.Join(deleteErr, closeErr, absenceErr, syncErr)
}

func separateOwnerWindowsNotFound(err error) bool {
	return secureWindowsNotFound(err) || err == windows.STATUS_DELETE_PENDING
}

func separateOwnerWindowsCollision(err error) bool {
	return err == windows.STATUS_OBJECT_NAME_COLLISION || err == windows.STATUS_OBJECT_NAME_EXISTS
}

type separateOwnerWindowsContextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (reader *separateOwnerWindowsContextReader) Read(body []byte) (int, error) {
	if err := contextSeparateOwnerError(reader.ctx); err != nil {
		return 0, err
	}
	return reader.reader.Read(body)
}
