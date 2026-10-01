package thread

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	casethreadapp "analytix.local/runtime-go/internal/app/casethread"
	appcontextepoch "analytix.local/runtime-go/internal/app/contextepoch"
	gateprojection "analytix.local/runtime-go/internal/app/gateprojection"
	"analytix.local/runtime-go/internal/contracts"
	domaincontextepoch "analytix.local/runtime-go/internal/domain/contextepoch"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

type currentCaseAuthorityStub struct {
	context     domainsecurity.TurnSecurityContext
	restartHeld bool
}

func (stub currentCaseAuthorityStub) IsCaseThread(threadID string) bool {
	return strings.TrimSpace(threadID) == stub.context.ThreadID
}

func (stub currentCaseAuthorityStub) ContainsContext(securityContext domainsecurity.TurnSecurityContext) bool {
	return securityContext == stub.context
}

func (stub currentCaseAuthorityStub) ContextTurnIDs(threadID string) []string {
	if strings.TrimSpace(threadID) != stub.context.ThreadID {
		return nil
	}
	return []string{stub.context.TurnID}
}

type currentCaseInventoryStub struct {
	threadID string
	contexts []domainsecurity.TurnSecurityContext
}

func (stub currentCaseInventoryStub) IsCaseThread(threadID string) bool {
	return strings.TrimSpace(threadID) == stub.threadID
}

func (stub currentCaseInventoryStub) ContainsContext(securityContext domainsecurity.TurnSecurityContext) bool {
	for _, candidate := range stub.contexts {
		if candidate == securityContext {
			return true
		}
	}
	return false
}

func (stub currentCaseInventoryStub) ContextTurnIDs(threadID string) []string {
	if strings.TrimSpace(threadID) != stub.threadID {
		return nil
	}
	turnIDs := make([]string, 0, len(stub.contexts))
	for _, securityContext := range stub.contexts {
		turnIDs = append(turnIDs, securityContext.TurnID)
	}
	return turnIDs
}

type currentBindingReaderStub struct {
	workspace string
	binding   domainsecurity.CaseBinding
	err       error
}

func (stub currentBindingReaderStub) ReadCurrentBinding(workspace string) (domainsecurity.CaseBinding, error) {
	if stub.workspace != "" && strings.TrimSpace(workspace) != stub.workspace {
		return domainsecurity.CaseBinding{}, errors.New("unexpected workspace")
	}
	return stub.binding, stub.err
}

type currentBindingObserverStub struct {
	currentBindingReaderStub
	observation domainsecurity.CaseBindingObservationV1
	observeErr  error
}

func (stub currentBindingObserverStub) Observe(string) (domainsecurity.CaseBindingObservationV1, error) {
	return stub.observation, stub.observeErr
}

type currentSnapshotValidatorStub struct{ err error }

func (stub currentSnapshotValidatorStub) ValidateCurrentSnapshot(domainsecurity.TurnSecurityContext) error {
	return stub.err
}

