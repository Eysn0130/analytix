package caseentity

import (
	"context"
	"errors"
	"strings"
	"testing"

	domaincaseentity "analytix.local/runtime-go/internal/domain/caseentity"
	domaincontrolledaccount "analytix.local/runtime-go/internal/domain/controlledaccount"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	caseentityport "analytix.local/runtime-go/internal/ports/caseentity"
	datasetsnapshotport "analytix.local/runtime-go/internal/ports/datasetsnapshot"
	currentdatasettest "analytix.local/runtime-go/internal/testsupport/currentdataset"
)

func TestAppendCaseLongitudinalUsesExactDatasetContextValidation(t *testing.T) {
	for _, scoped := range []bool{false, true} {
		name := "full-fallback"
		if scoped {
			name = "inside-exact"
		}
		t.Run(name, func(t *testing.T) {
			fixture := newAppendExactValidationFixtureV1(t)
			fullCalls, insideCalls := 0, 0
			validateFull := func(ctx context.Context, current domainsecurity.TurnSecurityContext) error {
				fullCalls++
				return fixture.harness.ValidateCurrent(ctx, current)
			}
			validateInside := func(ctx context.Context, current domainsecurity.TurnSecurityContext) error {
				insideCalls++
				if !fixture.authority.exactActive || current != fixture.securityContext {
					return errors.New("append context validator escaped its exact dataset lease")
				}
				return ctx.Err()
			}
			service := NewPersistentService(fixture.keyed, fixture.store, fixture.authority, fixture.harness, validateFull)
			if scoped {
				service = NewPersistentServiceWithExactAppendValidationV1(
					fixture.keyed, fixture.store, fixture.authority, fixture.harness, validateFull, validateInside,
				)
			}
			err := service.AppendCaseLongitudinalOwnerStateV1(context.Background(), AppendCaseLongitudinalOwnerStateInputV1{
				SecurityContext:    fixture.securityContext,
				ContinuationDigest: domainsecurity.SHA256Hex([]byte("append-continuation")),
			})
			if err != nil {
				t.Fatal(err)
			}
			wantFull, wantInside := 4, 0
			if scoped {
				wantFull, wantInside = 2, 2
			}
			if fullCalls != wantFull || insideCalls != wantInside || fixture.authority.selectionCalls != 1 || fixture.authority.exactCalls != 1 {
				t.Fatalf("append repeated dataset validation or lost its exact lease: full=%d want=%d inside=%d want=%d selections=%d exact_uses=%d",
					fullCalls, wantFull, insideCalls, wantInside, fixture.authority.selectionCalls, fixture.authority.exactCalls)
			}
			index, err := fixture.store.ResolveLatestCaseLongitudinalContext(context.Background(), fixture.securityContext)
			if err != nil || len(index.Continuations) != 1 || index.Continuations[0].ContinuationDigest != domainsecurity.SHA256Hex([]byte("append-continuation")) {
				t.Fatal("append lost its canonical continuation record")
			}
			fullCalls, insideCalls = 0, 0
			fixture.authority.selectionCalls, fixture.authority.exactCalls = 0, 0
			if _, err := service.BindReferenceV1(context.Background(), NewDeriveReferenceInputV1(
				fixture.securityContext, domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1, "6222021234567890",
			)); err != nil {
				t.Fatal(err)
			}
			if fullCalls != 4 || insideCalls != 0 || fixture.authority.selectionCalls != 1 || fixture.authority.exactCalls != 1 {
				t.Fatalf("append validator changed a non-append operation: full=%d inside=%d selections=%d exact_uses=%d",
					fullCalls, insideCalls, fixture.authority.selectionCalls, fixture.authority.exactCalls)
			}
		})
	}
}

