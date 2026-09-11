package localdisplay

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	caseentityapp "analytix.local/runtime-go/internal/app/caseentity"
	domaincaseentity "analytix.local/runtime-go/internal/domain/caseentity"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainfundsquerysource "analytix.local/runtime-go/internal/domain/fundsquerysource"
	domainidentity "analytix.local/runtime-go/internal/domain/identity"
	domainnative "analytix.local/runtime-go/internal/domain/nativecomponent"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	registryport "analytix.local/runtime-go/internal/ports/evidenceregistry"
	fundsquerysourceport "analytix.local/runtime-go/internal/ports/fundsquerysource"
	datasetsnapshotfixture "analytix.local/runtime-go/internal/testsupport/datasetsnapshotv2fixture"
	fundsquerysourcefixture "analytix.local/runtime-go/internal/testsupport/fundsquerysourcefixture"
	securitycontexttest "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

type localDisplayCaseEntityStubV1 struct {
	exact     string
	digest    string
	err       error
	calls     int
	lastInput caseentityapp.ResolveVerifiedBindingByReferenceInputV1
}

type localDisplayRetainedCaseEntityStubV1 struct {
	localDisplayCaseEntityStubV1
	retainedCalls int
	lastRetained  caseentityapp.UseRetainedBindingByReferenceInputV1
}

type localDisplayAcceptedEvidenceStubV1 struct {
	registry     domainevidence.EvidenceReceiptRegistry
	resolveErr   error
	replayErr    error
	resolveCalls int
	replayCalls  int
}

func (stub *localDisplayAcceptedEvidenceStubV1) Resolve(
	_ context.Context,
	query registryport.MembershipQuery,
) (domainevidence.RegisteredEvidence, error) {
	stub.resolveCalls++
	if stub.resolveErr != nil {
		return domainevidence.RegisteredEvidence{}, stub.resolveErr
	}
	return domainevidence.VerifyEvidenceReceiptMembership(stub.registry, query.Context, query.ReceiptID)
}

func (stub *localDisplayAcceptedEvidenceStubV1) ReplayAt(
	_ context.Context,
	securityContext domainsecurity.TurnSecurityContext,
	sequence uint64,
) (domainevidence.EvidenceReceiptRegistry, error) {
	stub.replayCalls++
	if stub.replayErr != nil {
		return domainevidence.EvidenceReceiptRegistry{}, stub.replayErr
	}
	if sequence != stub.registry.Sequence ||
		!domainevidence.EvidenceReceiptRegistryMatchesContext(stub.registry, securityContext) {
		return domainevidence.EvidenceReceiptRegistry{}, errors.New("accepted display registry replay mismatch")
	}
	return stub.registry, nil
}

func (stub *localDisplayRetainedCaseEntityStubV1) UseRetainedBindingByReferenceV1(
	_ context.Context,
	input caseentityapp.UseRetainedBindingByReferenceInputV1,
	use func(string, string) error,
) error {
	stub.retainedCalls++
	stub.lastRetained = input
	if stub.err != nil {
		return stub.err
	}
	return use(stub.exact, stub.digest)
}

func (stub *localDisplayCaseEntityStubV1) UseVerifiedBindingByReferenceV1(
	_ context.Context,
	input caseentityapp.ResolveVerifiedBindingByReferenceInputV1,
	use func(string, string) error,
) error {
	stub.calls++
	stub.lastInput = input
	if stub.err != nil {
		return stub.err
	}
	return use(stub.exact, stub.digest)
}

func TestDirectSourcePreviewProjectsOnlyAfterCurrentIdentityChecks(t *testing.T) {
	workspaceRoot := t.TempDir()
	identity := newLocalDisplayIdentityStubV1(t)
	runner := &localDisplayPreviewRunnerStubV1{
		result: localDisplayNativeResultV1(t, localDisplayDirectDescriptorV1(t), []string{"account", "amountText"}),
	}
	currentCalls := 0
	descriptor := localDisplayDirectDescriptorV1(t)
	var responseForShape DirectSourcePreviewResponseV1
	service := NewServiceWithDirectSourcePreview(nil, nil, DirectSourcePreviewDependenciesV1{
		Identity: identity,
		UseCurrentLocalDisplay: func(
			ctx context.Context,
			workspace, tenant, user string,
			use func(context.Context, domainfundsquerysource.DescriptorV1, fundsquerysourceport.ExactReadLease) error,
		) error {
			currentCalls++
			if workspace != workspaceRoot || tenant != identity.principal.TenantID || user != identity.principal.UserID {
				return errors.New("current identity binding mismatch")
			}
			return use(ctx, descriptor, fundsquerysourcefixture.NewExactReadLeaseV1(nil))
		},
		Runner: runner,
	})

	for _, test := range []struct {
		mode    string
		account string
		amount  string
	}{
		{mode: DisplayModeFull, account: "synthetic-account-1234", amount: "12.00"},
		{mode: DisplayModeMasked, account: "****1234", amount: "12.00"},
	} {
		response, err := service.DirectSourcePreview(context.Background(), DirectSourcePreviewInputV1{
			WorkspaceRoot: workspaceRoot, View: DisplayViewTransactions,
			Fields: []string{"account", "amountText"}, RowOffset: 0, RowLimit: 25,
			DisplayMode: test.mode,
		})
		if err != nil || response.SchemaVersion != ResponseSchemaVersionV1 ||
			response.Kind != KindDirectSourcePreview || response.DisplayMode != test.mode ||
			response.CaseID != descriptor.CaseID || response.DatasetSnapshotID != descriptor.DatasetSnapshotID ||
			len(response.Rows) != 1 || len(response.Rows[0].Cells) != 2 ||
			response.Rows[0].Cells[0].DisplayValue != test.account ||
			response.Rows[0].Cells[1].DisplayValue != test.amount {
			t.Fatalf("direct preview projection failed for mode %s", test.mode)
		}
		responseForShape = response
	}
	if identity.resolveCalls != 2 || identity.validateCalls != 4 || currentCalls != 2 || runner.calls != 2 {
		t.Fatalf("direct preview authority call counts are invalid")
	}
	body, err := json.Marshal(responseForShape)
	for _, forbidden := range [][]byte{
		[]byte("threadId"), []byte("turnId"), []byte("contextEpoch"), []byte("entityRef"),
		[]byte("slots"), []byte("queryHash"), []byte("resultHash"), []byte("claims"),
		[]byte("receipts"),
	} {
		if err != nil || bytes.Contains(body, forbidden) {
			t.Fatal("direct response shape admitted a forbidden identity or publication field")
		}
	}
}

