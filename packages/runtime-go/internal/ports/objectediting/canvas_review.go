package objectediting

import (
	"context"
	"encoding/json"
	"regexp"

	canvas "analytix.local/runtime-go/internal/domain/canvas"
)

const MaxCanvasReviewIntents = 16
const MaxCanvasReviewRecordBytes = 1 << 20

var canvasReviewID = regexp.MustCompile(`^[a-f0-9]{48}$`)

// CanvasReviewIntent is protected-local review data, not an accepted change or
// an executable grant. It never contains selection handles, aliases or Provider
// context. Reopening must mint new proposal IDs, reauthorize, reread the source
// and rederive the candidate before explicit acceptance can commit it.
type CanvasReviewIntent struct {
	ID              string `json:"id"`
	Kind            string `json:"kind"`
	BaseRevision    string `json:"baseRevision"`
	CandidateDigest string `json:"candidateDigest"`
	// A fingerprint can invalidate old intent after epoch/plugin changes. It
	// cannot grant authority: the current trusted projector and Host still decide.
	ContextFingerprint string                  `json:"contextFingerprint"`
	SelectedIDs        []string                `json:"selectedIds,omitempty"`
	SceneOperations    []canvas.Operation      `json:"sceneOperations,omitempty"`
	ImageOperations    []canvas.ImageOperation `json:"imageOperations,omitempty"`
	Review             json.RawMessage         `json:"review"`
}
type CanvasReviewSnapshot struct {
	ObjectID string               `json:"objectId"`
	ThreadID string               `json:"threadId"`
	Revision string               `json:"revision"`
	Intents  []CanvasReviewIntent `json:"intents"`
}

// Both review records and annotation drafts share the existing Core-derived
// object/thread target, receipt root, private-file policy and CAS owner.
type CanvasReviewTarget = AnnotationDraftTarget
type CanvasReviewWriteInput struct {
	CanvasReviewTarget
	ExpectedRevision string
	Intents          []CanvasReviewIntent
}
type CanvasReviewFiles interface {
	ReadCanvasReview(context.Context, CanvasReviewTarget) (CanvasReviewSnapshot, error)
	WriteCanvasReview(context.Context, CanvasReviewWriteInput) (CanvasReviewSnapshot, error)
}

func ValidCanvasReview(expected string, intents []CanvasReviewIntent) bool {
	if expected != "" && !annotationRevision.MatchString(expected) || intents == nil || len(intents) > MaxCanvasReviewIntents {
		return false
	}
	seen := map[string]bool{}
	for _, v := range intents {
		if !canvasReviewID.MatchString(v.ID) || seen[v.ID] || !annotationRevision.MatchString(v.BaseRevision) || !annotationRevision.MatchString(v.CandidateDigest) || !annotationRevision.MatchString(v.ContextFingerprint) || len(v.Review) == 0 || len(v.Review) > 65536 || !json.Valid(v.Review) {
			return false
		}
		seen[v.ID] = true
		if v.Kind == "canvas" {
			if len(v.SelectedIDs) == 0 || len(v.SelectedIDs) > canvas.MaxNodes+canvas.MaxEdges || len(v.ImageOperations) != 0 {
				return false
			}
			selected := map[string]bool{}
			for _, id := range v.SelectedIDs {
				if !annotationThread.MatchString(id) || selected[id] {
					return false
				}
				selected[id] = true
			}
			raw, err := json.Marshal(v.SceneOperations)
			if err != nil {
				return false
			}
			ops, err := canvas.ParseOperations(raw)
			if err != nil {
				return false
			}
			for _, op := range ops {
				if !selected[op.ID] {
					return false
				}
			}
		} else if v.Kind == "png" {
			if len(v.SelectedIDs) != 0 || len(v.SceneOperations) != 0 {
				return false
			}
			raw, err := json.Marshal(v.ImageOperations)
			if err != nil {
				return false
			}
			if _, err = canvas.ParseImageOperations(raw); err != nil {
				return false
			}
		} else {
			return false
		}
	}
	raw, err := json.Marshal(intents)
	return err == nil && len(raw) < MaxCanvasReviewRecordBytes-4096
}
