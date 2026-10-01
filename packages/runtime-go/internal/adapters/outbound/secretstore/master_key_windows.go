//go:build windows

package secretstore

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"
	"unsafe"

	portsecretstore "analytix.local/runtime-go/internal/ports/secretstore"

	"golang.org/x/sys/windows"
)

const (
	dpapiBlobVersion = 1
	dpapiHeaderSize  = 9
	maxDPAPIBlobSize = 1 << 20
)

var (
	dpapiBlobMagic                 = [4]byte{'A', 'X', 'M', 'K'}
	errDPAPIInitializationProgress = errors.New("secret store: master key initialization in progress")
)

type dpapiMasterKeyProvider struct {
	path      string
	storePath string
	mu        sync.Mutex
}

func (provider *dpapiMasterKeyProvider) LoadOrCreate(context.Context) ([]byte, error) {
	provider.mu.Lock()
	defer provider.mu.Unlock()

	key, err := provider.readWithBoundedInitializationWait()
	if err == nil {
		return key, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, portsecretstore.ErrMasterKeyUnavailable
	}
	if _, err := os.Lstat(provider.storePath); err == nil || !errors.Is(err, os.ErrNotExist) {
		return nil, portsecretstore.ErrMasterKeyUnavailable
	}
	if err := ensurePrivateStoreDirectory(filepath.Dir(provider.path)); err != nil {
		return nil, portsecretstore.ErrMasterKeyUnavailable
	}
	candidate, err := (randomMasterKeySource{random: rand.Reader}).Generate()
	if err != nil {
		return nil, portsecretstore.ErrMasterKeyUnavailable
	}
	protected, err := protectWithDPAPI(candidate)
	if err != nil {
		clearBytes(candidate)
		return nil, portsecretstore.ErrMasterKeyUnavailable
	}
	blob := encodeDPAPIBlob(protected)
	clearBytes(protected)
	created, err := provider.createExclusive(blob)
	clearBytes(blob)
	if err != nil {
		clearBytes(candidate)
		return nil, portsecretstore.ErrMasterKeyUnavailable
	}
	if !created {
		clearBytes(candidate)
		winner, err := provider.readWithBoundedInitializationWait()
		if err != nil {
			return nil, portsecretstore.ErrMasterKeyUnavailable
		}
		return winner, nil
	}
	readback, err := provider.readWithBoundedInitializationWait()
	if err != nil || !bytes.Equal(candidate, readback) {
		clearBytes(candidate)
		clearBytes(readback)
		return nil, portsecretstore.ErrMasterKeyUnavailable
	}
	clearBytes(readback)
	return candidate, nil
}

func (provider *dpapiMasterKeyProvider) createExclusive(blob []byte) (bool, error) {
	file, err := createPrivateWindowsFile(provider.path)
	if errors.Is(err, os.ErrExist) || errors.Is(err, windows.ERROR_FILE_EXISTS) || errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if err := writeAll(file, blob); err != nil {
		_ = file.Close()
		return false, err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return false, err
	}
	if err := file.Close(); err != nil {
		return false, err
	}
	return true, nil
}

func (provider *dpapiMasterKeyProvider) readWithBoundedInitializationWait() ([]byte, error) {
	for attempt := 0; attempt < 6; attempt++ {
		key, err := provider.readExisting()
		if !errors.Is(err, errDPAPIInitializationProgress) {
			return key, err
		}
		time.Sleep(time.Duration(1<<attempt) * time.Millisecond)
	}
	return nil, portsecretstore.ErrMasterKeyUnavailable
}

func (provider *dpapiMasterKeyProvider) readExisting() ([]byte, error) {
	blob, err := readPrivateCommittedFile(provider.path, maxDPAPIBlobSize)
	if err != nil {
		return nil, err
	}
	defer clearBytes(blob)
	if len(blob) < dpapiHeaderSize {
		return nil, errDPAPIInitializationProgress
	}
	protected, err := decodeDPAPIBlob(blob)
	if err != nil {
		return nil, portsecretstore.ErrMasterKeyUnavailable
	}
	defer clearBytes(protected)
	key, err := unprotectWithDPAPI(protected)
	if err != nil || len(key) != masterKeySize {
		clearBytes(key)
		return nil, portsecretstore.ErrMasterKeyUnavailable
	}
	return key, nil
}

