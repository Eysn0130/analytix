package filestore

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	filetoolsapp "analytix.local/runtime-go/internal/app/filetools"
	"analytix.local/runtime-go/internal/domain/jsonstrict"
	objectediting "analytix.local/runtime-go/internal/ports/objectediting"
)

// The gate covers all instances, including stores with different receipt roots.
// It is deliberately process-local; this does not implement a cross-process lease.
var objectEditingGate = make(chan struct{}, 1)
var objectEditingOperationID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{7,127}$`)

type ObjectEditingFiles struct {
	receiptRoot  string
	rootIdentity string
	protected    []string
	// Set only by the native Office constructor; the default text path retains
	// its original encoding and size limits.
	officeKind string
	// Narrow internal fault seams; production uses the existing atomic primitive.
	replaceDocument func(atomicTextReplaceRequest) error
	replaceJournal  func(atomicTextReplaceRequest) error
}

var _ objectediting.Files = (*ObjectEditingFiles)(nil)

type objectEditingTarget struct{ workspace, path, identity, binding string }

// This record deliberately contains no document text, raw paths or principal.
type objectEditingRecord struct {
	Version        int    `json:"version"`
	ObjectIdentity string `json:"objectIdentity"`
	OperationID    string `json:"operationId"`
	PathBinding    string `json:"pathBinding"`
	RequestHash    string `json:"requestHash"`
	BeforeHash     string `json:"beforeHash"`
	AfterHash      string `json:"afterHash"`
	Status         string `json:"status"`
	CreatedAt      string `json:"createdAt"`
	SavedAt        string `json:"savedAt,omitempty"`
}

// NewObjectEditingFiles never creates parents or changes root permissions.
func NewObjectEditingFiles(receiptRoot string, protectedRoots []string) (*ObjectEditingFiles, error) {
	if receiptRoot == "" || !filepath.IsAbs(receiptRoot) || strings.TrimSpace(receiptRoot) != receiptRoot || mutationPathContainsSymlink(receiptRoot) {
		return nil, objectediting.ErrForbidden
	}
	receiptRoot = filepath.Clean(receiptRoot)
	id, err := objectEditingPrivateRootIdentity(receiptRoot)
	if err != nil {
		return nil, err
	}
	s := &ObjectEditingFiles{receiptRoot: receiptRoot, rootIdentity: id}
	for _, root := range append(append([]string(nil), protectedRoots...), receiptRoot) {
		root = strings.TrimPrefix(root, mandatoryProtectedRootPrefix)
		if !filepath.IsAbs(root) {
			return nil, objectediting.ErrForbidden
		}
		identity, ok := ResolveMutationIdentityPath(root, root)
		if !ok {
			return nil, objectediting.ErrForbidden
		}
		s.protected = append(s.protected, identity)
	}
	return s, nil
}

func objectEditingLock(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case objectEditingGate <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *ObjectEditingFiles) checkRoot() error {
	id, err := objectEditingPrivateRootIdentity(s.receiptRoot)
	if err != nil || id != s.rootIdentity {
		return objectediting.ErrForbidden
	}
	return nil
}

func objectEditingWithin(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

func (s *ObjectEditingFiles) target(workspace, path string) (objectEditingTarget, error) {
	if workspace == "" || !filepath.IsAbs(workspace) || path == "" || strings.TrimSpace(workspace) != workspace || strings.TrimSpace(path) != path || strings.ContainsRune(workspace+path, 0) {
		return objectEditingTarget{}, objectediting.ErrInvalidInput
	}
	if err := s.checkRoot(); err != nil {
		return objectEditingTarget{}, err
	}
	if s.officeKind != "" && strings.ToLower(filepath.Ext(path)) != "."+s.officeKind {
		return objectEditingTarget{}, objectediting.ErrNotText
	}
	base, err := canonicalMutationIdentitySystemAlias(filepath.Clean(workspace))
	if err != nil || mutationPathContainsSymlink(base) {
		return objectEditingTarget{}, objectediting.ErrForbidden
	}
	info, err := os.Lstat(base)
	if err != nil || !info.IsDir() {
		return objectEditingTarget{}, objectediting.ErrForbidden
	}
	resolved := path
	if !filepath.IsAbs(resolved) {
		resolved = filepath.Join(base, resolved)
	}
	resolved, err = canonicalMutationIdentitySystemAlias(filepath.Clean(resolved))
	if err != nil {
		return objectEditingTarget{}, objectediting.ErrForbidden
	}
	baseIdentity, baseOK := ResolveMutationIdentityPath(base, base)
	identity, targetOK := ResolveMutationIdentityPath(base, resolved)
	if !baseOK || !targetOK || !objectEditingWithin(baseIdentity, identity) || baseIdentity == identity {
		return objectEditingTarget{}, objectediting.ErrForbidden
	}
	// Host-owned metadata is never an editable document even when a workspace
	// itself includes it and no extra protected root was supplied.
	for _, component := range strings.Split(filepath.ToSlash(identity), "/") {
		if component == ".analytix" || component == ".git" {
			return objectEditingTarget{}, objectediting.ErrForbidden
		}
	}
	for _, root := range s.protected {
		if objectEditingWithin(root, identity) {
			return objectEditingTarget{}, objectediting.ErrForbidden
		}
	}
	return objectEditingTarget{workspace: baseIdentity, path: resolved, identity: identity, binding: digestAtomicText([]byte(baseIdentity + "\x00" + identity))}, nil
}

func objectEditingText(content string) bool {
	if !utf8.ValidString(content) {
		return false
	}
	for _, c := range content {
		if c < 0x20 && c != '\n' && c != '\r' && c != '\t' {
			return false
		}
	}
	return !strings.HasPrefix(content, "%PDF-")
}

func objectEditingInspect(path string) (atomicTextState, error) {
	return objectEditingInspectBounded(path, objectediting.MaxTextBytes)
}

func objectEditingInspectBounded(path string, maxBytes int64) (atomicTextState, error) {
	info, err := os.Lstat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return atomicTextState{}, os.ErrNotExist
		}
		return atomicTextState{}, objectediting.ErrForbidden
	}
	if !info.Mode().IsRegular() {
		return atomicTextState{}, objectediting.ErrForbidden
	}
	if info.Size() > maxBytes {
		return atomicTextState{}, objectediting.ErrTooLarge
	}
	state, err := inspectAtomicTextTargetWithPolicy(path, false, maxBytes, atomicTextReadPolicy{RequireSingleLink: true})
	if err != nil {
		return atomicTextState{}, objectEditingError(err)
	}
	if !state.Exists {
		return atomicTextState{}, os.ErrNotExist
	}
	if int64(len(state.Content)) > maxBytes {
		return atomicTextState{}, objectediting.ErrTooLarge
	}
	return state, nil
}

func objectEditingDecode(raw []byte) (string, string, error) {
	content, encoding, ok := filetoolsapp.DecodeTextBytes(raw)
	if len(content) > objectediting.MaxTextBytes {
		return "", "", objectediting.ErrTooLarge
	}
	if !ok || !objectEditingText(content) || !bytes.Equal(filetoolsapp.EncodeTextBytes(content, encoding), raw) {
		return "", "", objectediting.ErrNotText
	}
	return content, encoding, nil
}

func (s *ObjectEditingFiles) Read(ctx context.Context, workspace, path string) (objectediting.Document, error) {
	if err := objectEditingLock(ctx); err != nil {
		return objectediting.Document{}, err
	}
	defer func() { <-objectEditingGate }()
	target, err := s.target(workspace, path)
	if err != nil {
		return objectediting.Document{}, err
	}
	state, err := s.inspectObject(target.path)
	if err != nil {
		return objectediting.Document{}, err
	}
	content, encoding, err := s.decodeObject(state.Content)
	if err != nil {
		return objectediting.Document{}, err
	}
	if err := ctx.Err(); err != nil {
		return objectediting.Document{}, err
	}
	return objectediting.Document{Workspace: target.workspace, IdentityPath: target.identity, Path: target.path, Content: content, Revision: digestAtomicText(state.Content), Encoding: encoding}, nil
}

func objectEditingHash(value string) bool {
	if len(value) != 64 || strings.ToLower(value) != value {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}
func objectEditingIDs(identity, operation string) bool {
	return objectEditingHash(identity) && objectEditingOperationID.MatchString(operation)
}
func (s *ObjectEditingFiles) recordPath(identity, operation string) string {
	return filepath.Join(s.receiptRoot, digestAtomicText([]byte(identity+"\x00"+operation))+".json")
}

func (s *ObjectEditingFiles) readRecord(identity, operation string) (objectEditingRecord, string, error) {
	if err := s.checkRoot(); err != nil {
		return objectEditingRecord{}, "", err
	}
	path := s.recordPath(identity, operation)
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return objectEditingRecord{}, "", objectediting.ErrOperationNotFound
	}
	if err != nil || info.Size() > 16384 || !info.Mode().IsRegular() {
		return objectEditingRecord{}, "", objectediting.ErrPersistence
	}
	if err := objectEditingPrivateReceipt(path); err != nil {
		return objectEditingRecord{}, "", err
	}
	state, err := inspectAtomicTextTargetBounded(path, false, 16384)
	if err != nil || !state.Exists || len(state.Content) > 16384 {
		return objectEditingRecord{}, "", objectediting.ErrPersistence
	}
	if err := jsonstrict.Validate(state.Content, jsonstrict.Options{RequireObject: true, MaxBytes: 16384, MaxDepth: 2, MaxTokens: 32, MaxStringBytes: 128}); err != nil {
		return objectEditingRecord{}, "", objectediting.ErrPersistence
	}
	var r objectEditingRecord
	d := json.NewDecoder(bytes.NewReader(state.Content))
	d.DisallowUnknownFields()
	if d.Decode(&r) != nil || d.Decode(&struct{}{}) != io.EOF || r.Version != 1 || r.ObjectIdentity != identity || r.OperationID != operation || !objectEditingHash(r.PathBinding) || !objectEditingHash(r.RequestHash) || !objectEditingHash(r.BeforeHash) || !objectEditingHash(r.AfterHash) {
		return objectEditingRecord{}, "", objectediting.ErrPersistence
	}
	canonical, err := json.Marshal(r)
	if err != nil || !bytes.Equal(canonical, state.Content) {
		return objectEditingRecord{}, "", objectediting.ErrPersistence
	}
	if _, err := time.Parse(time.RFC3339Nano, r.CreatedAt); err != nil {
		return objectEditingRecord{}, "", objectediting.ErrPersistence
	}
	if r.Status != objectediting.StatusPending && r.Status != objectediting.StatusCommitted && r.Status != objectediting.StatusConflict {
		return objectEditingRecord{}, "", objectediting.ErrPersistence
	}
	if r.Status == objectediting.StatusCommitted {
		if _, err := time.Parse(time.RFC3339Nano, r.SavedAt); err != nil {
			return objectEditingRecord{}, "", objectediting.ErrPersistence
		}
	}
	return r, digestAtomicText(state.Content), nil
}

func (s *ObjectEditingFiles) writeRecord(r objectEditingRecord, expected string) error {
	if err := s.checkRoot(); err != nil {
		return err
	}
	body, err := json.Marshal(r)
	if err != nil {
		return objectediting.ErrPersistence
	}
	request := atomicTextReplaceRequest{Path: s.recordPath(r.ObjectIdentity, r.OperationID), Content: body, MaxBytes: 16384, ExpectedExists: expected != "", ExpectedHash: expected, DefaultMode: 0o600}
	replace := s.replaceJournal
	if replace == nil {
		replace = atomicReplaceText
	}
	if err := replace(request); err != nil {
		return objectediting.ErrPersistence
	}
	if err := s.checkRoot(); err != nil {
		return objectediting.ErrPersistence
	}
	return nil
}

func objectEditingReceipt(r objectEditingRecord) objectediting.Receipt {
	revision := ""
	if r.Status == objectediting.StatusCommitted {
		revision = r.AfterHash
	}
	status := r.Status
	if status == objectediting.StatusPending {
		status = objectediting.StatusUnknown
	}
	return objectediting.Receipt{OperationID: r.OperationID, Revision: revision, Status: status, SavedAt: r.SavedAt}
}

func (s *ObjectEditingFiles) reconcile(target objectEditingTarget, r objectEditingRecord, recordHash string) (objectediting.Receipt, error) {
	if r.PathBinding != target.binding {
		return objectediting.Receipt{}, objectediting.ErrOperationMismatch
	}
	if r.Status != objectediting.StatusPending {
		return objectEditingReceipt(r), nil
	}
	state, err := s.inspectObject(target.path)
	if err != nil {
		return objectEditingReceipt(r), nil
	}
	current := digestAtomicText(state.Content)
	if current == r.AfterHash {
		if err := objectEditingSyncTarget(target.path); err != nil {
			return objectEditingReceipt(r), objectediting.ErrPersistence
		}
		confirmed, err := s.inspectObject(target.path)
		if err != nil || digestAtomicText(confirmed.Content) != r.AfterHash {
			return objectEditingReceipt(r), objectediting.ErrPersistence
		}
		r.Status = objectediting.StatusCommitted
		r.SavedAt = time.Now().UTC().Format(time.RFC3339Nano)
	} else if current != r.BeforeHash {
		r.Status = objectediting.StatusConflict
	} else {
		return objectEditingReceipt(r), nil
	}
	if err := s.writeRecord(r, recordHash); err != nil {
		return objectediting.Receipt{OperationID: r.OperationID, Status: objectediting.StatusUnknown}, objectediting.ErrPersistence
	}
	return objectEditingReceipt(r), nil
}

func (s *ObjectEditingFiles) Status(ctx context.Context, identity, operation, workspace, path string) (objectediting.Receipt, error) {
	if !objectEditingIDs(identity, operation) {
		return objectediting.Receipt{}, objectediting.ErrInvalidInput
	}
	if err := objectEditingLock(ctx); err != nil {
		return objectediting.Receipt{}, err
	}
	defer func() { <-objectEditingGate }()
	target, err := s.target(workspace, path)
	if err != nil {
		return objectediting.Receipt{}, err
	}
	r, hash, err := s.readRecord(identity, operation)
	if err != nil {
		return objectediting.Receipt{}, err
	}
	if err := ctx.Err(); err != nil {
		return objectediting.Receipt{}, err
	}
	return s.reconcile(target, r, hash)
}

func (s *ObjectEditingFiles) Commit(ctx context.Context, input objectediting.CommitInput) (objectediting.Receipt, error) {
	if !objectEditingIDs(input.ObjectIdentity, input.OperationID) || !objectEditingHash(input.BaseRevision) {
		return objectediting.Receipt{}, objectediting.ErrInvalidInput
	}
	if len(input.Content) > s.maxContentBytes() {
		return objectediting.Receipt{}, objectediting.ErrTooLarge
	}
	if !objectEditingText(input.Content) {
		return objectediting.Receipt{}, objectediting.ErrNotText
	}
	if err := objectEditingLock(ctx); err != nil {
		return objectediting.Receipt{}, err
	}
	defer func() { <-objectEditingGate }()
	target, err := s.target(input.Workspace, input.Path)
	if err != nil {
		return objectediting.Receipt{}, err
	}
	requestHash := digestAtomicText([]byte(input.BaseRevision + "\x00" + input.Content))
	existing, recordHash, err := s.readRecord(input.ObjectIdentity, input.OperationID)
	if err == nil {
		if existing.PathBinding != target.binding || existing.RequestHash != requestHash {
			return objectediting.Receipt{}, objectediting.ErrOperationMismatch
		}
		return s.reconcile(target, existing, recordHash)
	}
	if !errors.Is(err, objectediting.ErrOperationNotFound) {
		return objectediting.Receipt{}, err
	}
	state, err := s.inspectObject(target.path)
	if err != nil {
		return objectediting.Receipt{}, err
	}
	_, encoding, err := s.decodeObject(state.Content)
	if err != nil {
		return objectediting.Receipt{}, err
	}
	encoded, err := s.encodeObject(input.Content, encoding)
	if err != nil {
		return objectediting.Receipt{}, err
	}
	if int64(len(encoded)) > s.maxObjectBytes() {
		return objectediting.Receipt{}, objectediting.ErrTooLarge
	}
	r := objectEditingRecord{Version: 1, ObjectIdentity: input.ObjectIdentity, OperationID: input.OperationID, PathBinding: target.binding, RequestHash: requestHash, BeforeHash: input.BaseRevision, AfterHash: digestAtomicText(encoded), Status: objectediting.StatusPending, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)}
	if digestAtomicText(state.Content) != input.BaseRevision {
		r.Status = objectediting.StatusConflict
	}
	if err := ctx.Err(); err != nil {
		return objectediting.Receipt{}, err
	}
	// The intent is durable before the document can change, including conflicts.
	if err := s.writeRecord(r, ""); err != nil {
		return objectediting.Receipt{OperationID: input.OperationID, Status: objectediting.StatusUnknown}, err
	}
	if r.Status == objectediting.StatusConflict {
		return objectEditingReceipt(r), objectediting.ErrConflict
	}
	if err := ctx.Err(); err != nil {
		return objectEditingReceipt(r), err
	}
	replace := s.replaceDocument
	if replace == nil {
		replace = atomicReplaceText
	}
	writeErr := replace(atomicTextReplaceRequest{Path: target.path, Content: encoded, MaxBytes: s.maxObjectBytes(), ReadPolicy: atomicTextReadPolicy{RequireSingleLink: true}, ExpectedExists: true, ExpectedHash: input.BaseRevision, PreserveMode: true, DefaultMode: state.Mode})
	// A replacement error may follow a successful rename/fsync. Resolve from
	// durable intent + actual bytes; never project every error as "not saved".
	r, recordHash, err = s.readRecord(input.ObjectIdentity, input.OperationID)
	if err != nil {
		return objectediting.Receipt{OperationID: input.OperationID, Status: objectediting.StatusUnknown}, objectediting.ErrPersistence
	}
	receipt, recoveryErr := s.reconcile(target, r, recordHash)
	if recoveryErr != nil {
		return receipt, recoveryErr
	}
	if receipt.Status == objectediting.StatusConflict {
		return receipt, objectediting.ErrConflict
	}
	if receipt.Status == objectediting.StatusCommitted {
		return receipt, nil
	}
	if writeErr != nil {
		return receipt, objectEditingError(writeErr)
	}
	return receipt, objectediting.ErrPersistence
}

func objectEditingError(err error) error {
	switch {
	case errors.Is(err, ErrAtomicTextBeforeDrift):
		return objectediting.ErrConflict
	case errors.Is(err, ErrAtomicTextTooLarge):
		return objectediting.ErrTooLarge
	case errors.Is(err, ErrAtomicTextUnsafePath), errors.Is(err, ErrAtomicTextUnsupportedPlatform):
		return objectediting.ErrForbidden
	case errors.Is(err, os.ErrNotExist):
		return os.ErrNotExist
	default:
		return objectediting.ErrPersistence
	}
}
