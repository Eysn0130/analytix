//go:build darwin

package nativecomponentregistry

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

type pinnedObject struct {
	file *os.File
	stat unix.Stat_t
}

func load(config Config) (*Registry, error) {
	var loadedComponents map[string]Component
	loaded := false
	defer func() {
		if loaded {
			return
		}
		for _, component := range loadedComponents {
			if component.opened != nil {
				_ = component.opened.Close()
			}
		}
	}()
	root, err := openPinnedObject(config.RuntimeRoot, true)
	if err != nil {
		return nil, ErrUnavailable
	}
	defer root.file.Close()
	if root.stat.Mode&unix.S_IFMT != unix.S_IFDIR || root.stat.Mode&0o022 != 0 {
		return nil, ErrTrustInvalid
	}
	receiptFileName := ReceiptFileName
	if config.AuthorityUse == AuthorityUseLocalBuild {
		receiptFileName = LocalBuildMarkerFileName
	}
	receiptObject, err := openPinnedChild(root, receiptFileName, false)
	if err != nil {
		return nil, ErrReceiptInvalid
	}
	receiptRaw, err := readBoundedFile(receiptObject.file, MaxReceiptBytes)
	if err != nil || !pinnedObjectUnchanged(receiptObject) {
		_ = receiptObject.file.Close()
		return nil, ErrReceiptInvalid
	}
	_ = receiptObject.file.Close()
	receipt, err := parseReceipt(receiptRaw, config.Trust, config.AuthorityUse)
	if err != nil {
		return nil, err
	}
	components := make(map[string]Component, len(receipt.Components))
	loadedComponents = components
	for index, recorded := range receipt.Components {
		expected := expectedComponents[index]
		opened, openErr := openPinnedChild(root, expected.BinaryName, false)
		if openErr != nil {
			return nil, wrapComponentError(expected.ID, openErr)
		}
		component, inspectErr := inspectDarwinComponent(
			filepath.Join(filepath.Clean(config.RuntimeRoot), expected.BinaryName),
			opened,
			recorded,
			config.Verifier,
		)
		if inspectErr != nil {
			_ = opened.file.Close()
			return nil, wrapComponentError(expected.ID, inspectErr)
		}
		component.opened = opened.file
		component.fileIdentity = componentIdentity(opened.stat)
		components[component.ID] = component
	}
	if !pinnedObjectUnchanged(root) {
		return nil, ErrTrustInvalid
	}
	registry, err := newRegistry(receipt, components, config.Trust)
	if err != nil {
		return nil, err
	}
	loaded = true
	return registry, nil
}

func componentIdentity(stat unix.Stat_t) componentFileIdentity {
	return componentFileIdentity{
		device: uint64(stat.Dev), inode: stat.Ino, size: stat.Size, mode: uint32(stat.Mode),
		modifiedSec: stat.Mtim.Sec, modifiedNS: stat.Mtim.Nsec,
		changedSec: stat.Ctim.Sec, changedNS: stat.Ctim.Nsec,
	}
}