func TestCurrentCaseThreadAuthorityRequiresSignedBindingAndAcceptedEpoch(t *testing.T) {
	securityContext := newTrustedProjectionContext("thread-live", "turn-live", "case-live", "snapshot-live", 7)
	authority := currentCaseAuthorityStub{context: securityContext}
	bindingReader := currentBindingReaderStub{
		workspace: securityContext.WorkspaceRealPath,
		binding: domainsecurity.CaseBinding{
			CaseID: securityContext.CaseID, WorkspaceRealPath: securityContext.WorkspaceRealPath,
			CaseBindingHash: securityContext.CaseBindingHash,
		},
	}
	base := currentAuthorityThread(t, securityContext)
	validator := NewCurrentCaseThreadAuthorityValidator(authority, bindingReader, nil)
	if current, err := validator.ValidateCurrent(securityContext.ThreadID, base); err != nil || current != securityContext {
		t.Fatalf("valid live case authority was rejected: current=%#v err=%v", current, err)
	}

	tests := map[string]struct {
		thread   map[string]any
		bindings currentBindingReaderStub
	}{
		"missing signed security state": {
			thread: mutateCurrentAuthorityThread(base, func(thread map[string]any) { delete(thread, "securityState") }), bindings: bindingReader,
		},
		"unregistered security state": {
			thread: mutateCurrentAuthorityThread(base, func(thread map[string]any) {
				unsigned := domainsecurity.NewTurnSecurityContext(domainsecurity.TurnSecurityContextInput{
					ThreadID: securityContext.ThreadID, TurnID: "turn-forged", WorkspaceRealPath: securityContext.WorkspaceRealPath,
					CaseID: securityContext.CaseID, CaseBindingHash: securityContext.CaseBindingHash,
					DatasetSnapshotID: securityContext.DatasetSnapshotID, SourceManifestHash: securityContext.SourceManifestHash,
					ContextEpoch: securityContext.ContextEpoch, IssuedAt: time.Date(2026, 7, 11, 8, 1, 0, 0, time.UTC),
				})
				thread["securityState"] = publicProjectionSecurityRecord(unsigned)
			}), bindings: bindingReader,
		},
		"missing accepted epoch": {
			thread: mutateCurrentAuthorityThread(base, func(thread map[string]any) { delete(thread, "contextEpochState") }), bindings: bindingReader,
		},
		"accepted epoch number mismatch": {
			thread: mutateCurrentAuthorityThread(base, func(thread map[string]any) {
				thread["contextEpochState"] = currentAuthorityEpochState(t, securityContext, securityContext.ContextEpoch+1)
			}), bindings: bindingReader,
		},
		"accepted security binding digest mismatch": {
			thread: mutateCurrentAuthorityThread(base, func(thread map[string]any) {
				mismatch := domainsecurity.NewTurnSecurityContext(domainsecurity.TurnSecurityContextInput{
					ThreadID: securityContext.ThreadID, TurnID: securityContext.TurnID, WorkspaceRealPath: securityContext.WorkspaceRealPath,
					CaseID: "case-other", CaseBindingHash: domainsecurity.SHA256Hex([]byte("binding-other")),
					DatasetSnapshotID: securityContext.DatasetSnapshotID, SourceManifestHash: securityContext.SourceManifestHash,
					ContextEpoch: securityContext.ContextEpoch, IssuedAt: time.Date(2026, 7, 11, 7, 59, 0, 0, time.UTC),
				})
				thread["contextEpochState"] = currentAuthorityEpochState(t, mismatch, securityContext.ContextEpoch)
			}), bindings: bindingReader,
		},
		"missing live binding": {
			thread: base, bindings: currentBindingReaderStub{workspace: securityContext.WorkspaceRealPath, err: errors.New("missing")},
		},
		"live workspace mismatch": {
			thread: base, bindings: currentBindingReaderStub{workspace: securityContext.WorkspaceRealPath, binding: domainsecurity.CaseBinding{
				CaseID: securityContext.CaseID, WorkspaceRealPath: "/cases/other", CaseBindingHash: securityContext.CaseBindingHash,
			}},
		},
		"live case mismatch": {
			thread: base, bindings: currentBindingReaderStub{workspace: securityContext.WorkspaceRealPath, binding: domainsecurity.CaseBinding{
				CaseID: "case-other", WorkspaceRealPath: securityContext.WorkspaceRealPath, CaseBindingHash: securityContext.CaseBindingHash,
			}},
		},
		"live binding hash mismatch": {
			thread: base, bindings: currentBindingReaderStub{workspace: securityContext.WorkspaceRealPath, binding: domainsecurity.CaseBinding{
				CaseID: securityContext.CaseID, WorkspaceRealPath: securityContext.WorkspaceRealPath,
				CaseBindingHash: domainsecurity.SHA256Hex([]byte("other-binding")),
			}},
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			validator := NewCurrentCaseThreadAuthorityValidator(authority, test.bindings, nil)
			if current, err := validator.ValidateCurrent(securityContext.ThreadID, test.thread); err == nil || current.ContextDigest != "" {
				t.Fatalf("invalid live authority was accepted: current=%#v err=%v", current, err)
			}
		})
	}
}

