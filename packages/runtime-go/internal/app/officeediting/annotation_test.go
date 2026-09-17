package officeediting

import (
	"context"
	"strings"
	"testing"

	fileport "analytix.local/runtime-go/internal/ports/objectediting"
)

type annotationFakeService struct {
	*fakeNativeEditing
	annotation      fileport.AnnotationDraft
	annotationCalls int
	onAnnotation    func()
}

func (f *annotationFakeService) ReadAnnotationDraft(_ context.Context, id, thread string) (fileport.AnnotationDraft, error) {
	f.annotationCalls++
	if f.onAnnotation != nil {
		f.onAnnotation()
	}
	out := f.annotation
	out.ObjectID = f.document.ObjectID
	out.ThreadID = thread
	return out, nil
}
func (f *annotationFakeService) WriteAnnotationDraft(ctx context.Context, id, thread, expected, note, source string) (fileport.AnnotationDraft, error) {
	f.annotation = fileport.AnnotationDraft{Note: note, SourceRevision: source, DraftRevision: strings.Repeat("e", 64), UpdatedAt: "2026-09-15T00:00:00Z"}
	return f.ReadAnnotationDraft(ctx, id, thread)
}

func TestAnnotationHostStrictProtectedLane(t *testing.T) {
	a, fake, projector, leases := selectionFixture(t)
	service := &annotationFakeService{fakeNativeEditing: a.service.(*fakeNativeEditing)}
	a.service = service
	ready, err := a.Readiness(context.Background(), validCall(t, "annotation-read", `{}`).Binding)
	if err != nil || len(ready.Operations) != 18 {
		t.Fatal("readiness", ready, err)
	}
	fields := map[string]any{"sessionId": fake.document.SessionID, "threadId": "thread-main", "expectedDraftRevision": "", "note": strings.Repeat("中", 4096), "sourceRevision": strings.Repeat("f", 64)}
	out := nativeInvoke(t, a, "annotation-write", fields)
	if out["ok"] != true || out["annotation"].(map[string]any)["note"] != fields["note"] || *leases != 0 || len(a.scopes) != 0 {
		t.Fatal("write", out)
	}
	for _, authority := range projector.seen {
		if authority.Purpose != "discuss" || authority.ThreadID != "thread-main" {
			t.Fatal("wrong authority", authority)
		}
	}
	for _, extra := range []string{"selectionToken", "token", "editable", "selection", "scopeId", "path", "objectId"} {
		fields[extra] = "forged"
		before := service.annotationCalls
		out := nativeInvoke(t, a, "annotation-write", fields)
		if out["ok"] != false || service.annotationCalls != before {
			t.Fatal("extra capability accepted", extra, out)
		}
		delete(fields, extra)
	}
	for _, note := range []any{nil, 123, strings.Repeat("😀", 2048) + "a"} {
		fields["note"] = note
		if out := nativeInvoke(t, a, "annotation-write", fields); out["ok"] != false {
			t.Fatal("invalid note", out)
		}
	}
	fields["note"] = strings.Repeat("😀", 2048)
	if out := nativeInvoke(t, a, "annotation-write", fields); out["ok"] != true {
		t.Fatal("emoji boundary", out)
	}
	read := map[string]any{"sessionId": fake.document.SessionID, "threadId": "thread-main"}
	if out := nativeInvoke(t, a, "annotation-read", read); out["ok"] != true {
		t.Fatal("read", out)
	}
	projector.invalid = true
	before := service.annotationCalls
	if out := nativeInvoke(t, a, "annotation-read", read); out["ok"] != false || out["annotation"] != nil || service.annotationCalls != before {
		t.Fatal("unauthorized read", out)
	}
}

func TestAnnotationHostLateRevocationDoesNotDisclose(t *testing.T) {
	for _, operation := range []string{"annotation-read", "annotation-write"} {
		a, fake, projector, _ := selectionFixture(t)
		service := &annotationFakeService{fakeNativeEditing: a.service.(*fakeNativeEditing), annotation: fileport.AnnotationDraft{Note: "private note"}}
		a.service = service
		service.onAnnotation = func() { projector.invalid = true }
		fields := map[string]any{"sessionId": fake.document.SessionID, "threadId": "thread-main"}
		if operation == "annotation-write" {
			fields["expectedDraftRevision"] = ""
			fields["note"] = "private note"
			fields["sourceRevision"] = strings.Repeat("f", 64)
		}
		if out := nativeInvoke(t, a, operation, fields); out["ok"] != false || out["annotation"] != nil {
			t.Fatal("late revocation", out)
		}
	}
}
