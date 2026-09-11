package fundsquerysource

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainfundsquerysource "analytix.local/runtime-go/internal/domain/fundsquerysource"
	domainnative "analytix.local/runtime-go/internal/domain/nativecomponent"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	datasetsnapshotport "analytix.local/runtime-go/internal/ports/datasetsnapshot"
	fundsquerysourceport "analytix.local/runtime-go/internal/ports/fundsquerysource"
	datasetsnapshotfixture "analytix.local/runtime-go/internal/testsupport/datasetsnapshotv2fixture"
	fundsquerysourcefixture "analytix.local/runtime-go/internal/testsupport/fundsquerysourcefixture"
	securitycontextfixture "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

func TestServiceUsesCurrentAnalyticalDuckDBOnlyInsideExactDSV2Scope(t *testing.T) {
	fixture := newServiceFixtureV1(t, snapshotKindAnalyticalV1, "fixed-profile")
	var (
		copied   = fundsquerysourcefixture.MustNewUnlinkedDestinationV1(t)
		retained fundsquerysourceport.ExactReadLease
		got      domainfundsquerysource.DescriptorV1
		uses     int
	)
	err := fixture.service.UseCurrent(
		context.Background(),
		fixture.securityContext,
		func(
			_ context.Context,
			descriptor domainfundsquerysource.DescriptorV1,
			lease fundsquerysourceport.ExactReadLease,
			_ domainnative.AccountFlowSourceRowResolverV1,
		) error {
			uses++
			got = descriptor
			retained = lease
			return lease.CopyExactTo(context.Background(), copied)
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if uses != 1 || fixture.authority.calls != 1 || fixture.authority.callbackCalls != 1 ||
		fixture.authority.exactUseCalls != 1 || fixture.authority.exactCallbackCalls != 1 ||
		fixture.source.calls != 1 || fixture.source.callbackCalls != 1 {
		t.Fatalf(
			"callbacks were not exactly once: user=%d authority=%d/%d exact=%d/%d source=%d/%d",
			uses,
			fixture.authority.calls,
			fixture.authority.callbackCalls,
			fixture.authority.exactUseCalls,
			fixture.authority.exactCallbackCalls,
			fixture.source.calls,
			fixture.source.callbackCalls,
		)
	}
	if !bytes.Equal(fundsquerysourcefixture.MustReadExactDestinationV1(t, copied), fixture.duckDB) {
		t.Fatal("host exact source returned different private DuckDB bytes")
	}
	if err := domainfundsquerysource.ValidateDescriptorV1(got); err != nil ||
		got.SnapshotRecordDigest != fixture.selection.Snapshot.Record.RecordDigest ||
		got.DatasetSnapshotID != fixture.securityContext.DatasetSnapshotID ||
		got.SourceManifestHash != fixture.securityContext.SourceManifestHash ||
		got.CaseID != fixture.observation.CaseID ||
		got.CaseBindingHash != fixture.observation.CaseBindingHash ||
		got.BindingObservationDigest != fixture.observation.ObservationDigest ||
		got.DuckDBSHA256 != domainsecurity.SHA256Hex(fixture.duckDB) ||
		got.QueryProfileDigest != domainfundsquerysource.FixedAccountFlowQueryProfileDigestV1() {
		t.Fatalf("derived descriptor lost exact DSV2 provenance: %#v err=%v", got, err)
	}
	wantResolve := datasetsnapshotport.ResolveInputV2{
		TenantID:                  fixture.securityContext.TenantID,
		UserID:                    fixture.securityContext.UserID,
		Observation:               fixture.observation,
		ExpectedDatasetSnapshotID: fixture.securityContext.DatasetSnapshotID,
	}
	if fixture.authority.resolveInput != wantResolve ||
		fixture.authority.securityContext != fixture.securityContext ||
		fixture.source.descriptor != got {
		t.Fatal("service did not pass the exact observed DSV2 provenance downstream")
	}
	if err := retained.CopyExactTo(context.Background(), copied); !errors.Is(err, fundsquerysourceport.ErrUnavailable) {
		t.Fatalf("retained source lease error = %v", err)
	}
	called := false
	if fixture.authority.retainedCapability == nil || fixture.authority.retainedCapability.UseExact(
		fixture.selection,
		fixture.securityContext,
		func(context.Context) error {
			called = true
			return nil
		},
	) == nil || called {
		t.Fatal("retained DSV2 capability remained usable after UseCurrent returned")
	}
}

func TestServiceAccountFlowPostNativeCurrentnessIsOneShotExactAndCallbackScoped(t *testing.T) {
	t.Run("healthy then copied reused and late", func(t *testing.T) {
		fixture := newServiceFixtureV1(t, snapshotKindAnalyticalV1, "fixed-profile")
		var retained func(
			context.Context,
			domainsecurity.TurnSecurityContext,
			domainfundsquerysource.DescriptorV1,
			func(context.Context) error,
		) error
		continuations := 0
		err := fixture.service.UseCurrentAccountFlow(
			context.Background(),
			fixture.securityContext,
			func(
				sourceContext context.Context,
				descriptor domainfundsquerysource.DescriptorV1,
				_ fundsquerysourceport.ExactReadLease,
				_ domainnative.AccountFlowSourceRowResolverV1,
				revalidate func(context.Context, domainsecurity.TurnSecurityContext, domainfundsquerysource.DescriptorV1, func(context.Context) error) error,
			) error {
				retained = revalidate
				if _, marshalErr := json.Marshal(revalidate); marshalErr == nil {
					t.Fatal("post-native currentness closure became serializable")
				}
				if err := revalidate(
					sourceContext,
					fixture.securityContext,
					descriptor,
					func(currentnessContext context.Context) error {
						continuations++
						return currentnessContext.Err()
					},
				); err != nil {
					return err
				}
				copied := revalidate
				return copied(
					sourceContext,
					fixture.securityContext,
					descriptor,
					func(context.Context) error {
						continuations++
						return nil
					},
				)
			},
		)
		if !errors.Is(err, fundsquerysourceport.ErrUnavailable) || continuations != 1 ||
			fixture.authority.exactUseCalls != 2 || fixture.authority.postNativeUseCalls != 1 {
			t.Fatalf(
				"copied/reused post-native currentness escaped: err=%v continuations=%d exact=%d post_native=%d",
				err,
				continuations,
				fixture.authority.exactUseCalls,
				fixture.authority.postNativeUseCalls,
			)
		}
		lateContinuations := 0
		if lateErr := retained(
			context.Background(),
			fixture.securityContext,
			fixture.source.descriptor,
			func(context.Context) error {
				lateContinuations++
				return nil
			},
		); !errors.Is(lateErr, fundsquerysourceport.ErrUnavailable) || lateContinuations != 0 {
			t.Fatalf("late post-native currentness escaped: err=%v continuation=%d", lateErr, lateContinuations)
		}
	})

	t.Run("cross context and descriptor burn before continuation", func(t *testing.T) {
		for _, test := range []struct {
			name   string
			mutate func(*domainsecurity.TurnSecurityContext, *domainfundsquerysource.DescriptorV1)
		}{
			{
				name: "context",
				mutate: func(securityContext *domainsecurity.TurnSecurityContext, _ *domainfundsquerysource.DescriptorV1) {
					securityContext.TurnID = "turn-cross-context"
				},
			},
			{
				name: "descriptor",
				mutate: func(_ *domainsecurity.TurnSecurityContext, descriptor *domainfundsquerysource.DescriptorV1) {
					descriptor.CaseID = "case-cross-descriptor"
				},
			},
		} {
			t.Run(test.name, func(t *testing.T) {
				fixture := newServiceFixtureV1(t, snapshotKindAnalyticalV1, "fixed-profile")
				continuations := 0
				err := fixture.service.UseCurrentAccountFlow(
					context.Background(),
					fixture.securityContext,
					func(
						sourceContext context.Context,
						descriptor domainfundsquerysource.DescriptorV1,
						_ fundsquerysourceport.ExactReadLease,
						_ domainnative.AccountFlowSourceRowResolverV1,
						revalidate func(context.Context, domainsecurity.TurnSecurityContext, domainfundsquerysource.DescriptorV1, func(context.Context) error) error,
					) error {
						currentContext := fixture.securityContext
						currentDescriptor := descriptor
						test.mutate(&currentContext, &currentDescriptor)
						mismatchErr := revalidate(
							sourceContext,
							currentContext,
							currentDescriptor,
							func(context.Context) error {
								continuations++
								return nil
							},
						)
						if !errors.Is(mismatchErr, fundsquerysourceport.ErrMismatch) {
							return mismatchErr
						}
						return revalidate(
							sourceContext,
							fixture.securityContext,
							descriptor,
							func(context.Context) error {
								continuations++
								return nil
							},
						)
					},
				)
				if !errors.Is(err, fundsquerysourceport.ErrUnavailable) || continuations != 0 ||
					fixture.authority.exactUseCalls != 1 {
					t.Fatalf(
						"cross-binding post-native use escaped: err=%v continuations=%d exact=%d",
						err,
						continuations,
						fixture.authority.exactUseCalls,
					)
				}
			})
		}
	})

	t.Run("concurrent copy reaches one continuation", func(t *testing.T) {
		fixture := newServiceFixtureV1(t, snapshotKindAnalyticalV1, "fixed-profile")
		continuationStarted := make(chan struct{})
		continuationRelease := make(chan struct{})
		continuations := 0
		var concurrentErr error
		err := fixture.service.UseCurrentAccountFlow(
			context.Background(),
			fixture.securityContext,
			func(
				sourceContext context.Context,
				descriptor domainfundsquerysource.DescriptorV1,
				_ fundsquerysourceport.ExactReadLease,
				_ domainnative.AccountFlowSourceRowResolverV1,
				revalidate func(context.Context, domainsecurity.TurnSecurityContext, domainfundsquerysource.DescriptorV1, func(context.Context) error) error,
			) error {
				firstDone := make(chan error, 1)
				go func() {
					firstDone <- revalidate(
						sourceContext,
						fixture.securityContext,
						descriptor,
						func(context.Context) error {
							continuations++
							close(continuationStarted)
							<-continuationRelease
							return nil
						},
					)
				}()
				<-continuationStarted
				concurrentErr = revalidate(
					sourceContext,
					fixture.securityContext,
					descriptor,
					func(context.Context) error {
						continuations++
						return nil
					},
				)
				close(continuationRelease)
				return <-firstDone
			},
		)
		if err != nil || !errors.Is(concurrentErr, fundsquerysourceport.ErrUnavailable) ||
			continuations != 1 || fixture.authority.exactUseCalls != 2 {
			t.Fatalf(
				"concurrent post-native currentness escaped: err=%v concurrent=%v continuations=%d exact=%d",
				err,
				concurrentErr,
				continuations,
				fixture.authority.exactUseCalls,
			)
		}
	})
}

func TestDescriptorForCurrentSelectionPreservesActualAcceptedQueryProfile(t *testing.T) {
	for _, testCase := range []struct {
		seed string
		want string
	}{
		{seed: "fixed-profile", want: domainfundsquerysource.FixedAccountFlowQueryProfileDigestV1()},
		{seed: "combined-query-profile", want: domainfundsquerysource.FixedFundsQueryProfileDigestV1()},
	} {
		t.Run(testCase.seed, func(t *testing.T) {
			fixture := newServiceFixtureV1(t, snapshotKindAnalyticalV1, testCase.seed)
			descriptor, err := DescriptorForCurrentSelectionV1(
				fixture.selection,
				fixture.securityContext,
				fixture.observation,
			)
			if err != nil || descriptor.QueryProfileDigest != testCase.want {
				t.Fatalf(
					"descriptor profile=%s want=%s err=%v",
					descriptor.QueryProfileDigest,
					testCase.want,
					err,
				)
			}
		})
	}
}

func TestServiceUsesCurrentLocalDisplayWithoutTurnAuthorityAndRevalidatesSnapshot(t *testing.T) {
	fixture := newServiceFixtureV1(t, snapshotKindAnalyticalV1, "local-display-profile")
	destination := fundsquerysourcefixture.MustNewUnlinkedDestinationV1(t)
	var got domainfundsquerysource.DescriptorV1
	uses := 0
	err := fixture.service.UseCurrentLocalDisplay(
		context.Background(),
		fixture.observation.WorkspaceRealPath,
		domainsecurity.LocalTenantID,
		domainsecurity.LocalUserID,
		func(
			_ context.Context,
			descriptor domainfundsquerysource.DescriptorV1,
			lease fundsquerysourceport.ExactReadLease,
		) error {
			uses++
			got = descriptor
			return lease.CopyExactTo(context.Background(), destination)
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if uses != 1 || fixture.authority.resolveCalls != 3 || fixture.source.calls != 1 ||
		fixture.source.callbackCalls != 1 ||
		got.QueryProfileDigest != domainfundsquerysource.FixedFundsLocalDisplayQueryProfileDigestV1() ||
		!bytes.Equal(fundsquerysourcefixture.MustReadExactDestinationV1(t, destination), fixture.duckDB) {
		t.Fatalf(
			"local display exact use lost authority: uses=%d resolves=%d source=%d/%d descriptor=%#v",
			uses, fixture.authority.resolveCalls, fixture.source.calls, fixture.source.callbackCalls, got,
		)
	}
	if fixture.authority.calls != 0 || fixture.authority.exactUseCalls != 0 {
		t.Fatal("local display manufactured or consumed a turn-scoped DSV2 capability")
	}
}

func TestServiceDiscardsLocalDisplayWhenSnapshotChangesAfterExactUse(t *testing.T) {
	fixture := newServiceFixtureV1(t, snapshotKindAnalyticalV1, "local-display-profile")
	fixture.authority.resolveFailureAt = 3
	called := false
	err := fixture.service.UseCurrentLocalDisplay(
		context.Background(),
		fixture.observation.WorkspaceRealPath,
		domainsecurity.LocalTenantID,
		domainsecurity.LocalUserID,
		func(context.Context, domainfundsquerysource.DescriptorV1, fundsquerysourceport.ExactReadLease) error {
			called = true
			return nil
		},
	)
	if !called || !errors.Is(err, fundsquerysourceport.ErrMismatch) || fixture.authority.resolveCalls != 3 {
		t.Fatalf("late snapshot drift was not discarded: called=%t resolves=%d err=%v", called, fixture.authority.resolveCalls, err)
	}
}

func TestServiceAcceptedSlotDisplayKeepsHistoricalRetainedSnapshotAfterCurrentEvolution(t *testing.T) {
	const (
		oldOriginal = "6222 0212-3456 7890"
		canonical   = "6222021234567890"
	)
	historical := newAcceptedSlotSourceResolverFixtureV1(t, oldOriginal, canonical)
	current := newServiceFixtureV1(t, snapshotKindAnalyticalV1, "accepted-slot-new-current-snapshot")
	if current.securityContext.DatasetSnapshotID == historical.securityContext.DatasetSnapshotID ||
		current.securityContext.SourceManifestHash == historical.securityContext.SourceManifestHash {
		t.Fatal("snapshot evolution fixture did not produce distinct current and retained snapshots")
	}
	current.authority.retainedSelection = datasetsnapshotport.RetainedSelectionV2{
		Current: current.selection, Snapshot: historical.snapshot,
	}
	service, err := NewService(
		current.authority, current.observer, historical.materials, current.source,
	)
	if err != nil {
		t.Fatal(err)
	}
	uses := 0
	err = service.UseRetainedAcceptedSlotDisplay(
		context.Background(), current.securityContext, historical.securityContext,
		[]domainevidence.AcceptedSlotSourceBindingV1{historical.binding}, canonical,
		func(value string) error {
			uses++
			if value != oldOriginal || value == canonical || bytes.Contains(current.duckDB, []byte(value)) {
				t.Fatalf("historical accepted slot upgraded to current/canonical material: %q", value)
			}
			return nil
		},
	)
	if err != nil || uses != 1 || current.authority.retainedCalls != 1 ||
		current.authority.retainedCallbackCalls != 1 || current.source.calls != 0 ||
		current.observer.calls != 2 ||
		current.authority.retainedInput.RetainedDatasetSnapshotID != historical.securityContext.DatasetSnapshotID ||
		current.authority.retainedInput.RetainedSourceManifestHash != historical.securityContext.SourceManifestHash {
		t.Fatalf(
			"historical retained display lost authority: uses=%d retained=%d/%d currentSource=%d observations=%d err=%v",
			uses, current.authority.retainedCalls, current.authority.retainedCallbackCalls,
			current.source.calls, current.observer.calls, err,
		)
	}

	for _, test := range []struct {
		name   string
		mutate func(*domainsecurity.TurnSecurityContext)
	}{
		{
			name: "snapshot swap",
			mutate: func(candidate *domainsecurity.TurnSecurityContext) {
				candidate.DatasetSnapshotID = securitycontextfixture.DatasetSnapshotID("accepted-slot-swapped-history")
			},
		},
		{
			name: "manifest swap",
			mutate: func(candidate *domainsecurity.TurnSecurityContext) {
				candidate.SourceManifestHash = digestV1("accepted-slot-swapped-manifest")
			},
		},
		{
			name: "epoch mismatch",
			mutate: func(candidate *domainsecurity.TurnSecurityContext) {
				candidate.ContextEpoch++
			},
		},
		{
			name: "case mismatch",
			mutate: func(candidate *domainsecurity.TurnSecurityContext) {
				candidate.CaseID = "case-accepted-slot-other"
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			wrongHistorical := historical.securityContext
			test.mutate(&wrongHistorical)
			called := false
			err := service.UseRetainedAcceptedSlotDisplay(
				context.Background(), current.securityContext, wrongHistorical,
				[]domainevidence.AcceptedSlotSourceBindingV1{historical.binding}, canonical,
				func(string) error { called = true; return nil },
			)
			if err == nil || called {
				t.Fatalf("mismatched retained authority projected source bytes: called=%t err=%v", called, err)
			}
		})
	}
}

func TestServiceRejectsCountOnlyAndMissingAnalyticalSources(t *testing.T) {
	for _, test := range []struct {
		name string
		kind snapshotKindV1
		want error
	}{
		{name: "count-only FPC2", kind: snapshotKindCountOnlyV1, want: fundsquerysourceport.ErrMismatch},
		{name: "FPC1 without analytical binding", kind: snapshotKindNoAnalyticalV1, want: fundsquerysourceport.ErrUnavailable},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newServiceFixtureV1(t, test.kind, "fixed-profile")
			called := false
			err := fixture.service.UseCurrent(
				context.Background(),
				fixture.securityContext,
				func(context.Context, domainfundsquerysource.DescriptorV1, fundsquerysourceport.ExactReadLease, domainnative.AccountFlowSourceRowResolverV1) error {
					called = true
					return nil
				},
			)
			if !errors.Is(err, test.want) || called || fixture.source.calls != 0 ||
				fixture.authority.exactUseCalls != 0 {
				t.Fatalf(
					"rejection = err=%v called=%t source=%d exact=%d",
					err,
					called,
					fixture.source.calls,
					fixture.authority.exactUseCalls,
				)
			}
		})
	}
}

func TestServiceRejectsEveryCurrentProvenanceMismatchBeforePrivateUse(t *testing.T) {
	t.Run("fresh case binding", func(t *testing.T) {
		fixture := newServiceFixtureV1(t, snapshotKindAnalyticalV1, "fixed-profile")
		other, err := domainsecurity.NewCaseBindingObservationV1(
			domainsecurity.CaseBindingObservationInputV1{
				WorkspaceRealPath: fixture.observation.WorkspaceRealPath,
				State:             domainsecurity.CaseBindingStateValid,
				CaseID:            "case-other-002",
				BindingSHA256:     digestV1("other-binding-file"),
				CaseBindingHash:   digestV1("other-binding"),
			},
		)
		if err != nil {
			t.Fatal(err)
		}
		fixture.observer.observation = other
		assertUseRejectedBeforeSourceV1(t, fixture, fundsquerysourceport.ErrMismatch, true)
	})

	for _, test := range []struct {
		name   string
		mutate func(*domainsecurity.TurnSecurityContextInput)
	}{
		{
			name: "snapshot id",
			mutate: func(input *domainsecurity.TurnSecurityContextInput) {
				input.DatasetSnapshotID = domainsecurity.DatasetSnapshotIDPrefixV2 + digestV1("other-snapshot")
			},
		},
		{
			name: "source manifest",
			mutate: func(input *domainsecurity.TurnSecurityContextInput) {
				input.SourceManifestHash = digestV1("other-source-manifest")
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newServiceFixtureV1(t, snapshotKindAnalyticalV1, "fixed-profile")
			fixture.securityContext = securityContextV1(
				t,
				fixture.observation,
				fixture.selection.Snapshot,
				test.mutate,
			)
			assertUseRejectedBeforeSourceV1(t, fixture, fundsquerysourceport.ErrMismatch, false)
		})
	}

	t.Run("selected index record", func(t *testing.T) {
		fixture := newServiceFixtureV1(t, snapshotKindAnalyticalV1, "fixed-profile")
		other := newServiceFixtureV1(t, snapshotKindAnalyticalV1, "other-selection")
		fixture.selection.SelectedIndex = other.selection.SelectedIndex
		fixture.selection.DatasetIndexPath = []domainsecurity.DatasetSnapshotIndexV1{other.selection.SelectedIndex}
		fixture.authority.selection = fixture.selection
		assertUseRejectedBeforeSourceV1(t, fixture, fundsquerysourceport.ErrMismatch, false)
	})

	t.Run("fixed query profile", func(t *testing.T) {
		fixture := newServiceFixtureV1(t, snapshotKindAnalyticalV1, "other-query-profile")
		assertUseRejectedBeforeSourceV1(t, fixture, fundsquerysourceport.ErrMismatch, false)
	})

	t.Run("host exact source", func(t *testing.T) {
		fixture := newServiceFixtureV1(t, snapshotKindAnalyticalV1, "fixed-profile")
		fixture.source.failure = errors.Join(
			fundsquerysourceport.ErrMismatch,
			errors.New("host immutable source provenance changed"),
		)
		called := false
		err := fixture.service.UseCurrent(
			context.Background(),
			fixture.securityContext,
			func(context.Context, domainfundsquerysource.DescriptorV1, fundsquerysourceport.ExactReadLease, domainnative.AccountFlowSourceRowResolverV1) error {
				called = true
				return nil
			},
		)
		if !errors.Is(err, fundsquerysourceport.ErrMismatch) || called || fixture.source.calls != 1 {
			t.Fatalf("host source mismatch = err=%v called=%t calls=%d", err, called, fixture.source.calls)
		}
	})
}

func TestServiceRejectsRepeatedAuthorityCallbacksWithoutRepeatingConsumer(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*serviceFixtureV1)
	}{
		{
			name: "current selection",
			mutate: func(fixture *serviceFixtureV1) {
				fixture.authority.callbackInvocations = 2
			},
		},
		{
			name: "DSV2 exact use",
			mutate: func(fixture *serviceFixtureV1) {
				fixture.authority.exactCallbackInvocations = 2
			},
		},
		{
			name: "host exact source",
			mutate: func(fixture *serviceFixtureV1) {
				fixture.source.callbackInvocations = 2
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newServiceFixtureV1(t, snapshotKindAnalyticalV1, "fixed-profile")
			test.mutate(fixture)
			consumerCalls := 0
			err := fixture.service.UseCurrent(
				context.Background(),
				fixture.securityContext,
				func(context.Context, domainfundsquerysource.DescriptorV1, fundsquerysourceport.ExactReadLease, domainnative.AccountFlowSourceRowResolverV1) error {
					consumerCalls++
					return nil
				},
			)
			if !errors.Is(err, fundsquerysourceport.ErrMismatch) || consumerCalls != 1 {
				t.Fatalf("repeated callback = err=%v consumerCalls=%d", err, consumerCalls)
			}
		})
	}
}

