package runtimeapp

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	httpapi "analytix.local/runtime-go/internal/adapters/inbound/httpapi"
	caseentityadapter "analytix.local/runtime-go/internal/adapters/outbound/caseentity"
	finalauthorityadapter "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	caseentityapp "analytix.local/runtime-go/internal/app/caseentity"
	evidenceapp "analytix.local/runtime-go/internal/app/evidence"
	localdisplayapp "analytix.local/runtime-go/internal/app/localdisplay"
	domaincaseentity "analytix.local/runtime-go/internal/domain/caseentity"
	domaincontrolledaccount "analytix.local/runtime-go/internal/domain/controlledaccount"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainidentity "analytix.local/runtime-go/internal/domain/identity"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	registryport "analytix.local/runtime-go/internal/ports/evidenceregistry"
	acceptedslotfixture "analytix.local/runtime-go/internal/testsupport/acceptedslotdisplay"
	privatecastest "analytix.local/runtime-go/internal/testsupport/privatecas"
	securitycontexttest "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

type acceptedSlotHTTPRegistryV1 struct {
	registry domainevidence.EvidenceReceiptRegistry
}

func (store *acceptedSlotHTTPRegistryV1) CommitPrepared(context.Context, registryport.CommitPreparedInput) (domainevidence.EvidenceReceipt, error) {
	return domainevidence.EvidenceReceipt{}, errors.New("accepted-slot test registry commit is closed")
}

func (store *acceptedSlotHTTPRegistryV1) Resolve(
	_ context.Context,
	query registryport.MembershipQuery,
) (domainevidence.RegisteredEvidence, error) {
	return domainevidence.VerifyEvidenceReceiptMembership(store.registry, query.Context, query.ReceiptID)
}

func (store *acceptedSlotHTTPRegistryV1) Revoke(_ context.Context, input registryport.RevokeInput) error {
	next, err := domainevidence.RevokeEvidenceReceipt(store.registry, input.ReceiptID, input.ReasonCode, input.RevokedAt)
	if err == nil {
		store.registry = next
	}
	return err
}

func (store *acceptedSlotHTTPRegistryV1) Replay(
	_ context.Context,
	securityContext domainsecurity.TurnSecurityContext,
) (domainevidence.EvidenceReceiptRegistry, error) {
	if !domainevidence.EvidenceReceiptRegistryMatchesContext(store.registry, securityContext) {
		return domainevidence.EvidenceReceiptRegistry{}, errors.New("accepted-slot registry context changed")
	}
	return store.registry, nil
}

func (store *acceptedSlotHTTPRegistryV1) ReplayAt(
	ctx context.Context,
	securityContext domainsecurity.TurnSecurityContext,
	sequence uint64,
) (domainevidence.EvidenceReceiptRegistry, error) {
	registry, err := store.Replay(ctx, securityContext)
	if err != nil || registry.Sequence != sequence {
		return domainevidence.EvidenceReceiptRegistry{}, errors.New("accepted-slot registry head changed")
	}
	return registry, nil
}

type acceptedSlotHTTPIdentityV1 struct {
	principal       domainidentity.PrincipalV1
	postValidateErr error
	validateCalls   int
}

type acceptedSlotHTTPCaseAuthorityV1 struct {
	active               domainsecurity.TurnSecurityContext
	fork                 domainsecurity.TurnSecurityContext
	observation          domainsecurity.CaseBindingObservationV1
	driftObservation     domainsecurity.CaseBindingObservationV1
	validateCalls        int
	observeCalls         int
	failValidateCall     int
	driftObservationCall int
}

func (authority *acceptedSlotHTTPCaseAuthorityV1) ValidateCurrent(
	_ context.Context,
	candidate domainsecurity.TurnSecurityContext,
) error {
	authority.validateCalls++
	if candidate != authority.active && candidate != authority.fork ||
		authority.failValidateCall != 0 && authority.validateCalls == authority.failValidateCall {
		return errors.New("accepted-slot active context changed")
	}
	return nil
}

func (authority *acceptedSlotHTTPCaseAuthorityV1) Observe(
	workspaceRealPath string,
) (domainsecurity.CaseBindingObservationV1, error) {
	authority.observeCalls++
	if workspaceRealPath != authority.observation.WorkspaceRealPath {
		return domainsecurity.CaseBindingObservationV1{}, errors.New("accepted-slot workspace changed")
	}
	if authority.driftObservationCall != 0 && authority.observeCalls == authority.driftObservationCall {
		return authority.driftObservation, nil
	}
	return authority.observation, nil
}

func (authority *acceptedSlotHTTPCaseAuthorityV1) Reset() {
	authority.validateCalls = 0
	authority.observeCalls = 0
	authority.failValidateCall = 0
	authority.driftObservationCall = 0
}

func (identity *acceptedSlotHTTPIdentityV1) ResolveCurrent(context.Context) (domainidentity.PrincipalV1, error) {
	return identity.principal, nil
}

func (identity *acceptedSlotHTTPIdentityV1) ValidateCurrent(_ context.Context, principal domainidentity.PrincipalV1) error {
	identity.validateCalls++
	if principal != identity.principal || identity.validateCalls > 1 && identity.postValidateErr != nil {
		return errors.New("accepted-slot principal changed")
	}
	return nil
}

func TestAcceptedSlotDisplayHTTPReadsOriginalRetainedDSV2AfterCurrentEvolutionAndRestart(t *testing.T) {
	for _, test := range []struct {
		field, original, canonical string
	}{
		{field: domainevidence.AcceptedSlotSourceFieldAccountV1, original: "6222 0212-3456 7890", canonical: "6222021234567890"},
		{field: domainevidence.AcceptedSlotSourceFieldCardV1, original: "4111-2222 3333-4444", canonical: "4111222233334444"},
	} {
		t.Run(test.field, func(t *testing.T) {
			testAcceptedSlotDisplayHTTPReadsOriginalRetainedDSV2AfterCurrentEvolutionAndRestartV1(
				t, test.field, test.original, test.canonical,
			)
		})
	}
}