func TestDirectSourcePreviewFailsClosedOnCurrentSnapshotOrIdentityDrift(t *testing.T) {
	identity := newLocalDisplayIdentityStubV1(t)
	descriptor := localDisplayDirectDescriptorV1(t)
	runner := &localDisplayPreviewRunnerStubV1{
		result: localDisplayNativeResultV1(t, descriptor, []string{"account"}),
	}
	for name, sourceErr := range map[string]error{
		"snapshot": errors.New("snapshot unavailable"),
		"source":   errors.New("source unavailable"),
	} {
		t.Run(name, func(t *testing.T) {
			workspaceRoot := t.TempDir()
			identity := newLocalDisplayIdentityStubV1(t)
			service := NewServiceWithDirectSourcePreview(nil, nil, DirectSourcePreviewDependenciesV1{
				Identity: identity,
				UseCurrentLocalDisplay: func(
					context.Context,
					string,
					string,
					string,
					func(context.Context, domainfundsquerysource.DescriptorV1, fundsquerysourceport.ExactReadLease) error,
				) error {
					return sourceErr
				},
				Runner: runner,
			})
			response, err := service.DirectSourcePreview(context.Background(), DirectSourcePreviewInputV1{
				WorkspaceRoot: workspaceRoot, View: DisplayViewTransactions,
				Fields: []string{"account"}, RowLimit: 25, DisplayMode: DisplayModeFull,
			})
			if response.SchemaVersion != 0 || !errors.Is(err, ErrUnavailable) || runner.calls != 0 {
				t.Fatal("direct preview did not fail closed before projecting source data")
			}
		})
	}

	identity = newLocalDisplayIdentityStubV1(t)
	workspaceRoot := t.TempDir()
	identity.postValidateErr = errors.New("identity changed")
	service := NewServiceWithDirectSourcePreview(nil, nil, DirectSourcePreviewDependenciesV1{
		Identity: identity,
		UseCurrentLocalDisplay: func(
			ctx context.Context,
			_ string,
			_ string,
			_ string,
			use func(context.Context, domainfundsquerysource.DescriptorV1, fundsquerysourceport.ExactReadLease) error,
		) error {
			return use(ctx, descriptor, fundsquerysourcefixture.NewExactReadLeaseV1(nil))
		},
		Runner: &localDisplayPreviewRunnerStubV1{result: localDisplayNativeResultV1(t, descriptor, []string{"account"})},
	})
	_, err := service.DirectSourcePreview(context.Background(), DirectSourcePreviewInputV1{
		WorkspaceRoot: workspaceRoot, View: DisplayViewTransactions,
		Fields: []string{"account"}, RowLimit: 25, DisplayMode: DisplayModeFull,
	})
	if !errors.Is(err, ErrUnavailable) {
		t.Fatal("identity post-validation drift was not fail-closed")
	}
}

func TestValidateDirectSourcePreviewResultRejectsRowIndexAboveMaximum(t *testing.T) {
	descriptor := localDisplayDirectDescriptorV1(t)
	fields := []domainnative.DirectSourcePreviewFieldV1{domainnative.DirectSourcePreviewFieldAccountV1}
	arguments, err := domainnative.NewDirectSourcePreviewArgumentsV1(
		descriptor,
		fields,
		domainnative.DirectSourcePreviewMaximumOffsetV1,
		2,
	)
	if err != nil {
		t.Fatal(err)
	}
	result := localDisplayNativeResultV1(t, descriptor, []string{"account"})
	result.RowOffset = arguments.RowOffset
	result.RowLimit = arguments.RowLimit
	result.Rows = []domainnative.DirectSourcePreviewRowV1{
		{RowIndex: domainnative.DirectSourcePreviewMaximumOffsetV1, Cells: result.Rows[0].Cells},
		{RowIndex: domainnative.DirectSourcePreviewMaximumOffsetV1 + 1, Cells: result.Rows[0].Cells},
	}

	if err := validateDirectSourcePreviewResultV1(result, arguments, descriptor); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("app seam accepted row index above maximum: %v", err)
	}
}

