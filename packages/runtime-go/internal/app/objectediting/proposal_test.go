package objectediting

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"unicode/utf16"

	identitydomain "analytix.local/runtime-go/internal/domain/identity"
	fileport "analytix.local/runtime-go/internal/ports/objectediting"
)

type testSelectionProjector struct {
	denied    bool
	calls     int
	onProject func()
	ranges    func(string) []ProtectedRange
}

func (p *testSelectionProjector) ValidateCurrent(_ context.Context, authority ScopeAuthority) error {
	if p.denied || authority.ThreadID != "thread-1" || authority.ObjectID == "" || identitydomain.ValidatePrincipalV1(authority.Principal) != nil {
		return errors.New("private authorization failure")
	}
	return nil
}
func (p *testSelectionProjector) AuthorizeAndProject(ctx context.Context, input ProjectionInput) ([]ProtectedRange, error) {
	p.calls++
	if p.onProject != nil {
		p.onProject()
	}
	if err := p.ValidateCurrent(ctx, input.ScopeAuthority); err != nil {
		return nil, err
	}
	if p.ranges != nil {
		return p.ranges(input.Text), nil
	}
	var ranges []ProtectedRange
	for _, value := range []string{"Alice", "6222020202020202"} {
		if start := strings.Index(input.Text, value); start >= 0 {
			ranges = append(ranges, ProtectedRange{start, start + len(value)})
		}
	}
	return ranges, nil
}
func utf16Length(text string) int { return len(utf16.Encode([]rune(text))) }
func proposalFixture(t *testing.T) (*Service, *testIdentity, *testFiles, *testSelectionProjector, Opened, Draft, ScopeBinding, Scope) {
	t.Helper()
	_, identity, files := fixture(t)
	files.doc.Content = "prefix Hello Alice paid 6222020202020202 😀 suffix"
	projector := &testSelectionProjector{}
	s := NewWithProjector(identity, files, projector)
	opened, err := s.Open(context.Background(), "/workspace", "file.md")
	if err != nil {
		t.Fatal(err)
	}
	draft, err := s.UpdateDraft(context.Background(), UpdateDraftInput{SessionID: opened.SessionID, BaseRevision: opened.Revision, Content: files.doc.Content})
	if err != nil {
		t.Fatal(err)
	}
	selection := UTF16Range{utf16Length("prefix "), utf16Length("prefix Hello Alice paid 6222020202020202 😀")}
	scope, err := s.CaptureScope(context.Background(), CaptureScopeInput{SessionID: opened.SessionID, DraftVersion: draft.Version, ThreadID: "thread-1", Purpose: "edit", Range: selection})
	if err != nil {
		t.Fatal(err)
	}
	binding := ScopeBinding{SessionID: opened.SessionID, ScopeID: scope.ScopeID, ThreadID: "thread-1", Purpose: "edit", DraftVersion: draft.Version}
	return s, identity, files, projector, opened, draft, binding, scope
}
func proposalReplacement(scope Scope) []PatchPart {
	var refs []PatchPart
	for _, part := range scope.Parts {
		if part.Kind == "protected" {
			refs = append(refs, part)
		}
	}
	return []PatchPart{{Kind: "literal", Text: "Welcome "}, refs[0], {Kind: "literal", Text: " received "}, refs[1], {Kind: "literal", Text: " 😀"}}
}
func mustPropose(t *testing.T, s *Service, binding ScopeBinding, scope Scope) Proposal {
	t.Helper()
	proposal, err := s.Propose(context.Background(), ProposeInput{binding, "propose-0001", proposalReplacement(scope)})
	if err != nil {
		t.Fatal(err)
	}
	return proposal
}