func TestTrustedProjectionHidesAcceptedFinalWhenLiveCaseAuthorityDrifts(t *testing.T) {
	signingAuthority := newProjectionTestAuthority(23)
	securityContext := newTrustedProjectionContext("thread-live-project", "turn-live-project", "case-live-project", "snapshot-live-project", 4)
	fixture := newTrustedProjectionFixture(t, signingAuthority, securityContext)
	index := gateprojection.NewTrustedFinalProjectionIndex(signingAuthority)
	if err := index.RegisterTerminalComplete(context.Background(), fixture.terminalAuthority); err != nil {
		t.Fatal(err)
	}
	authority := currentCaseAuthorityStub{context: securityContext}
	bindings := currentBindingReaderStub{workspace: securityContext.WorkspaceRealPath, binding: domainsecurity.CaseBinding{
		CaseID: securityContext.CaseID, WorkspaceRealPath: securityContext.WorkspaceRealPath, CaseBindingHash: securityContext.CaseBindingHash,
	}}
	thread := fixture.thread()
	thread["contextEpochState"] = currentAuthorityEpochState(t, securityContext, securityContext.ContextEpoch)
	validProjector := NewTrustedPublicProjectorWithPrimaryCAS(
		index, authority, NewCurrentCaseThreadAuthorityValidator(authority, bindings, nil), fixture.casReader(t, securityContext),
	)
	projected, err := validProjector.ProjectThread(thread)
	if err != nil || !strings.Contains(string(mustProjectionJSON(t, projected)), fixture.privateRecord.RenderedText) {
		t.Fatalf("live-authority-matched final was hidden: projected=%#v err=%v", projected, err)
	}
	withoutValidator, err := NewTrustedPublicProjectorWithPrimaryCAS(index, authority, nil, fixture.casReader(t, securityContext)).ProjectThread(thread)
	if err != nil {
		t.Fatal(err)
	}
	if body := string(mustProjectionJSON(t, withoutValidator)); strings.Contains(body, fixture.privateRecord.RenderedText) {
		t.Fatalf("authority-known case final bypassed a missing live validator: %s", body)
	}

	tests := map[string]struct {
		thread   map[string]any
		bindings currentBindingReaderStub
	}{
		"security state removed": {
			thread: mutateCurrentAuthorityThread(thread, func(record map[string]any) { delete(record, "securityState") }), bindings: bindings,
		},
		"epoch state removed": {
			thread: mutateCurrentAuthorityThread(thread, func(record map[string]any) { delete(record, "contextEpochState") }), bindings: bindings,
		},
		"workspace moved": {
			thread: mutateCurrentAuthorityThread(thread, func(record map[string]any) { record["workspace"] = "/cases/other" }), bindings: bindings,
		},
		"binding changed": {
			thread: thread, bindings: currentBindingReaderStub{workspace: securityContext.WorkspaceRealPath, binding: domainsecurity.CaseBinding{
				CaseID: "case-other", WorkspaceRealPath: securityContext.WorkspaceRealPath,
				CaseBindingHash: domainsecurity.SHA256Hex([]byte("other-binding")),
			}},
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			projector := NewTrustedPublicProjectorWithPrimaryCAS(
				index, authority, NewCurrentCaseThreadAuthorityValidator(authority, test.bindings, nil), fixture.casReader(t, securityContext),
			)
			projected, err := projector.ProjectThread(test.thread)
			if err != nil && projected != nil {
				t.Fatalf("failed projection returned public state: %#v err=%v", projected, err)
			}
			body := string(mustProjectionJSON(t, projected))
			if strings.Contains(body, fixture.privateRecord.RenderedText) || strings.Contains(body, fixture.privateRecord.AcceptedFinal.RecordDigest) {
				t.Fatalf("drifted live authority exposed accepted final: %s", body)
			}
			if event, visible, err := projector.ProjectEvent(securityContext.ThreadID, test.thread, fixture.event("terminal")); visible || event != nil {
				t.Fatalf("drifted live authority exposed terminal event: event=%#v visible=%t err=%v", event, visible, err)
			}
		})
	}
}