func TestAcceptedEntitySlotsUseOneHistoricalBindingForFullAndMaskedProjection(t *testing.T) {
	workspace := t.TempDir()
	bindingHash := strings.Repeat("d", 64)
	active, err := securitycontexttest.CaseExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-retained-display", TurnID: "turn-current", WorkspaceRealPath: workspace,
		TenantID: "tenant-a", UserID: "user-a", CaseID: "case-retained-display", CaseBindingHash: bindingHash,
		DatasetSnapshotID:  securitycontexttest.DatasetSnapshotID("display-current"),
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("display-current-manifest")),
		ContextEpoch:       2, IssuedAt: time.Unix(1_750_000_010, 0).UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	historical, err := securitycontexttest.CaseExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-retained-display", TurnID: "turn-historical", WorkspaceRealPath: workspace,
		TenantID: "tenant-a", UserID: "user-a", CaseID: "case-retained-display", CaseBindingHash: bindingHash,
		DatasetSnapshotID:  securitycontexttest.DatasetSnapshotID("display-historical"),
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("display-historical-manifest")),
		ContextEpoch:       1, IssuedAt: time.Unix(1_750_000_009, 0).UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	reference, err := domaincaseentity.NewReferenceV1FromKeyedDigest(strings.Repeat("e", 64))
	if err != nil {
		t.Fatal(err)
	}
	const (
		originalExactSentinel = "6222 0212-3456 7890"
		canonicalSentinel     = "6222021234567890"
		currentSentinel       = "CURRENT-SNAPSHOT-9999"
		labelSentinel         = "ACCOUNT-LABEL-8888"
	)
	envelope, evidenceStub, expectedSourceBindings := localDisplayAcceptedRetainedFixtureV1(
		t, historical, string(reference), canonicalSentinel,
	)
	resolver := &localDisplayRetainedCaseEntityStubV1{
		localDisplayCaseEntityStubV1: localDisplayCaseEntityStubV1{
			exact:  canonicalSentinel,
			digest: domainsecurity.SHA256Hex([]byte(labelSentinel)),
		},
	}
	type sourceCallV1 struct {
		active, historical domainsecurity.TurnSecurityContext
		bindings           []domainevidence.AcceptedSlotSourceBindingV1
		expectedCanonical  string
	}
	sourceCalls := make([]sourceCallV1, 0, 2)
	useRetainedSource := func(
		_ context.Context,
		gotActive domainsecurity.TurnSecurityContext,
		gotHistorical domainsecurity.TurnSecurityContext,
		gotBindings []domainevidence.AcceptedSlotSourceBindingV1,
		expectedCanonical string,
		use func(string) error,
	) error {
		sourceCalls = append(sourceCalls, sourceCallV1{
			active: gotActive, historical: gotHistorical,
			bindings:          append([]domainevidence.AcceptedSlotSourceBindingV1(nil), gotBindings...),
			expectedCanonical: expectedCanonical,
		})
		if gotActive != active || gotHistorical != historical || expectedCanonical != canonicalSentinel ||
			expectedCanonical == currentSentinel || expectedCanonical == labelSentinel ||
			!reflect.DeepEqual(gotBindings, expectedSourceBindings) {
			return errors.New("retained source callback authority mismatch")
		}
		return use(originalExactSentinel)
	}
	var fullSlots []SlotV1
	for _, test := range []struct {
		mode string
		want string
	}{
		{mode: DisplayModeFull, want: originalExactSentinel},
		{mode: DisplayModeMasked, want: "****7890"},
	} {
		slots, err := resolveAcceptedEntitySlotsRetainedV1(
			context.Background(), resolver, evidenceStub, useRetainedSource,
			active, historical, envelope, test.mode,
		)
		if err != nil || len(slots) != 1 || slots[0].DisplayValue != test.want ||
			slots[0].DisplayValue == canonicalSentinel || slots[0].DisplayValue == currentSentinel ||
			slots[0].DisplayValue == labelSentinel {
			t.Fatalf("retained accepted slot projection = %#v err=%v", slots, err)
		}
		if test.mode == DisplayModeFull {
			fullSlots = slots
		} else if !reflect.DeepEqual(fullSlots[0].ClaimIDs, slots[0].ClaimIDs) ||
			!reflect.DeepEqual(fullSlots[0].ReceiptIDs, slots[0].ReceiptIDs) {
			t.Fatal("full and masked projections changed accepted evidence artifacts")
		}
	}
	if resolver.retainedCalls != 2 || resolver.lastRetained.ActiveSecurityContext != active ||
		resolver.lastRetained.HistoricalSecurityContext != historical || resolver.lastRetained.Reference != reference ||
		evidenceStub.resolveCalls != 2 || len(sourceCalls) != 2 ||
		!reflect.DeepEqual(sourceCalls[0].bindings, sourceCalls[1].bindings) {
		t.Fatalf("retained display binding contexts drifted: calls=%d input=%#v", resolver.retainedCalls, resolver.lastRetained)
	}
	projected, err := json.Marshal(fullSlots)
	if err != nil || !bytes.Contains(projected, []byte(originalExactSentinel)) {
		t.Fatalf("typed local response lost its source-exact positive: %s err=%v", projected, err)
	}
	for _, forbidden := range []string{
		expectedSourceBindings[0].SourceFileID,
		expectedSourceBindings[0].SourceRecordID,
		string(reference),
		"sourceRowNumber",
		"acceptedSlotSourceBindings",
		canonicalSentinel,
		currentSentinel,
		labelSentinel,
	} {
		if bytes.Contains(projected, []byte(forbidden)) {
			t.Fatalf("typed local response leaked private lineage or fallback %q: %s", forbidden, projected)
		}
	}
	oversizedSource := func(
		_ context.Context,
		_ domainsecurity.TurnSecurityContext,
		_ domainsecurity.TurnSecurityContext,
		_ []domainevidence.AcceptedSlotSourceBindingV1,
		_ string,
		use func(string) error,
	) error {
		return use(strings.Repeat("💠", 1_025))
	}
	if slots, err := resolveAcceptedEntitySlotsRetainedV1(
		context.Background(), resolver, evidenceStub, oversizedSource,
		active, historical, envelope, DisplayModeFull,
	); err == nil || len(slots) != 0 {
		t.Fatal("accepted-slot display admitted a cell beyond the canonical UTF-8 byte limit")
	}
}

