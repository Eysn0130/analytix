//go:build darwin || linux

package secretstore

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	portsecretstore "analytix.local/runtime-go/internal/ports/secretstore"

	"golang.org/x/sys/unix"
)

var errFallbackInitializationInProgress = errors.New("secret store: master key initialization in progress")

type fallbackMasterKeyProvider struct {
	path   string
	random io.Reader
	mu     sync.Mutex
}

func newFallbackMasterKeyProvider(path string, random io.Reader) *fallbackMasterKeyProvider {
	return &fallbackMasterKeyProvider{path: path, random: random}
}

func (provider *fallbackMasterKeyProvider) LoadOrCreate(context.Context) ([]byte, error) {
	provider.mu.Lock()
	defer provider.mu.Unlock()

	key, err := provider.readWithBoundedInitializationWait()
	if err == nil {
		return key, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, portsecretstore.ErrMasterKeyUnavailable
	}
	if err := ensurePrivateStoreDirectory(filepath.Dir(provider.path)); err != nil {
		return nil, portsecretstore.ErrMasterKeyUnavailable
	}
	candidate, err := (randomMasterKeySource{random: provider.random}).Generate()
	if err != nil {
		return nil, portsecretstore.ErrMasterKeyUnavailable
	}
	created, err := provider.createExclusive(candidate)
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

func (provider *fallbackMasterKeyProvider) createExclusive(candidate []byte) (bool, error) {
	fd, err := unix.Open(provider.path, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0o600)
	if errors.Is(err, unix.EEXIST) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	file := os.NewFile(uintptr(fd), filepath.Base(provider.path))
	if file == nil {
		_ = unix.Close(fd)
		return false, errors.New("secret store: master key file open failed")
	}
	if err := writeAll(file, candidate); err != nil {
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
	if err := syncPrivateStoreDirectory(filepath.Dir(provider.path)); err != nil {
		return false, err
	}
	return true, nil
}

func (provider *fallbackMasterKeyProvider) readWithBoundedInitializationWait() ([]byte, error) {
	for attempt := 0; attempt < 12; attempt++ {
		key, err := provider.readExisting()
		if !errors.Is(err, errFallbackInitializationInProgress) {
			return key, err
		}
		time.Sleep(time.Duration(1<<attempt) * time.Millisecond)
	}
	return nil, portsecretstore.ErrMasterKeyUnavailable
}

func (provider *fallbackMasterKeyProvider) readExisting() ([]byte, error) {
	content, err := readPrivateCommittedFile(provider.path, masterKeySize+1)
	if err != nil {
		return nil, err
	}
	if len(content) < masterKeySize {
		clearBytes(content)
		return nil, errFallbackInitializationInProgress
	}
	if len(content) != masterKeySize {
		clearBytes(content)
		return nil, portsecretstore.ErrMasterKeyUnavailable
	}
	return content, nil
}
