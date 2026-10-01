package objectediting

import (
	"context"
	"regexp"
	"unicode/utf8"
)

const MaxAnnotationNoteUTF16 = 4096
const MaxAnnotationNoteBytes = 4 * MaxAnnotationNoteUTF16
const MaxAnnotationRecordBytes = 64 << 10

var annotationThread = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)
var annotationRevision = regexp.MustCompile(`^[a-f0-9]{64}$`)

// AnnotationDraft is protected-local user text, never a selection capability.
// DraftRevision hashes the stored record; SourceRevision is informational only.
type AnnotationDraft struct {
	ObjectID       string `json:"objectId"`
	ThreadID       string `json:"threadId"`
	DraftRevision  string `json:"draftRevision"`
	Note           string `json:"note"`
	SourceRevision string `json:"sourceRevision"`
	UpdatedAt      string `json:"updatedAt"`
}

// AnnotationDraftTarget is constructed by Core from its current session.
type AnnotationDraftTarget struct {
	Workspace, Path, ObjectIdentity, ThreadID string
}

type AnnotationDraftWriteInput struct {
	AnnotationDraftTarget
	ExpectedDraftRevision, Note, SourceRevision string
}

type AnnotationDraftFiles interface {
	ReadAnnotationDraft(context.Context, AnnotationDraftTarget) (AnnotationDraft, error)
	WriteAnnotationDraft(context.Context, AnnotationDraftWriteInput) (AnnotationDraft, error)
}

func ValidAnnotationThread(thread string) bool { return annotationThread.MatchString(thread) }

func ValidAnnotationWrite(expected, note, source string) bool {
	if expected != "" && !annotationRevision.MatchString(expected) || !annotationRevision.MatchString(source) || len(note) > MaxAnnotationNoteBytes || !utf8.ValidString(note) {
		return false
	}
	units := 0
	for _, r := range note {
		units++
		if r > 0xffff {
			units++
		}
	}
	return units <= MaxAnnotationNoteUTF16
}