func TestAcceptedEntitySlotsRetainedFailsClosedWithoutSourceExactFallback(t *testing.T) {
	securityContext := localDisplayCaseContextV1(t)
	reference, err := domaincaseentity.NewReferenceV1FromKeyedDigest(strings.Repeat("8", 64))
	if err != nil {
		t.Fatal(err)
	}
	const canonical = "6222021234567890"
	envelope, evidenceStub, _ := localDisplayAcceptedRetainedFixtureV1(
		t, securityContext, string(reference), canonical,
	)
	resolver := &localDisplayRetainedCaseEntityStubV1{
		localDisplayCaseEntityStubV1: localDisplayCaseEntityStubV1{
			exact: canonical, digest: domainsecurity.SHA256Hex([]byte("retained-negative-label")),
		},
	}

	tests := []struct {
		name       string
		envelope   domainevidence.FinalAnswerEnvelope
		evidence   *localDisplayAcceptedEvidenceStubV1
		caseErr    error
		sourceErr  error
		wantSource int
	}{
		{
			name: "claim fact mismatch",
			envelope: localDisplayAcceptedEnvelopeForReceiptAndCountV1(
				t, securityContext, string(reference), envelope.EvidenceReceiptIDs[0], "2",
			),
			evidence: evidenceStub,
		},
		{
			name: "receipt mismatch",
			envelope: localDisplayAcceptedEnvelopeForReceiptV1(
				t, securityContext, string(reference), "receipt-other-local-display",
			),
			evidence: evidenceStub,
		},
		{
			name: "case binding unavailable", envelope: envelope, evidence: evidenceStub,
			caseErr: errors.New("retained case binding unavailable"),
		},
		{
			name: "retained source unavailable", envelope: envelope, evidence: evidenceStub,
			sourceErr: errors.New("retained source unavailable"), wantSource: 1,
		},
	}
	revoked, err := domainevidence.RevokeEvidenceReceipt(
		evidenceStub.registry, envelope.EvidenceReceiptIDs[0], "source_retracted",
		time.Unix(1_750_000_020, 0).UTC(),
	)
	if err != nil {
		t.Fatal(err)
	}
	tests = append(tests, struct {
		name       string
		envelope   domainevidence.FinalAnswerEnvelope
		evidence   *localDisplayAcceptedEvidenceStubV1
		caseErr    error
		sourceErr  error
		wantSource int
	}{
		name: "revoked receipt", envelope: envelope,
		evidence: &localDisplayAcceptedEvidenceStubV1{registry: revoked},
	})

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidateResolver := *resolver
			candidateResolver.err = test.caseErr
			sourceCalls := 0
			exactCallbacks := 0
			slots, err := resolveAcceptedEntitySlotsRetainedV1(
				context.Background(), &candidateResolver, test.evidence,
				func(
					context.Context,
					domainsecurity.TurnSecurityContext,
					domainsecurity.TurnSecurityContext,
					[]domainevidence.AcceptedSlotSourceBindingV1,
					string,
					func(string) error,
				) error {
					sourceCalls++
					return test.sourceErr
				},
				securityContext, securityContext, test.envelope, DisplayModeFull,
			)
			if !errors.Is(err, ErrUnavailable) || len(slots) != 0 || sourceCalls != test.wantSource || exactCallbacks != 0 {
				t.Fatalf("closed retained display projected fallback bytes: slots=%#v source=%d exact=%d err=%v", slots, sourceCalls, exactCallbacks, err)
			}
		})
	}
}

func TestAcceptedEntitySlotsRetainedReclosesTypedOutcomeAgainstReceiptAndV3Binding(t *testing.T) {
	securityContext := localDisplayCaseContextV1(t)
	reference, err := domaincaseentity.NewReferenceV1FromKeyedDigest(strings.Repeat("7", 64))
	if err != nil {
		t.Fatal(err)
	}
	const canonical = "6222021234567890"
	baseEnvelope, baseEvidence, _ := localDisplayAcceptedRetainedFixtureV1(
		t, securityContext, string(reference), canonical,
	)
	type candidateV1 struct {
		envelope domainevidence.FinalAnswerEnvelope
		evidence *localDisplayAcceptedEvidenceStubV1
	}
	candidates := map[string]candidateV1{}

	wrongQueryScopeEnvelope, wrongQueryScopeEvidence, _ := localDisplayAcceptedRetainedFixtureWithReceiptMutationV1(
		t, securityContext, string(reference), canonical,
		func(input *domainevidence.EvidenceReceiptInput) {
			input.QueryRange.SourceIDs = []string{
				"qscope1_" + domainsecurity.SHA256Hex([]byte("local-display-other-query-scope")),
			}
		},
	)
	candidates["wrong receipt qscope"] = candidateV1{wrongQueryScopeEnvelope, wrongQueryScopeEvidence}

	partialEnvelope, partialEvidence, _ := localDisplayAcceptedRetainedFixtureWithReceiptMutationV1(
		t, securityContext, string(reference), canonical,
		func(input *domainevidence.EvidenceReceiptInput) {
			input.PaginationCompleteness = domainevidence.PaginationPartial
		},
	)
	candidates["partial pagination"] = candidateV1{partialEnvelope, partialEvidence}

	legacyEnvelope, legacyEvidence, _ := localDisplayAcceptedRetainedFixtureWithReceiptMutationV1(
		t, securityContext, string(reference), canonical,
		func(input *domainevidence.EvidenceReceiptInput) {
			input.TransformationLineage = []domainevidence.TransformationLineageStep{{
				StepID: "legacy-normalize-account-flow", Transformer: "legacy-account-flow-normalizer", TransformerVersion: "1",
				InputHash: input.RawSHA256, OutputHash: input.ResultHash,
			}}
		},
	)
	candidates["legacy one-step lineage"] = candidateV1{legacyEnvelope, legacyEvidence}

	candidates["wrong group query hash with recomputed afslot"] = candidateV1{
		envelope: rebuildLocalDisplayAccountFlowGroupV1(t, securityContext, baseEnvelope, func(group *domainevidence.AccountFlowTypedAnswerGroupV1) {
			group.QueryHash = domainsecurity.SHA256Hex([]byte("local-display-other-query-hash"))
		}),
		evidence: baseEvidence,
	}
	candidates["wrong group result hash with recomputed afslot"] = candidateV1{
		envelope: rebuildLocalDisplayAccountFlowGroupV1(t, securityContext, baseEnvelope, func(group *domainevidence.AccountFlowTypedAnswerGroupV1) {
			group.ResultHash = domainsecurity.SHA256Hex([]byte("local-display-other-native-result"))
		}),
		evidence: baseEvidence,
	}
	candidates["wrong typed source field with recomputed afslot"] = candidateV1{
		envelope: rebuildLocalDisplayAccountFlowGroupV1(t, securityContext, baseEnvelope, func(group *domainevidence.AccountFlowTypedAnswerGroupV1) {
			group.SourceFieldReference.Field = domainevidence.AcceptedSlotSourceFieldCardV1
		}),
		evidence: baseEvidence,
	}

	for name, candidate := range candidates {
		t.Run(name, func(t *testing.T) {
			resolver := &localDisplayRetainedCaseEntityStubV1{
				localDisplayCaseEntityStubV1: localDisplayCaseEntityStubV1{
					exact: canonical, digest: domainsecurity.SHA256Hex([]byte("typed-outcome-reclose-label")),
				},
			}
			sourceCallbacks := 0
			slots, err := resolveAcceptedEntitySlotsRetainedV1(
				context.Background(), resolver, candidate.evidence,
				func(
					context.Context,
					domainsecurity.TurnSecurityContext,
					domainsecurity.TurnSecurityContext,
					[]domainevidence.AcceptedSlotSourceBindingV1,
					string,
					func(string) error,
				) error {
					sourceCallbacks++
					return errors.New("forged typed outcome reached the source-exact callback")
				},
				securityContext, securityContext, candidate.envelope, DisplayModeFull,
			)
			if !errors.Is(err, ErrUnavailable) || len(slots) != 0 || sourceCallbacks != 0 || resolver.retainedCalls != 0 {
				t.Fatalf("forged typed outcome reached exact display: slots=%#v source=%d retained=%d err=%v", slots, sourceCallbacks, resolver.retainedCalls, err)
			}
		})
	}
}