func testAcceptedSlotDisplayHTTPReadsOriginalRetainedDSV2AfterCurrentEvolutionAndRestartV1(
	t *testing.T,
	field string,
	originalSentinel string,
	canonicalSentinel string,
) {
	const (
		currentSentinel = "CURRENT-DB-99990000"
		labelSentinel   = "〔账户槽位 1〕"
	)
	retained, err := acceptedslotfixture.NewRetainedSourceFixtureV1(
		originalSentinel, canonicalSentinel, field,
	)
	if err != nil {
		t.Fatal(err)
	}
	if retained.ActiveContext.DatasetSnapshotID == retained.HistoricalContext.DatasetSnapshotID {
		t.Fatal("accepted-slot HTTP fixture did not evolve the current snapshot")
	}
	entityReference := acceptedSlotHTTPReferenceV1(t, retained, canonicalSentinel, field)
	retained.Binding, err = domainevidence.NewAcceptedSlotSourceBindingV1(
		domainevidence.AcceptedSlotSourceBindingInputV1{
			FactIDs: []string{"accepted-slot-fact"}, EntityReference: string(entityReference),
			SourceRecordID: retained.Binding.SourceRecordID, SourceFileID: retained.Binding.SourceFileID,
			SourceRowNumber: retained.Binding.SourceRowNumber, Field: retained.Binding.Field,
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	facts, sourceBinding := acceptedSlotHTTPFactsAndBindingV1(t, retained.Binding)
	registry, receipt := acceptedSlotHTTPSettlementV1(t, retained.HistoricalContext, facts, sourceBinding)
	registryStore := &acceptedSlotHTTPRegistryV1{registry: registry}
	claims := acceptedSlotHTTPVerifiedClaimsV1(t, registryStore, retained.HistoricalContext, receipt, facts)
	envelope, err := (evidenceapp.FinalEvidenceGate{Registry: registryStore}).Finalize(
		context.Background(), evidenceapp.FinalGateInput{
			Context: retained.HistoricalContext, TerminalReason: evidenceapp.TerminalSuccess,
			Claims: claims, CheckedScope: &receipt.QueryRange, RequestedScope: &receipt.QueryRange,
			IssuedAt: time.Date(2026, 8, 21, 2, 30, 0, 0, time.UTC),
		},
	)
	if err != nil || envelope.Variant != domainevidence.EvidenceBackedAnswer || len(envelope.Claims) != 3 {
		t.Fatalf("real V3 account-flow facts did not cross Final Gate: envelope=%#v err=%v", envelope, err)
	}
	eligibleSlots, err := domainevidence.BuildAcceptedEntitySlotBindingsV1(envelope)
	if err != nil || len(eligibleSlots) != 1 {
		t.Fatalf("real V3 account-flow final did not produce one eligible slot: slots=%#v err=%v", eligibleSlots, err)
	}
	finalEnvelopeBody, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	for _, private := range []string{
		sourceBinding.SourceFileID, sourceBinding.SourceRecordID,
		"sourceRowNumber", "acceptedSlotSourceBindings", "authorityEntityRef",
	} {
		if bytes.Contains(finalEnvelopeBody, []byte(private)) {
			t.Fatalf("Final Gate envelope leaked V3 private lineage %q: %s", private, finalEnvelopeBody)
		}
	}
	renderedFinal, err := domainevidence.RenderFinalAnswer(envelope)
	if err != nil || !strings.Contains(renderedFinal, labelSentinel) || strings.Contains(renderedFinal, originalSentinel) {
		t.Fatalf("Final Gate did not retain only its typed public label: rendered=%q err=%v", renderedFinal, err)
	}
	privateRecord, err := acceptedslotfixture.NewWitnessedPrivateFinalV1(
		acceptedslotfixture.WitnessedFinalInputV1{
			Context: retained.HistoricalContext, Registry: registry, Envelope: envelope, Receipt: receipt,
			Snapshot: retained.HistoricalSnapshot, BindingObservation: retained.Observation,
			AuthorityKeyID: domainsecurity.SHA256Hex(retained.PublicKey), AuthorityPublicKey: retained.PublicKey,
			Sign: func(message []byte) ([]byte, error) { return ed25519.Sign(retained.PrivateKey, message), nil },
			Now:  time.Date(2026, 8, 21, 2, 31, 0, 0, time.UTC),
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	privateRoot := filepath.Join(t.TempDir(), "accepted-finals")
	access, err := privatecastest.NewAccessAuthority(privateRoot)
	if err != nil {
		t.Fatal(err)
	}
	privateStore, err := finalauthorityadapter.NewPrivateStore(privateRoot, access)
	if err != nil {
		t.Fatal(err)
	}
	if err := privateStore.PutIfAbsent(context.Background(), privateRecord); err != nil {
		t.Fatal(err)
	}
	observation, err := domainevidence.NewAcceptedFinalCASObservationV1(domainevidence.AcceptedFinalCASObservationV1{
		ThreadID: retained.HistoricalContext.ThreadID, TurnID: retained.HistoricalContext.TurnID,
		Status: "completed", FrozenContext: retained.HistoricalContext, CurrentContext: retained.HistoricalContext,
		HasWinner: true, Winner: privateRecord.AcceptedFinal,
		ThreadFileSHA256:     acceptedSlotHTTPDigestV1("accepted-slot-http-thread"),
		TurnProjectionSHA256: acceptedSlotHTTPDigestV1("accepted-slot-http-turn"),
	})
	if err != nil {
		t.Fatal(err)
	}
	disposition, err := domainevidence.NewAcceptedFinalDispositionRecordV2(
		domainevidence.AcceptedFinalDispositionInput{
			AcceptedFinal: privateRecord.AcceptedFinal, State: domainevidence.AcceptedFinalCommitted,
			EventManifestDigest: acceptedSlotHTTPDigestV1("accepted-slot-http-event-manifest"),
			DecidedAt:           time.Date(2026, 8, 21, 2, 32, 0, 0, time.UTC),
			AuthorityKeyID:      domainsecurity.SHA256Hex(retained.PublicKey), AuthorityPublicKey: retained.PublicKey,
		},
		observation, domainevidence.AcceptedFinalDecisionSamePublicWinner,
		func(message []byte) ([]byte, error) { return ed25519.Sign(retained.PrivateKey, message), nil },
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := privateStore.PutDispositionIfAbsent(context.Background(), disposition); err != nil {
		t.Fatal(err)
	}
	caseEntities, longitudinalBindingDigest, hostileBindingDigests, forkContext, caseAuthority := acceptedSlotHTTPReopenedCaseEntityV1(
		t, retained, canonicalSentinel, field, entityReference, privateRecord, disposition, receipt, claims,
	)

	// Simulate process restart/reopen before the HTTP composition reads either record.
	reopenedAccess, err := privatecastest.NewAccessAuthority(privateRoot)
	if err != nil {
		t.Fatal(err)
	}
	reopenedFinals, err := finalauthorityadapter.NewPrivateStore(privateRoot, reopenedAccess)
	if err != nil {
		t.Fatal(err)
	}
	principal, err := domainidentity.NewPrincipalV1(
		retained.HistoricalSnapshot.Record.InstallationID,
		retained.HistoricalContext.TenantID,
		retained.HistoricalContext.UserID,
	)
	if err != nil {
		t.Fatal(err)
	}
	identity := &acceptedSlotHTTPIdentityV1{principal: principal}
	var lateCurrentSecurityContext *domainsecurity.TurnSecurityContext
	lateCurrentLoadCount := 0
	service := localdisplayapp.NewServiceWithTypedLocalDisplay(
		caseEntities, reopenedFinals,
		localdisplayapp.DirectSourcePreviewDependenciesV1{},
		localdisplayapp.AcceptedSlotDisplayDependenciesV1{
			Identity: identity, Evidence: registryStore,
			UseRetainedSource: retained.Service.UseRetainedAcceptedSlotDisplay,
		},
	)
	handler := httpapi.LocalDisplayMuxV1{
		RuntimeToken: httpapi.DefaultRuntimeToken,
		Next: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			httpapi.WriteJSON(w, http.StatusNotFound, map[string]any{"code": "not_found"})
		}),
		LocalDisplay: httpapi.LocalDisplayHandlerV1{
			Service: service,
			LoadCurrentSecurityContext: func(threadID string) (domainsecurity.TurnSecurityContext, error) {
				if threadID != retained.HistoricalContext.ThreadID {
					return domainsecurity.TurnSecurityContext{}, errors.New("accepted-slot thread changed")
				}
				if lateCurrentSecurityContext != nil {
					lateCurrentLoadCount++
					if lateCurrentLoadCount > 1 {
						return *lateCurrentSecurityContext, nil
					}
				}
				return retained.ActiveContext, nil
			},
		},
	}

	call := func(mode, threadID, turnID, finalDigest string) *httptest.ResponseRecorder {
		body, marshalErr := json.Marshal(map[string]any{
			"kind":     "accepted_slot_display",
			"threadId": threadID, "turnId": turnID,
			"acceptedFinalDigest": finalDigest, "displayMode": mode,
		})
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		request := httptest.NewRequest(http.MethodPost, httpapi.LocalDisplayAcceptedSlotsPathV1, bytes.NewReader(body))
		request.Header.Set("Authorization", "Bearer "+httpapi.DefaultRuntimeToken)
		request.Header.Set(httpapi.LocalDisplayHeaderV1, httpapi.LocalDisplayHeaderValueV1)
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		return recorder
	}
	recorder := call(
		localdisplayapp.DisplayModeFull, retained.HistoricalContext.ThreadID,
		retained.HistoricalContext.TurnID, privateRecord.AcceptedFinal.RecordDigest,
	)
	if recorder.Code != http.StatusOK || recorder.Header().Get("Cache-Control") != "no-store" ||
		recorder.Header().Get("Pragma") != "no-cache" ||
		!bytes.Contains(recorder.Body.Bytes(), []byte(originalSentinel)) ||
		!bytes.Contains(recorder.Body.Bytes(), []byte(`"field":"`+field+`"`)) ||
		bytes.Contains(recorder.Body.Bytes(), retained.CurrentSentinel) {
		t.Fatalf("production retained DSV2 HTTP seam failed: status=%d headers=%v body=%s",
			recorder.Code, recorder.Header(), recorder.Body.Bytes())
	}
	for _, forbidden := range []string{
		sourceBinding.EntityReference, sourceBinding.SourceFileID, sourceBinding.SourceRecordID,
		"sourceRowNumber", "acceptedSlotSourceBindings", "authorityEntityRef",
		canonicalSentinel, currentSentinel, labelSentinel,
	} {
		if strings.Contains(recorder.Body.String(), forbidden) {
			t.Fatalf("production typed response leaked locator/fallback %q: %s", forbidden, recorder.Body.String())
		}
	}
	var fullResponse localdisplayapp.ResponseV1
	if err := json.Unmarshal(recorder.Body.Bytes(), &fullResponse); err != nil || len(fullResponse.Slots) != 1 ||
		fullResponse.AcceptedFinalDigest != privateRecord.AcceptedFinal.RecordDigest {
		t.Fatalf("full retained response was not typed: response=%#v err=%v", fullResponse, err)
	}
	longitudinalResponse, err := service.CaseAcceptedSlotDisplay(
		context.Background(),
		localdisplayapp.CaseAcceptedSlotDisplayInputV1{
			BindingDigest: longitudinalBindingDigest, DisplayMode: localdisplayapp.DisplayModeFull,
			ActiveSecurityContext: retained.ActiveContext,
		},
	)
	if err != nil || len(longitudinalResponse.Slots) != 1 ||
		longitudinalResponse.AcceptedFinalDigest != privateRecord.AcceptedFinal.RecordDigest ||
		longitudinalResponse.Slots[0].DisplayValue != originalSentinel ||
		strings.Contains(longitudinalResponse.Slots[0].DisplayValue, canonicalSentinel) ||
		strings.Contains(longitudinalResponse.Slots[0].DisplayValue, currentSentinel) {
		t.Fatalf("longitudinal binding did not reopen the original retained snapshot: response=%#v err=%v", longitudinalResponse, err)
	}
	forkResponse, forkErr := service.CaseAcceptedSlotDisplay(
		context.Background(), localdisplayapp.CaseAcceptedSlotDisplayInputV1{
			BindingDigest: longitudinalBindingDigest, DisplayMode: localdisplayapp.DisplayModeFull,
			ActiveSecurityContext: forkContext,
		},
	)
	if forkErr != nil || len(forkResponse.Slots) != 1 ||
		forkResponse.AcceptedFinalDigest != privateRecord.AcceptedFinal.RecordDigest ||
		forkResponse.Slots[0].DisplayValue != originalSentinel ||
		strings.Contains(forkResponse.Slots[0].DisplayValue, canonicalSentinel) ||
		strings.Contains(forkResponse.Slots[0].DisplayValue, currentSentinel) {
		t.Fatalf("authorized fork did not reopen the original retained snapshot: response=%#v err=%v", forkResponse, forkErr)
	}
	for name, hostileDigest := range hostileBindingDigests {
		t.Run("hostile "+name, func(t *testing.T) {
			response, hostileErr := service.CaseAcceptedSlotDisplay(
				context.Background(), localdisplayapp.CaseAcceptedSlotDisplayInputV1{
					BindingDigest: hostileDigest, DisplayMode: localdisplayapp.DisplayModeFull,
					ActiveSecurityContext: retained.ActiveContext,
				},
			)
			if !errors.Is(hostileErr, localdisplayapp.ErrUnavailable) ||
				response.SchemaVersion != 0 || len(response.Slots) != 0 {
				t.Fatalf("hostile longitudinal identity projected exact bytes: response=%#v err=%v", response, hostileErr)
			}
		})
	}
	masked := call(
		localdisplayapp.DisplayModeMasked, retained.HistoricalContext.ThreadID,
		retained.HistoricalContext.TurnID, privateRecord.AcceptedFinal.RecordDigest,
	)
	var maskedResponse localdisplayapp.ResponseV1
	if err := json.Unmarshal(masked.Body.Bytes(), &maskedResponse); err != nil || masked.Code != http.StatusOK ||
		len(maskedResponse.Slots) != 1 || maskedResponse.Slots[0].DisplayValue != "****"+originalSentinel[len(originalSentinel)-4:] ||
		!equalStringSlicesV1(maskedResponse.Slots[0].ClaimIDs, fullResponse.Slots[0].ClaimIDs) ||
		!equalStringSlicesV1(maskedResponse.Slots[0].ReceiptIDs, fullResponse.Slots[0].ReceiptIDs) {
		t.Fatalf("masked retained response changed accepted artifacts or source input: full=%#v masked=%#v err=%v",
			fullResponse, maskedResponse, err)
	}
	if strings.Contains(masked.Body.String(), canonicalSentinel) || strings.Contains(masked.Body.String(), currentSentinel) ||
		strings.Contains(masked.Body.String(), labelSentinel) {
		t.Fatalf("masked retained response used a fallback sentinel: %s", masked.Body.String())
	}

	// A same-case independent thread rehydrates entity/currentness owners for
	// new work; it cannot directly open another thread's historical final.
	independent := call(
		localdisplayapp.DisplayModeFull, "thread-independent-same-case",
		retained.HistoricalContext.TurnID, privateRecord.AcceptedFinal.RecordDigest,
	)
	if independent.Code != http.StatusConflict || strings.Contains(independent.Body.String(), originalSentinel) {
		t.Fatalf("independent thread opened a foreign historical final: status=%d body=%s", independent.Code, independent.Body.String())
	}
	internalRequestBody, err := json.Marshal(map[string]any{
		"kind": "case_accepted_slot_display", "bindingDigest": longitudinalBindingDigest,
		"displayMode": localdisplayapp.DisplayModeFull,
	})
	if err != nil {
		t.Fatal(err)
	}
	internalRequest := httptest.NewRequest(
		http.MethodPost, httpapi.LocalDisplayAcceptedSlotsPathV1, bytes.NewReader(internalRequestBody),
	)
	internalRequest.Header.Set("Authorization", "Bearer "+httpapi.DefaultRuntimeToken)
	internalRequest.Header.Set(httpapi.LocalDisplayHeaderV1, httpapi.LocalDisplayHeaderValueV1)
	internalRecorder := httptest.NewRecorder()
	handler.ServeHTTP(internalRecorder, internalRequest)
	if internalRecorder.Code == http.StatusOK ||
		strings.Contains(internalRecorder.Body.String(), originalSentinel) ||
		strings.Contains(internalRecorder.Body.String(), longitudinalBindingDigest) ||
		strings.Contains(internalRecorder.Body.String(), "bindingDigest") {
		t.Fatalf("HTTP accepted or reflected the internal longitudinal seam: status=%d body=%s", internalRecorder.Code, internalRecorder.Body.String())
	}
	for _, hostile := range []struct {
		name    string
		digest  string
		context domainsecurity.TurnSecurityContext
	}{
		{name: "missing binding", digest: acceptedSlotHTTPDigestV1("missing-case-display-binding"), context: retained.ActiveContext},
		{name: "source binding digest", digest: sourceBinding.BindingDigest, context: retained.ActiveContext},
		{name: "cross case", digest: longitudinalBindingDigest, context: acceptedSlotHTTPEvolvedContextV1(
			t, retained.ActiveContext, func(input *domainsecurity.TurnSecurityContextInput) {
				input.CaseID = "case-accepted-slot-display-cross-internal"
				input.CaseBindingHash = acceptedSlotHTTPDigestV1("cross-case-display-binding")
			},
		)},
		{name: "cross binding", digest: longitudinalBindingDigest, context: acceptedSlotHTTPEvolvedContextV1(
			t, retained.ActiveContext, func(input *domainsecurity.TurnSecurityContextInput) {
				input.CaseBindingHash = acceptedSlotHTTPDigestV1("cross-display-binding")
			},
		)},
	} {
		t.Run("internal "+hostile.name, func(t *testing.T) {
			response, hostileErr := service.CaseAcceptedSlotDisplay(
				context.Background(), localdisplayapp.CaseAcceptedSlotDisplayInputV1{
					BindingDigest: hostile.digest, DisplayMode: localdisplayapp.DisplayModeFull,
					ActiveSecurityContext: hostile.context,
				},
			)
			if !errors.Is(hostileErr, localdisplayapp.ErrUnavailable) ||
				response.SchemaVersion != 0 || len(response.Slots) != 0 {
				t.Fatalf("hostile internal binding projected exact bytes: response=%#v err=%v", response, hostileErr)
			}
		})
	}

	for _, lateAuthority := range []struct {
		name   string
		mutate func(*domainsecurity.TurnSecurityContextInput)
	}{
		{name: "case switch", mutate: func(input *domainsecurity.TurnSecurityContextInput) {
			input.CaseID = "case-accepted-slot-display-switched"
			input.CaseBindingHash = acceptedSlotHTTPDigestV1("accepted-slot-switched-binding")
		}},
		{name: "snapshot evolution", mutate: func(input *domainsecurity.TurnSecurityContextInput) {
			input.DatasetSnapshotID = "dsv2_" + acceptedSlotHTTPDigestV1("accepted-slot-evolved-snapshot")
			input.SourceManifestHash = acceptedSlotHTTPDigestV1("accepted-slot-evolved-manifest")
		}},
		{name: "epoch advance", mutate: func(input *domainsecurity.TurnSecurityContextInput) {
			input.ContextEpoch++
		}},
	} {
		t.Run("late "+lateAuthority.name, func(t *testing.T) {
			lateContext := acceptedSlotHTTPEvolvedContextV1(t, retained.ActiveContext, lateAuthority.mutate)
			lateCurrentSecurityContext = &lateContext
			lateCurrentLoadCount = 0
			late := call(
				localdisplayapp.DisplayModeFull, retained.HistoricalContext.ThreadID,
				retained.HistoricalContext.TurnID, privateRecord.AcceptedFinal.RecordDigest,
			)
			lateCurrentSecurityContext = nil
			if late.Code != http.StatusConflict || strings.Contains(late.Body.String(), originalSentinel) ||
				!acceptedSlotHTTPUnavailableResponseV1(late) {
				t.Fatalf("late %s projected bytes: status=%d body=%s", lateAuthority.name, late.Code, late.Body.String())
			}
		})
	}

	identity.validateCalls = 0
	identity.postValidateErr = errors.New("internal principal expired after exact read")
	latePrincipalInternal, latePrincipalInternalErr := service.CaseAcceptedSlotDisplay(
		context.Background(), localdisplayapp.CaseAcceptedSlotDisplayInputV1{
			BindingDigest: longitudinalBindingDigest, DisplayMode: localdisplayapp.DisplayModeFull,
			ActiveSecurityContext: retained.ActiveContext,
		},
	)
	if !errors.Is(latePrincipalInternalErr, localdisplayapp.ErrUnavailable) ||
		latePrincipalInternal.SchemaVersion != 0 || len(latePrincipalInternal.Slots) != 0 ||
		identity.validateCalls != 2 {
		t.Fatalf("post-read principal drift left internal exact bytes: calls=%d response=%#v err=%v", identity.validateCalls, latePrincipalInternal, latePrincipalInternalErr)
	}
	identity.validateCalls = 0
	identity.postValidateErr = nil

	caseAuthority.Reset()
	caseAuthority.failValidateCall = 4
	lateContextInternal, lateContextInternalErr := service.CaseAcceptedSlotDisplay(
		context.Background(), localdisplayapp.CaseAcceptedSlotDisplayInputV1{
			BindingDigest: longitudinalBindingDigest, DisplayMode: localdisplayapp.DisplayModeFull,
			ActiveSecurityContext: retained.ActiveContext,
		},
	)
	if !errors.Is(lateContextInternalErr, localdisplayapp.ErrUnavailable) ||
		lateContextInternal.SchemaVersion != 0 || len(lateContextInternal.Slots) != 0 ||
		caseAuthority.validateCalls != 4 {
		t.Fatalf("post-read current-context drift left internal exact bytes: calls=%d response=%#v err=%v", caseAuthority.validateCalls, lateContextInternal, lateContextInternalErr)
	}

	caseAuthority.Reset()
	caseAuthority.driftObservationCall = 4
	lateObservationInternal, lateObservationInternalErr := service.CaseAcceptedSlotDisplay(
		context.Background(), localdisplayapp.CaseAcceptedSlotDisplayInputV1{
			BindingDigest: longitudinalBindingDigest, DisplayMode: localdisplayapp.DisplayModeFull,
			ActiveSecurityContext: retained.ActiveContext,
		},
	)
	if !errors.Is(lateObservationInternalErr, localdisplayapp.ErrUnavailable) ||
		lateObservationInternal.SchemaVersion != 0 || len(lateObservationInternal.Slots) != 0 ||
		caseAuthority.observeCalls != 4 {
		t.Fatalf("post-read binding observation drift left internal exact bytes: calls=%d response=%#v err=%v", caseAuthority.observeCalls, lateObservationInternal, lateObservationInternalErr)
	}
	caseAuthority.Reset()

	// Expired/deleted retained authority and late authority changes both discard
	// the already-read projection before any exact byte reaches HTTP or its caller.
	retained.SetRetainedUnavailable(errors.New("retained snapshot explicitly deleted"))
	closedInternal, internalErr := service.CaseAcceptedSlotDisplay(
		context.Background(), localdisplayapp.CaseAcceptedSlotDisplayInputV1{
			BindingDigest: longitudinalBindingDigest, DisplayMode: localdisplayapp.DisplayModeFull,
			ActiveSecurityContext: retained.ActiveContext,
		},
	)
	if !errors.Is(internalErr, localdisplayapp.ErrUnavailable) || closedInternal.SchemaVersion != 0 || len(closedInternal.Slots) != 0 {
		t.Fatalf("deleted retained snapshot left internal exact bytes: response=%#v err=%v", closedInternal, internalErr)
	}
	unavailable := call(
		localdisplayapp.DisplayModeFull, retained.HistoricalContext.ThreadID,
		retained.HistoricalContext.TurnID, privateRecord.AcceptedFinal.RecordDigest,
	)
	if unavailable.Code != http.StatusConflict || strings.Contains(unavailable.Body.String(), originalSentinel) {
		t.Fatalf("deleted retained snapshot projected bytes: status=%d body=%s", unavailable.Code, unavailable.Body.String())
	}
	if !acceptedSlotHTTPUnavailableResponseV1(unavailable) {
		t.Fatalf("deleted retained snapshot did not return the fixed closed response: %s", unavailable.Body.String())
	}
	retained.SetRetainedUnavailable(errors.New("retained snapshot lease expired"))
	expired := call(
		localdisplayapp.DisplayModeFull, retained.HistoricalContext.ThreadID,
		retained.HistoricalContext.TurnID, privateRecord.AcceptedFinal.RecordDigest,
	)
	if expired.Code != http.StatusConflict || strings.Contains(expired.Body.String(), originalSentinel) ||
		!acceptedSlotHTTPUnavailableResponseV1(expired) {
		t.Fatalf("expired retained snapshot projected bytes: status=%d body=%s", expired.Code, expired.Body.String())
	}
	retained.SetRetainedUnavailable(nil)
	identity.validateCalls = 0
	identity.postValidateErr = errors.New("renderer principal expired")
	late := call(
		localdisplayapp.DisplayModeFull, retained.HistoricalContext.ThreadID,
		retained.HistoricalContext.TurnID, privateRecord.AcceptedFinal.RecordDigest,
	)
	if late.Code != http.StatusConflict || strings.Contains(late.Body.String(), originalSentinel) {
		t.Fatalf("late principal response projected bytes: status=%d body=%s", late.Code, late.Body.String())
	}
	identity.validateCalls = 0
	identity.postValidateErr = nil
	retained.CorruptParsedPage()
	corruptInternal, corruptInternalErr := service.CaseAcceptedSlotDisplay(
		context.Background(), localdisplayapp.CaseAcceptedSlotDisplayInputV1{
			BindingDigest: longitudinalBindingDigest, DisplayMode: localdisplayapp.DisplayModeFull,
			ActiveSecurityContext: retained.ActiveContext,
		},
	)
	if !errors.Is(corruptInternalErr, localdisplayapp.ErrUnavailable) ||
		corruptInternal.SchemaVersion != 0 || len(corruptInternal.Slots) != 0 {
		t.Fatalf("corrupt retained source left internal exact bytes: response=%#v err=%v", corruptInternal, corruptInternalErr)
	}
	corrupt := call(
		localdisplayapp.DisplayModeFull, retained.HistoricalContext.ThreadID,
		retained.HistoricalContext.TurnID, privateRecord.AcceptedFinal.RecordDigest,
	)
	if corrupt.Code != http.StatusConflict || strings.Contains(corrupt.Body.String(), originalSentinel) ||
		!acceptedSlotHTTPUnavailableResponseV1(corrupt) {
		t.Fatalf("corrupt retained parsed page projected bytes: status=%d body=%s", corrupt.Code, corrupt.Body.String())
	}
	retained.ClearMaterialCorruption()
	if err := registryStore.Revoke(context.Background(), registryport.RevokeInput{
		ReceiptID: receipt.ReceiptID, ReasonCode: "source_retracted",
		RevokedAt: time.Date(2026, 8, 21, 2, 40, 0, 0, time.UTC),
	}); err != nil {
		t.Fatal(err)
	}
	revokedInternal, revokedInternalErr := service.CaseAcceptedSlotDisplay(
		context.Background(), localdisplayapp.CaseAcceptedSlotDisplayInputV1{
			BindingDigest: longitudinalBindingDigest, DisplayMode: localdisplayapp.DisplayModeFull,
			ActiveSecurityContext: retained.ActiveContext,
		},
	)
	if !errors.Is(revokedInternalErr, localdisplayapp.ErrUnavailable) ||
		revokedInternal.SchemaVersion != 0 || len(revokedInternal.Slots) != 0 {
		t.Fatalf("revoked evidence left internal exact bytes: response=%#v err=%v", revokedInternal, revokedInternalErr)
	}
	revoked := call(
		localdisplayapp.DisplayModeFull, retained.HistoricalContext.ThreadID,
		retained.HistoricalContext.TurnID, privateRecord.AcceptedFinal.RecordDigest,
	)
	if revoked.Code != http.StatusConflict || strings.Contains(revoked.Body.String(), originalSentinel) ||
		!acceptedSlotHTTPUnavailableResponseV1(revoked) {
		t.Fatalf("revoked retained receipt projected bytes: status=%d body=%s", revoked.Code, revoked.Body.String())
	}
}

type acceptedSlotHTTPKeyedDigesterV1 struct {
	key []byte
}

func (digester acceptedSlotHTTPKeyedDigesterV1) KeyedPayloadHash(
	ctx context.Context,
	purpose string,
	payload []byte,
) (string, error) {
	if ctx == nil || ctx.Err() != nil || len(digester.key) == 0 || strings.TrimSpace(purpose) == "" || len(payload) == 0 {
		return "", errors.New("accepted-slot keyed digest input is invalid")
	}
	mac := hmac.New(sha256.New, digester.key)
	_, _ = mac.Write([]byte(purpose))
	_, _ = mac.Write([]byte{0})
	_, _ = mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil)), nil
}

func acceptedSlotHTTPReferenceV1(
	t *testing.T,
	retained *acceptedslotfixture.RetainedSourceFixtureV1,
	canonicalValue string,
	field string,
) domaincaseentity.ReferenceV1 {
	t.Helper()
	entityType := domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1
	if field == domainevidence.AcceptedSlotSourceFieldCardV1 {
		entityType = domaincontrolledaccount.ControlledAccountFinancialFieldBankCardNumberV1
	}
	digester := acceptedSlotHTTPKeyedDigesterV1{key: []byte("accepted-slot-display-installation-key")}
	reference, err := caseentityapp.NewService(digester).DeriveReferenceV1(
		context.Background(),
		caseentityapp.NewDeriveReferenceInputV1(
			retained.HistoricalContext, entityType, canonicalValue,
		),
	)
	if err != nil {
		t.Fatal(err)
	}
	return reference
}

func acceptedSlotHTTPReopenedCaseEntityV1(
	t *testing.T,
	retained *acceptedslotfixture.RetainedSourceFixtureV1,
	canonicalValue string,
	field string,
	reference domaincaseentity.ReferenceV1,
	privateFinal domainevidence.PrivateAcceptedFinalRecord,
	disposition domainevidence.AcceptedFinalDispositionRecord,
	receipt domainevidence.EvidenceReceipt,
	claims []domainevidence.ClaimRecord,
) (*caseentityapp.Service, string, map[string]string, domainsecurity.TurnSecurityContext, *acceptedSlotHTTPCaseAuthorityV1) {
	t.Helper()
	entityType := domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1
	if field == domainevidence.AcceptedSlotSourceFieldCardV1 {
		entityType = domaincontrolledaccount.ControlledAccountFinancialFieldBankCardNumberV1
	}
	digester := acceptedSlotHTTPKeyedDigesterV1{key: []byte("accepted-slot-display-installation-key")}
	root := filepath.Join(t.TempDir(), "case-entities")
	access, err := privatecastest.NewAccessAuthority(root)
	if err != nil {
		t.Fatal(err)
	}
	store, err := caseentityadapter.NewStore(root, access)
	if err != nil {
		t.Fatal(err)
	}
	record, err := store.EnsureBinding(
		context.Background(),
		domaincaseentity.NewCaseEntityBindingRecordInputV1(
			retained.HistoricalContext, entityType, reference, canonicalValue,
		),
	)
	if err != nil || record.Reference != reference {
		t.Fatalf("durable historical caseentity binding failed: record=%#v err=%v", record, err)
	}
	eligibleSlots, err := domainevidence.BuildAcceptedEntitySlotBindingsV1(privateFinal.Envelope)
	if err != nil || len(eligibleSlots) != 1 || len(claims) == 0 ||
		privateFinal.AcceptedFinal.RecordDigest != disposition.AcceptedFinalDigest {
		t.Fatalf("longitudinal accepted-slot fixture is invalid: slots=%#v err=%v", eligibleSlots, err)
	}
	referenceCalls := 0
	if err := eligibleSlots[0].UseReferenceV1(func(candidate domaincaseentity.ReferenceV1) error {
		referenceCalls++
		if candidate != reference {
			return errors.New("accepted-slot longitudinal entity changed")
		}
		return nil
	}); err != nil || referenceCalls != 1 {
		t.Fatal("accepted-slot longitudinal entity binding is unavailable")
	}
	claimStates := make([]domaincaseentity.CaseClaimStateV1, len(claims))
	claimBindings := make([]domaincaseentity.CaseAcceptedDisplayClaimBindingV1, len(claims))
	for index, claim := range claims {
		typedState, typedErr := domaincaseentity.NewCaseClaimTypedStateV1(string(claim.ClaimType))
		if typedErr != nil || claim.SupportState != domainevidence.ClaimVerified {
			t.Fatalf("accepted-slot longitudinal claim is invalid: claim=%#v err=%v", claim, typedErr)
		}
		claimStates[index] = domaincaseentity.CaseClaimStateV1{
			ClaimReference: claim.ClaimID, ClaimDigest: claim.RecordDigest, TypedState: typedState,
			DatasetSnapshotID:         retained.HistoricalContext.DatasetSnapshotID,
			Currentness:               domaincaseentity.SnapshotCurrentV1,
			InvestigationState:        domaincaseentity.InvestigationConfirmedV1,
			EvidenceReferences:        append([]string{}, claim.EvidenceIDs...),
			CounterEvidenceReferences: append([]string{}, claim.CounterEvidenceIDs...),
		}
		claimBindings[index] = domaincaseentity.CaseAcceptedDisplayClaimBindingV1{
			ClaimReference: claim.ClaimID, ClaimDigest: claim.RecordDigest,
		}
	}
	evidenceState := domaincaseentity.CaseEvidenceStateV1{
		EvidenceReference: receipt.ReceiptID, EvidenceDigest: receipt.ReceiptDigest,
		DatasetSnapshotID: retained.HistoricalContext.DatasetSnapshotID,
		Currentness:       domaincaseentity.SnapshotCurrentV1,
	}
	displayInput := domaincaseentity.CaseAcceptedDisplayBindingInputV1{
		CaseBindingHash:     retained.HistoricalContext.CaseBindingHash,
		OriginalThreadID:    retained.HistoricalContext.ThreadID,
		OriginalTurnID:      retained.HistoricalContext.TurnID,
		AcceptedFinalDigest: privateFinal.AcceptedFinal.RecordDigest,
		DispositionDigest:   disposition.RecordDigest,
		FinalGateVersion:    privateFinal.AcceptedFinal.FinalGateVersion,
		ContextDigest:       retained.HistoricalContext.ContextDigest,
		DatasetSnapshotID:   retained.HistoricalContext.DatasetSnapshotID,
		ContextEpoch:        retained.HistoricalContext.ContextEpoch,
		EntityReference:     reference, EntityBindingDigest: record.RecordDigest,
		SlotID: eligibleSlots[0].SlotID, ClaimBindings: claimBindings,
		EvidenceReceiptBindings: []domaincaseentity.CaseAcceptedDisplayEvidenceBindingV1{{
			EvidenceReference: receipt.ReceiptID, EvidenceDigest: receipt.ReceiptDigest,
		}},
		Currentness: domaincaseentity.SnapshotCurrentV1,
	}
	displayBinding, err := domaincaseentity.NewCaseAcceptedDisplayBindingV1(displayInput)
	if err != nil {
		t.Fatal(err)
	}
	hostileBindings := make([]domaincaseentity.CaseAcceptedDisplayBindingV1, 0, 6)
	hostileBindingDigests := make(map[string]string, 6)
	for _, hostile := range []struct {
		name   string
		mutate func(*domaincaseentity.CaseAcceptedDisplayBindingInputV1)
	}{
		{name: "accepted final digest", mutate: func(input *domaincaseentity.CaseAcceptedDisplayBindingInputV1) {
			input.AcceptedFinalDigest = acceptedSlotHTTPDigestV1("hostile-accepted-final")
		}},
		{name: "original thread", mutate: func(input *domaincaseentity.CaseAcceptedDisplayBindingInputV1) {
			input.OriginalThreadID = "thread-hostile-original"
			input.SlotID = "account-slot-2"
		}},
		{name: "original turn", mutate: func(input *domaincaseentity.CaseAcceptedDisplayBindingInputV1) {
			input.OriginalTurnID = "turn-hostile-original"
			input.SlotID = "account-slot-3"
		}},
		{name: "disposition digest", mutate: func(input *domaincaseentity.CaseAcceptedDisplayBindingInputV1) {
			input.DispositionDigest = acceptedSlotHTTPDigestV1("hostile-disposition")
			input.SlotID = "account-slot-4"
		}},
		{name: "slot", mutate: func(input *domaincaseentity.CaseAcceptedDisplayBindingInputV1) {
			input.SlotID = "account-slot-5"
		}},
		{name: "entity binding digest", mutate: func(input *domaincaseentity.CaseAcceptedDisplayBindingInputV1) {
			input.EntityBindingDigest = acceptedSlotHTTPDigestV1("hostile-entity-binding")
			input.SlotID = "account-slot-6"
		}},
	} {
		candidateInput := displayInput
		candidateInput.ClaimBindings = append(
			[]domaincaseentity.CaseAcceptedDisplayClaimBindingV1(nil), displayInput.ClaimBindings...,
		)
		candidateInput.EvidenceReceiptBindings = append(
			[]domaincaseentity.CaseAcceptedDisplayEvidenceBindingV1(nil), displayInput.EvidenceReceiptBindings...,
		)
		hostile.mutate(&candidateInput)
		candidate, candidateErr := domaincaseentity.NewCaseAcceptedDisplayBindingV1(candidateInput)
		if candidateErr != nil {
			t.Fatalf("construct hostile %s binding: %v", hostile.name, candidateErr)
		}
		hostileBindings = append(hostileBindings, candidate)
		hostileBindingDigests[hostile.name] = candidate.BindingDigest
	}
	allHistoricalBindings := append(
		[]domaincaseentity.CaseAcceptedDisplayBindingV1{displayBinding}, hostileBindings...,
	)
	historicalSnapshots := []domaincaseentity.CaseSnapshotStateV1{{
		DatasetSnapshotID: retained.HistoricalContext.DatasetSnapshotID,
		ContextEpoch:      retained.HistoricalContext.ContextEpoch,
		Currentness:       domaincaseentity.SnapshotCurrentV1,
	}}
	historicalIndexInput := domaincaseentity.ThreadCaseContextRecordInputV1{
		SecurityContext: retained.HistoricalContext, Generation: 1,
		EntityReferences: []domaincaseentity.ReferenceV1{reference},
		EntityIdentities: []domaincaseentity.CaseEntityIdentityStateV1{{
			Reference: reference, EntityType: entityType, StableOrdinal: record.StableOrdinal,
		}},
		Snapshots: historicalSnapshots, Claims: claimStates,
		Evidence:               []domaincaseentity.CaseEvidenceStateV1{evidenceState},
		DisplayBindings:        allHistoricalBindings,
		OpenQuestionReferences: []string{}, DataGapReferences: []string{},
	}
	historicalIndex, err := domaincaseentity.NewCaseLongitudinalIndexRecordV1(historicalIndexInput)
	if err != nil {
		t.Fatal(err)
	}
	historicalThreadInput := historicalIndexInput
	historicalThreadInput.EntityIdentities = nil
	historicalThread, err := domaincaseentity.NewThreadCaseContextRecordV1(historicalThreadInput)
	if err != nil {
		t.Fatal(err)
	}
	activeClaims := append([]domaincaseentity.CaseClaimStateV1(nil), claimStates...)
	for index := range activeClaims {
		activeClaims[index].Currentness = domaincaseentity.SnapshotStaleV1
	}
	activeEvidence := evidenceState
	activeEvidence.Currentness = domaincaseentity.SnapshotStaleV1
	activeDisplayBindings := make([]domaincaseentity.CaseAcceptedDisplayBindingV1, len(allHistoricalBindings))
	for index, historicalBinding := range allHistoricalBindings {
		activeDisplayBindings[index] = historicalBinding
		activeDisplayBindings[index].ClaimBindings = append(
			[]domaincaseentity.CaseAcceptedDisplayClaimBindingV1(nil), historicalBinding.ClaimBindings...,
		)
		activeDisplayBindings[index].EvidenceReceiptBindings = append(
			[]domaincaseentity.CaseAcceptedDisplayEvidenceBindingV1(nil), historicalBinding.EvidenceReceiptBindings...,
		)
		activeDisplayBindings[index].Currentness = domaincaseentity.SnapshotStaleV1
	}
	activeSnapshots := []domaincaseentity.CaseSnapshotStateV1{
		{DatasetSnapshotID: retained.HistoricalContext.DatasetSnapshotID, ContextEpoch: retained.HistoricalContext.ContextEpoch, Currentness: domaincaseentity.SnapshotStaleV1},
		{DatasetSnapshotID: retained.ActiveContext.DatasetSnapshotID, ContextEpoch: retained.ActiveContext.ContextEpoch, Currentness: domaincaseentity.SnapshotCurrentV1},
	}
	activeIndexInput := domaincaseentity.ThreadCaseContextRecordInputV1{
		SecurityContext: retained.ActiveContext, Generation: 2, PreviousRecordDigest: historicalIndex.RecordDigest,
		EntityReferences: []domaincaseentity.ReferenceV1{reference},
		EntityIdentities: append([]domaincaseentity.CaseEntityIdentityStateV1(nil), historicalIndex.EntityIdentities...),
		Snapshots:        activeSnapshots, Claims: activeClaims,
		Evidence:               []domaincaseentity.CaseEvidenceStateV1{activeEvidence},
		DisplayBindings:        activeDisplayBindings,
		OpenQuestionReferences: []string{}, DataGapReferences: []string{},
	}
	activeIndex, err := domaincaseentity.NewCaseLongitudinalIndexRecordV1(activeIndexInput)
	if err != nil || domaincaseentity.ValidateThreadCaseContextEvolutionV1(historicalIndex, activeIndex) != nil {
		t.Fatalf("active longitudinal index evolution failed: %v", err)
	}
	activeThreadInput := activeIndexInput
	activeThreadInput.EntityIdentities = nil
	activeThreadInput.PreviousRecordDigest = historicalThread.RecordDigest
	activeThread, err := domaincaseentity.NewThreadCaseContextRecordV1(activeThreadInput)
	if err != nil || domaincaseentity.ValidateThreadCaseContextEvolutionV1(historicalThread, activeThread) != nil {
		t.Fatalf("active longitudinal thread evolution failed: %v", err)
	}
	forkContext := acceptedSlotHTTPForkContextV1(t, retained.ActiveContext, retained.Observation)
	forkThreadInput := activeIndexInput
	forkThreadInput.SecurityContext = forkContext
	forkThreadInput.EntityIdentities = nil
	forkThreadInput.Generation = 1
	forkThreadInput.PreviousRecordDigest = ""
	forkThread, err := domaincaseentity.NewThreadCaseContextRecordV1(forkThreadInput)
	if err != nil {
		t.Fatalf("authorized fork longitudinal thread failed: %v", err)
	}
	for _, contextRecord := range []domaincaseentity.ThreadCaseContextRecord{
		historicalIndex, historicalThread, activeIndex, activeThread, forkThread,
	} {
		if err := store.PutThreadContextIfAbsent(context.Background(), contextRecord); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := caseentityadapter.NewStore(root, access)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := reopened.Close(); err != nil {
			t.Errorf("close reopened accepted-slot caseentity store: %v", err)
		}
	})
	driftObservation, err := domainsecurity.NewCaseBindingObservationV1(
		domainsecurity.CaseBindingObservationInputV1{
			WorkspaceRealPath: retained.Observation.WorkspaceRealPath,
			State:             domainsecurity.CaseBindingStateValid,
			CaseID:            retained.Observation.CaseID,
			BindingSHA256:     acceptedSlotHTTPDigestV1("accepted-slot-binding-observation-drift"),
			CaseBindingHash:   retained.Observation.CaseBindingHash,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	caseAuthority := &acceptedSlotHTTPCaseAuthorityV1{
		active: retained.ActiveContext, observation: retained.Observation,
		fork: forkContext, driftObservation: driftObservation,
	}
	service := caseentityapp.NewPersistentService(
		digester,
		reopened,
		retained.DatasetAuthorityV2(),
		caseAuthority,
		caseAuthority.ValidateCurrent,
	)
	return service, displayBinding.BindingDigest, hostileBindingDigests, forkContext, caseAuthority
}

func acceptedSlotHTTPForkContextV1(
	t *testing.T,
	current domainsecurity.TurnSecurityContext,
	observation domainsecurity.CaseBindingObservationV1,
) domainsecurity.TurnSecurityContext {
	t.Helper()
	const threadID = "thread-accepted-slot-authorized-fork"
	policyDigest := acceptedSlotHTTPDigestV1("accepted-slot-authorized-fork-risk-policy")
	publication, err := domainsecurity.NewTurnPublicationPolicyV1(domainsecurity.TurnPublicationPolicyInputV1{
		ThreadRiskPolicyDigest: policyDigest, RiskClass: domainsecurity.RiskClassCase,
		Disposition:      domainsecurity.PublicationDispositionCaseEvidenceGate,
		CaseBindingState: domainsecurity.CaseBindingStateValid, BindingObservationDigest: observation.ObservationDigest,
		BlockerCode: domainsecurity.PublicationBlockerNone,
	})
	if err != nil {
		t.Fatal(err)
	}
	riskBinding, err := securitycontexttest.WitnessedRiskBinding(
		threadID, current.WorkspaceRealPath, domainsecurity.RiskClassCase, policyDigest,
	)
	if err != nil {
		t.Fatal(err)
	}
	fork, err := domainsecurity.NewTurnSecurityContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: threadID, TurnID: "turn-accepted-slot-authorized-fork", WorkspaceRealPath: current.WorkspaceRealPath,
		TenantID: current.TenantID, UserID: current.UserID, CaseID: current.CaseID, CaseBindingHash: current.CaseBindingHash,
		DatasetSnapshotID: current.DatasetSnapshotID, SourceManifestHash: current.SourceManifestHash,
		ContextEpoch: current.ContextEpoch, IssuedAt: time.Date(2026, 8, 21, 1, 15, 0, 0, time.UTC),
		PublicationPolicy: publication, RiskAuthorityBinding: riskBinding,
	})
	if err != nil {
		t.Fatal(err)
	}
	return fork
}

