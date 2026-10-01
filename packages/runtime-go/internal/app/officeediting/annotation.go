package officeediting

import (
	"context"

	fileport "analytix.local/runtime-go/internal/ports/objectediting"
	adapterport "analytix.local/runtime-go/internal/ports/pluginpackagehost"
)

type AnnotationDraftService interface {
	ReadAnnotationDraft(context.Context, string, string) (fileport.AnnotationDraft, error)
	WriteAnnotationDraft(context.Context, string, string, string, string, string) (fileport.AnnotationDraft, error)
}

func (a *Adapter) invokeAnnotation(ctx context.Context, call adapterport.Call, input map[string]any) (adapterport.Result, error) {
	service, ok := a.service.(AnnotationDraftService)
	if !ok {
		return failure(ErrUnavailable)
	}
	id, _ := input["sessionId"].(string)
	thread, _ := input["threadId"].(string)
	if !sessionPattern.MatchString(id) {
		return failure(fileport.ErrInvalidInput)
	}
	if err := a.validateRecoveryThread(ctx, call, id, thread, "discuss"); err != nil {
		return failure(err)
	}
	var draft fileport.AnnotationDraft
	var err error
	switch call.Operation {
	case "annotation-read":
		if !exactKeys(input, "sessionId", "threadId") {
			return failure(fileport.ErrInvalidInput)
		}
		draft, err = service.ReadAnnotationDraft(ctx, id, thread)
	case "annotation-write":
		expected, expectedOK := input["expectedDraftRevision"].(string)
		note, noteOK := input["note"].(string)
		source, sourceOK := input["sourceRevision"].(string)
		if !exactKeys(input, "sessionId", "threadId", "expectedDraftRevision", "note", "sourceRevision") || !expectedOK || !noteOK || !sourceOK || !fileport.ValidAnnotationWrite(expected, note, source) {
			return failure(fileport.ErrInvalidInput)
		}
		draft, err = service.WriteAnnotationDraft(ctx, id, thread, expected, note, source)
	default:
		return failure(fileport.ErrInvalidInput)
	}
	if err != nil {
		return failure(err)
	}
	// A delayed read must not disclose text after its thread authority changes.
	if err := a.validateRecoveryThread(ctx, call, id, thread, "discuss"); err != nil {
		return failure(err)
	}
	if draft.ObjectID != a.sessions[id].document.ObjectID || draft.ThreadID != thread {
		return failure(ErrUnavailable)
	}
	return output(map[string]any{"ok": true, "annotation": draft})
}