type localDisplayIdentityStubV1 struct {
	principal       domainidentity.PrincipalV1
	resolveErr      error
	preValidateErr  error
	postValidateErr error
	resolveCalls    int
	validateCalls   int
}

func newLocalDisplayIdentityStubV1(t *testing.T) *localDisplayIdentityStubV1 {
	t.Helper()
	principal, err := domainidentity.NewPrincipalV1(
		domainsecurity.SHA256Hex([]byte("local-display-installation")),
		"tenant-local-display", "user-local-display",
	)
	if err != nil {
		t.Fatal(err)
	}
	return &localDisplayIdentityStubV1{principal: principal}
}

func (stub *localDisplayIdentityStubV1) ResolveCurrent(context.Context) (domainidentity.PrincipalV1, error) {
	stub.resolveCalls++
	return stub.principal, stub.resolveErr
}

func (stub *localDisplayIdentityStubV1) ValidateCurrent(context.Context, domainidentity.PrincipalV1) error {
	stub.validateCalls++
	if stub.validateCalls > 1 && stub.postValidateErr != nil {
		return stub.postValidateErr
	}
	return stub.preValidateErr
}

type localDisplayPreviewRunnerStubV1 struct {
	result domainnative.DirectSourcePreviewResultV1
	err    error
	calls  int
}

func (stub *localDisplayPreviewRunnerStubV1) DirectSourcePreview(
	context.Context,
	domainnative.DirectSourcePreviewArgumentsV1,
	domainfundsquerysource.DescriptorV1,
	fundsquerysourceport.ExactReadLease,
) (domainnative.DirectSourcePreviewResultV1, error) {
	stub.calls++
	return stub.result, stub.err
}

