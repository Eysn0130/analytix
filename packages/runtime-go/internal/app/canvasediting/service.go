// Package canvasediting owns reviewed scene and deterministic image changes.
// It uses the existing object persistence authority; it has no filesystem or
// Provider access. All document and review values are protected-local data.
package canvasediting

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"regexp"
	"sync"

	objectapp "analytix.local/runtime-go/internal/app/objectediting"
	canvas "analytix.local/runtime-go/internal/domain/canvas"
	identity "analytix.local/runtime-go/internal/domain/identity"
	identityport "analytix.local/runtime-go/internal/ports/identity"
	files "analytix.local/runtime-go/internal/ports/objectediting"
)

var ErrUnavailable = errors.New("canvas_unavailable")
var ErrStale = errors.New("canvas_revision_stale")
var ErrInvalid = errors.New("canvas_request_invalid")
var threadPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)

type Objects interface {
	Open(context.Context, string, string) (objectapp.Opened, error)
	Close(context.Context, string) error
	Status(context.Context, string, string) (files.Receipt, error)
	PrepareNativeChange(context.Context, string, files.NativeChangeDraft) (files.NativeChangeStatus, error)
	CommitNativeChange(context.Context, string, string, string, string, string, string) (files.Receipt, error)
	NativeRecovery(context.Context, string, string) (files.NativeRecovery, error)
	UndoNativeChange(context.Context, string, string, string, string) (files.Receipt, error)
	CancelNativeChange(context.Context, string, string, string, string) (files.NativeRecovery, error)
	ResumeNativeChange(context.Context, string, string, string, string) (files.Receipt, error)
}

type Document struct {
	SessionID string `json:"sessionId"`
	ObjectID  string `json:"objectId"`
	ThreadID  string `json:"threadId"`
	Kind      string `json:"kind"`
	Path      string `json:"path"`
	Revision  string `json:"revision"`
	Content   string `json:"content"`
}
type Proposal struct {
	ID              string               `json:"proposalId"`
	BaseRevision    string               `json:"baseRevision"`
	Kind            string               `json:"kind"`
	FactsDigest     string               `json:"factsDigest,omitempty"`
	SceneDiff       []canvas.Change      `json:"sceneDiff,omitempty"`
	ImageDiff       []canvas.ImageChange `json:"imageDiff,omitempty"`
	CandidateDigest string               `json:"candidateDigest"`
	Status          string               `json:"status"`
}
type proposal struct {
	view      Proposal
	candidate []byte
	changeID  string
}
type session struct {
	id, thread, workspace, kind string
	principal                   identity.PrincipalV1
	object                      objectapp.Opened
	proposals                   map[string]*proposal
}
type Service struct {
	mu        sync.Mutex
	identity  identityport.Authority
	objects   map[string]Objects
	projector objectapp.TrustedSelectionProjector
	capture   func(context.Context, string, string, func() error) (func(), error)
	sessions  map[string]*session
}

// Construction and host binding are trusted composition operations. They are
// unavailable as public requests and do not themselves activate a plugin.
func New(authority identityport.Authority, objects map[string]Objects) *Service {
	owned := map[string]Objects{}
	for _, kind := range []string{"canvas", "png"} {
		if objects[kind] != nil {
			owned[kind] = objects[kind]
		}
	}
	return &Service{identity: authority, objects: owned, sessions: map[string]*session{}}
}
func (s *Service) BindHost(projector objectapp.TrustedSelectionProjector, capture func(context.Context, string, string, func() error) (func(), error)) error {
	if s == nil || projector == nil || capture == nil {
		return ErrUnavailable
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.sessions) != 0 || s.projector != nil {
		return ErrUnavailable
	}
	s.projector, s.capture = projector, capture
	return nil
}
func token() (string, error) {
	var raw [24]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", ErrUnavailable
	}
	return hex.EncodeToString(raw[:]), nil
}
func digest(raw []byte) string { sum := sha256.Sum256(raw); return hex.EncodeToString(sum[:]) }
func (s *Service) principal(ctx context.Context, p identity.PrincipalV1) error {
	if s == nil || ctx == nil || ctx.Err() != nil || s.identity == nil || s.projector == nil || s.capture == nil || identity.ValidatePrincipalV1(p) != nil || s.identity.ValidateCurrent(ctx, p) != nil {
		return ErrUnavailable
	}
	return nil
}
func (s *Service) authority(current *session, purpose string) objectapp.ScopeAuthority {
	return objectapp.ScopeAuthority{Principal: current.principal, ObjectID: current.object.ObjectID, ThreadID: current.thread, Purpose: purpose, Workspace: current.workspace, Path: current.object.Path}
}
func decode(kind string, opened objectapp.Opened) ([]byte, error) {
	if len(opened.Content) > base64.StdEncoding.EncodedLen(16<<20) {
		return nil, ErrInvalid
	}
	raw, err := base64.StdEncoding.Strict().DecodeString(opened.Content)
	if err != nil || base64.StdEncoding.EncodeToString(raw) != opened.Content || digest(raw) != opened.Revision {
		return nil, ErrInvalid
	}
	switch kind {
	case "canvas":
		_, err = canvas.ParseScene(raw)
	case "png":
		_, err = canvas.InspectImage(raw)
	default:
		return nil, ErrInvalid
	}
	if err != nil {
		return nil, ErrInvalid
	}
	return raw, nil
}
func view(current *session, opened objectapp.Opened) Document {
	return Document{current.id, opened.ObjectID, current.thread, current.kind, opened.Path, opened.Revision, opened.Content}
}

