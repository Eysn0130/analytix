package providerregistryfs

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	registryport "analytix.local/runtime-go/internal/ports/providerregistry"
)

const legacyMigrationSourceLockSuffix = ".analytix-provider-credential-migration.lock"

// LegacySourceReader performs bounded physical observations only. Migration
// challenge and expected owner/source authorization remain application-owned.
type LegacySourceReader struct{}

var _ registryport.LegacySourceReader = LegacySourceReader{}

func (LegacySourceReader) ReadLegacySource(request registryport.LegacySourceRequest) (snapshot registryport.LegacySourceSnapshot, err error) {
	if len(request.SourcePath) == 0 || len(request.SourcePath) > 4096 ||
		strings.IndexByte(request.SourcePath, 0) >= 0 || !filepath.IsAbs(request.SourcePath) ||
		request.SourceInode == 0 || len(request.LockOwnerToken) != 36 ||
		strings.ContainsAny(request.LockOwnerToken, "/\\\x00") {
		return snapshot, registryport.ErrInvalidRequest
	}
	defer func() {
		if err != nil {
			snapshot.Clear()
		}
	}()
	before, err := os.Lstat(request.SourcePath)
	if err != nil || !legacyMigrationRegularSingleLinkFileInfo(before) {
		return snapshot, registryport.ErrVerification
	}
	device, deviceOK := legacyMigrationFileInfoUintField(before, "Dev")
	inode, inodeOK := legacyMigrationFileInfoUintField(before, "Ino")
	if !deviceOK || !inodeOK || device != request.SourceDevice || inode != request.SourceInode {
		return snapshot, registryport.ErrVerification
	}
	physicalPath, err := filepath.EvalSymlinks(request.SourcePath)
	if err != nil || !filepath.IsAbs(physicalPath) ||
		legacySourcePhysicalIdentity(request.SourceLocator, physicalPath) != request.SourcePhysicalIdentitySHA256 {
		return snapshot, registryport.ErrVerification
	}
	snapshot.Source, err = readLegacySourceFile(request.SourcePath, before, registryport.MaxLegacySourceSnapshotBytes)
	if err != nil {
		return snapshot, registryport.ErrVerification
	}
	after, err := os.Lstat(request.SourcePath)
	physicalAfter, physicalErr := filepath.EvalSymlinks(request.SourcePath)
	if err != nil || physicalErr != nil || !legacyMigrationRegularSingleLinkFileInfo(after) ||
		!os.SameFile(before, after) || physicalAfter != physicalPath {
		return snapshot, registryport.ErrVerification
	}
	lockPath := physicalPath + legacyMigrationSourceLockSuffix
	lockInfo, err := os.Lstat(lockPath)
	if err != nil || !lockInfo.IsDir() || lockInfo.Mode()&os.ModeSymlink != 0 {
		return snapshot, registryport.ErrVerification
	}
	entries, err := os.ReadDir(lockPath)
	ownerName := "owner-" + request.LockOwnerToken + ".json"
	if err != nil || len(entries) != 1 || entries[0].Name() != ownerName ||
		entries[0].Type()&os.ModeSymlink != 0 {
		return snapshot, registryport.ErrVerification
	}
	ownerPath := filepath.Join(lockPath, ownerName)
	ownerInfo, err := os.Lstat(ownerPath)
	if err != nil || !legacyMigrationRegularSingleLinkFileInfo(ownerInfo) {
		return snapshot, registryport.ErrVerification
	}
	snapshot.LockOwner, err = readLegacySourceFile(ownerPath, ownerInfo, registryport.MaxLegacySourceLockOwnerBytes)
	if err != nil {
		return snapshot, registryport.ErrVerification
	}
	lockAfter, err := os.Lstat(lockPath)
	if err != nil || !lockAfter.IsDir() || lockAfter.Mode()&os.ModeSymlink != 0 ||
		!os.SameFile(lockInfo, lockAfter) {
		return snapshot, registryport.ErrVerification
	}
	snapshot.PhysicalPath = physicalPath
	return snapshot, nil
}

func readLegacySourceFile(path string, before os.FileInfo, limit int64) ([]byte, error) {
	file, err := openLegacySourceFile(path)
	if err != nil {
		return nil, registryport.ErrVerification
	}
	opened, statErr := file.Stat()
	// Validate the opened object before reading: a path may have changed
	// since observation, and a byte limit does not bound a special-file read.
	if statErr != nil || !legacyMigrationRegularSingleLinkFileInfo(opened) ||
		!os.SameFile(before, opened) || limit <= 0 || opened.Size() <= 0 || opened.Size() > limit {
		_ = file.Close()
		return nil, registryport.ErrVerification
	}
	loaded, readErr := io.ReadAll(io.LimitReader(file, limit+1))
	closeErr := file.Close()
	after, afterErr := os.Lstat(path)
	if readErr != nil || closeErr != nil || afterErr != nil ||
		!legacyMigrationRegularSingleLinkFileInfo(opened) || !legacyMigrationRegularSingleLinkFileInfo(after) ||
		!os.SameFile(before, opened) || !os.SameFile(opened, after) || len(loaded) == 0 || int64(len(loaded)) > limit {
		for index := range loaded {
			loaded[index] = 0
		}
		return nil, registryport.ErrVerification
	}
	return loaded, nil
}

func legacyMigrationFileInfoUintField(info os.FileInfo, fieldName string) (uint64, bool) {
	if info == nil || info.Sys() == nil {
		return 0, false
	}
	value := reflect.ValueOf(info.Sys())
	for value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return 0, false
		}
		value = value.Elem()
	}
	if value.Kind() != reflect.Struct {
		return 0, false
	}
	field := value.FieldByName(fieldName)
	if !field.IsValid() {
		return 0, false
	}
	switch field.Kind() {
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return field.Uint(), true
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if field.Int() < 0 {
			return 0, false
		}
		return uint64(field.Int()), true
	default:
		return 0, false
	}
}

func legacyMigrationRegularSingleLinkFileInfo(info os.FileInfo) bool {
	if info == nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return false
	}
	links, ok := legacyMigrationFileInfoUintField(info, "Nlink")
	if !ok {
		links, ok = legacyMigrationFileInfoUintField(info, "NumberOfLinks")
	}
	return ok && links == 1
}

func legacySourcePhysicalIdentity(sourceLocator, physicalPath string) string {
	hash := sha256.New()
	_, _ = hash.Write([]byte("analytix-provider-settings-physical-source-v1"))
	_, _ = hash.Write([]byte{0})
	_, _ = hash.Write([]byte(sourceLocator))
	_, _ = hash.Write([]byte{0})
	_, _ = hash.Write([]byte(physicalPath))
	return hex.EncodeToString(hash.Sum(nil))
}