func TestServiceRejectsMissingAuthorityCallbacksAndInvalidConfiguration(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*serviceFixtureV1)
	}{
		{
			name: "current selection",
			mutate: func(fixture *serviceFixtureV1) {
				fixture.authority.callbackInvocations = 0
			},
		},
		{
			name: "DSV2 exact use",
			mutate: func(fixture *serviceFixtureV1) {
				fixture.authority.exactCallbackInvocations = 0
			},
		},
		{
			name: "host exact source",
			mutate: func(fixture *serviceFixtureV1) {
				fixture.source.callbackInvocations = 0
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newServiceFixtureV1(t, snapshotKindAnalyticalV1, "fixed-profile")
			test.mutate(fixture)
			called := false
			err := fixture.service.UseCurrent(
				context.Background(),
				fixture.securityContext,
				func(context.Context, domainfundsquerysource.DescriptorV1, fundsquerysourceport.ExactReadLease, domainnative.AccountFlowSourceRowResolverV1) error {
					called = true
					return nil
				},
			)
			if !errors.Is(err, fundsquerysourceport.ErrUnavailable) || called {
				t.Fatalf("missing callback = err=%v called=%t", err, called)
			}
		})
	}

	fixture := newServiceFixtureV1(t, snapshotKindAnalyticalV1, "fixed-profile")
	if _, err := NewService(nil, fixture.observer, fixture.materials, fixture.source); err == nil {
		t.Fatal("nil DSV2 authority configured a funds source service")
	}
	var typedNil *fakeCurrentAuthorityV2
	if _, err := NewService(typedNil, fixture.observer, fixture.materials, fixture.source); err == nil {
		t.Fatal("typed-nil DSV2 authority configured a funds source service")
	}
	if _, err := NewService(fixture.authority, fixture.observer, nil, fixture.source); err == nil {
		t.Fatal("nil material reader configured a funds source service")
	}
	var typedNilMaterials *fakeAdmissionMaterialReaderV2
	if _, err := NewService(fixture.authority, fixture.observer, typedNilMaterials, fixture.source); err == nil {
		t.Fatal("typed-nil material reader configured a funds source service")
	}
	if err := fixture.service.UseCurrent(
		context.Background(),
		fixture.securityContext,
		nil,
	); !errors.Is(err, fundsquerysourceport.ErrUnavailable) {
		t.Fatalf("nil use callback error = %v", err)
	}
}

