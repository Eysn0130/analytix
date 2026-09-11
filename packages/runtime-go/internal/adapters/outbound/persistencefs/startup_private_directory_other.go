//go:build !darwin && !linux && !windows

package persistencefs

import (
	"context"
	"errors"
	"os"
)

type startupPrivateDirectory struct{}

func secureStartupOpenRootDirectory(startupAuthorityRoot) (*startupPrivateDirectory, error) {
	return nil, errors.New("startup private directory is unsupported")
}

func secureStartupCreateDirectory(startupAuthorityRoot, string) (*startupPrivateDirectory, error) {
	return nil, errors.New("startup private directory is unsupported")
}
func secureStartupOpenDirectory(startupAuthorityRoot, string, string) (*startupPrivateDirectory, error) {
	return nil, errors.New("startup private directory is unsupported")
}
func (*startupPrivateDirectory) OpenDirectory(string, string) (*startupPrivateDirectory, error) {
	return nil, errors.New("startup private directory is unsupported")
}
func (*startupPrivateDirectory) CreateDirectory(string) (*startupPrivateDirectory, error) {
	return nil, errors.New("startup private directory is unsupported")
}
func (*startupPrivateDirectory) Close() error     { return nil }
func (*startupPrivateDirectory) Identity() string { return "" }
func (*startupPrivateDirectory) ReadFile(string, int64, bool) ([]byte, string, error) {
	return nil, "", errors.New("startup private directory is unsupported")
}
func (*startupPrivateDirectory) WriteExclusive(string, []byte, int64) error {
	return errors.New("startup private directory is unsupported")
}
func (*startupPrivateDirectory) writeExclusiveWithTemporaryName(string, string, []byte, int64) error {
	return errors.New("startup private directory is unsupported")
}
func (*startupPrivateDirectory) readStartupExclusiveWriteResidue(string, int64) ([]byte, string, error) {
	return nil, "", errors.New("startup private directory is unsupported")
}
func (*startupPrivateDirectory) removeStartupExclusiveWriteResidue(string, string) error {
	return errors.New("startup private directory is unsupported")
}
func (*startupPrivateDirectory) syncStartupExclusiveWriteResidues() error {
	return errors.New("startup private directory is unsupported")
}
func (*startupPrivateDirectory) WriteReplace(string, []byte, int64, func(string) error) error {
	return errors.New("startup private directory is unsupported")
}
func (*startupPrivateDirectory) Entries() ([]os.DirEntry, error) {
	return nil, errors.New("startup private directory is unsupported")
}
func (*startupPrivateDirectory) ReadEntriesBounded(int) ([]os.DirEntry, error) {
	return nil, errors.New("startup private directory is unsupported")
}
func (*startupPrivateDirectory) ReadEntriesBoundedContext(context.Context, int) ([]os.DirEntry, error) {
	return nil, errors.New("startup private directory is unsupported")
}
func (*startupPrivateDirectory) PreflightTree() error {
	return errors.New("startup private directory is unsupported")
}
func (*startupPrivateDirectory) PreflightTreeContext(context.Context) error {
	return errors.New("startup private directory is unsupported")
}
func (*startupPrivateDirectory) RetireDirectory(string, string, string) (string, *startupPrivateDirectory, error) {
	return "", nil, errors.New("startup private directory is unsupported")
}
func (*startupPrivateDirectory) RemoveRetiredDirectory(string, *startupPrivateDirectory) error {
	return errors.New("startup private directory is unsupported")
}
func (*startupPrivateDirectory) RemoveRetiredDirectoryContext(context.Context, string, *startupPrivateDirectory) error {
	return errors.New("startup private directory is unsupported")
}
func secureStartupRetireDirectory(startupAuthorityRoot, string, string, string) (string, *startupPrivateDirectory, error) {
	return "", nil, errors.New("startup private directory is unsupported")
}
func secureStartupRemoveRetiredDirectory(startupAuthorityRoot, string, *startupPrivateDirectory) error {
	return errors.New("startup private directory is unsupported")
}
func secureStartupRemoveRetiredDirectoryContext(context.Context, startupAuthorityRoot, string, *startupPrivateDirectory) error {
	return errors.New("startup private directory is unsupported")
}