func TestProposalAcceptOnlyChangesWorkingCopyUntilExplicitCASSave(t *testing.T) {
	s, _, files, _, opened, draft, binding, scope := proposalFixture(t)
	ctx := context.Background()
	original := files.doc.Content
	proposal := mustPropose(t, s, binding, scope)
	if files.commits != 0 || files.doc.Content != original {
		t.Fatal("propose changed disk")
	}
	decision, err := s.Accept(ctx, DecisionInput{binding, "accept-0001", proposal.ProposalID})
	if err != nil || decision.Status != "accepted" || decision.DraftVersion == draft.Version {
		t.Fatalf("accept: %#v %v", decision, err)
	}
	current, err := s.ReadDraft(ctx, opened.SessionID)
	if err != nil || current.Content != "prefix Welcome Alice received 6222020202020202 😀 suffix" || current.BaseRevision != opened.Revision || current.Version != decision.DraftVersion {
		t.Fatalf("working-copy mismatch: %v", err)
	}
	if files.commits != 0 || files.doc.Content != original {
		t.Fatal("accept bypassed explicit save")
	}
	old, err := s.ReadScope(ctx, binding)
	if err != nil || old.Current {
		t.Fatalf("accepted scope remained editable: %v", err)
	}
	if len(s.edits[opened.SessionID].scopes[scope.ScopeID].spans) != 0 {
		t.Fatal("old epoch retained raw protected mapping")
	}
	fresh, err := s.CaptureScope(ctx, CaptureScopeInput{SessionID: opened.SessionID, DraftVersion: current.Version, ThreadID: "thread-1", Purpose: "edit", Range: UTF16Range{0, utf16Length(current.Content)}})
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := s.Commit(ctx, opened.SessionID, "save-explicit", current.BaseRevision, current.Content)
	if err != nil || receipt.Status != fileport.StatusCommitted || files.commits != 1 || files.input.BaseRevision != opened.Revision || files.doc.Content != current.Content {
		t.Fatalf("explicit CAS: %#v %v", receipt, err)
	}
	if s.edits[opened.SessionID].scopes[fresh.ScopeID].view.Current {
		t.Fatal("save did not invalidate editing scope")
	}
	saved, err := s.ReadDraft(ctx, opened.SessionID)
	if err != nil || saved.BaseRevision != receipt.Revision || saved.Version != current.Version {
		t.Fatal("disk base and draft version were conflated")
	}
}

