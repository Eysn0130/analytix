package filestore

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

var (
	ErrAtomicTextBeforeDrift         = errors.New("atomic text mutation before state changed")
	ErrAtomicTextUnsafePath          = errors.New("atomic text mutation path is unsafe")
	ErrAtomicTextUnsupportedPlatform = errors.New("atomic text mutation is unsupported on this platform")
)

type atomicTextState struct {
	Exists  bool
	Content []byte
	Mode    os.FileMode
}

type atomicTextReplaceRequest struct {
	Path              string
	Content           []byte
	ExpectedExists    bool
	ExpectedHash      string
	CreateParents     bool
	PreserveMode      bool
	DefaultMode       os.FileMode
	Delete            bool
	MutationAuthority ConditionalMutationAuthority
}

type atomicTextTestHooks struct {
	AfterInitialValidation func()
	FailWriteAfter         int
	BeforeReplace          func() error
	AfterDeleteQuarantine  func() error
}

func inspectAtomicTextTarget(path string, missingParentsAreAbsent bool) (atomicTextState, error) {
	path = filepath.Clean(strings.TrimSpace(path))
	if path == "" || path == "." || !filepath.IsAbs(path) {
		return atomicTextState{}, fmt.Errorf("%w: target path must be absolute", ErrAtomicTextUnsafePath)
	}
	return inspectAtomicTextTargetPlatform(path, missingParentsAreAbsent)
}

func atomicReplaceText(request atomicTextReplaceRequest) error {
	return atomicReplaceTextWithHooks(request, nil)
}

func atomicReplaceTextWithHooks(request atomicTextReplaceRequest, hooks *atomicTextTestHooks) error {
	request.Path = filepath.Clean(strings.TrimSpace(request.Path))
	request.ExpectedHash = strings.ToLower(strings.TrimSpace(request.ExpectedHash))
	if request.Path == "" || request.Path == "." || !filepath.IsAbs(request.Path) {
		return fmt.Errorf("%w: target path must be absolute", ErrAtomicTextUnsafePath)
	}
	if request.ExpectedExists {
		if len(request.ExpectedHash) != sha256.Size*2 {
			return fmt.Errorf("%w: expected before hash is invalid", ErrAtomicTextBeforeDrift)
		}
		if _, err := hex.DecodeString(request.ExpectedHash); err != nil {
			return fmt.Errorf("%w: expected before hash is invalid", ErrAtomicTextBeforeDrift)
		}
	} else if request.ExpectedHash != "" {
		return fmt.Errorf("%w: absent target cannot carry a before hash", ErrAtomicTextBeforeDrift)
	}
	if request.Delete && len(request.Content) != 0 {
		return errors.New("atomic text delete cannot contain replacement bytes")
	}
	if request.Delete {
		return conditionalDeleteExact(request.Path, request.ExpectedHash, request.MutationAuthority, hooks)
	}
	if request.DefaultMode.Perm() == 0 {
		request.DefaultMode = 0o644
	}
	return atomicReplaceTextPlatform(request, hooks)
}

func validateAtomicTextBefore(request atomicTextReplaceRequest, state atomicTextState) error {
	if state.Exists != request.ExpectedExists {
		return fmt.Errorf("%w: target existence differs from the prepared state", ErrAtomicTextBeforeDrift)
	}
	if !state.Exists {
		return nil
	}
	if digestAtomicText(state.Content) != request.ExpectedHash {
		return fmt.Errorf("%w: target content hash differs from the prepared state", ErrAtomicTextBeforeDrift)
	}
	return nil
}

func digestAtomicText(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}

func atomicTextReplacementMode(request atomicTextReplaceRequest, state atomicTextState) os.FileMode {
	if request.PreserveMode && state.Exists && state.Mode.Perm() != 0 {
		return state.Mode.Perm()
	}
	return request.DefaultMode.Perm()
}

func atomicTextTempName(path string) (string, error) {
	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	return "." + filepath.Base(path) + ".analytix-" + hex.EncodeToString(nonce) + ".tmp", nil
}

func writeAtomicTextContent(write func([]byte) (int, error), content []byte, hooks *atomicTextTestHooks) error {
	failAfter := -1
	if hooks != nil {
		failAfter = hooks.FailWriteAfter
	}
	if failAfter > 0 && failAfter < len(content) {
		if err := writeAllAtomicText(write, content[:failAfter]); err != nil {
			return err
		}
		return errors.New("injected atomic text write failure")
	}
	return writeAllAtomicText(write, content)
}

func writeAllAtomicText(write func([]byte) (int, error), content []byte) error {
	for len(content) > 0 {
		written, err := write(content)
		if err != nil {
			return err
		}
		if written <= 0 || written > len(content) {
			return io.ErrShortWrite
		}
		content = content[written:]
	}
	return nil
}

func atomicTextBeforeReplace(hooks *atomicTextTestHooks) error {
	if hooks == nil || hooks.BeforeReplace == nil {
		return nil
	}
	return hooks.BeforeReplace()
}

func equalAtomicTextBytes(left, right []byte) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