func TestAppendCaseLongitudinalExactValidationFailsClosed(t *testing.T) {
	for _, mode := range []string{"outer-pre", "inside-pre", "inside-post", "outer-post", "dataset-drift", "cancelled", "write-error"} {
		t.Run(mode, func(t *testing.T) {
			fixture := newAppendExactValidationFixtureV1(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			fullCalls, insideCalls := 0, 0
			validateFull := func(ctx context.Context, current domainsecurity.TurnSecurityContext) error {
				fullCalls++
				if (mode == "outer-pre" && fullCalls == 1) || (mode == "outer-post" && fullCalls == 2) {
					return ErrPrivateStateUnavailable
				}
				return fixture.harness.ValidateCurrent(ctx, current)
			}
			validateInside := func(ctx context.Context, current domainsecurity.TurnSecurityContext) error {
				insideCalls++
				if !fixture.authority.exactActive || current != fixture.securityContext {
					return errors.New("append context validator escaped its exact dataset lease")
				}
				if (mode == "inside-pre" && insideCalls == 1) || (mode == "inside-post" && insideCalls == 2) {
					return ErrPrivateStateUnavailable
				}
				return ctx.Err()
			}
			store := &appendValidationStoreV1{caseEntityMemoryStoreV1: fixture.store}
			observer := fixture.harness.Observe
			if mode == "dataset-drift" {
				binding, err := fixture.harness.Observe(fixture.securityContext.WorkspaceRealPath)
				if err != nil {
					t.Fatal(err)
				}
				// Case binding stays valid; only the outer dataset capability
				// must detect dataset revocation after the append callback.
				observer = func(string) (domainsecurity.CaseBindingObservationV1, error) { return binding, nil }
				store.beforePut = func() { fixture.harness.SetRevoked(true) }
			}
			if mode == "cancelled" {
				store.beforePut = cancel
			}
			if mode == "write-error" {
				store.putError = caseentityport.ErrIntegrity
			}
			beforeRecords := len(fixture.store.threadContext)
			service := NewPersistentServiceWithExactAppendValidationV1(
				fixture.keyed, store, fixture.authority, appendValidationObserverV1(observer), validateFull, validateInside,
			)
			err := service.AppendCaseLongitudinalOwnerStateV1(ctx, AppendCaseLongitudinalOwnerStateInputV1{
				SecurityContext:    fixture.securityContext,
				ContinuationDigest: domainsecurity.SHA256Hex([]byte("append-continuation-failure")),
			})
			if err == nil {
				t.Fatal("append accepted drift, cancellation or a failed canonical write")
			}
			if (mode == "outer-pre" || mode == "inside-pre") && len(fixture.store.threadContext) != beforeRecords {
				t.Fatal("failed append prevalidation wrote canonical state")
			}
			if mode == "cancelled" && !errors.Is(err, context.Canceled) {
				t.Fatalf("append lost cancellation: %v", err)
			}
			if mode == "write-error" && !errors.Is(err, ErrPrivateStateIntegrity) {
				t.Fatalf("append lost canonical write integrity failure: %v", err)
			}
			if (mode == "inside-pre" && insideCalls != 1) || (mode == "inside-post" && insideCalls != 2) ||
				(mode == "outer-post" && (fullCalls != 2 || insideCalls != 2)) {
				t.Fatalf("append skipped live validation at the tested boundary: full=%d inside=%d", fullCalls, insideCalls)
			}
			if mode == "dataset-drift" && (fullCalls != 1 || insideCalls != 2 || fixture.authority.exactCalls != 1) {
				t.Fatalf("dataset revocation escaped the exact capability postcheck: full=%d inside=%d exact_uses=%d",
					fullCalls, insideCalls, fixture.authority.exactCalls)
			}
		})
	}
}

func TestAppendExactValidationConstructorFailsClosedWithoutValidator(t *testing.T) {
	fixture := newAppendExactValidationFixtureV1(t)
	if service := NewPersistentServiceWithExactAppendValidationV1(
		fixture.keyed, fixture.store, fixture.authority, fixture.harness, fixture.harness.ValidateCurrent, nil,
	); service != nil {
		t.Fatal("exact append construction accepted a missing live-context validator")
	}
}

type appendValidationObserverV1 func(string) (domainsecurity.CaseBindingObservationV1, error)

func (observe appendValidationObserverV1) Observe(workspace string) (domainsecurity.CaseBindingObservationV1, error) {
	return observe(workspace)
}

type appendValidationStoreV1 struct {
	*caseEntityMemoryStoreV1
	beforePut func()
	putError  error
}

func (store *appendValidationStoreV1) PutThreadContextIfAbsent(ctx context.Context, record domaincaseentity.ThreadCaseContextRecord) error {
	if store.beforePut != nil {
		store.beforePut()
	}
	if store.putError != nil {
		return store.putError
	}
	return store.caseEntityMemoryStoreV1.PutThreadContextIfAbsent(ctx, record)
}

type appendExactValidationFixtureV1 struct {
	keyed           *recordingKeyedDigesterV1
	store           *caseEntityMemoryStoreV1
	harness         *currentdatasettest.Harness
	authority       *appendExactDatasetAuthorityV1
	securityContext domainsecurity.TurnSecurityContext
}

func newAppendExactValidationFixtureV1(t *testing.T) appendExactValidationFixtureV1 {
	t.Helper()
	harness := currentdatasettest.NewHarness()
	current := caseEntityTestContextV1(t, caseEntityTestContextInputV1{
		ThreadID: "thread-append-exact", TurnID: "turn-append-exact", TenantID: "tenant-a", UserID: "user-a",
		CaseID: "case-a", CaseBindingHash: strings.Repeat("a", 64), SnapshotSeed: "snapshot-append-exact", ContextEpoch: 1,
	})
	keyed := &recordingKeyedDigesterV1{key: []byte("synthetic-append-exact")}
	store := newCaseEntityMemoryStoreV1()
	setup := NewPersistentService(keyed, store, harness, harness, harness.ValidateCurrent)
	ref, err := setup.BindReferenceV1(context.Background(), NewDeriveReferenceInputV1(
		current, domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1, "6222021234567890",
	))
	if err != nil {
		t.Fatal(err)
	}
	if err := setup.AppendCaseLongitudinalIngressV1(context.Background(), AppendCaseLongitudinalIngressInputV1{
		SecurityContext: current, References: []domaincaseentity.ReferenceV1{ref},
	}); err != nil {
		t.Fatal(err)
	}
	return appendExactValidationFixtureV1{keyed, store, harness, &appendExactDatasetAuthorityV1{delegate: harness}, current}
}

type appendExactDatasetAuthorityV1 struct {
	delegate                   datasetsnapshotport.CurrentAuthorityV2
	selectionCalls, exactCalls int
	exactActive                bool
}

func (authority *appendExactDatasetAuthorityV1) WithCurrentSelectionV2(
	ctx context.Context,
	input datasetsnapshotport.ResolveInputV2,
	current domainsecurity.TurnSecurityContext,
	use func(datasetsnapshotport.CurrentSelectionV2, datasetsnapshotport.CurrentSelectionCapabilityV2) error,
) error {
	authority.selectionCalls++
	return authority.delegate.WithCurrentSelectionV2(ctx, input, current, func(
		selection datasetsnapshotport.CurrentSelectionV2,
		capability datasetsnapshotport.CurrentSelectionCapabilityV2,
	) error {
		return use(selection, appendExactDatasetCapabilityV1{capability, authority})
	})
}

type appendExactDatasetCapabilityV1 struct {
	delegate  datasetsnapshotport.CurrentSelectionCapabilityV2
	authority *appendExactDatasetAuthorityV1
}

func (capability appendExactDatasetCapabilityV1) UseExact(
	selection datasetsnapshotport.CurrentSelectionV2,
	current domainsecurity.TurnSecurityContext,
	use func(context.Context) error,
) error {
	capability.authority.exactCalls++
	return capability.delegate.UseExact(selection, current, func(ctx context.Context) error {
		capability.authority.exactActive = true
		defer func() { capability.authority.exactActive = false }()
		return use(ctx)
	})
}