func TestCurrentCaseThreadAuthorityRejectsSignedEpochRollback(t *testing.T) {
	oldContext := newTrustedProjectionContext("thread-rollback", "turn-old", "case-old", "snapshot-old", 3)
	newContext := domainsecurity.NewTurnSecurityContext(domainsecurity.TurnSecurityContextInput{
		ThreadID: oldContext.ThreadID, TurnID: "turn-new", WorkspaceRealPath: "/cases/case-new", CaseID: "case-new",
		CaseBindingHash: domainsecurity.SHA256Hex([]byte("binding:case-new")), DatasetSnapshotID: "snapshot-new",
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("manifest:snapshot-new")), ContextEpoch: 4,
		IssuedAt: time.Date(2026, 7, 11, 8, 1, 0, 0, time.UTC),
	})
	authority := currentCaseInventoryStub{threadID: oldContext.ThreadID, contexts: []domainsecurity.TurnSecurityContext{oldContext, newContext}}
	thread := currentAuthorityThread(t, oldContext)
	thread["turns"] = append(thread["turns"].([]any), map[string]any{
		"id": newContext.TurnID, "threadId": newContext.ThreadID,
		"securityContext": publicProjectionSecurityRecord(newContext), "items": []any{},
	})
	oldBinding := currentBindingReaderStub{workspace: oldContext.WorkspaceRealPath, binding: domainsecurity.CaseBinding{
		CaseID: oldContext.CaseID, WorkspaceRealPath: oldContext.WorkspaceRealPath, CaseBindingHash: oldContext.CaseBindingHash,
	}}
	validator := NewCurrentCaseThreadAuthorityValidator(authority, oldBinding, nil)
	if current, err := validator.ValidateCurrent(oldContext.ThreadID, thread); err == nil || current.ContextDigest != "" {
		t.Fatalf("rollback to an older signed epoch was accepted: current=%#v err=%v", current, err)
	}
	deleteLatest := contracts.CloneMap(thread)
	deleteLatest["turns"] = deleteLatest["turns"].([]any)[:1]
	if current, err := validator.ValidateCurrent(oldContext.ThreadID, deleteLatest); err == nil || current.ContextDigest != "" {
		t.Fatalf("removing the signed high-water turn enabled rollback: current=%#v err=%v", current, err)
	}
}

func TestCurrentCaseAToBToAReturnRejectsOldHandleAndAllowsFreshContext(t *testing.T) {
	contexts := []domainsecurity.TurnSecurityContext{
		newTrustedProjectionContext("thread-case-return", "turn-a1", "case-a", "snapshot-a1", 3),
		newTrustedProjectionContext("thread-case-return", "turn-b", "case-b", "snapshot-b", 4),
		newTrustedProjectionContext("thread-case-return", "turn-a2", "case-a", "snapshot-a1", 5),
	}
	var handles []map[string]any
	for position, current := range contexts {
		authority := currentCaseInventoryStub{threadID: current.ThreadID, contexts: contexts[:position+1]}
		binding := currentBindingReaderStub{workspace: current.WorkspaceRealPath, binding: domainsecurity.CaseBinding{
			CaseID: current.CaseID, WorkspaceRealPath: current.WorkspaceRealPath, CaseBindingHash: current.CaseBindingHash,
		}}
		validator := NewCurrentCaseThreadAuthorityValidator(authority, binding, nil)
		thread := currentAuthorityThread(t, current)
		for _, old := range handles {
			thread["turns"] = append(thread["turns"].([]any), old["turns"].([]any)[0])
			if _, err := validator.ValidateCurrent(current.ThreadID, old); err == nil {
				t.Fatal("case return reactivated an older context handle")
			}
		}
		if got, err := validator.ValidateCurrent(current.ThreadID, thread); err != nil || got != current {
			t.Fatal("fresh current case context was refused", err)
		}
		handles = append(handles, currentAuthorityThread(t, current))
	}
	if !sameRetainedFactScopeV1(contexts[2], contexts[0]) || sameRetainedFactScopeV1(contexts[1], contexts[0]) {
		t.Fatal("historical eligibility did not follow the current case scope")
	}
}

func TestRetainedFactPostWitnessRechecksEpochHighWater(t *testing.T) {
	for _, changed := range []bool{false, true} {
		old := newTrustedProjectionContext("thread-retained-epoch", "turn-original", "case-a", "snapshot-a", 3)
		current := newTrustedProjectionContext(old.ThreadID, "turn-current", "case-a", "snapshot-b", 4)
		authority := &currentCaseInventoryStub{threadID: old.ThreadID, contexts: []domainsecurity.TurnSecurityContext{old, current}}
		binding := currentBindingReaderStub{workspace: current.WorkspaceRealPath, binding: domainsecurity.CaseBinding{
			CaseID: current.CaseID, WorkspaceRealPath: current.WorkspaceRealPath, CaseBindingHash: current.CaseBindingHash,
		}}
		thread := currentAuthorityThread(t, current)
		thread["turns"] = append(thread["turns"].([]any), currentAuthorityThread(t, old)["turns"].([]any)[0])
		validator := NewCurrentCaseThreadAuthorityValidator(authority, binding, nil)
		if _, err := validator.ValidateCurrent(current.ThreadID, thread); err != nil {
			t.Fatal(err)
		}
		projector := NewTrustedPublicProjectorWithCurrentCaseAuthority(nil, authority, validator)
		projector.retainedFactVerifier = func(domainevidence.PrivateAcceptedFinalRecord, domainsecurity.TurnSecurityContext) error {
			if changed {
				authority.contexts = append(authority.contexts,
					newTrustedProjectionContext(old.ThreadID, "turn-case-b", "case-b", "snapshot-c", 5),
					newTrustedProjectionContext(old.ThreadID, "turn-return-a", "case-a", "snapshot-b", 6))
			}
			return nil
		}
		err := projector.verifyRetainedFactsV1(thread, []retainedFactAdmissionV1{{current: current}})
		if (err != nil) != changed {
			t.Fatal("post-witness current epoch validation mismatch", err)
		}
		if changed && !errors.Is(err, ErrPublicProjectionPending) {
			t.Fatal("changed retained authority lost its bounded public pending classification")
		}
	}
}

