package canvasediting

import (
	"bytes"
	"context"
	"encoding/json"
	"sort"

	objectapp "analytix.local/runtime-go/internal/app/objectediting"
	canvas "analytix.local/runtime-go/internal/domain/canvas"
	identity "analytix.local/runtime-go/internal/domain/identity"
	files "analytix.local/runtime-go/internal/ports/objectediting"
)

type reviewObjects interface {
	ReadCanvasReview(context.Context, string, string) (files.CanvasReviewSnapshot, error)
	WriteCanvasReview(context.Context, string, string, string, []files.CanvasReviewIntent) (files.CanvasReviewSnapshot, error)
}

func reviewFingerprint(current *session) string {
	// This is evidence used to reject incompatible intent, never a persisted grant.
	raw, _ := json.Marshal(struct {
		Security string
		Binding  any
	}{current.securityBinding, current.binding})
	return digest(raw)
}
func reviewChangeID(current *session, intent files.CanvasReviewIntent) string {
	return digest([]byte(current.object.ObjectID + "\x00" + current.thread + "\x00" + intent.BaseRevision + "\x00" + intent.ID + "\x00" + intent.CandidateDigest))
}
func (s *Service) writeReviews(ctx context.Context, current *session, intents []files.CanvasReviewIntent) error {
	store, ok := s.objects[current.kind].(reviewObjects)
	if !ok {
		return nil
	} // Legacy isolated component fixtures, not production composition.
	result, err := store.WriteCanvasReview(ctx, current.object.SessionID, current.thread, current.reviewRevision, intents)
	if err != nil {
		return err
	}
	if s.principal(ctx, current.principal) != nil || s.projector.ValidateCurrent(ctx, s.authority(current, "discuss")) != nil {
		return ErrUnavailable
	}
	current.reviewRevision, current.reviews = result.Revision, result.Intents
	return nil
}
func (s *Service) removeReview(ctx context.Context, current *session, id string) error {
	if id == "" {
		return nil
	}
	next := []files.CanvasReviewIntent{}
	found := false
	for _, intent := range current.reviews {
		if intent.ID == id {
			found = true
		} else {
			next = append(next, intent)
		}
	}
	if !found {
		return nil
	}
	return s.writeReviews(ctx, current, next)
}
func (s *Service) appendReview(ctx context.Context, current *session, v Proposal, selected []string, sceneOps []canvas.Operation, imageOps []canvas.ImageOperation) (string, string, error) {
	store, ok := s.objects[current.kind].(reviewObjects)
	if !ok {
		return "", digest([]byte(current.id + "\x00" + current.thread + "\x00" + v.BaseRevision + "\x00" + v.ID + "\x00" + v.CandidateDigest)), nil
	}
	// Read does not adopt another writer's successor. A changed revision requires
	// reopening and reviewing that state, never an implicit merge of intentions.
	snapshot, err := store.ReadCanvasReview(ctx, current.object.SessionID, current.thread)
	if err != nil {
		return "", "", err
	}
	if snapshot.Revision != current.reviewRevision {
		return "", "", files.ErrConflict
	}
	id, err := selectionToken()
	if err != nil {
		return "", "", err
	}
	body, err := json.Marshal(v)
	if err != nil {
		return "", "", ErrInvalid
	}
	intent := files.CanvasReviewIntent{ID: id, Kind: v.Kind, BaseRevision: v.BaseRevision, CandidateDigest: v.CandidateDigest, ContextFingerprint: reviewFingerprint(current), SelectedIDs: selected, SceneOperations: sceneOps, ImageOperations: imageOps, Review: body}
	// Deep-clone caller-owned operations and nested slices before persistence.
	encoded, err := json.Marshal(intent)
	if err != nil {
		return "", "", ErrInvalid
	}
	if json.Unmarshal(encoded, &intent) != nil {
		return "", "", ErrInvalid
	}
	retired := map[string]bool{}
	for _, p := range current.proposals {
		if p.view.Status == "applied" || p.view.Status == "rejected" {
			retired[p.reviewID] = true
		}
	}
	next := []files.CanvasReviewIntent{}
	for _, old := range current.reviews {
		if !retired[old.ID] {
			next = append(next, old)
		}
	}
	next = append(next, intent)
	if !files.ValidCanvasReview(current.reviewRevision, next) {
		return "", "", ErrInvalid
	}
	if err = s.writeReviews(ctx, current, next); err != nil {
		return "", "", err
	}
	return id, reviewChangeID(current, intent), nil
}

