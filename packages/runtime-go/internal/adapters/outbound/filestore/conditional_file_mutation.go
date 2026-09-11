package filestore

import (
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var (
	ErrConditionalMutationResidue       = errors.New("conditional file mutation has crash-recovery residue")
	ErrConditionalMutationIndeterminate = errors.New("conditional file mutation result is indeterminate")
)

type conditionalMoveSourceObservation struct {
	Hash     string
	Size     int64
	Mode     os.FileMode
	Device   uint64
	Inode    uint64
	Revision string
}

// ConditionalMutationAuthority binds a host-private journal/quarantine root to
// the exact directory identity captured during runtime composition. A path
// string alone never grants mutation authority.
type ConditionalMutationAuthority struct {
	root      string
	device    uint64
	inode     uint64
	available bool
}

func OpenConditionalMutationAuthority(root string) (ConditionalMutationAuthority, error) {
	root = filepath.Clean(strings.TrimSpace(root))
	if root == "" || root == "." || !filepath.IsAbs(root) {
		return ConditionalMutationAuthority{}, errors.New("conditional mutation authority root is invalid")
	}
	return openConditionalMutationAuthorityPlatform(root)
}

// OpenExistingConditionalMutationAuthority is startup-plan read-only: it
// never creates the private authority root or any journal component.
func OpenExistingConditionalMutationAuthority(root string) (ConditionalMutationAuthority, bool, error) {
	root = filepath.Clean(strings.TrimSpace(root))
	if root == "" || root == "." || !filepath.IsAbs(root) {
		return ConditionalMutationAuthority{}, false, errors.New("conditional mutation authority root is invalid")
	}
	return openExistingConditionalMutationAuthorityPlatform(root)
}

func (authority ConditionalMutationAuthority) Available() bool {
	return authority.available && authority.root != "" && authority.device != 0 && authority.inode != 0
}

// conditionalMoveTestHooks are package-private production cut points. Runtime
// plans never set them; focused adapter tests use them to prove that a path
// exchange or injected failure cannot silently delete either participant.
type conditionalMoveTestHooks struct {
	AfterInitialValidation        func()
	BeforeRename                  func()
	AfterSourceRenameBeforeVerify func()
	AfterSourceQuarantine         func()
	BeforeDestinationInstall      func()
	AfterRename                   func() error
	ForceCrossDevice              bool
}

func prepareConditionalMoveSource(path string) (conditionalMoveSourceObservation, error) {
	path = filepath.Clean(strings.TrimSpace(path))
	if path == "" || path == "." || !filepath.IsAbs(path) {
		return conditionalMoveSourceObservation{}, fmt.Errorf("%w: move source path must be absolute", ErrAtomicTextUnsafePath)
	}
	return prepareConditionalMoveSourcePlatform(path)
}

func prepareConditionalMoveDestination(path string) (bool, error) {
	path = filepath.Clean(strings.TrimSpace(path))
	if path == "" || path == "." || !filepath.IsAbs(path) {
		return false, fmt.Errorf("%w: move destination path must be absolute", ErrAtomicTextUnsafePath)
	}
	return prepareConditionalMoveDestinationPlatform(path)
}

func applyConditionalMove(plan MoveRegularFilePlan) error {
	if plan.SourcePath == "" || plan.DestinationPath == "" || plan.sourceHash == "" ||
		plan.sourceDevice == 0 || plan.sourceInode == 0 || plan.sourceRevision == "" || plan.BytesMoved < 0 || !plan.mutationAuthority.Available() ||
		plan.operationGroupID == "" || plan.operationDigest == "" || plan.sourceAuthority == "" || plan.sourceRelative == "" ||
		plan.destinationAuthority == "" || plan.destinationRelative == "" || plan.sourceEncoding == "" || !plan.operationBound {
		return errors.New("conditional move plan is invalid")
	}
	return applyConditionalMovePlatform(plan)
}

func conditionalDeleteExact(path, expectedHash string, authority ConditionalMutationAuthority, hooks *atomicTextTestHooks) error {
	path = filepath.Clean(strings.TrimSpace(path))
	expectedHash = strings.ToLower(strings.TrimSpace(expectedHash))
	if len(expectedHash) != 64 {
		return fmt.Errorf("%w: conditional delete hash is invalid", ErrAtomicTextBeforeDrift)
	}
	if _, err := hex.DecodeString(expectedHash); err != nil {
		return fmt.Errorf("%w: conditional delete hash is invalid", ErrAtomicTextBeforeDrift)
	}
	if path == "" || path == "." || !filepath.IsAbs(path) || !authority.Available() {
		return fmt.Errorf("%w: conditional delete authority paths are invalid", ErrAtomicTextUnsupportedPlatform)
	}
	return conditionalDeleteExactPlatform(path, expectedHash, authority, hooks)
}