func TestCurrentCaseThreadAuthorityUsesOptionalSnapshotFreshnessPort(t *testing.T) {
	securityContext := newTrustedProjectionContext("thread-snapshot", "turn-snapshot", "case-snapshot", "snapshot-a", 2)
	authority := currentCaseAuthorityStub{context: securityContext}
	bindings := currentBindingReaderStub{workspace: securityContext.WorkspaceRealPath, binding: domainsecurity.CaseBinding{
		CaseID: securityContext.CaseID, WorkspaceRealPath: securityContext.WorkspaceRealPath, CaseBindingHash: securityContext.CaseBindingHash,
	}}
	thread := currentAuthorityThread(t, securityContext)
	if _, err := NewCurrentCaseThreadAuthorityValidator(authority, bindings, nil).ValidateCurrent(securityContext.ThreadID, thread); err != nil {
		t.Fatalf("binding/epoch validation incorrectly claimed a required source probe: %v", err)
	}
	stale := currentSnapshotValidatorStub{err: errors.New("stale snapshot")}
	if _, err := NewCurrentCaseThreadAuthorityValidator(authority, bindings, stale).ValidateCurrent(securityContext.ThreadID, thread); err == nil {
		t.Fatal("configured host snapshot freshness failure was ignored")
	}
}

func TestCurrentCaseBoundaryAuthorityRequiresExactLiveBindingObservation(t *testing.T) {
	workspace := "/cases/boundary-live"
	observation, err := domainsecurity.NewCaseBindingObservationV1(domainsecurity.CaseBindingObservationInputV1{
		WorkspaceRealPath: workspace,
		State:             domainsecurity.CaseBindingStateValid,
		CaseID:            "case-boundary-live",
		BindingSHA256:     domainsecurity.SHA256Hex([]byte("boundary-live-document")),
		CaseBindingHash:   domainsecurity.SHA256Hex([]byte("boundary-live-binding")),
	})
	if err != nil {
		t.Fatal(err)
	}
	policy, err := domainsecurity.NewTurnPublicationPolicyV1(domainsecurity.TurnPublicationPolicyInputV1{
		ThreadRiskPolicyDigest:   domainsecurity.SHA256Hex([]byte("boundary-live-risk")),
		RiskClass:                domainsecurity.RiskClassCase,
		Disposition:              domainsecurity.PublicationDispositionCaseBoundaryOnly,
		CaseBindingState:         domainsecurity.CaseBindingStateValid,
		BindingObservationDigest: observation.ObservationDigest,
		BlockerCode:              domainsecurity.PublicationBlockerDatasetSnapshotUnavailable,
	})
	if err != nil {
		t.Fatal(err)
	}
	securityContext, err := domainsecurity.NewTurnSecurityContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-boundary-live", TurnID: "turn-boundary-live", WorkspaceRealPath: workspace,
		TenantID: domainsecurity.LocalTenantID, UserID: domainsecurity.LocalUserID,
		CaseID: domainsecurity.UnboundCaseID, CaseBindingHash: domainsecurity.UnboundCaseBindingHash(workspace),
		DatasetSnapshotID: domainsecurity.NoDatasetSnapshotID, SourceManifestHash: domainsecurity.EmptySourceManifestHash,
		ContextEpoch: 3, IssuedAt: time.Date(2026, 7, 23, 8, 0, 0, 0, time.UTC), PublicationPolicy: policy,
		RiskAuthorityBinding: domainsecurity.NewQuarantinedRiskAuthorityBindingV1(),
	})
	if err != nil {
		t.Fatal(err)
	}
	authority := currentCaseAuthorityStub{context: securityContext}
	thread := currentAuthorityThread(t, securityContext)
	binding := domainsecurity.CaseBinding{
		CaseID: observation.CaseID, WorkspaceRealPath: observation.WorkspaceRealPath,
		CaseBindingHash: observation.CaseBindingHash,
	}
	reader := currentBindingObserverStub{
		currentBindingReaderStub: currentBindingReaderStub{workspace: workspace, binding: binding},
		observation:              observation,
	}
	validator := NewCurrentCaseThreadAuthorityValidator(authority, reader, nil)
	if current, validateErr := validator.ValidateCurrent(securityContext.ThreadID, thread); validateErr != nil || current != securityContext {
		t.Fatalf("exact boundary binding observation was rejected: current=%#v err=%v", current, validateErr)
	}
	changed, err := domainsecurity.NewCaseBindingObservationV1(domainsecurity.CaseBindingObservationInputV1{
		WorkspaceRealPath: workspace,
		State:             domainsecurity.CaseBindingStateValid,
		CaseID:            "case-boundary-changed",
		BindingSHA256:     domainsecurity.SHA256Hex([]byte("boundary-changed-document")),
		CaseBindingHash:   domainsecurity.SHA256Hex([]byte("boundary-changed-binding")),
	})
	if err != nil {
		t.Fatal(err)
	}
	reader.observation = changed
	if current, validateErr := NewCurrentCaseThreadAuthorityValidator(authority, reader, nil).ValidateCurrent(securityContext.ThreadID, thread); validateErr == nil || current.ContextDigest != "" {
		t.Fatalf("changed boundary binding observation retained old projection authority: current=%#v err=%v", current, validateErr)
	}
}