func assertUseRejectedBeforeSourceV1(
	t *testing.T,
	fixture *serviceFixtureV1,
	want error,
	wantBeforeAuthority bool,
) {
	t.Helper()
	called := false
	err := fixture.service.UseCurrent(
		context.Background(),
		fixture.securityContext,
		func(context.Context, domainfundsquerysource.DescriptorV1, fundsquerysourceport.ExactReadLease, domainnative.AccountFlowSourceRowResolverV1) error {
			called = true
			return nil
		},
	)
	if !errors.Is(err, want) || called || fixture.source.calls != 0 {
		t.Fatalf("provenance mismatch = err=%v called=%t source=%d", err, called, fixture.source.calls)
	}
	if wantBeforeAuthority && fixture.authority.calls != 0 {
		t.Fatalf("binding mismatch entered DSV2 authority: calls=%d", fixture.authority.calls)
	}
}

type snapshotKindV1 int

const (
	snapshotKindAnalyticalV1 snapshotKindV1 = iota
	snapshotKindNoAnalyticalV1
	snapshotKindCountOnlyV1
)

type serviceFixtureV1 struct {
	service         *Service
	authority       *fakeCurrentAuthorityV2
	observer        *fakeBindingObserverV1
	materials       *fakeAdmissionMaterialReaderV2
	source          *fakeHostExactSourceV1
	observation     domainsecurity.CaseBindingObservationV1
	selection       datasetsnapshotport.CurrentSelectionV2
	securityContext domainsecurity.TurnSecurityContext
	duckDB          []byte
}