func encodeDPAPIBlob(protected []byte) []byte {
	blob := make([]byte, dpapiHeaderSize+len(protected))
	copy(blob[:4], dpapiBlobMagic[:])
	blob[4] = dpapiBlobVersion
	binary.BigEndian.PutUint32(blob[5:9], uint32(len(protected)))
	copy(blob[dpapiHeaderSize:], protected)
	return blob
}

func decodeDPAPIBlob(blob []byte) ([]byte, error) {
	if len(blob) < dpapiHeaderSize || !bytes.Equal(blob[:4], dpapiBlobMagic[:]) || blob[4] != dpapiBlobVersion {
		return nil, portsecretstore.ErrMasterKeyUnavailable
	}
	protectedLength := int(binary.BigEndian.Uint32(blob[5:9]))
	if protectedLength == 0 || protectedLength != len(blob)-dpapiHeaderSize {
		return nil, portsecretstore.ErrMasterKeyUnavailable
	}
	return bytes.Clone(blob[dpapiHeaderSize:]), nil
}

func protectWithDPAPI(plaintext []byte) ([]byte, error) {
	input := windows.DataBlob{Size: uint32(len(plaintext)), Data: &plaintext[0]}
	var output windows.DataBlob
	err := windows.CryptProtectData(
		&input,
		nil,
		nil,
		0,
		nil,
		windows.CRYPTPROTECT_UI_FORBIDDEN,
		&output,
	)
	runtime.KeepAlive(plaintext)
	if err != nil {
		return nil, portsecretstore.ErrMasterKeyUnavailable
	}
	return copyClearAndLocalFree(output)
}

func unprotectWithDPAPI(protected []byte) ([]byte, error) {
	input := windows.DataBlob{Size: uint32(len(protected)), Data: &protected[0]}
	var output windows.DataBlob
	err := windows.CryptUnprotectData(
		&input,
		nil,
		nil,
		0,
		nil,
		windows.CRYPTPROTECT_UI_FORBIDDEN,
		&output,
	)
	runtime.KeepAlive(protected)
	if err != nil {
		return nil, portsecretstore.ErrMasterKeyUnavailable
	}
	return copyClearAndLocalFree(output)
}

func copyClearAndLocalFree(blob windows.DataBlob) ([]byte, error) {
	if blob.Data == nil || blob.Size == 0 {
		return nil, portsecretstore.ErrMasterKeyUnavailable
	}
	native := unsafe.Slice(blob.Data, int(blob.Size))
	copyOfBlob := bytes.Clone(native)
	clearBytes(native)
	_, err := windows.LocalFree(windows.Handle(uintptr(unsafe.Pointer(blob.Data))))
	if err != nil {
		clearBytes(copyOfBlob)
		return nil, portsecretstore.ErrMasterKeyUnavailable
	}
	return copyOfBlob, nil
}

func defaultMasterKeyProvider(storePath string, options Options) (masterKeyProvider, error) {
	if options.DevelopmentFileAuthority || !options.empty() || storePath == "" || !filepath.IsAbs(storePath) || filepath.Clean(storePath) != storePath {
		return nil, portsecretstore.ErrInvalidRequest
	}
	// Backend selection is also used during semantic planning. Directory
	// creation belongs to the first committed read/write, not construction.
	if err := validatePrivateWindowsPath(storePath); err != nil {
		return nil, portsecretstore.ErrMasterKeyUnavailable
	}
	return &dpapiMasterKeyProvider{
		path:      filepath.Join(filepath.Dir(storePath), "master-key", "master-key.dpapi"),
		storePath: storePath,
	}, nil
}
