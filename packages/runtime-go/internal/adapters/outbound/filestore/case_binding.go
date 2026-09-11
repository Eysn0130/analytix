package filestore

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	casecontextport "analytix.local/runtime-go/internal/ports/casecontext"
)

const maxCaseBindingBytes = 1024 * 1024

var caseIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{4,80}$`)

type AnalytixCaseBinding = domainsecurity.CaseBinding

type CaseBindingReader struct{}

var _ casecontextport.CurrentBindingReader = CaseBindingReader{}
var _ casecontextport.Observer = CaseBindingReader{}

var (
	ErrCaseBindingMissing       = errors.New("case binding is missing")
	ErrCaseBindingInvalid       = errors.New("case binding is invalid")
	ErrCaseBindingUnreadable    = errors.New("case binding is unreadable")
	ErrCaseBindingUnstable      = errors.New("case binding is unstable")
	ErrCaseWorkspaceUnavailable = errors.New("case workspace is unavailable")
)

type caseBindingReadError struct {
	state         string
	bindingSHA256 string
	cause         error
}

func (e *caseBindingReadError) Error() string {
	if e.cause == nil {
		return caseBindingStateError(e.state).Error()
	}
	return caseBindingStateError(e.state).Error() + ": " + e.cause.Error()
}

func (e *caseBindingReadError) Unwrap() error {
	cause := e.cause
	for {
		nested, ok := cause.(*caseBindingReadError)
		if !ok {
			return cause
		}
		cause = nested.cause
	}
}

func (e *caseBindingReadError) Is(target error) bool {
	return target == caseBindingStateError(e.state)
}

func caseBindingStateError(state string) error {
	switch state {
	case domainsecurity.CaseBindingStateMissing:
		return ErrCaseBindingMissing
	case domainsecurity.CaseBindingStateInvalid:
		return ErrCaseBindingInvalid
	case domainsecurity.CaseBindingStateUnreadable:
		return ErrCaseBindingUnreadable
	case domainsecurity.CaseBindingStateUnstable:
		return ErrCaseBindingUnstable
	case domainsecurity.CaseBindingStateWorkspaceMissing:
		return ErrCaseWorkspaceUnavailable
	default:
		return ErrCaseBindingInvalid
	}
}

func newCaseBindingReadError(state string, cause error) error {
	return &caseBindingReadError{state: state, cause: cause}
}

func newInvalidCaseBindingContentError(body []byte, cause error) error {
	digest := sha256.Sum256(body)
	return &caseBindingReadError{state: domainsecurity.CaseBindingStateInvalid, bindingSHA256: fmt.Sprintf("%x", digest), cause: cause}
}

func (CaseBindingReader) ReadCurrentBinding(workspace string) (domainsecurity.CaseBinding, error) {
	return ReadAnalytixCaseBinding(workspace)
}

func (CaseBindingReader) Read(workspace string) (domainsecurity.CaseBinding, error) {
	return ReadAnalytixCaseBinding(workspace)
}

func (CaseBindingReader) ReadOptional(workspace string) (domainsecurity.CaseBinding, bool, error) {
	if _, err := (CaseBindingReader{}).WorkspaceRealPath(workspace); err != nil {
		return domainsecurity.CaseBinding{}, false, err
	}
	binding, err := ReadAnalytixCaseBinding(workspace)
	if errors.Is(err, ErrCaseBindingMissing) {
		return domainsecurity.CaseBinding{}, false, nil
	}
	if err != nil {
		return domainsecurity.CaseBinding{}, true, err
	}
	return binding, true, nil
}

func (CaseBindingReader) Observe(workspace string) (domainsecurity.CaseBindingObservationV1, error) {
	return observeCaseBinding(workspace, nil)
}

func observeCaseBinding(workspace string, hook caseBindingReadHook) (domainsecurity.CaseBindingObservationV1, error) {
	workspaceRealPath, err := resolveExistingCaseWorkspace(workspace)
	if err != nil {
		state := domainsecurity.CaseBindingStateWorkspaceMissing
		if errors.Is(err, ErrCaseBindingUnreadable) {
			state = domainsecurity.CaseBindingStateUnreadable
		}
		fallback := strings.TrimSpace(workspace)
		if fallback == "" {
			return domainsecurity.CaseBindingObservationV1{}, err
		}
		if abs, absErr := filepath.Abs(fallback); absErr == nil {
			fallback = filepath.Clean(abs)
			if resolved, resolveErr := workspaceRealPathAllowMissing(fallback); resolveErr == nil {
				fallback = resolved
			}
		}
		return domainsecurity.NewCaseBindingObservationV1(domainsecurity.CaseBindingObservationInputV1{
			WorkspaceRealPath: fallback, State: state,
		})
	}
	binding, readErr := readAnalytixCaseBindingAtRealPath(workspaceRealPath, hook)
	if readErr == nil {
		return domainsecurity.NewCaseBindingObservationV1(domainsecurity.CaseBindingObservationInputV1{
			WorkspaceRealPath: workspaceRealPath, State: domainsecurity.CaseBindingStateValid,
			CaseID: binding.CaseID, BindingSHA256: binding.BindingSHA256, CaseBindingHash: binding.CaseBindingHash,
		})
	}
	state := caseBindingErrorState(readErr)
	input := domainsecurity.CaseBindingObservationInputV1{WorkspaceRealPath: workspaceRealPath, State: state}
	var classified *caseBindingReadError
	if state == domainsecurity.CaseBindingStateInvalid && errors.As(readErr, &classified) {
		input.BindingSHA256 = classified.bindingSHA256
	}
	return domainsecurity.NewCaseBindingObservationV1(input)
}

func resolveExistingCaseWorkspace(workspace string) (string, error) {
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		return "", newCaseBindingReadError(domainsecurity.CaseBindingStateWorkspaceMissing, errors.New("workspace is required"))
	}
	abs, err := filepath.Abs(workspace)
	if err != nil {
		return "", newCaseBindingReadError(domainsecurity.CaseBindingStateWorkspaceMissing, err)
	}
	realPath, err := filepath.EvalSymlinks(abs)
	if err != nil {
		if os.IsPermission(err) {
			return "", newCaseBindingReadError(domainsecurity.CaseBindingStateUnreadable, err)
		}
		return "", newCaseBindingReadError(domainsecurity.CaseBindingStateWorkspaceMissing, err)
	}
	info, err := os.Stat(realPath)
	if err != nil {
		if os.IsPermission(err) {
			return "", newCaseBindingReadError(domainsecurity.CaseBindingStateUnreadable, err)
		}
		return "", newCaseBindingReadError(domainsecurity.CaseBindingStateWorkspaceMissing, err)
	}
	if !info.IsDir() {
		return "", newCaseBindingReadError(domainsecurity.CaseBindingStateWorkspaceMissing, errors.New("workspace is not a directory"))
	}
	return filepath.Clean(realPath), nil
}

func caseBindingErrorState(err error) string {
	var classified *caseBindingReadError
	if errors.As(err, &classified) {
		return classified.state
	}
	switch {
	case errors.Is(err, ErrCaseBindingUnstable):
		return domainsecurity.CaseBindingStateUnstable
	case errors.Is(err, ErrCaseBindingUnreadable):
		return domainsecurity.CaseBindingStateUnreadable
	case errors.Is(err, ErrCaseBindingMissing):
		return domainsecurity.CaseBindingStateMissing
	case errors.Is(err, ErrCaseWorkspaceUnavailable):
		return domainsecurity.CaseBindingStateWorkspaceMissing
	default:
		return domainsecurity.CaseBindingStateInvalid
	}
}

func (CaseBindingReader) WorkspaceRealPath(workspace string) (string, error) {
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		return "", errors.New("workspace is required")
	}
	abs, err := filepath.Abs(workspace)
	if err != nil {
		return "", err
	}
	return workspaceRealPathAllowMissing(abs)
}

type caseBindingDocument struct {
	Version       int    `json:"version"`
	WorkspaceRoot string `json:"workspaceRoot"`
	CaseID        string `json:"caseId"`
	Source        string `json:"source"`
	UpdatedAt     string `json:"updatedAt"`
}

func WorkspaceHasAnalytixCaseBinding(workspace string) bool {
	_, err := ReadAnalytixCaseBinding(workspace)
	return err == nil
}

func ReadAnalytixCaseBinding(workspace string) (domainsecurity.CaseBinding, error) {
	return readAnalytixCaseBinding(workspace, nil)
}

type caseBindingReadHook func(stage string)

func readAnalytixCaseBinding(workspace string, hook caseBindingReadHook) (domainsecurity.CaseBinding, error) {
	workspaceRealPath, err := resolveExistingCaseWorkspace(workspace)
	if err != nil {
		return domainsecurity.CaseBinding{}, err
	}
	return readAnalytixCaseBindingAtRealPath(workspaceRealPath, hook)
}

func readAnalytixCaseBindingAtRealPath(workspaceRealPath string, hook caseBindingReadHook) (domainsecurity.CaseBinding, error) {
	body, err := secureReadCaseBinding(workspaceRealPath, maxCaseBindingBytes, hook)
	if err != nil {
		return domainsecurity.CaseBinding{}, err
	}
	if len(body) == 0 || len(body) > maxCaseBindingBytes {
		return domainsecurity.CaseBinding{}, newInvalidCaseBindingContentError(body, errors.New("case binding is empty or too large"))
	}
	if err := domainjsonstrict.Validate(body, domainjsonstrict.Options{
		RequireObject: true, MaxBytes: maxCaseBindingBytes, MaxDepth: 8, MaxTokens: 128, MaxStringBytes: 64 * 1024,
	}); err != nil {
		return domainsecurity.CaseBinding{}, newInvalidCaseBindingContentError(body, fmt.Errorf("validate case binding JSON: %w", err))
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var document caseBindingDocument
	if err := decoder.Decode(&document); err != nil {
		return domainsecurity.CaseBinding{}, newInvalidCaseBindingContentError(body, fmt.Errorf("decode case binding: %w", err))
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return domainsecurity.CaseBinding{}, newInvalidCaseBindingContentError(body, errors.New("case binding contains trailing JSON values"))
	}
	caseID := strings.TrimSpace(document.CaseID)
	if document.Version != 1 || document.CaseID != caseID || !caseIDPattern.MatchString(caseID) || document.Source != "analytix-data-analysis" {
		return domainsecurity.CaseBinding{}, newInvalidCaseBindingContentError(body, errors.New("case binding has an invalid version, caseId, or source"))
	}
	declaredWorkspace := strings.TrimSpace(document.WorkspaceRoot)
	if declaredWorkspace == "" || document.WorkspaceRoot != declaredWorkspace {
		return domainsecurity.CaseBinding{}, newInvalidCaseBindingContentError(body, errors.New("case binding is missing workspaceRoot"))
	}
	declaredAbs, err := filepath.Abs(declaredWorkspace)
	if err != nil {
		return domainsecurity.CaseBinding{}, newInvalidCaseBindingContentError(body, fmt.Errorf("resolve declared workspace: %w", err))
	}
	declaredRealPath, err := filepath.EvalSymlinks(declaredAbs)
	if err != nil || declaredRealPath != workspaceRealPath {
		return domainsecurity.CaseBinding{}, newInvalidCaseBindingContentError(body, errors.New("case binding workspaceRoot does not match the active workspace"))
	}
	bindingDigest := sha256.Sum256(body)
	bindingSHA256 := fmt.Sprintf("%x", bindingDigest)
	contextDigest := sha256.Sum256([]byte(strings.Join([]string{
		"case-binding-v1",
		workspaceRealPath,
		caseID,
		bindingSHA256,
	}, "\x00")))
	return domainsecurity.CaseBinding{
		Version:           document.Version,
		CaseID:            caseID,
		WorkspaceRealPath: workspaceRealPath,
		BindingPath:       filepath.Join(workspaceRealPath, workspaceHostMetadataDir, "case-project.json"),
		BindingSHA256:     bindingSHA256,
		CaseBindingHash:   fmt.Sprintf("%x", contextDigest),
	}, nil
}

func workspaceRealPathAllowMissing(path string) (string, error) {
	path = filepath.Clean(path)
	suffix := []string{}
	for {
		resolved, err := filepath.EvalSymlinks(path)
		if err == nil {
			for index := len(suffix) - 1; index >= 0; index-- {
				resolved = filepath.Join(resolved, suffix[index])
			}
			return filepath.Clean(resolved), nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		parent := filepath.Dir(path)
		if parent == path {
			return "", err
		}
		suffix = append(suffix, filepath.Base(path))
		path = parent
	}
}