func newServiceFixtureV1(
	t *testing.T,
	kind snapshotKindV1,
	seed string,
) *serviceFixtureV1 {
	t.Helper()
	observation, err := domainsecurity.NewCaseBindingObservationV1(
		domainsecurity.CaseBindingObservationInputV1{
			WorkspaceRealPath: "/private/cases/funds-query-source",
			State:             domainsecurity.CaseBindingStateValid,
			CaseID:            "case-funds-001",
			BindingSHA256:     digestV1("binding-file"),
			CaseBindingHash:   digestV1("binding"),
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	privateKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{0x31}, ed25519.SeedSize))
	publicKey := privateKey.Public().(ed25519.PublicKey)
	duckDB := []byte("immutable-duckdb-account-flow-fixture")

	base, err := datasetsnapshotfixture.NewResolvedSnapshotV2(
		datasetsnapshotfixture.ResolvedInput{
			TenantID:           domainsecurity.LocalTenantID,
			UserID:             domainsecurity.LocalUserID,
			Observation:        observation,
			Material:           "funds-query-source-" + seed,
			InstallationID:     digestV1("installation"),
			AcceptedAt:         time.Date(2026, 7, 28, 2, 0, 0, 0, time.UTC),
			AuthorityKeyID:     domainsecurity.SHA256Hex(publicKey),
			AuthorityPublicKey: publicKey,
			Sign: func(message []byte) ([]byte, error) {
				return ed25519.Sign(privateKey, message), nil
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	var snapshot datasetsnapshotport.ResolvedSnapshotV2
	switch kind {
	case snapshotKindAnalyticalV1:
		profile := domainfundsquerysource.FixedAccountFlowQueryProfileDigestV1()
		if seed == "combined-query-profile" {
			profile = domainfundsquerysource.FixedFundsQueryProfileDigestV1()
		}
		if seed == "local-display-profile" {
			profile = domainfundsquerysource.FixedFundsLocalDisplayQueryProfileDigestV1()
		}
		if seed == "other-query-profile" {
			profile = digestV1("other-query-profile")
		}
		binding, bindingErr := domainsecurity.NewDatasetSnapshotAnalyticalDuckDBBindingV2(
			domainsecurity.DatasetSnapshotAnalyticalDuckDBBindingInputV2{
				DuckDBSHA256:                 domainsecurity.SHA256Hex(duckDB),
				DuckDBByteLength:             uint64(len(duckDB)),
				DuckDBContentSnapshotDigest:  digestV1("duckdb-content-" + seed),
				DuckDBSnapshotManifestSHA256: digestV1("duckdb-manifest-" + seed),
				MaterializationIdentity: domainsecurity.FundsMaterializationIdentityPrefixV1 +
					digestV1("materialization-"+seed),
				SchemaDigest:            domainfundsquerysource.FixedFundsAnalyticalSchemaDigestV1(),
				QueryProfileDigest:      profile,
				DatasetUTCOffsetMinutes: 480,
				ExpectedCurrency:        "CNY",
				MinorUnitScale:          domainfundsquerysource.AccountFlowMinorUnitScaleV1,
			},
		)
		if bindingErr != nil {
			t.Fatal(bindingErr)
		}
		snapshot = rebuildFPC1SnapshotV1(t, base, binding, privateKey, publicKey)
	case snapshotKindNoAnalyticalV1:
		snapshot = base
	case snapshotKindCountOnlyV1:
		snapshot = rebuildFPC2SnapshotV1(t, base, privateKey, publicKey)
	default:
		t.Fatal("unsupported snapshot fixture kind")
	}

	index, err := domainsecurity.NewDatasetSnapshotIndexV1(
		domainsecurity.DatasetSnapshotIndexInputV1{
			InstallationID:       snapshot.Record.InstallationID,
			EnrollmentID:         digestV1("enrollment"),
			Generation:           1,
			PreviousIndexDigest:  domainsecurity.DatasetSnapshotIndexGenesisDigestV1(),
			MutationID:           digestV1("index-mutation-" + seed),
			Binding:              snapshot.Record.Binding,
			SnapshotRecordDigest: snapshot.Record.RecordDigest,
			AuthorityKeyID:       domainsecurity.SHA256Hex(publicKey),
			AuthorityPublicKey:   publicKey,
		},
		func(message []byte) ([]byte, error) {
			return ed25519.Sign(privateKey, message), nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	selection := datasetsnapshotport.CurrentSelectionV2{
		DatasetIndexPath: []domainsecurity.DatasetSnapshotIndexV1{index},
		SelectedIndex:    index,
		Snapshot:         snapshot,
		SelectionDigest:  digestV1("selection-" + seed),
	}
	securityContext := securityContextV1(t, observation, snapshot, nil)
	authority := &fakeCurrentAuthorityV2{
		selection:                selection,
		expectedSecurityContext:  securityContext,
		callbackInvocations:      1,
		exactCallbackInvocations: 1,
	}
	observer := &fakeBindingObserverV1{observation: observation}
	source := &fakeHostExactSourceV1{
		body:                duckDB,
		callbackInvocations: 1,
	}
	materials := &fakeAdmissionMaterialReaderV2{
		values: map[datasetsnapshotport.MaterialKindV2]map[string][]byte{},
	}
	service, err := NewService(authority, observer, materials, source)
	if err != nil {
		t.Fatal(err)
	}
	return &serviceFixtureV1{
		service: service, authority: authority, observer: observer, materials: materials, source: source,
		observation: observation, selection: selection, securityContext: securityContext,
		duckDB: duckDB,
	}
}

func rebuildFPC1SnapshotV1(
	t *testing.T,
	base datasetsnapshotport.ResolvedSnapshotV2,
	analytical domainsecurity.DatasetSnapshotAnalyticalDuckDBBindingV2,
	privateKey ed25519.PrivateKey,
	publicKey ed25519.PublicKey,
) datasetsnapshotport.ResolvedSnapshotV2 {
	t.Helper()
	input := manifestInputV1(t, base)
	policy, ok := domainevidence.ResolveSourceRowProducerPolicyV1(
		domainevidence.FundsTransactionSourceRowPolicyIDV1,
	)
	if !ok {
		t.Fatal("transaction source-row policy is unavailable")
	}
	input.SourceType = policy.SourceType
	input.ProducerPolicyID = policy.PolicyID
	input.ProducerPolicyDigest = policy.PolicyDigest
	input.ProducerComponentID = policy.ProducerComponentID
	input.ProducerComponentVersion = policy.ProducerComponentVersion
	input.ProducerOperation = policy.Operation
	input.ProducerOperationSchemaHash = policy.OperationSchemaHash
	input.ParserID = policy.ParserID
	input.ParserVersion = policy.ParserVersion
	input.AnalyticalDuckDB = analytical
	manifest, err := domainsecurity.NewDatasetSnapshotManifestV2(input)
	if err != nil {
		t.Fatal(err)
	}
	record := issueSnapshotRecordV1(t, manifest, base.FundsProducerContent, domainsecurity.FundsProducerContentManifestV2{}, privateKey, publicKey)
	return datasetsnapshotport.ResolvedSnapshotV2{
		Record: record, Manifest: manifest, FundsProducerContent: base.FundsProducerContent,
	}
}

func rebuildFPC2SnapshotV1(
	t *testing.T,
	base datasetsnapshotport.ResolvedSnapshotV2,
	privateKey ed25519.PrivateKey,
	publicKey ed25519.PublicKey,
) datasetsnapshotport.ResolvedSnapshotV2 {
	t.Helper()
	input := manifestInputV1(t, base)
	input.FundsProducerContentManifest = domainsecurity.FundsProducerContentManifestV1{}
	producer, err := domainsecurity.NewFundsProducerContentManifestV2(
		domainsecurity.FundsProducerContentManifestInputV2{
			CaseID:                    base.Manifest.Binding.CaseID,
			SourceRevision:            1,
			RawArtifactManifestSHA256: base.Manifest.RawArtifactManifestSHA256,
			NormalizedContentSHA256:   digestV1("fpc2-normalized"),
			DetailContentSHA256:       digestV1("fpc2-detail"),
			SourceRowCount:            base.Manifest.SourceRecordCount,
			AcceptedRowCount:          base.Manifest.AcceptedRecordCount,
			RejectedRowCount:          base.Manifest.RejectedRecordCount,
			DuplicateRowCount:         base.Manifest.DuplicateRecordCount,
			DetailRowCount:            base.Manifest.AcceptedRecordCount,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := domainsecurity.NewDatasetSnapshotManifestForFundsProducerContentV2(input, producer)
	if err != nil {
		t.Fatal(err)
	}
	record := issueSnapshotRecordV1(t, manifest, domainsecurity.FundsProducerContentManifestV1{}, producer, privateKey, publicKey)
	return datasetsnapshotport.ResolvedSnapshotV2{
		Record: record, Manifest: manifest, FundsProducerContentV2: producer,
	}
}

func issueSnapshotRecordV1(
	t *testing.T,
	manifest domainsecurity.DatasetSnapshotManifestV2,
	producerV1 domainsecurity.FundsProducerContentManifestV1,
	producerV2 domainsecurity.FundsProducerContentManifestV2,
	privateKey ed25519.PrivateKey,
	publicKey ed25519.PublicKey,
) domainsecurity.DatasetSnapshotAuthorityRecordV2 {
	t.Helper()
	var record domainsecurity.DatasetSnapshotAuthorityRecordV2
	issue := func(issuer domainsecurity.DatasetSnapshotAuthoritySealedAdmissionV2) error {
		var err error
		record, err = issuer.Issue(
			domainsecurity.DatasetSnapshotAuthoritySealedIssueInputV2{
				InstallationID:     digestV1("installation"),
				AcceptedAt:         time.Date(2026, 7, 28, 2, 0, 0, 0, time.UTC),
				AuthorityKeyID:     domainsecurity.SHA256Hex(publicKey),
				AuthorityPublicKey: publicKey,
			},
			func(message []byte) ([]byte, error) {
				return ed25519.Sign(privateKey, message), nil
			},
		)
		return err
	}
	var err error
	if producerV1 != (domainsecurity.FundsProducerContentManifestV1{}) {
		err = manifest.WithExactFundsProducerAuthorityAdmissionV2(producerV1, issue)
	} else {
		err = manifest.WithExactFundsProducerContentV2AuthorityAdmissionV2(producerV2, issue)
	}
	if err != nil {
		t.Fatal(err)
	}
	return record
}

func manifestInputV1(
	t *testing.T,
	base datasetsnapshotport.ResolvedSnapshotV2,
) domainsecurity.DatasetSnapshotManifestInputV2 {
	t.Helper()
	manifest := base.Manifest
	acquiredAt, err := time.Parse(time.RFC3339Nano, manifest.AcquiredAt)
	if err != nil {
		t.Fatal(err)
	}
	return domainsecurity.DatasetSnapshotManifestInputV2{
		Binding:                           manifest.Binding,
		AcquisitionMethod:                 manifest.AcquisitionMethod,
		AcquiredAt:                        acquiredAt,
		AcquisitionActorDigest:            manifest.AcquisitionActorDigest,
		RawArtifactManifestDigest:         manifest.RawArtifactManifestDigest,
		RawArtifactManifestSHA256:         manifest.RawArtifactManifestSHA256,
		RawArtifactManifestByteLength:     manifest.RawArtifactManifestByteLength,
		RawArtifactCount:                  manifest.RawArtifactCount,
		FundsProducerContentManifest:      base.FundsProducerContent,
		SourceType:                        manifest.SourceType,
		ProducerPolicyID:                  manifest.ProducerPolicyID,
		ProducerPolicyDigest:              manifest.ProducerPolicyDigest,
		ProducerComponentID:               manifest.ProducerComponentID,
		ProducerComponentVersion:          manifest.ProducerComponentVersion,
		ProducerOperation:                 manifest.ProducerOperation,
		ProducerOperationSchemaHash:       manifest.ProducerOperationSchemaHash,
		ParserID:                          manifest.ParserID,
		ParserVersion:                     manifest.ParserVersion,
		ParsedGenerationReceiptDigest:     manifest.ParsedGenerationReceiptDigest,
		ParsedGenerationReceiptSHA256:     manifest.ParsedGenerationReceiptSHA256,
		ParsedGenerationReceiptByteLength: manifest.ParsedGenerationReceiptByteLength,
		ClassificationLedgerDigest:        manifest.ClassificationLedgerDigest,
		ClassificationLedgerSHA256:        manifest.ClassificationLedgerSHA256,
		ClassificationLedgerByteLength:    manifest.ClassificationLedgerByteLength,
		TimezoneSemantics:                 manifest.TimezoneSemantics,
		CurrencySemantics:                 manifest.CurrencySemantics,
		SourceRowLedgerRootDigest:         manifest.SourceRowLedgerRootDigest,
		SourceRowLedgerRootSHA256:         manifest.SourceRowLedgerRootSHA256,
		SourceRowLedgerRootByteLength:     manifest.SourceRowLedgerRootByteLength,
		SourceRowLedgerPageCount:          manifest.SourceRowLedgerPageCount,
		SourceRecordCount:                 manifest.SourceRecordCount,
		AcceptedRecordCount:               manifest.AcceptedRecordCount,
		RejectedRecordCount:               manifest.RejectedRecordCount,
		DuplicateRecordCount:              manifest.DuplicateRecordCount,
	}
}

func securityContextV1(
	t *testing.T,
	observation domainsecurity.CaseBindingObservationV1,
	snapshot datasetsnapshotport.ResolvedSnapshotV2,
	mutate func(*domainsecurity.TurnSecurityContextInput),
) domainsecurity.TurnSecurityContext {
	t.Helper()
	policyDigest := digestV1("case-risk-policy")
	publication, err := domainsecurity.NewTurnPublicationPolicyV1(
		domainsecurity.TurnPublicationPolicyInputV1{
			ThreadRiskPolicyDigest:   policyDigest,
			RiskClass:                domainsecurity.RiskClassCase,
			Disposition:              domainsecurity.PublicationDispositionCaseEvidenceGate,
			CaseBindingState:         domainsecurity.CaseBindingStateValid,
			BindingObservationDigest: observation.ObservationDigest,
			BlockerCode:              domainsecurity.PublicationBlockerNone,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	riskBinding, err := securitycontextfixture.WitnessedRiskBinding(
		"thread-funds-query-source",
		observation.WorkspaceRealPath,
		domainsecurity.RiskClassCase,
		policyDigest,
	)
	if err != nil {
		t.Fatal(err)
	}
	input := domainsecurity.TurnSecurityContextInput{
		ThreadID:             "thread-funds-query-source",
		TurnID:               "turn-funds-query-source",
		WorkspaceRealPath:    observation.WorkspaceRealPath,
		TenantID:             domainsecurity.LocalTenantID,
		UserID:               domainsecurity.LocalUserID,
		CaseID:               observation.CaseID,
		CaseBindingHash:      observation.CaseBindingHash,
		DatasetSnapshotID:    snapshot.Record.DatasetSnapshotID,
		SourceManifestHash:   snapshot.Record.SourceManifestHash,
		ContextEpoch:         7,
		IssuedAt:             time.Date(2026, 7, 28, 2, 5, 0, 0, time.UTC),
		PublicationPolicy:    publication,
		RiskAuthorityBinding: riskBinding,
	}
	if mutate != nil {
		mutate(&input)
	}
	securityContext, err := domainsecurity.NewTurnSecurityContextV2(input)
	if err != nil {
		t.Fatal(err)
	}
	return securityContext
}

type fakeBindingObserverV1 struct {
	observation domainsecurity.CaseBindingObservationV1
	err         error
	calls       int
}

func (observer *fakeBindingObserverV1) Observe(workspace string) (domainsecurity.CaseBindingObservationV1, error) {
	observer.calls++
	if observer.err != nil {
		return domainsecurity.CaseBindingObservationV1{}, observer.err
	}
	if workspace != observer.observation.WorkspaceRealPath {
		return domainsecurity.CaseBindingObservationV1{}, errors.New("unexpected workspace")
	}
	return observer.observation, nil
}

type fakeCurrentAuthorityV2 struct {
	selection                datasetsnapshotport.CurrentSelectionV2
	expectedSecurityContext  domainsecurity.TurnSecurityContext
	callbackInvocations      int
	exactCallbackInvocations int
	failure                  error
	calls                    int
	callbackCalls            int
	exactUseCalls            int
	postNativeUseCalls       int
	exactCallbackCalls       int
	resolveInput             datasetsnapshotport.ResolveInputV2
	securityContext          domainsecurity.TurnSecurityContext
	retainedCapability       *fakeCurrentSelectionCapabilityV2
	resolveCalls             int
	resolveFailureAt         int
	retainedSelection        datasetsnapshotport.RetainedSelectionV2
	retainedFailure          error
	retainedCalls            int
	retainedCallbackCalls    int
	retainedInput            datasetsnapshotport.RetainedSelectionInputV2
}

func (authority *fakeCurrentAuthorityV2) WithRetainedSelectionV2(
	ctx context.Context,
	input datasetsnapshotport.RetainedSelectionInputV2,
	callback func(context.Context, datasetsnapshotport.RetainedSelectionV2) error,
) error {
	authority.retainedCalls++
	authority.retainedInput = input
	if authority.retainedFailure != nil {
		return authority.retainedFailure
	}
	if ctx == nil || callback == nil || ctx.Err() != nil {
		return datasetsnapshotport.ErrUnavailable
	}
	leaseContext, cancel := context.WithCancel(ctx)
	defer cancel()
	authority.retainedCallbackCalls++
	return callback(leaseContext, authority.retainedSelection)
}

func (authority *fakeCurrentAuthorityV2) ResolveWitnessedV2(
	ctx context.Context,
	input datasetsnapshotport.ResolveInputV2,
) (datasetsnapshotport.ResolvedSnapshotV2, error) {
	authority.resolveCalls++
	authority.resolveInput = input
	if ctx == nil || ctx.Err() != nil || authority.resolveFailureAt == authority.resolveCalls {
		return datasetsnapshotport.ResolvedSnapshotV2{}, datasetsnapshotport.ErrStale
	}
	return authority.selection.Snapshot, nil
}

func (authority *fakeCurrentAuthorityV2) WithCurrentSelectionV2(
	ctx context.Context,
	input datasetsnapshotport.ResolveInputV2,
	securityContext domainsecurity.TurnSecurityContext,
	callback func(datasetsnapshotport.CurrentSelectionV2, datasetsnapshotport.CurrentSelectionCapabilityV2) error,
) error {
	authority.calls++
	authority.resolveInput = input
	authority.securityContext = securityContext
	if authority.failure != nil {
		return authority.failure
	}
	if ctx == nil || callback == nil {
		return datasetsnapshotport.ErrUnavailable
	}
	capability := &fakeCurrentSelectionCapabilityV2{
		authority:           authority,
		active:              true,
		selection:           authority.selection,
		securityContext:     securityContext,
		callbackInvocations: authority.exactCallbackInvocations,
	}
	authority.retainedCapability = capability
	defer capability.close()
	for index := 0; index < authority.callbackInvocations; index++ {
		authority.callbackCalls++
		if err := callback(authority.selection, capability); err != nil {
			return err
		}
	}
	return nil
}

type fakeCurrentSelectionCapabilityV2 struct {
	mu                  sync.Mutex
	authority           *fakeCurrentAuthorityV2
	active              bool
	selection           datasetsnapshotport.CurrentSelectionV2
	securityContext     domainsecurity.TurnSecurityContext
	callbackInvocations int
}

func (capability *fakeCurrentSelectionCapabilityV2) UseExact(
	selection datasetsnapshotport.CurrentSelectionV2,
	securityContext domainsecurity.TurnSecurityContext,
	callback func(context.Context) error,
) error {
	return capability.useExact(selection, securityContext, callback, false)
}

func (capability *fakeCurrentSelectionCapabilityV2) UsePostNativeExact(
	selection datasetsnapshotport.CurrentSelectionV2,
	securityContext domainsecurity.TurnSecurityContext,
	callback func(context.Context) error,
) error {
	return capability.useExact(selection, securityContext, callback, true)
}

func (capability *fakeCurrentSelectionCapabilityV2) useExact(
	selection datasetsnapshotport.CurrentSelectionV2,
	securityContext domainsecurity.TurnSecurityContext,
	callback func(context.Context) error,
	postNative bool,
) error {
	capability.mu.Lock()
	if !capability.active || callback == nil ||
		!reflect.DeepEqual(selection, capability.selection) ||
		securityContext != capability.securityContext {
		capability.mu.Unlock()
		return datasetsnapshotport.ErrUnavailable
	}
	capability.authority.exactUseCalls++
	if postNative {
		capability.authority.postNativeUseCalls++
	}
	capability.mu.Unlock()

	leaseContext, cancel := context.WithCancel(context.Background())
	defer cancel()
	for index := 0; index < capability.callbackInvocations; index++ {
		capability.authority.exactCallbackCalls++
		if err := callback(leaseContext); err != nil {
			return err
		}
	}
	return nil
}

func (capability *fakeCurrentSelectionCapabilityV2) CommitExact(
	selection datasetsnapshotport.CurrentSelectionV2,
	securityContext domainsecurity.TurnSecurityContext,
	commit func(context.Context) error,
) error {
	capability.mu.Lock()
	if !capability.active || commit == nil ||
		!reflect.DeepEqual(selection, capability.selection) ||
		securityContext != capability.securityContext {
		capability.mu.Unlock()
		return datasetsnapshotport.ErrUnavailable
	}
	capability.authority.exactUseCalls++
	capability.mu.Unlock()
	leaseContext, cancel := context.WithCancel(context.Background())
	defer cancel()
	capability.authority.exactCallbackCalls++
	return commit(leaseContext)
}

func (capability *fakeCurrentSelectionCapabilityV2) close() {
	capability.mu.Lock()
	capability.active = false
	capability.mu.Unlock()
}

type fakeHostExactSourceV1 struct {
	body                []byte
	failure             error
	callbackInvocations int
	calls               int
	callbackCalls       int
	descriptor          domainfundsquerysource.DescriptorV1
	retainedLease       *fundsquerysourcefixture.ExactReadLeaseV1
}

func (source *fakeHostExactSourceV1) WithExact(
	ctx context.Context,
	descriptor domainfundsquerysource.DescriptorV1,
	callback func(context.Context, fundsquerysourceport.ExactReadLease) error,
) error {
	source.calls++
	source.descriptor = descriptor
	if source.failure != nil {
		return source.failure
	}
	if ctx == nil || callback == nil || domainfundsquerysource.ValidateDescriptorV1(descriptor) != nil {
		return fundsquerysourceport.ErrMismatch
	}
	lease := fundsquerysourcefixture.NewExactReadLeaseV1(source.body)
	source.retainedLease = lease
	defer lease.Close()
	for index := 0; index < source.callbackInvocations; index++ {
		source.callbackCalls++
		if err := callback(ctx, lease); err != nil {
			return err
		}
	}
	return nil
}

func digestV1(value string) string {
	return domainsecurity.SHA256Hex([]byte("funds-query-source-test:\x00" + value))
}