func (s *Service) loadReviews(ctx context.Context, current *session, opened objectapp.Opened, raw []byte) error {
	store, ok := s.objects[current.kind].(reviewObjects)
	if !ok {
		return nil
	}
	snapshot, err := store.ReadCanvasReview(ctx, current.object.SessionID, current.thread)
	if err != nil {
		return err
	}
	if !files.ValidCanvasReview(snapshot.Revision, snapshot.Intents) {
		return files.ErrPersistence
	}
	current.reviewRevision, current.reviews = snapshot.Revision, snapshot.Intents
	if len(snapshot.Intents) == 0 {
		return nil
	}
	recovery, err := s.objects[current.kind].NativeRecovery(ctx, current.object.SessionID, current.thread)
	if err != nil {
		return err
	}
	retained := 0
	for _, intent := range snapshot.Intents {
		var v Proposal
		if json.Unmarshal(intent.Review, &v) != nil {
			return files.ErrPersistence
		}
		canonical, err := json.Marshal(v)
		if err != nil || !bytes.Equal(canonical, intent.Review) || !sessionPattern.MatchString(v.ID) || v.Status != "proposed" || v.Kind != current.kind || v.Kind != intent.Kind || v.BaseRevision != intent.BaseRevision || v.CandidateDigest != intent.CandidateDigest {
			return files.ErrPersistence
		}
		change := reviewChangeID(current, intent)
		// Old proposal IDs and captured selectors are not revived. An explicitly
		// accepted pending operation keeps its original change/operation identities.
		v.ID, err = selectionToken()
		if err != nil {
			return err
		}
		var candidate []byte
		if recovery.Pending != nil && recovery.Pending.ChangeID == change {
			v.Status = "pending"
		} else if recovery.Current != nil && recovery.Current.ChangeID == change {
			switch recovery.Current.Status {
			case "committed":
				v.Status = "applied"
			case "undone", "cancelled":
				v.Status = "rejected"
			default:
				v.Status = "pending"
			}
		} else if opened.Revision != intent.BaseRevision || intent.ContextFingerprint != reviewFingerprint(current) {
			v.Status = "stale"
		} else {
			// Recompute from current, revision-verified bytes. Stored diffs and hashes
			// are checked against the real transformation, not treated as authority.
			if intent.Kind == "canvas" {
				scene, e := canvas.ParseScene(raw)
				if e != nil {
					return files.ErrPersistence
				}
				changed, e := canvas.Apply(scene, intent.SceneOperations)
				if e != nil {
					return files.ErrPersistence
				}
				a, _ := json.Marshal(changed.Diff)
				b, _ := json.Marshal(v.SceneDiff)
				if changed.FactsDigest != v.FactsDigest || !bytes.Equal(a, b) {
					return files.ErrPersistence
				}
				candidate, err = json.Marshal(changed.Scene)
			} else {
				changed, e := canvas.TransformImage(raw, intent.ImageOperations)
				if e != nil {
					return files.ErrPersistence
				}
				a, _ := json.Marshal(changed.Diff)
				b, _ := json.Marshal(v.ImageDiff)
				if !bytes.Equal(a, b) {
					return files.ErrPersistence
				}
				candidate = changed.PNG
			}
			if err != nil || digest(candidate) != intent.CandidateDigest {
				return files.ErrPersistence
			}
		}
		retained += len(candidate)
		if retained > 32<<20 {
			return ErrUnavailable
		}
		current.proposals[v.ID] = &proposal{view: v, candidate: candidate, changeID: change, reviewID: intent.ID}
	}
	global := retained
	for _, other := range s.sessions {
		for _, p := range other.proposals {
			global += len(p.candidate)
		}
	}
	if global > 64<<20 {
		return ErrUnavailable
	}
	if s.principal(ctx, current.principal) != nil || s.projector.ValidateCurrent(ctx, s.authority(current, "discuss")) != nil {
		return ErrUnavailable
	}
	return nil
}

func (s *Service) reviewCurrent(ctx context.Context, current *session, item *proposal) error {
	store, ok := s.objects[current.kind].(reviewObjects)
	if !ok {
		return nil
	}
	snapshot, err := store.ReadCanvasReview(ctx, current.object.SessionID, current.thread)
	if err != nil {
		return err
	}
	if snapshot.Revision != current.reviewRevision {
		return files.ErrConflict
	}
	for _, intent := range snapshot.Intents {
		if intent.ID == item.reviewID {
			if intent.ContextFingerprint != reviewFingerprint(current) || reviewChangeID(current, intent) != item.changeID || intent.CandidateDigest != item.view.CandidateDigest {
				return ErrStale
			}
			return nil
		}
	}
	return ErrStale
}

func (s *Service) Proposals(ctx context.Context, p identity.PrincipalV1, id, thread string) ([]Proposal, error) {
	if s == nil {
		return nil, ErrUnavailable
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	current, opened, _, err := s.current(ctx, p, id, thread, "discuss")
	if err != nil {
		return nil, err
	}
	result := []Proposal{}
	ids := []string{}
	for id := range current.proposals {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		v := current.proposals[id].view
		if v.Status == "proposed" && v.BaseRevision != opened.Revision {
			v.Status = "stale"
		}
		body, e := json.Marshal(v)
		if e != nil {
			return nil, ErrInvalid
		}
		var cloned Proposal
		if json.Unmarshal(body, &cloned) != nil {
			return nil, ErrInvalid
		}
		result = append(result, cloned)
	}
	return result, nil
}

// ProposalStatus is genuinely read-only: no Apply, prepare, resume or journal
// write, even when an earlier request has an unknown result. It queries only
// the original operation and rechecks authority after the storage read.
func (s *Service) ProposalStatus(ctx context.Context, p identity.PrincipalV1, id, thread, proposalID string) (files.Receipt, error) {
	if s == nil {
		return files.Receipt{}, ErrUnavailable
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	current, _, _, err := s.current(ctx, p, id, thread, "edit")
	if err != nil {
		return files.Receipt{}, err
	}
	item := current.proposals[proposalID]
	if item == nil || item.view.Status != "pending" && item.view.Status != "applied" && item.view.Status != "proposed" {
		return files.Receipt{}, ErrInvalid
	}
	operation := "native_save_" + item.changeID
	receipt, err := s.objects[current.kind].Status(ctx, current.object.SessionID, operation)
	if s.principal(ctx, p) != nil || s.projector.ValidateCurrent(ctx, s.authority(current, "edit")) != nil {
		return files.Receipt{OperationID: operation, Status: files.StatusUnknown}, ErrUnavailable
	}
	if err == nil && receipt.Status == files.StatusCommitted {
		item.view.Status = "applied"
		item.candidate = nil
		releaseSelections(current)
	}
	return receipt, err
}