func currentAuthorityThread(t *testing.T, securityContext domainsecurity.TurnSecurityContext) map[string]any {
	t.Helper()
	return map[string]any{
		"id": securityContext.ThreadID, "workspace": securityContext.WorkspaceRealPath,
		"securityState":     publicProjectionSecurityRecord(securityContext),
		"contextEpochState": currentAuthorityEpochState(t, securityContext, securityContext.ContextEpoch),
		"turns": []any{map[string]any{
			"id": securityContext.TurnID, "threadId": securityContext.ThreadID,
			"securityContext": publicProjectionSecurityRecord(securityContext), "items": []any{},
		}},
	}
}

func currentAuthorityEpochState(t *testing.T, securityContext domainsecurity.TurnSecurityContext, epoch uint64) map[string]any {
	t.Helper()
	state, err := appcontextepoch.BootstrapState(
		securityContext.ThreadID, epoch, []domaincontextepoch.SourceEntry{appcontextepoch.SecurityBindingEntry(securityContext)},
		time.Date(2026, 7, 11, 8, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatal(err)
	}
	return appcontextepoch.PublicState(state)
}

func mutateCurrentAuthorityThread(thread map[string]any, mutate func(map[string]any)) map[string]any {
	clone := contracts.CloneMap(thread)
	mutate(clone)
	return clone
}

func (stub currentCaseAuthorityStub) RestartPreservesThreadV1(threadID string) bool {
	return stub.restartHeld && threadID == stub.context.ThreadID
}

func TestCurrentCaseAuthorityForwardsRestartPreservationThroughProjectorOverride(t *testing.T) {
	stub := currentCaseAuthorityStub{context: domainsecurity.TurnSecurityContext{ThreadID: "held"}, restartHeld: true}
	validator := NewCurrentCaseThreadAuthorityValidator(stub, nil, nil)
	projector := NewTrustedPublicProjectorWithCurrentCaseAuthority(nil, nil, validator)
	if !projector.caseThreads.RestartPreservesThreadV1("held") || projector.caseThreads.RestartPreservesThreadV1("independent") {
		t.Fatal("current authority override lost exact restart preservation")
	}
	if _, err := validator.ValidateCurrent("held", nil); !errors.Is(err, casethreadapp.ErrRestartPreserved) {
		t.Fatal("current context validation ignored preservation before dependency reads")
	}
}

func (currentCaseInventoryStub) RestartPreservesThreadV1(string) bool { return false }