func localDisplayDirectDescriptorV1(t *testing.T) domainfundsquerysource.DescriptorV1 {
	t.Helper()
	digest := func(material string) string {
		return domainsecurity.SHA256Hex([]byte("local-display-descriptor:\x00" + material))
	}
	descriptor, err := domainfundsquerysource.NewDescriptorV1(domainfundsquerysource.DescriptorInputV1{
		SnapshotRecordDigest:                   digest("snapshot-record"),
		DatasetSnapshotID:                      securitycontexttest.DatasetSnapshotID("local-display"),
		SourceManifestHash:                     digest("source-manifest"),
		CaseID:                                 "case-local-display",
		CaseBindingHash:                        digest("case-binding"),
		DatasetBindingDigest:                   digest("dataset-binding"),
		BindingObservationDigest:               digest("binding-observation"),
		FundsProducerContentID:                 domainsecurity.FundsProducerContentIDPrefixV1 + digest("producer"),
		FundsProducerContentManifestSHA256:     digest("producer-manifest"),
		FundsProducerContentManifestByteLength: 2048,
		DuckDBSHA256:                           digest("duckdb"),
		DuckDBByteLength:                       8192,
		DuckDBContentSnapshotDigest:            digest("duckdb-content"),
		DuckDBSnapshotManifestSHA256:           digest("duckdb-manifest"),
		MaterializationIdentity:                "txn_daily_snapshot:v12:" + digest("materialization"),
		SchemaDigest:                           domainfundsquerysource.FixedFundsAnalyticalSchemaDigestV1(),
		DatasetUTCOffsetMinutes:                0,
		ExpectedCurrency:                       "CNY",
		MinorUnitScale:                         domainfundsquerysource.AccountFlowMinorUnitScaleV1,
		QueryProfileDigest:                     domainfundsquerysource.FixedFundsLocalDisplayQueryProfileDigestV1(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return descriptor
}

func localDisplayNativeResultV1(
	t *testing.T,
	descriptor domainfundsquerysource.DescriptorV1,
	fields []string,
) domainnative.DirectSourcePreviewResultV1 {
	t.Helper()
	nativeFields := make([]domainnative.DirectSourcePreviewFieldV1, len(fields))
	for index, field := range fields {
		nativeFields[index] = domainnative.DirectSourcePreviewFieldV1(field)
	}
	arguments, err := domainnative.NewDirectSourcePreviewArgumentsV1(descriptor, nativeFields, 0, 25)
	if err != nil {
		t.Fatal(err)
	}
	type cell struct {
		Field domainnative.DirectSourcePreviewFieldV1 `json:"field"`
		Value string                                  `json:"value"`
	}
	type row struct {
		RowIndex uint32 `json:"rowIndex"`
		Cells    []cell `json:"cells"`
	}
	queryHash := domainsecurity.SHA256Hex([]byte("local-display-query"))
	values := make([]string, len(fields))
	for index, field := range fields {
		switch field {
		case "account":
			values[index] = "synthetic-account-1234"
		case "amountText":
			values[index] = "12.00"
		default:
			values[index] = "synthetic-value"
		}
	}
	rows := []row{{RowIndex: 0, Cells: make([]cell, len(fields))}}
	for index, field := range nativeFields {
		rows[0].Cells[index] = cell{Field: field, Value: values[index]}
	}
	type resultWire struct {
		SchemaVersion     uint8                                     `json:"schemaVersion"`
		Purpose           string                                    `json:"purpose"`
		DatasetSnapshotID string                                    `json:"datasetSnapshotId"`
		Fields            []domainnative.DirectSourcePreviewFieldV1 `json:"fields"`
		RowOffset         uint32                                    `json:"rowOffset"`
		RowLimit          uint16                                    `json:"rowLimit"`
		Rows              []row                                     `json:"rows"`
		HasMore           bool                                      `json:"hasMore"`
		QueryHash         string                                    `json:"queryHash"`
		ResultHash        string                                    `json:"resultHash"`
		Currentness       string                                    `json:"currentness"`
	}
	type hashMaterial struct {
		SchemaVersion     uint8                                     `json:"schemaVersion"`
		Purpose           string                                    `json:"purpose"`
		DatasetSnapshotID string                                    `json:"datasetSnapshotId"`
		Fields            []domainnative.DirectSourcePreviewFieldV1 `json:"fields"`
		RowOffset         uint32                                    `json:"rowOffset"`
		RowLimit          uint16                                    `json:"rowLimit"`
		Rows              []row                                     `json:"rows"`
		HasMore           bool                                      `json:"hasMore"`
		QueryHash         string                                    `json:"queryHash"`
		Currentness       string                                    `json:"currentness"`
	}
	material := hashMaterial{
		SchemaVersion: 1, Purpose: domainnative.DirectSourcePreviewPurposeV1,
		DatasetSnapshotID: descriptor.DatasetSnapshotID, Fields: nativeFields,
		RowOffset: 0, RowLimit: 25, Rows: rows, HasMore: false,
		QueryHash: queryHash, Currentness: domainnative.DirectSourcePreviewCurrentnessRequiredV1,
	}
	encoded, err := json.Marshal(material)
	if err != nil {
		t.Fatal(err)
	}
	hasher := sha256.New()
	writeFrame := func(value []byte) {
		var length [8]byte
		binary.BigEndian.PutUint64(length[:], uint64(len(value)))
		_, _ = hasher.Write(length[:])
		_, _ = hasher.Write(value)
	}
	writeFrame([]byte("analytix.direct-source-preview-result-hash/v1"))
	writeFrame(encoded)
	resultHash := hex.EncodeToString(hasher.Sum(nil))
	wire := resultWire{
		SchemaVersion: 1, Purpose: domainnative.DirectSourcePreviewPurposeV1,
		DatasetSnapshotID: descriptor.DatasetSnapshotID, Fields: nativeFields,
		RowOffset: 0, RowLimit: 25, Rows: rows, HasMore: false,
		QueryHash: queryHash, ResultHash: resultHash,
		Currentness: domainnative.DirectSourcePreviewCurrentnessRequiredV1,
	}
	raw, err := json.Marshal(wire)
	if err != nil {
		t.Fatal(err)
	}
	result, err := domainnative.ParseDirectSourcePreviewResultV1(raw, arguments)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func localDisplayAcceptedRetainedFixtureV1(
	t *testing.T,
	securityContext domainsecurity.TurnSecurityContext,
	reference string,
	canonicalValue string,
) (domainevidence.FinalAnswerEnvelope, *localDisplayAcceptedEvidenceStubV1, []domainevidence.AcceptedSlotSourceBindingV1) {
	return localDisplayAcceptedRetainedFixtureWithReceiptMutationV1(
		t, securityContext, reference, canonicalValue, nil,
	)
}

func localDisplayAcceptedRetainedFixtureWithReceiptMutationV1(
	t *testing.T,
	securityContext domainsecurity.TurnSecurityContext,
	reference string,
	canonicalValue string,
	mutate func(*domainevidence.EvidenceReceiptInput),
) (domainevidence.FinalAnswerEnvelope, *localDisplayAcceptedEvidenceStubV1, []domainevidence.AcceptedSlotSourceBindingV1) {
	t.Helper()
	proof := domainevidence.EvidenceSettlementProof{
		SettlementID:         domainsecurity.SHA256Hex([]byte("local-display-settlement")),
		PreparedRecordDigest: domainsecurity.SHA256Hex([]byte("local-display-prepared")),
	}
	receiptID := domainevidence.EvidenceSettlementReceiptID(proof.SettlementID)
	envelope := localDisplayAccountFlowAcceptedEnvelopeV1(t, securityContext, reference, receiptID)
	facts := make([]domainevidence.CanonicalEvidenceFact, len(envelope.Claims))
	factIDs := make([]string, len(envelope.Claims))
	for index, claim := range envelope.Claims {
		factID := "fact_aggregate_" + domainsecurity.SHA256Hex([]byte("local-display-accepted-fact:\x00"+claim.ClaimID))
		factIDs[index] = factID
		facts[index] = domainevidence.CanonicalEvidenceFact{
			FactID: factID, ClaimType: claim.ClaimType, NormalizedPayload: claim.NormalizedPayload,
		}
	}
	sourceIdentity, err := datasetsnapshotfixture.NewSourceRowIdentityV1(
		securityContext,
		"0123456789abcdefabcd",
		7,
	)
	if err != nil {
		t.Fatal(err)
	}
	sourceRecordID := sourceIdentity.SourceRecordID
	binding, err := domainevidence.NewAcceptedSlotSourceBindingV1(
		domainevidence.AcceptedSlotSourceBindingInputV1{
			FactIDs: factIDs, EntityReference: reference,
			SourceRecordID: sourceRecordID, SourceFileID: sourceIdentity.SourceFileID,
			SourceRowNumber: sourceIdentity.SourceRowNumber, Field: domainevidence.AcceptedSlotSourceFieldAccountV1,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	bindingDigest, err := domainevidence.AcceptedSlotSourceBindingSetDigestV1(
		[]domainevidence.AcceptedSlotSourceBindingV1{binding},
	)
	if err != nil {
		t.Fatal(err)
	}
	materialBody, err := json.Marshal(domainevidence.CanonicalEvidenceMaterial{
		SchemaVersion:                      domainevidence.CanonicalEvidenceVersionV3,
		Purpose:                            domainevidence.CanonicalEvidencePurposeV3,
		Facts:                              facts,
		AcceptedSlotSourceBindings:         []domainevidence.AcceptedSlotSourceBindingV1{binding},
		AcceptedSlotSourceBindingSetDigest: bindingDigest,
	})
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := domainevidence.CanonicalEvidenceBytes(materialBody)
	if err != nil {
		t.Fatal(err)
	}
	registry, err := domainevidence.NewEvidenceReceiptRegistry(securityContext)
	if err != nil {
		t.Fatal(err)
	}
	serverIdentity, err := domainsecurity.NewVerifiedMCPServerIdentity(
		"analytix_funds", "analytix-fund-analysis", "1.0.0",
		domainsecurity.SHA256Hex([]byte("local-display-server-instance")), 1,
	)
	if err != nil {
		t.Fatal(err)
	}
	toolCallEntropy := sha256.Sum256([]byte("local-display-host-tool-call"))
	toolCallID, err := domainsecurity.NewHostToolCallIDV1(toolCallEntropy[:])
	if err != nil {
		t.Fatal(err)
	}
	rawHash := domainsecurity.SHA256Hex([]byte("local-display-raw-provider-result:\x00" + canonicalValue))
	nativeHash := domainsecurity.SHA256Hex([]byte("local-display-native-account-flow-result"))
	canonicalHash := domainsecurity.CanonicalJSONHash(canonical)
	receiptInput := domainevidence.EvidenceReceiptInput{
		ReceiptID: receiptID, Context: securityContext,
		ExecutionGrantID: domainsecurity.SHA256Hex([]byte("local-display-grant")),
		ToolCallID:       toolCallID, ServerIdentity: serverIdentity, ServerVersion: "1.0.0",
		ConnectionEpoch: 1, ToolName: "mcp__analytix_funds__analyze_account_flows",
		ArgsHash:   domainsecurity.SHA256Hex([]byte("local-display-args")),
		ResultHash: canonicalHash, SourceType: "transactions",
		DatasetSnapshotID: securityContext.DatasetSnapshotID,
		QueryHash:         domainsecurity.SHA256Hex([]byte("local-display-query")),
		QueryRange:        *envelope.CheckedScope, Granularity: "aggregate",
		Currency: "CNY", Timezone: "Z", PaginationCompleteness: domainevidence.PaginationComplete,
		SourceRecordIDs: []string{sourceRecordID},
		RawSHA256:       rawHash,
		TransformationLineage: []domainevidence.TransformationLineageStep{
			{
				StepID: "bind-account-flow-native-result-v1", Transformer: "analytix-host-account-flow-oracle-binder", TransformerVersion: "1",
				InputHash: rawHash, OutputHash: nativeHash,
			},
			{
				StepID: "normalize-account-flow-v1", Transformer: "analytix-host-account-flow-normalizer", TransformerVersion: "1",
				InputHash: nativeHash, OutputHash: canonicalHash,
			},
		},
		PIIClassification: domainevidence.PIINone,
		IssuedAt:          time.Unix(1_750_000_003, 0).UTC(),
	}
	if mutate != nil {
		mutate(&receiptInput)
	}
	draft, err := domainevidence.NewEvidenceReceiptDraft(receiptInput)
	if err != nil {
		t.Fatal(err)
	}
	registry, _, err = domainevidence.RegisterEvidenceReceipt(
		registry, draft, canonical, proof, time.Unix(1_750_000_004, 0).UTC(),
	)
	if err != nil {
		t.Fatal(err)
	}
	return envelope, &localDisplayAcceptedEvidenceStubV1{registry: registry},
		[]domainevidence.AcceptedSlotSourceBindingV1{binding}
}

func localDisplayAccountFlowAcceptedEnvelopeV1(
	t *testing.T,
	securityContext domainsecurity.TurnSecurityContext,
	reference string,
	receiptID string,
) domainevidence.FinalAnswerEnvelope {
	t.Helper()
	const (
		start = "2026-01-01T00:00:00Z"
		end   = "2026-01-31T23:59:59Z"
	)
	scope := domainevidence.EvidenceQueryRange{
		EntityIDs: []string{reference}, AccountIDs: []string{reference}, Directions: []string{"in", "out"},
		SourceIDs: []string{"qscope1_" + domainsecurity.SHA256Hex([]byte("local-display-query-scope"))}, StartAt: start, EndAt: end,
		FiltersHash: domainsecurity.SHA256Hex([]byte("local-display-filter")),
	}
	claimInputs := []struct {
		id        string
		claimType domainevidence.ClaimType
		payload   domainevidence.NormalizedClaimPayload
	}{
		{
			id: "claim-local-display-in", claimType: domainevidence.ClaimAmount,
			payload: domainevidence.NormalizedClaimPayload{
				SubjectID: reference, EntityID: reference, AccountID: reference,
				AmountMinor: "1000", Currency: "CNY", Direction: "in",
				StartAt: start, EndAt: end, Granularity: "aggregate",
			},
		},
		{
			id: "claim-local-display-out", claimType: domainevidence.ClaimAmount,
			payload: domainevidence.NormalizedClaimPayload{
				SubjectID: reference, EntityID: reference, AccountID: reference,
				AmountMinor: "400", Currency: "CNY", Direction: "out",
				StartAt: start, EndAt: end, Granularity: "aggregate",
			},
		},
		{
			id: "claim-local-display-count", claimType: domainevidence.ClaimCount,
			payload: domainevidence.NormalizedClaimPayload{
				SubjectID: reference, EntityID: reference, Count: "2",
				StartAt: start, EndAt: end, Granularity: "aggregate",
			},
		},
	}
	claims := make([]domainevidence.ClaimRecord, len(claimInputs))
	for index, input := range claimInputs {
		proposal := domainevidence.ClaimProposal{
			SchemaVersion:     domainevidence.ClaimProposalVersion,
			ProposalID:        "proposal-" + input.id,
			ClaimType:         input.claimType,
			NormalizedPayload: input.payload,
			EvidenceIDs:       []string{}, CounterEvidenceIDs: []string{},
		}
		claim, err := domainevidence.NewClaimRecord(domainevidence.ClaimRecordInput{
			ClaimID: input.id, Proposal: proposal, SupportState: domainevidence.ClaimVerified,
			EvidenceIDs: []string{receiptID}, CounterEvidenceIDs: []string{}, SupportedScope: &scope,
			AllowedWording: []string{"exact_verified_fact"}, ProhibitedUpgrades: []string{"whole_case_conclusion"},
			VerifierReceiptID: domainevidence.VerifierReceiptDigest(
				input.id, input.claimType, input.payload,
				[]string{receiptID}, nil, domainevidence.ClaimVerified,
			),
			VerificationReason: "test", VerifiedAt: time.Unix(1_750_000_001, 0).UTC(),
		})
		if err != nil {
			t.Fatal(err)
		}
		claims[index] = claim
	}
	queryHash := domainsecurity.SHA256Hex([]byte("local-display-query"))
	nativeResultHash := domainsecurity.SHA256Hex([]byte("local-display-native-account-flow-result"))
	sourceFieldReference, err := domainevidence.NewAccountFlowTypedSourceFieldReferenceV1(
		queryHash, nativeResultHash, domainevidence.AcceptedSlotSourceFieldAccountV1,
	)
	if err != nil {
		t.Fatal(err)
	}
	claimIDs := make([]string, len(claims))
	for index, claim := range claims {
		claimIDs[index] = claim.ClaimID
	}
	sort.Strings(claimIDs)
	typedOutcome, err := domainevidence.NewAccountFlowTypedAnswerOutcomeV1([]domainevidence.AccountFlowTypedAnswerGroupV1{{
		EvidenceReceiptID: receiptID, ClaimIDs: claimIDs, QueryScopeRef: scope.SourceIDs[0],
		QueryHash: queryHash, ResultHash: nativeResultHash, SourceFieldReference: sourceFieldReference,
	}})
	if err != nil {
		t.Fatal(err)
	}
	envelope, err := domainevidence.NewFinalAnswerEnvelope(domainevidence.FinalAnswerEnvelopeInput{
		Variant: domainevidence.EvidenceBackedAnswer, Context: securityContext, TerminalReason: "success",
		AccountFlowOutcome: &typedOutcome,
		Claims:             claims, EvidenceReceiptIDs: []string{receiptID},
		CheckedScope: &scope, IssuedAt: time.Unix(1_750_000_002, 0).UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return envelope
}

func rebuildLocalDisplayAccountFlowGroupV1(
	t *testing.T,
	securityContext domainsecurity.TurnSecurityContext,
	envelope domainevidence.FinalAnswerEnvelope,
	mutate func(*domainevidence.AccountFlowTypedAnswerGroupV1),
) domainevidence.FinalAnswerEnvelope {
	t.Helper()
	if envelope.AccountFlowOutcome == nil || len(envelope.AccountFlowOutcome.Groups) != 1 || mutate == nil {
		t.Fatal("account-flow envelope fixture is incomplete")
	}
	group := envelope.AccountFlowOutcome.Groups[0]
	mutate(&group)
	sourceField, err := domainevidence.NewAccountFlowTypedSourceFieldReferenceV1(
		group.QueryHash, group.ResultHash, group.SourceFieldReference.Field,
	)
	if err != nil {
		t.Fatal(err)
	}
	group.SourceFieldReference = sourceField
	outcome, err := domainevidence.NewAccountFlowTypedAnswerOutcomeV1(
		[]domainevidence.AccountFlowTypedAnswerGroupV1{group},
	)
	if err != nil {
		t.Fatal(err)
	}
	rebuilt, err := domainevidence.NewFinalAnswerEnvelope(domainevidence.FinalAnswerEnvelopeInput{
		Variant: domainevidence.EvidenceBackedAnswer, Context: securityContext, TerminalReason: envelope.TerminalReason,
		AccountFlowOutcome: &outcome, Claims: envelope.Claims, EvidenceReceiptIDs: envelope.EvidenceReceiptIDs,
		CheckedScope: envelope.CheckedScope, IssuedAt: time.Unix(1_750_000_002, 0).UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return rebuilt
}

func localDisplayAcceptedEnvelopeForReceiptV1(
	t *testing.T,
	securityContext domainsecurity.TurnSecurityContext,
	reference string,
	receiptID string,
) domainevidence.FinalAnswerEnvelope {
	return localDisplayAcceptedEnvelopeForReceiptAndCountV1(
		t, securityContext, reference, receiptID, "1",
	)
}

func localDisplayAcceptedEnvelopeForReceiptAndCountV1(
	t *testing.T,
	securityContext domainsecurity.TurnSecurityContext,
	reference string,
	receiptID string,
	count string,
) domainevidence.FinalAnswerEnvelope {
	t.Helper()
	payload := domainevidence.NormalizedClaimPayload{
		SubjectID: reference, EntityID: reference, Count: count,
		StartAt: "2026-01-01T00:00:00Z", EndAt: "2026-01-31T23:59:59Z", Granularity: "transaction",
	}
	proposal := domainevidence.ClaimProposal{
		SchemaVersion: domainevidence.ClaimProposalVersion, ProposalID: "proposal-local-display",
		ClaimType: domainevidence.ClaimCount, NormalizedPayload: payload,
		EvidenceIDs: []string{}, CounterEvidenceIDs: []string{},
	}
	scope := domainevidence.EvidenceQueryRange{
		EntityIDs: []string{reference}, AccountIDs: []string{reference}, Directions: []string{"in", "out"},
		SourceIDs: []string{"source-local-display"}, StartAt: payload.StartAt, EndAt: payload.EndAt,
		FiltersHash: domainsecurity.SHA256Hex([]byte("local-display-filter")),
	}
	claim, err := domainevidence.NewClaimRecord(domainevidence.ClaimRecordInput{
		ClaimID: "claim-local-display", Proposal: proposal, SupportState: domainevidence.ClaimVerified,
		EvidenceIDs: []string{receiptID}, CounterEvidenceIDs: []string{}, SupportedScope: &scope,
		AllowedWording: []string{"exact_verified_fact"}, ProhibitedUpgrades: []string{"whole_case_conclusion"},
		VerifierReceiptID: domainevidence.VerifierReceiptDigest(
			"claim-local-display", proposal.ClaimType, payload,
			[]string{receiptID}, nil, domainevidence.ClaimVerified,
		),
		VerificationReason: "test", VerifiedAt: time.Unix(1_750_000_001, 0).UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	envelope, err := domainevidence.NewFinalAnswerEnvelope(domainevidence.FinalAnswerEnvelopeInput{
		Variant: domainevidence.EvidenceBackedAnswer, Context: securityContext, TerminalReason: "success",
		Claims: []domainevidence.ClaimRecord{claim}, EvidenceReceiptIDs: []string{receiptID},
		CheckedScope: &scope, IssuedAt: time.Unix(1_750_000_002, 0).UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return envelope
}

func localDisplayCaseContextV1(t *testing.T) domainsecurity.TurnSecurityContext {
	t.Helper()
	securityContext, err := securitycontexttest.CaseExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-local-display", TurnID: "turn-local-display",
		WorkspaceRealPath: t.TempDir(), CaseID: "case-local-display",
		ContextEpoch: 7, IssuedAt: time.Unix(1_750_000_000, 0).UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return securityContext
}
