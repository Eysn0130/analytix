package objectediting

import (
	"context"
	"errors"
	"strings"
	"testing"

	fileport "analytix.local/runtime-go/internal/ports/objectediting"
)

type workbookCandidateTestFiles struct {
	*nativeRecoveryTestFiles
	input      fileport.NativeWorkbookCandidateInput
	calls      int
	afterBuild func()
}

func (f *workbookCandidateTestFiles) BuildNativeWorkbookCandidate(_ context.Context, input fileport.NativeWorkbookCandidateInput) (fileport.NativeWorkbookCandidate, error) {
	f.calls++
	f.input = input
	if f.afterBuild != nil {
		f.afterBuild()
	}
	return fileport.NativeWorkbookCandidate{ChangeID: input.ChangeID, SaveOperationID: input.OperationID, BaseRevision: input.BaseRevision}, nil
}

func TestWorkbookCandidateServiceDerivesFileAuthorityAndRechecksPrincipal(t *testing.T) {
	_, identity, files, _ := nativeRecoveryServiceFixture(t)
	store := &workbookCandidateTestFiles{nativeRecoveryTestFiles: files}
	service := New(identity, store)
	opened, err := service.Open(context.Background(), "/workspace", "file.md")
	if err != nil {
		t.Fatal(err)
	}
	change, operation := strings.Repeat("e", 64), "native_save_"+strings.Repeat("e", 64)
	if _, err = service.BuildNativeWorkbookCandidate(context.Background(), opened.SessionID, "thread_main", change, operation, opened.Revision); err != nil {
		t.Fatal(err)
	}
	if store.input.Workspace != "/workspace" || store.input.Path != opened.Path || store.input.ObjectIdentity != opened.ObjectID || store.input.ThreadID != "thread_main" || store.input.ChangeID != change || store.input.OperationID != operation || store.input.BaseRevision != opened.Revision {
		t.Fatal("candidate builder received caller-independent file authority incorrectly", store.input)
	}
	store.afterBuild = func() { identity.unavailable = true }
	if candidate, err := service.BuildNativeWorkbookCandidate(context.Background(), opened.SessionID, "thread_main", change, operation, opened.Revision); !errors.Is(err, ErrUnavailable) || candidate.ChangeID != "" {
		t.Fatal("revoked principal received candidate", candidate, err)
	}
	identity.unavailable = false
	before := store.calls
	if _, err := service.BuildNativeWorkbookCandidate(context.Background(), "missing_session", "thread_main", change, operation, opened.Revision); err == nil || store.calls != before {
		t.Fatal("unknown session reached candidate store", err)
	}
}