func duplicateExecutionFile(ctx context.Context, component Component) (*os.File, error) {
	if component.opened == nil || component.fileIdentity == (componentFileIdentity{}) {
		return nil, ErrComponentInvalid
	}
	fd, err := unix.FcntlInt(component.opened.Fd(), unix.F_DUPFD_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	duplicated := os.NewFile(uintptr(fd), "analytix-native-component:"+component.ID)
	if duplicated == nil {
		_ = unix.Close(fd)
		return nil, ErrComponentInvalid
	}
	var stat unix.Stat_t
	if unix.Fstat(fd, &stat) != nil || componentIdentity(stat) != component.fileIdentity {
		_ = duplicated.Close()
		return nil, ErrComponentInvalid
	}
	hash, err := fullSHA256At(ctx, duplicated, stat.Size)
	if err != nil || hash != component.CurrentSHA256 || componentIdentity(stat) != component.fileIdentity {
		_ = duplicated.Close()
		return nil, ErrComponentInvalid
	}
	var after unix.Stat_t
	if unix.Fstat(fd, &after) != nil || componentIdentity(after) != component.fileIdentity {
		_ = duplicated.Close()
		return nil, ErrComponentInvalid
	}
	return duplicated, nil
}

func fullSHA256At(ctx context.Context, file *os.File, expectedSize int64) (string, error) {
	if ctx == nil || file == nil || expectedSize <= 0 || expectedSize > MaxNativeBinaryBytes || ctx.Err() != nil {
		return "", ErrComponentInvalid
	}
	hash := sha256.New()
	reader := io.NewSectionReader(file, 0, expectedSize)
	buffer := make([]byte, 1024*1024)
	var written int64
	for written < expectedSize {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		limit := int64(len(buffer))
		if remaining := expectedSize - written; remaining < limit {
			limit = remaining
		}
		read, err := reader.Read(buffer[:limit])
		if read > 0 {
			if _, writeErr := hash.Write(buffer[:read]); writeErr != nil {
				return "", writeErr
			}
			written += int64(read)
		}
		if err != nil {
			if errors.Is(err, io.EOF) && written == expectedSize {
				break
			}
			return "", ErrComponentInvalid
		}
		if read == 0 {
			return "", ErrComponentInvalid
		}
	}
	if ctx.Err() != nil || written != expectedSize {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "", ErrComponentInvalid
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func inspectDarwinComponent(path string, opened *pinnedObject, recorded ComponentReceipt, verifier PlatformVerifier) (Component, error) {
	if opened == nil || opened.file == nil || verifier == nil || opened.stat.Mode&unix.S_IFMT != unix.S_IFREG ||
		opened.stat.Nlink != 1 || opened.stat.Size <= 0 || opened.stat.Size > MaxNativeBinaryBytes || opened.stat.Mode&0o022 != 0 {
		return Component{}, ErrComponentInvalid
	}
	if err := verifier.Verify(path, opened.file, recorded); err != nil || !pinnedObjectUnchanged(opened) {
		return Component{}, ErrComponentInvalid
	}
	currentHash, err := fullSHA256(opened.file, opened.stat.Size)
	if err != nil || !pinnedObjectUnchanged(opened) {
		return Component{}, ErrComponentInvalid
	}
	payloadHash, payloadSize, arch, err := machOPayloadIdentity(opened.file, opened.stat.Size)
	if err != nil || payloadHash != recorded.PayloadSHA256 || payloadSize != recorded.PayloadSize || arch != recorded.Arch ||
		!pinnedObjectUnchanged(opened) {
		return Component{}, ErrComponentInvalid
	}
	return Component{
		ID: recorded.ID, BinaryName: recorded.BinaryName, PackagePath: recorded.PackagePath,
		PayloadSHA256: payloadHash, PayloadSize: payloadSize,
		CurrentSHA256: currentHash, CurrentSize: opened.stat.Size,
		Format: recorded.Format, Arch: recorded.Arch, path: path,
	}, nil
}

func openPinnedObject(path string, directory bool) (*pinnedObject, error) {
	clean := filepath.Clean(path)
	if !filepath.IsAbs(clean) || clean == string(filepath.Separator) || strings.ContainsRune(clean, 0) {
		return nil, ErrTrustInvalid
	}
	components := strings.Split(strings.TrimPrefix(clean, string(filepath.Separator)), string(filepath.Separator))
	current, err := unix.Open(string(filepath.Separator), unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}
	for index, component := range components {
		if component == "" || component == "." || component == ".." {
			_ = unix.Close(current)
			return nil, ErrTrustInvalid
		}
		flags := unix.O_RDONLY | unix.O_CLOEXEC | unix.O_NOFOLLOW
		if index < len(components)-1 || directory {
			flags |= unix.O_DIRECTORY
		}
		next, openErr := unix.Openat(current, component, flags, 0)
		_ = unix.Close(current)
		if openErr != nil {
			return nil, openErr
		}
		current = next
	}
	return pinnedObjectFromFD(current, clean)
}

func openPinnedChild(parent *pinnedObject, name string, directory bool) (*pinnedObject, error) {
	if parent == nil || parent.file == nil || name == "" || name != filepath.Base(name) || strings.ContainsRune(name, 0) {
		return nil, ErrTrustInvalid
	}
	flags := unix.O_RDONLY | unix.O_CLOEXEC | unix.O_NOFOLLOW
	if directory {
		flags |= unix.O_DIRECTORY
	}
	fd, err := unix.Openat(int(parent.file.Fd()), name, flags, 0)
	if err != nil {
		return nil, err
	}
	return pinnedObjectFromFD(fd, filepath.Join(parent.file.Name(), name))
}

func pinnedObjectFromFD(fd int, name string) (*pinnedObject, error) {
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil {
		_ = unix.Close(fd)
		return nil, err
	}
	return &pinnedObject{file: os.NewFile(uintptr(fd), name), stat: stat}, nil
}

func pinnedObjectUnchanged(object *pinnedObject) bool {
	if object == nil || object.file == nil {
		return false
	}
	var current unix.Stat_t
	return unix.Fstat(int(object.file.Fd()), &current) == nil && current.Dev == object.stat.Dev &&
		current.Ino == object.stat.Ino && current.Size == object.stat.Size && current.Mode == object.stat.Mode &&
		current.Mtim == object.stat.Mtim && current.Ctim == object.stat.Ctim
}

func readBoundedFile(file *os.File, limit int) ([]byte, error) {
	if file == nil || limit <= 0 {
		return nil, ErrReceiptInvalid
	}
	payload, err := io.ReadAll(io.LimitReader(file, int64(limit)+1))
	if err != nil || len(payload) == 0 || len(payload) > limit {
		return nil, ErrReceiptInvalid
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	return payload, nil
}

func ignoreNotExist(err error) error {
	if errors.Is(err, unix.ENOENT) {
		return nil
	}
	return err
}
