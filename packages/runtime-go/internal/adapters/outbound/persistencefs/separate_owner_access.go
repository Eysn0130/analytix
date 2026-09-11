package persistencefs

import (
	"context"
	"errors"
	"os"
	"strings"
)

// SeparateOwnerFileState is an opaque, content-complete identity for one
// regular file beneath a separate-owner directory. SecurityDigest contains
// platform owner/permission state; it is not a Unix-mode compatibility field.
type SeparateOwnerFileState struct {
	Identity         string
	SecurityDigest   string
	Size             int64
	ModifiedUnixNano int64
	SHA256           string
}

func (state SeparateOwnerFileState) Valid() bool {
	return state.Identity != "" && separateOwnerIsSHA256(state.SecurityDigest) && state.Size >= 0 &&
		separateOwnerIsSHA256(state.SHA256)
}

type SeparateOwnerDirectoryState struct {
	Identity       string
	SecurityDigest string
	Protected      bool
}

func (state SeparateOwnerDirectoryState) Valid() bool {
	return state.Identity != "" && separateOwnerIsSHA256(state.SecurityDigest)
}

// SeparateOwnerRootAccess holds a root handle for the duration of one
// CompositeLease access. A missing root is observable but cannot be created by
// this API.
type SeparateOwnerRootAccess struct {
	authority *SeparateOwnerRootAuthority
	root      *SeparateOwnerDirectory
	present   bool
}

func (access *SeparateOwnerRootAccess) Present() bool {
	return access != nil && access.present
}

func (access *SeparateOwnerRootAccess) Directory() (*SeparateOwnerDirectory, error) {
	if access == nil || !access.present || access.root == nil {
		return nil, errors.New("separate-owner root directory is unavailable")
	}
	if err := access.root.Validate(); err != nil {
		return nil, err
	}
	return access.root, nil
}

func (access *SeparateOwnerRootAccess) Close() error {
	if access == nil || access.root == nil {
		return nil
	}
	err := access.root.close()
	access.root = nil
	return err
}

// SeparateOwnerDirectory wraps the existing startupPrivateDirectory
// handle-relative/no-follow primitive while retaining separate-owner security
// metadata and a live-lease validator.
type SeparateOwnerDirectory struct {
	directory        *startupPrivateDirectory
	state            SeparateOwnerDirectoryState
	requireProtected bool
	validateLease    func() error
	parent           *SeparateOwnerDirectory
	name             string
}

func (directory *SeparateOwnerDirectory) State() SeparateOwnerDirectoryState {
	if directory == nil || directory.Validate() != nil {
		return SeparateOwnerDirectoryState{}
	}
	return directory.state
}

func (directory *SeparateOwnerDirectory) Validate() error {
	if directory == nil || directory.directory == nil || !directory.state.Valid() || directory.validateLease == nil {
		return errors.New("separate-owner directory authority is unavailable")
	}
	if err := directory.validateLease(); err != nil {
		return err
	}
	if directory.parent != nil {
		if !startupAuthorityNamedComponent(directory.name) || directory.parent == directory {
			return errors.New("separate-owner child directory binding is invalid")
		}
		if err := directory.parent.Validate(); err != nil {
			return err
		}
		if err := platformValidateSeparateOwnerChildDirectory(
			directory.parent.directory, directory.name, directory.state.Identity,
		); err != nil {
			return err
		}
	}
	return platformValidateSeparateOwnerOpenedDirectory(directory.directory, directory.state, directory.requireProtected)
}

func (directory *SeparateOwnerDirectory) close() error {
	if directory == nil || directory.directory == nil {
		return nil
	}
	err := directory.directory.Close()
	directory.directory = nil
	return err
}

// Close releases the pinned directory handle. It does not change the
// directory or its contents.
func (directory *SeparateOwnerDirectory) Close() error {
	return directory.close()
}

func (directory *SeparateOwnerDirectory) Entries(ctx context.Context, limit int) ([]string, error) {
	if err := directory.Validate(); err != nil {
		return nil, err
	}
	entries, err := directory.directory.ReadEntriesBoundedContext(ctx, limit)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || entry.Type()&os.ModeType != 0 {
			return nil, errors.New("separate-owner directory contains an unsafe object")
		}
		names = append(names, entry.Name())
	}
	if err := directory.Validate(); err != nil {
		return nil, err
	}
	return names, nil
}

type SeparateOwnerWriteFault func(stage string) error

// RemoveFileExactObserved exposes the same exact deletion primitive while
// allowing a caller to persist and test crash cuts at the named durability
// stages. Returning an observer error aborts before any later stage.
func (directory *SeparateOwnerDirectory) RemoveFileExactObserved(
	ctx context.Context,
	name string,
	expected SeparateOwnerFileState,
	requireProtected bool,
	observer SeparateOwnerWriteFault,
) error {
	return directory.removeFileExact(ctx, name, expected, requireProtected, observer)
}

func IsSeparateOwnerAtomicTempName(target string, candidate string) bool {
	prefix := "." + target + "-"
	if !strings.HasPrefix(candidate, prefix) || !strings.HasSuffix(candidate, ".tmp") {
		return false
	}
	nonce := strings.TrimSuffix(strings.TrimPrefix(candidate, prefix), ".tmp")
	return len(nonce) == 32 && isLowerHex(nonce)
}

func contextSeparateOwnerError(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	return ctx.Err()
}

func callSeparateOwnerFault(fault SeparateOwnerWriteFault, stage string) error {
	if fault == nil {
		return nil
	}
	return fault(stage)
}