func equalStringSlicesV1(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func acceptedSlotHTTPUnavailableResponseV1(recorder *httptest.ResponseRecorder) bool {
	return recorder != nil && recorder.Code == http.StatusConflict &&
		recorder.Body.String() == "{\"code\":\"conflict\",\"message\":\"The request conflicts with the current runtime state.\"}\n"
}

func acceptedSlotHTTPFactsAndBindingV1(
	t *testing.T,
	base domainevidence.AcceptedSlotSourceBindingV1,
) ([]domainevidence.CanonicalEvidenceFact, domainevidence.AcceptedSlotSourceBindingV1) {
	t.Helper()
	const (
		start = "2026-01-01T00:00:00Z"
		end   = "2026-01-31T23:59:59Z"
	)
	reference := base.EntityReference
	facts := []domainevidence.CanonicalEvidenceFact{
		{FactID: "fact_aggregate_" + acceptedSlotHTTPDigestV1("accepted-slot-http-in"), ClaimType: domainevidence.ClaimAmount,
			NormalizedPayload: domainevidence.NormalizedClaimPayload{SubjectID: reference, EntityID: reference, AccountID: reference,
				AmountMinor: "100", Currency: "CNY", Direction: "in", StartAt: start, EndAt: end, Granularity: "aggregate"}},
		{FactID: "fact_aggregate_" + acceptedSlotHTTPDigestV1("accepted-slot-http-out"), ClaimType: domainevidence.ClaimAmount,
			NormalizedPayload: domainevidence.NormalizedClaimPayload{SubjectID: reference, EntityID: reference, AccountID: reference,
				AmountMinor: "25", Currency: "CNY", Direction: "out", StartAt: start, EndAt: end, Granularity: "aggregate"}},
		{FactID: "fact_aggregate_" + acceptedSlotHTTPDigestV1("accepted-slot-http-count"), ClaimType: domainevidence.ClaimCount,
			NormalizedPayload: domainevidence.NormalizedClaimPayload{SubjectID: reference, EntityID: reference,
				Count: "2", StartAt: start, EndAt: end, Granularity: "aggregate"}},
	}
	input := domainevidence.AcceptedSlotSourceBindingInputV1{
		FactIDs: make([]string, len(facts)), EntityReference: base.EntityReference,
		SourceRecordID: base.SourceRecordID, SourceFileID: base.SourceFileID,
		SourceRowNumber: base.SourceRowNumber, Field: base.Field,
	}
	for index := range facts {
		input.FactIDs[index] = facts[index].FactID
	}
	binding, err := domainevidence.NewAcceptedSlotSourceBindingV1(input)
	if err != nil {
		t.Fatal(err)
	}
	return facts, binding
}

func acceptedSlotHTTPSettlementV1(
	t *testing.T,
	securityContext domainsecurity.TurnSecurityContext,
	facts []domainevidence.CanonicalEvidenceFact,
	binding domainevidence.AcceptedSlotSourceBindingV1,
) (domainevidence.EvidenceReceiptRegistry, domainevidence.EvidenceReceipt) {
	t.Helper()
	bindingDigest, err := domainevidence.AcceptedSlotSourceBindingSetDigestV1([]domainevidence.AcceptedSlotSourceBindingV1{binding})
	if err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(domainevidence.CanonicalEvidenceMaterial{
		SchemaVersion: domainevidence.CanonicalEvidenceVersionV3, Purpose: domainevidence.CanonicalEvidencePurposeV3,
		Facts: facts, AcceptedSlotSourceBindings: []domainevidence.AcceptedSlotSourceBindingV1{binding},
		AcceptedSlotSourceBindingSetDigest: bindingDigest,
	})
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := domainevidence.CanonicalEvidenceBytes(body)
	if err != nil {
		t.Fatal(err)
	}
	settlementID := acceptedSlotHTTPDigestV1("accepted-slot-http-settlement")
	identity, err := domainsecurity.NewVerifiedMCPServerIdentity(
		"analytix_funds", "analytix-fund-analysis", "1.0.0", acceptedSlotHTTPDigestV1("accepted-slot-http-server"), 1,
	)
	if err != nil {
		t.Fatal(err)
	}
	entropy := sha256.Sum256([]byte("accepted-slot-http-tool-call"))
	toolCallID, err := domainsecurity.NewHostToolCallIDV1(entropy[:])
	if err != nil {
		t.Fatal(err)
	}
	rawHash := acceptedSlotHTTPDigestV1("accepted-slot-http-raw")
	nativeHash := acceptedSlotHTTPDigestV1("accepted-slot-http-native")
	canonicalHash := domainsecurity.CanonicalJSONHash(canonical)
	draft, err := domainevidence.NewEvidenceReceiptDraft(domainevidence.EvidenceReceiptInput{
		ReceiptID: domainevidence.EvidenceSettlementReceiptID(settlementID), Context: securityContext,
		ExecutionGrantID: acceptedSlotHTTPDigestV1("accepted-slot-http-grant"), ToolCallID: toolCallID,
		ServerIdentity: identity, ServerVersion: "1.0.0", ConnectionEpoch: 1,
		ToolName: "mcp__analytix_funds__analyze_account_flows", ArgsHash: acceptedSlotHTTPDigestV1("accepted-slot-http-args"),
		ResultHash: canonicalHash, SourceType: "transactions", DatasetSnapshotID: securityContext.DatasetSnapshotID,
		QueryHash: acceptedSlotHTTPDigestV1("accepted-slot-http-query"), QueryRange: domainevidence.EvidenceQueryRange{
			EntityIDs: []string{binding.EntityReference}, AccountIDs: []string{binding.EntityReference}, Directions: []string{"in", "out"},
			StartAt: "2026-01-01T00:00:00Z", EndAt: "2026-01-31T23:59:59Z",
			SourceIDs: []string{"qscope1_" + domainsecurity.SHA256Hex([]byte(
				securityContext.ContextDigest+"\x00"+acceptedSlotHTTPDigestV1("accepted-slot-http-query"),
			))},
			FiltersHash: acceptedSlotHTTPDigestV1("accepted-slot-http-filters"),
		},
		Granularity: "aggregate", Currency: "CNY", Timezone: "Z", PaginationCompleteness: domainevidence.PaginationComplete,
		SourceRecordIDs: []string{binding.SourceRecordID}, RawSHA256: rawHash,
		TransformationLineage: []domainevidence.TransformationLineageStep{
			{StepID: "bind-account-flow-native-result-v1", Transformer: "analytix-host-account-flow-oracle-binder", TransformerVersion: "1", InputHash: rawHash, OutputHash: nativeHash},
			{StepID: "normalize-account-flow-v1", Transformer: "analytix-host-account-flow-normalizer", TransformerVersion: "1", InputHash: nativeHash, OutputHash: canonicalHash},
		},
		PIIClassification: domainevidence.PIINone,
		IssuedAt:          time.Date(2026, 8, 21, 2, 20, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	registry, err := domainevidence.NewEvidenceReceiptRegistry(securityContext)
	if err != nil {
		t.Fatal(err)
	}
	registry, receipt, err := domainevidence.RegisterEvidenceReceipt(
		registry, draft, canonical,
		domainevidence.EvidenceSettlementProof{SettlementID: settlementID, PreparedRecordDigest: acceptedSlotHTTPDigestV1("accepted-slot-http-prepared")},
		time.Date(2026, 8, 21, 2, 21, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatal(err)
	}
	return registry, receipt
}

func acceptedSlotHTTPVerifiedClaimsV1(
	t *testing.T,
	registry *acceptedSlotHTTPRegistryV1,
	securityContext domainsecurity.TurnSecurityContext,
	receipt domainevidence.EvidenceReceipt,
	facts []domainevidence.CanonicalEvidenceFact,
) []domainevidence.ClaimRecord {
	t.Helper()
	claimIDs := []string{"claim-accepted-slot-in", "claim-accepted-slot-out", "claim-accepted-slot-count"}
	claimIndex := 0
	verifier := evidenceapp.ClaimVerifier{
		Registry: registry,
		IDGenerator: func() (string, error) {
			claimID := claimIDs[claimIndex]
			claimIndex++
			return claimID, nil
		},
		Now: func() time.Time { return time.Date(2026, 8, 21, 2, 22, 0, 0, time.UTC) },
	}
	claims := make([]domainevidence.ClaimRecord, len(facts))
	for index := range facts {
		claim, err := verifier.Verify(context.Background(), securityContext, domainevidence.ClaimProposal{
			SchemaVersion: domainevidence.ClaimProposalVersion, ProposalID: "proposal-" + claimIDs[index],
			ClaimType: facts[index].ClaimType, NormalizedPayload: facts[index].NormalizedPayload,
			EvidenceIDs: []string{receipt.ReceiptID}, CounterEvidenceIDs: []string{},
		})
		if err != nil || claim.SupportState != domainevidence.ClaimVerified {
			t.Fatalf("accepted-slot claim %d was not verified: claim=%#v err=%v", index, claim, err)
		}
		claims[index] = claim
	}
	return claims
}

func acceptedSlotHTTPDigestV1(value string) string {
	return domainsecurity.SHA256Hex([]byte(value))
}

func acceptedSlotHTTPEvolvedContextV1(
	t *testing.T,
	current domainsecurity.TurnSecurityContext,
	mutate func(*domainsecurity.TurnSecurityContextInput),
) domainsecurity.TurnSecurityContext {
	t.Helper()
	issuedAt, err := time.Parse(time.RFC3339Nano, current.IssuedAt)
	if err != nil {
		t.Fatal(err)
	}
	input := domainsecurity.TurnSecurityContextInput{
		ThreadID: current.ThreadID, TurnID: current.TurnID, WorkspaceRealPath: current.WorkspaceRealPath,
		TenantID: current.TenantID, UserID: current.UserID, CaseID: current.CaseID,
		CaseBindingHash: current.CaseBindingHash, DatasetSnapshotID: current.DatasetSnapshotID,
		SourceManifestHash: current.SourceManifestHash, ContextEpoch: current.ContextEpoch, IssuedAt: issuedAt,
		PublicationPolicy: current.PublicationPolicy, RiskAuthorityBinding: current.RiskAuthorityBinding,
	}
	mutate(&input)
	evolved, err := domainsecurity.NewTurnSecurityContextV2(input)
	if err != nil {
		t.Fatal(err)
	}
	return evolved
}