func (s *Service) Open(ctx context.Context, p identity.PrincipalV1, thread, workspace, path, kind string) (Document, error) {
	if s.principal(ctx, p) != nil || !threadPattern.MatchString(thread) || s.objects[kind] == nil {
		return Document{}, ErrUnavailable
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	opened, err := s.objects[kind].Open(ctx, workspace, path)
	if err != nil {
		return Document{}, err
	}
	retained := false
	defer func() {
		if retained {
			return
		}
		for _, existing := range s.sessions {
			if existing.kind == kind && existing.object.SessionID == opened.SessionID {
				return
			}
		}
		// Housekeeping after a rejected open grants no new read/write authority.
		// Keep a shared underlying session and do not let request cancellation
		// prevent releasing an unclaimed, principal-owned in-memory handle.
		_ = s.objects[kind].Close(context.WithoutCancel(ctx), opened.SessionID)
	}()
	current := &session{thread: thread, workspace: workspace, kind: kind, principal: p, object: opened, proposals: map[string]*proposal{}}
	if s.projector.ValidateCurrent(ctx, s.authority(current, "discuss")) != nil || s.principal(ctx, p) != nil {
		return Document{}, ErrUnavailable
	}
	if _, err = decode(kind, opened); err != nil {
		return Document{}, err
	}
	for _, old := range s.sessions {
		if old.object.SessionID == opened.SessionID && old.thread == thread && old.kind == kind && identity.SamePrincipalV1(old.principal, p) {
			return view(old, opened), nil
		}
	}
	if len(s.sessions) >= 64 {
		return Document{}, ErrUnavailable
	}
	current.id, err = token()
	if err != nil {
		return Document{}, err
	}
	// Bytes remain only in bounded proposals; reread the current object for every operation.
	current.object.Content = ""
	s.sessions[current.id] = current
	retained = true
	return view(current, opened), nil
}
func (s *Service) current(ctx context.Context, p identity.PrincipalV1, id, thread, purpose string) (*session, objectapp.Opened, []byte, error) {
	if s.principal(ctx, p) != nil {
		return nil, objectapp.Opened{}, nil, ErrUnavailable
	}
	current := s.sessions[id]
	if current == nil || current.thread != thread || !identity.SamePrincipalV1(current.principal, p) || s.projector.ValidateCurrent(ctx, s.authority(current, purpose)) != nil {
		return nil, objectapp.Opened{}, nil, ErrUnavailable
	}
	opened, err := s.objects[current.kind].Open(ctx, current.workspace, current.object.Path)
	if err != nil || opened.SessionID != current.object.SessionID || opened.ObjectID != current.object.ObjectID || opened.Path != current.object.Path {
		return nil, objectapp.Opened{}, nil, ErrStale
	}
	raw, err := decode(current.kind, opened)
	if err != nil || s.principal(ctx, p) != nil || s.projector.ValidateCurrent(ctx, s.authority(current, purpose)) != nil {
		return nil, objectapp.Opened{}, nil, ErrUnavailable
	}
	return current, opened, raw, nil
}
func (s *Service) Read(ctx context.Context, p identity.PrincipalV1, id, thread string) (Document, error) {
	if s == nil {
		return Document{}, ErrUnavailable
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	current, opened, _, err := s.current(ctx, p, id, thread, "discuss")
	if err != nil {
		return Document{}, err
	}
	return view(current, opened), nil
}

// ProposeScene derives both candidate and Diff in Core. The selection is a
// complete list of original stable IDs, and every operation must stay inside it.
// Layout/display changes cannot rewrite source facts or remove relationships.
func (s *Service) ProposeScene(ctx context.Context, p identity.PrincipalV1, id, thread, base string, selected []string, operations []canvas.Operation) (Proposal, error) {
	if s == nil {
		return Proposal{}, ErrUnavailable
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	current, opened, raw, err := s.current(ctx, p, id, thread, "edit")
	if err != nil {
		return Proposal{}, err
	}
	if current.kind != "canvas" || opened.Revision != base || len(selected) == 0 || len(selected) > canvas.MaxNodes+canvas.MaxEdges {
		return Proposal{}, ErrStale
	}
	scene, err := canvas.ParseScene(raw)
	if err != nil {
		return Proposal{}, ErrInvalid
	}
	known := map[string]bool{}
	for _, n := range scene.Facts.Nodes {
		known[n.ID] = true
	}
	for _, e := range scene.Facts.Edges {
		known[e.ID] = true
	}
	selection := map[string]bool{}
	for _, id := range selected {
		if !known[id] || selection[id] {
			return Proposal{}, ErrInvalid
		}
		selection[id] = true
	}
	for _, op := range operations {
		if !selection[op.ID] {
			return Proposal{}, ErrInvalid
		}
	}
	result, err := canvas.Apply(scene, operations)
	if err != nil || len(result.Diff) == 0 {
		return Proposal{}, ErrInvalid
	}
	candidate, err := json.Marshal(result.Scene)
	if err != nil {
		return Proposal{}, ErrInvalid
	}
	return s.storeProposal(current, base, candidate, Proposal{Kind: "canvas", FactsDigest: result.FactsDigest, SceneDiff: result.Diff})
}

// Image coordinates refer to natural pixels. Semantic Provider edits require
// the separate admitted media path; these finite operations are purely local.
func (s *Service) ProposeImage(ctx context.Context, p identity.PrincipalV1, id, thread, base string, operations []canvas.ImageOperation) (Proposal, error) {
	if s == nil {
		return Proposal{}, ErrUnavailable
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	current, opened, raw, err := s.current(ctx, p, id, thread, "edit")
	if err != nil {
		return Proposal{}, err
	}
	if current.kind != "png" || opened.Revision != base {
		return Proposal{}, ErrStale
	}
	result, err := canvas.TransformImage(raw, operations)
	if err != nil || result.Digest == opened.Revision || len(result.PNG) > 16<<20 {
		return Proposal{}, ErrInvalid
	}
	return s.storeProposal(current, base, result.PNG, Proposal{Kind: "png", ImageDiff: result.Diff})
}
func (s *Service) storeProposal(current *session, base string, candidate []byte, v Proposal) (Proposal, error) {
	if len(current.proposals) >= 16 {
		for id, p := range current.proposals {
			if p.view.Status == "applied" || p.view.Status == "rejected" {
				delete(current.proposals, id)
				break
			}
		}
	}
	if len(current.proposals) >= 16 {
		return Proposal{}, ErrUnavailable
	}
	// Bound retained image candidates across this session, not just per proposal.
	retained := len(candidate)
	for _, p := range current.proposals {
		retained += len(p.candidate)
	}
	if retained > 32<<20 {
		return Proposal{}, ErrUnavailable
	}
	global := len(candidate)
	for _, session := range s.sessions {
		for _, p := range session.proposals {
			global += len(p.candidate)
		}
	}
	if global > 64<<20 {
		return Proposal{}, ErrUnavailable
	}
	id, err := token()
	if err != nil {
		return Proposal{}, err
	}
	v.ID, v.BaseRevision, v.CandidateDigest, v.Status = id, base, digest(candidate), "proposed"
	encoded, err := json.Marshal(v)
	if err != nil || len(encoded) > 65536 {
		return Proposal{}, ErrInvalid
	}
	// Clone nested slices before returning a value to an in-process caller.
	var owned Proposal
	if json.Unmarshal(encoded, &owned) != nil {
		return Proposal{}, ErrInvalid
	}
	current.proposals[id] = &proposal{view: owned, candidate: append([]byte(nil), candidate...), changeID: digest([]byte(current.id + "\x00" + current.thread + "\x00" + base + "\x00" + id + "\x00" + v.CandidateDigest))}
	return v, nil
}
func (s *Service) Apply(ctx context.Context, p identity.PrincipalV1, id, thread, proposalID string) (files.Receipt, error) {
	if s == nil {
		return files.Receipt{}, ErrUnavailable
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	current, opened, _, err := s.current(ctx, p, id, thread, "edit")
	if err != nil {
		return files.Receipt{}, err
	}
	proposal := current.proposals[proposalID]
	if proposal == nil {
		return files.Receipt{}, ErrInvalid
	}
	objects := s.objects[current.kind]
	if proposal.view.Status == "applied" || proposal.view.Status == "pending" {
		receipt, err := objects.Status(ctx, current.object.SessionID, "native_save_"+proposal.changeID)
		if err == nil && receipt.Status == files.StatusCommitted {
			proposal.view.Status = "applied"
			proposal.candidate = nil
		}
		// A repeated click queries the original operation. Retrying a pending
		// replacement requires the separate explicit recovery operation.
		return receipt, err
	}
	if proposal.view.Status != "proposed" {
		return files.Receipt{}, ErrInvalid
	}
	if opened.Revision != proposal.view.BaseRevision {
		return files.Receipt{}, ErrStale
	}
	validate := func() error {
		if s.principal(ctx, p) != nil || s.projector.ValidateCurrent(ctx, s.authority(current, "edit")) != nil {
			return ErrUnavailable
		}
		return nil
	}
	release, err := s.capture(ctx, current.workspace, current.object.Path, validate)
	if err != nil || release == nil {
		return files.Receipt{}, ErrUnavailable
	}
	defer release()
	if err = validate(); err != nil {
		return files.Receipt{}, err
	}
	review, err := json.Marshal(proposal.view)
	if err != nil || len(review) > 65536 {
		return files.Receipt{}, ErrInvalid
	}
	draft := files.NativeChangeDraft{ChangeID: proposal.changeID, ThreadID: thread, ProposalID: proposalID, BaseRevision: proposal.view.BaseRevision, BeforeText: "Core original retained; source revision " + proposal.view.BaseRevision, AfterText: string(review)}
	prepared, err := objects.PrepareNativeChange(ctx, current.object.SessionID, draft)
	if err != nil {
		return files.Receipt{}, err
	}
	if err = validate(); err != nil {
		return files.Receipt{}, err
	}
	proposal.view.Status = "pending"
	receipt, err := objects.CommitNativeChange(ctx, current.object.SessionID, thread, proposal.changeID, prepared.SaveOperationID, proposal.view.BaseRevision, base64.StdEncoding.EncodeToString(proposal.candidate))
	if validate() != nil {
		return files.Receipt{OperationID: prepared.SaveOperationID, Status: files.StatusUnknown}, ErrUnavailable
	}
	if err == nil && receipt.Status == files.StatusCommitted {
		proposal.view.Status = "applied"
		proposal.candidate = nil
	}
	return receipt, err
}

func (s *Service) Recovery(ctx context.Context, p identity.PrincipalV1, id, thread string) (files.NativeRecovery, error) {
	if s == nil {
		return files.NativeRecovery{}, ErrUnavailable
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	current, _, _, err := s.current(ctx, p, id, thread, "discuss")
	if err != nil {
		return files.NativeRecovery{}, err
	}
	result, err := s.objects[current.kind].NativeRecovery(ctx, current.object.SessionID, thread)
	if s.principal(ctx, p) != nil || s.projector.ValidateCurrent(ctx, s.authority(current, "discuss")) != nil {
		return files.NativeRecovery{}, ErrUnavailable
	}
	return result, err
}

func (s *Service) Proposal(ctx context.Context, p identity.PrincipalV1, id, thread, proposalID string) (Proposal, error) {
	if s == nil {
		return Proposal{}, ErrUnavailable
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	current, _, _, err := s.current(ctx, p, id, thread, "discuss")
	if err != nil {
		return Proposal{}, err
	}
	item := current.proposals[proposalID]
	if item == nil {
		return Proposal{}, ErrInvalid
	}
	encoded, err := json.Marshal(item.view)
	if err != nil {
		return Proposal{}, ErrInvalid
	}
	var result Proposal
	if json.Unmarshal(encoded, &result) != nil {
		return Proposal{}, ErrInvalid
	}
	return result, nil
}

func (s *Service) Reject(ctx context.Context, p identity.PrincipalV1, id, thread, proposalID string) error {
	if s == nil {
		return ErrUnavailable
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	current, _, _, err := s.current(ctx, p, id, thread, "discuss")
	if err != nil {
		return err
	}
	item := current.proposals[proposalID]
	if item == nil || item.view.Status == "pending" || item.view.Status == "applied" {
		return ErrInvalid
	}
	item.view.Status = "rejected"
	item.candidate = nil
	return nil
}

func (s *Service) Cancel(ctx context.Context, p identity.PrincipalV1, id, thread, change, base string) (files.NativeRecovery, error) {
	if s == nil {
		return files.NativeRecovery{}, ErrUnavailable
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	current, opened, _, err := s.current(ctx, p, id, thread, "edit")
	if err != nil {
		return files.NativeRecovery{}, err
	}
	if opened.Revision != base {
		return files.NativeRecovery{}, ErrStale
	}
	recovery, err := s.objects[current.kind].CancelNativeChange(ctx, current.object.SessionID, thread, change, base)
	if s.principal(ctx, p) != nil || s.projector.ValidateCurrent(ctx, s.authority(current, "edit")) != nil {
		// Cancellation may already have settled; return no revoked review data
		// and require a fresh authorized recovery query for its actual outcome.
		return files.NativeRecovery{}, ErrUnavailable
	}
	if err != nil {
		return recovery, err
	}
	for _, item := range current.proposals {
		if item.changeID == change {
			item.view.Status = "rejected"
			item.candidate = nil
		}
	}
	return recovery, nil
}

func (s *Service) Close(ctx context.Context, p identity.PrincipalV1, id, thread string) error {
	if s == nil {
		return ErrUnavailable
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	current := s.sessions[id]
	if s.principal(ctx, p) != nil || current == nil || current.thread != thread || !identity.SamePrincipalV1(current.principal, p) {
		return ErrUnavailable
	}
	// Closing only releases in-memory ownership; it neither reads a damaged
	// file nor deletes durable originals, pending changes or recovery records.
	shared := false
	for otherID, other := range s.sessions {
		if otherID != id && other.kind == current.kind && other.object.SessionID == current.object.SessionID {
			shared = true
			break
		}
	}
	if !shared {
		if err := s.objects[current.kind].Close(ctx, current.object.SessionID); err != nil {
			return err
		}
	}
	delete(s.sessions, id)
	return nil
}
func (s *Service) RecoverOperation(ctx context.Context, p identity.PrincipalV1, id, thread, action, change, base string) (files.Receipt, error) {
	if s == nil {
		return files.Receipt{}, ErrUnavailable
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	current, opened, _, err := s.current(ctx, p, id, thread, "edit")
	if err != nil {
		return files.Receipt{}, err
	}
	if opened.Revision != base {
		return files.Receipt{}, ErrStale
	}
	validate := func() error {
		if s.principal(ctx, p) != nil || s.projector.ValidateCurrent(ctx, s.authority(current, "edit")) != nil {
			return ErrUnavailable
		}
		return nil
	}
	release, err := s.capture(ctx, current.workspace, current.object.Path, validate)
	if err != nil || release == nil {
		return files.Receipt{}, ErrUnavailable
	}
	defer release()
	if err = validate(); err != nil {
		return files.Receipt{}, err
	}
	var receipt files.Receipt
	switch action {
	case "undo":
		receipt, err = s.objects[current.kind].UndoNativeChange(ctx, current.object.SessionID, thread, change, base)
	case "resume":
		receipt, err = s.objects[current.kind].ResumeNativeChange(ctx, current.object.SessionID, thread, change, base)
	default:
		return files.Receipt{}, ErrInvalid
	}
	if validate() != nil {
		return files.Receipt{OperationID: receipt.OperationID, Status: files.StatusUnknown}, ErrUnavailable
	}
	return receipt, err
}