func TestProposalOperationIdsBindPayloadAndHistoricalAcceptDoesNotOverwriteNewDraft(t *testing.T) {
	s, _, files, _, opened, _, binding, scope := proposalFixture(t)
	ctx := context.Background()
	input := ProposeInput{binding, "propose-0001", proposalReplacement(scope)}
	proposal, err := s.Propose(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	again, err := s.Propose(ctx, input)
	if err != nil || !reflect.DeepEqual(again, proposal) || len(s.edits[opened.SessionID].proposals) != 1 {
		t.Fatal("propose replay was not idempotent")
	}
	input.Parts[0].Text = "Different "
	if _, err := s.Propose(ctx, input); !errors.Is(err, fileport.ErrOperationMismatch) {
		t.Fatalf("changed payload reused ID: %v", err)
	}
	accept := DecisionInput{binding, "accept-0001", proposal.ProposalID}
	decision, err := s.Accept(ctx, accept)
	if err != nil {
		t.Fatal(err)
	}
	againDecision, err := s.Accept(ctx, accept)
	if err != nil || againDecision != decision {
		t.Fatalf("accept replay: %#v %v", againDecision, err)
	}
	current, _ := s.ReadDraft(ctx, opened.SessionID)
	updated, err := s.UpdateDraft(ctx, UpdateDraftInput{opened.SessionID, current.BaseRevision, current.Version, "Later local input"})
	if err != nil {
		t.Fatal(err)
	}
	againDecision, err = s.Accept(ctx, accept)
	if err != nil || againDecision != decision {
		t.Fatal("historical decision unavailable")
	}
	latest, _ := s.ReadDraft(ctx, opened.SessionID)
	if latest != updated || files.commits != 0 {
		t.Fatal("replay overwrote later draft or wrote disk")
	}
	accept.ProposalID = strings.Repeat("f", 48)
	if _, err := s.Accept(ctx, accept); !errors.Is(err, fileport.ErrOperationMismatch) {
		t.Fatalf("accept payload mismatch: %v", err)
	}
	if _, err := s.Reject(ctx, DecisionInput{binding, "accept-0001", proposal.ProposalID}); !errors.Is(err, fileport.ErrOperationMismatch) {
		t.Fatalf("cross-method ID reuse: %v", err)
	}
}

func TestProposalRejectDoesNotChangeDraftAndIsIdempotent(t *testing.T) {
	s, _, files, _, opened, draft, binding, scope := proposalFixture(t)
	proposal := mustPropose(t, s, binding, scope)
	input := DecisionInput{binding, "reject-0001", proposal.ProposalID}
	decision, err := s.Reject(context.Background(), input)
	if err != nil || decision.Status != "rejected" {
		t.Fatal(err)
	}
	again, err := s.Reject(context.Background(), input)
	if err != nil || again != decision {
		t.Fatal("reject replay changed")
	}
	current, _ := s.ReadDraft(context.Background(), opened.SessionID)
	if current != draft || files.commits != 0 {
		t.Fatal("reject changed original")
	}
	if _, err := s.Accept(context.Background(), DecisionInput{binding, "accept-0001", proposal.ProposalID}); !errors.Is(err, ErrProposal) {
		t.Fatalf("accepted rejected proposal: %v", err)
	}
}

func TestProposalScopeEpochThreadPurposeAndRevocation(t *testing.T) {
	s, _, _, _, opened, draft, binding, scope := proposalFixture(t)
	ctx := context.Background()
	proposal := mustPropose(t, s, binding, scope)
	for _, change := range []func(*ScopeBinding){func(b *ScopeBinding) { b.ThreadID = "thread-2" }, func(b *ScopeBinding) { b.Purpose = "discuss" }, func(b *ScopeBinding) { b.DraftVersion = strings.Repeat("f", 48) }, func(b *ScopeBinding) { b.ScopeID = strings.Repeat("f", 48) }} {
		bad := binding
		change(&bad)
		if _, err := s.ReadScope(ctx, bad); !errors.Is(err, ErrScope) {
			t.Fatalf("mismatched scope binding: %v", err)
		}
	}
	updated, err := s.UpdateDraft(ctx, UpdateDraftInput{opened.SessionID, draft.BaseRevision, draft.Version, draft.Content + " changed"})
	if err != nil {
		t.Fatal(err)
	}
	old, err := s.ReadScope(ctx, binding)
	if err != nil || old.Current {
		t.Fatal("old discussion projection was not retained as stale")
	}
	if _, err := s.Accept(ctx, DecisionInput{binding, "accept-stale", proposal.ProposalID}); !errors.Is(err, ErrDraftStale) {
		t.Fatalf("stale scope edited: %v", err)
	}
	fresh, err := s.CaptureScope(ctx, CaptureScopeInput{opened.SessionID, updated.Version, "thread-1", "edit", scope.Range})
	if err != nil {
		t.Fatal(err)
	}
	freshBinding := ScopeBinding{opened.SessionID, fresh.ScopeID, "thread-1", "edit", updated.Version}
	if _, err := s.Propose(ctx, ProposeInput{freshBinding, "propose-stale-token", proposalReplacement(scope)}); !errors.Is(err, ErrProtected) {
		t.Fatalf("old token accepted in new epoch: %v", err)
	}
	if err := s.RevokeScope(ctx, binding); err != nil {
		t.Fatal(err)
	}
	if err := s.RevokeScope(ctx, binding); err != nil {
		t.Fatal("revoke was not idempotent")
	}
	if _, err := s.ReadScope(ctx, binding); !errors.Is(err, ErrScope) {
		t.Fatal("revoked scope remained readable")
	}
	if _, err := s.Propose(ctx, ProposeInput{binding, "propose-0001", proposalReplacement(scope)}); !errors.Is(err, ErrScope) {
		t.Fatal("revocation bypassed by replay")
	}
	if err := s.Close(ctx, opened.SessionID); err != nil {
		t.Fatal(err)
	}
	if len(s.edits) != 0 {
		t.Fatal("close retained private proposals")
	}
	if _, err := s.ReadDraft(ctx, opened.SessionID); !errors.Is(err, ErrSession) {
		t.Fatal("closed draft accessible")
	}
}

func TestProposalProtectedReferencesAreOrderedExactAndCannotBeLiteralPollution(t *testing.T) {
	for _, mutation := range []string{"missing", "duplicate", "reorder", "foreign", "text_on_reference", "reference_on_literal", "placeholder", "split_placeholder", "raw_protected", "split_raw_protected", "pii"} {
		t.Run(mutation, func(t *testing.T) {
			s, _, files, _, _, _, binding, scope := proposalFixture(t)
			parts := proposalReplacement(scope)
			switch mutation {
			case "missing":
				parts = append(parts[:1], parts[2:]...)
			case "duplicate":
				parts = append(parts, parts[1])
			case "reorder":
				parts[1], parts[3] = parts[3], parts[1]
			case "foreign":
				parts[1].ProtectedRef = "protected_" + strings.Repeat("f", 48)
			case "text_on_reference":
				parts[1].Text = "forged"
			case "reference_on_literal":
				parts[0].ProtectedRef = parts[1].ProtectedRef
			case "placeholder":
				parts[0].Text = "PERSON_001 ACCOUNT_002"
			case "split_placeholder":
				parts = append([]PatchPart{{Kind: "literal", Text: "PER"}, {Kind: "literal", Text: "SON_001"}}, parts...)
			case "raw_protected":
				parts[0].Text = "Alice "
			case "split_raw_protected":
				parts = append([]PatchPart{{Kind: "literal", Text: "Al"}, {Kind: "literal", Text: "ice"}}, parts...)
			case "pii":
				parts[0].Text = "account 4111111111111111"
			}
			if _, err := s.Propose(context.Background(), ProposeInput{binding, "propose-bad-1", parts}); !errors.Is(err, ErrProtected) {
				t.Fatalf("protected mutation admitted: %v", err)
			}
			if files.commits != 0 {
				t.Fatal("invalid proposal wrote disk")
			}
		})
	}
}

func TestProposalUTF16RangeAndInvalidUnicodeBoundaries(t *testing.T) {
	for _, test := range []struct {
		selection UTF16Range
		valid     bool
	}{{UTF16Range{0, 4}, true}, {UTF16Range{1, 3}, true}, {UTF16Range{0, 0}, true}, {UTF16Range{1, 1}, true}, {UTF16Range{2, 2}, false}, {UTF16Range{1, 2}, false}, {UTF16Range{2, 3}, false}, {UTF16Range{-1, 1}, false}, {UTF16Range{3, 2}, false}, {UTF16Range{0, 5}, false}} {
		_, _, ok := utf16ByteRange("A😀B", test.selection)
		if ok != test.valid {
			t.Fatalf("range %#v validity %v", test.selection, ok)
		}
	}
	s, _, _, _, opened, draft, _, _ := proposalFixture(t)
	if _, err := s.UpdateDraft(context.Background(), UpdateDraftInput{opened.SessionID, draft.BaseRevision, draft.Version, string([]byte{0xff})}); !errors.Is(err, fileport.ErrInvalidInput) {
		t.Fatalf("invalid UTF-8: %v", err)
	}
	for _, selection := range []UTF16Range{{2, 2}, {1, 2}} {
		_, identity, files := fixture(t)
		files.doc.Content = "A😀B"
		service := NewWithProjector(identity, files, &testSelectionProjector{})
		doc, _ := service.Open(context.Background(), "/workspace", "file.md")
		working, _ := service.UpdateDraft(context.Background(), UpdateDraftInput{SessionID: doc.SessionID, BaseRevision: doc.Revision, Content: files.doc.Content})
		if _, err := service.CaptureScope(context.Background(), CaptureScopeInput{doc.SessionID, working.Version, "thread-1", "edit", selection}); !errors.Is(err, fileport.ErrInvalidInput) {
			t.Fatalf("surrogate boundary capture: %v", err)
		}
	}
}

func TestProposalIdentityAndThreadAuthorityAreRevalidated(t *testing.T) {
	s, identity, files, projector, opened, draft, binding, scope := proposalFixture(t)
	proposal := mustPropose(t, s, binding, scope)
	projector.denied = true
	if _, err := s.ReadScope(context.Background(), binding); !errors.Is(err, ErrProjection) {
		t.Fatal("revoked thread could read scope")
	}
	if _, err := s.Propose(context.Background(), ProposeInput{binding, "propose-0001", proposalReplacement(scope)}); !errors.Is(err, ErrProjection) {
		t.Fatal("revoked thread replayed proposal")
	}
	if _, err := s.Accept(context.Background(), DecisionInput{binding, "accept-0001", proposal.ProposalID}); !errors.Is(err, ErrProjection) {
		t.Fatal("revoked thread accepted")
	}
	current, _ := s.ReadDraft(context.Background(), opened.SessionID)
	if current != draft || files.commits != 0 {
		t.Fatal("authority failure changed data")
	}
	projector.denied = false
	identity.principal, _ = identitydomain.NewPrincipalV1(strings.Repeat("f", 64), "local", "local")
	if _, err := s.ReadDraft(context.Background(), opened.SessionID); !errors.Is(err, ErrSession) {
		t.Fatal("another principal read draft")
	}
	if _, err := s.ReadScope(context.Background(), binding); !errors.Is(err, ErrSession) {
		t.Fatal("another principal read scope")
	}
	if _, err := s.Accept(context.Background(), DecisionInput{binding, "accept-0001", proposal.ProposalID}); !errors.Is(err, ErrSession) {
		t.Fatal("another principal accepted")
	}
}

func TestProposalMissingMalformedOrRevokedProjectorFailsClosed(t *testing.T) {
	for _, test := range []string{"missing", "uncovered", "overlap", "out_of_bounds", "surrogate", "identity_changed"} {
		t.Run(test, func(t *testing.T) {
			_, identity, files := fixture(t)
			files.doc.Content = "Alice 6222020202020202 😀"
			projector := &testSelectionProjector{}
			s := NewWithProjector(identity, files, projector)
			if test == "missing" {
				s = New(identity, files)
			}
			opened, _ := s.Open(context.Background(), "/workspace", "file.md")
			draft, _ := s.UpdateDraft(context.Background(), UpdateDraftInput{SessionID: opened.SessionID, BaseRevision: opened.Revision, Content: files.doc.Content})
			switch test {
			case "uncovered":
				projector.ranges = func(string) []ProtectedRange { return nil }
			case "overlap":
				projector.ranges = func(string) []ProtectedRange { return []ProtectedRange{{0, 5}, {4, 8}} }
			case "out_of_bounds":
				projector.ranges = func(string) []ProtectedRange { return []ProtectedRange{{0, 999}} }
			case "surrogate":
				projector.ranges = func(text string) []ProtectedRange { return []ProtectedRange{{0, len(text) - 1}} }
			case "identity_changed":
				projector.onProject = func() { identity.unavailable = true }
			}
			_, err := s.CaptureScope(context.Background(), CaptureScopeInput{opened.SessionID, draft.Version, "thread-1", "edit", UTF16Range{0, utf16Length(draft.Content)}})
			if !errors.Is(err, ErrProjection) && !errors.Is(err, ErrUnavailable) {
				t.Fatalf("projector failure not closed: %v", err)
			}
			if len(s.edits[opened.SessionID].scopes) != 0 || files.commits != 0 {
				t.Fatal("failed projection retained scope or wrote disk")
			}
		})
	}
}

func TestProposalSameObjectReuseBoundsAndOtherCommitInvalidateScopes(t *testing.T) {
	s, _, files, _, opened, draft, binding, scope := proposalFixture(t)
	ctx := context.Background()
	reopened, err := s.Open(ctx, "/workspace", "file.md")
	if err != nil || reopened.SessionID != opened.SessionID {
		t.Fatal("object session was duplicated")
	}
	current, _ := s.ReadDraft(ctx, reopened.SessionID)
	if current != draft {
		t.Fatal("reopen reset working copy")
	}
	for count := 1; count < MaxScopesPerSession; count++ {
		if _, err := s.CaptureScope(ctx, CaptureScopeInput{opened.SessionID, draft.Version, "thread-1", "edit", scope.Range}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.CaptureScope(ctx, CaptureScopeInput{opened.SessionID, draft.Version, "thread-1", "edit", scope.Range}); !errors.Is(err, ErrCapacity) {
		t.Fatal("scope quota ignored")
	}
	if _, err := s.Commit(ctx, opened.SessionID, "other-save-01", draft.BaseRevision, "another working copy"); err != nil {
		t.Fatal(err)
	}
	stale, err := s.ReadScope(ctx, binding)
	if err != nil || stale.Current {
		t.Fatal("other commit left scope current")
	}
	current, _ = s.ReadDraft(ctx, opened.SessionID)
	if current.Content != draft.Content || current.BaseRevision != draft.BaseRevision || files.commits != 1 {
		t.Fatal("other commit silently rebased the working copy")
	}
	tooLarge := strings.Repeat("x", fileport.MaxTextBytes+1)
	if _, err := s.UpdateDraft(ctx, UpdateDraftInput{opened.SessionID, current.BaseRevision, current.Version, tooLarge}); !errors.Is(err, fileport.ErrInvalidInput) {
		t.Fatal("draft quota ignored")
	}
}

func TestProposalMaximumProtectedSpanProjectionCanBePreserved(t *testing.T) {
	_, identity, files := fixture(t)
	files.doc.Content = " " + strings.Repeat("X ", MaxProtectedSpans)
	projector := &testSelectionProjector{ranges: func(string) []ProtectedRange {
		var spans []ProtectedRange
		for index := 0; index < MaxProtectedSpans; index++ {
			spans = append(spans, ProtectedRange{1 + index*2, 2 + index*2})
		}
		return spans
	}}
	s := NewWithProjector(identity, files, projector)
	ctx := context.Background()
	opened, _ := s.Open(ctx, "/workspace", "file.md")
	draft, err := s.UpdateDraft(ctx, UpdateDraftInput{SessionID: opened.SessionID, BaseRevision: opened.Revision, Content: files.doc.Content})
	if err != nil {
		t.Fatal(err)
	}
	scope, err := s.CaptureScope(ctx, CaptureScopeInput{opened.SessionID, draft.Version, "thread-1", "edit", UTF16Range{0, utf16Length(draft.Content)}})
	if err != nil || len(scope.Parts) != MaxProtectedSpans*2+1 {
		t.Fatalf("maximum protected scope: %v", err)
	}
	binding := ScopeBinding{opened.SessionID, scope.ScopeID, "thread-1", "edit", draft.Version}
	if _, err := s.Propose(ctx, ProposeInput{binding, "propose-max-spans", scope.Parts}); err != nil {
		t.Fatalf("valid maximum scope cannot be preserved: %v", err)
	}
	if _, err := s.Propose(ctx, ProposeInput{binding, "propose-over-limit", make([]PatchPart, MaxPatchParts+1)}); !errors.Is(err, fileport.ErrInvalidInput) {
		t.Fatal("part count limit ignored")
	}
}

func TestProposalDiscussionScopeCannotAuthorizeModification(t *testing.T) {
	s, _, files, _, opened, draft, _, scope := proposalFixture(t)
	ctx := context.Background()
	discussion, err := s.CaptureScope(ctx, CaptureScopeInput{opened.SessionID, draft.Version, "thread-1", "discuss", scope.Range})
	if err != nil {
		t.Fatal(err)
	}
	binding := ScopeBinding{opened.SessionID, discussion.ScopeID, "thread-1", "discuss", draft.Version}
	if _, err := s.Propose(ctx, ProposeInput{binding, "propose-discussion", discussion.Parts}); !errors.Is(err, ErrScope) {
		t.Fatalf("discussion scope edited: %v", err)
	}
	if files.commits != 0 {
		t.Fatal("discussion wrote disk")
	}
}

func TestProposalRecoveredCASStatusAdvancesMatchingDraftBaseOnce(t *testing.T) {
	s, _, files, _, opened, draft, binding, scope := proposalFixture(t)
	ctx := context.Background()
	// A prior unknown save is recovered by the Files journal. Its content is
	// already on disk; this status read must not write it or rebase a different draft.
	files.doc.Revision = files.receipt.Revision
	files.doc.Content = draft.Content
	receipt, err := s.Status(ctx, opened.SessionID, files.receipt.OperationID)
	if err != nil || receipt.Status != fileport.StatusCommitted {
		t.Fatal(err)
	}
	current, _ := s.ReadDraft(ctx, opened.SessionID)
	if current.BaseRevision != receipt.Revision || current.Version != draft.Version || files.commits != 0 {
		t.Fatal("recovered confirmation did not bind the matching working copy")
	}
	stale, _ := s.ReadScope(ctx, binding)
	if stale.Current {
		t.Fatal("recovered commit left old edit scope current")
	}
	fresh, err := s.CaptureScope(ctx, CaptureScopeInput{opened.SessionID, current.Version, "thread-1", "edit", scope.Range})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Status(ctx, opened.SessionID, files.receipt.OperationID); err != nil {
		t.Fatal(err)
	}
	if !s.edits[opened.SessionID].scopes[fresh.ScopeID].view.Current {
		t.Fatal("same confirmation replay invalidated a new scope")
	}
}
