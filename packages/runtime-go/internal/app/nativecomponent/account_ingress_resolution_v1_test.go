package nativecomponent

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	domainfundsquerysource "analytix.local/runtime-go/internal/domain/fundsquerysource"
	domainnative "analytix.local/runtime-go/internal/domain/nativecomponent"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	fundsquerysourceport "analytix.local/runtime-go/internal/ports/fundsquerysource"
	"analytix.local/runtime-go/internal/testsupport/fundsquerysourcefixture"
)

func TestResolveAccountIngressUsesOneExactCurrentSourceAndReturnsOnlySafeOrderedResult(t *testing.T) {
	fixture := newNativeFlowAdmissionFixture(t)
	first, err := domainnative.NewResolveAccountIngressCandidateInputV1(2, "6222021234567890123")
	if err != nil {
		t.Fatal(err)
	}
	second, err := domainnative.NewResolveAccountIngressCandidateInputV1(7, "6217009876543210987")
	if err != nil {
		t.Fatal(err)
	}
	runner := &accountIngressRunnerStubV1{
		securityContext: fixture.securityContext,
		descriptor:      fixture.descriptor,
		resolutions: []domainnative.AccountIngressResolutionV1{
			{
				Ordinal: 2, Disposition: domainnative.AccountIngressResolutionDispositionResolvedV1,
				EntityType: "bank_account_number", BankInstitution: "中国银行", AccountType: "储蓄账户",
			},
			{Ordinal: 7, Disposition: domainnative.AccountIngressResolutionDispositionNotFoundV1},
		},
	}
	service := fixture.service(&nativeRunnerStub{})
	service.dependencies.AccountIngressRunner = runner
	effectAcquires := 0
	effectReleases := 0
	service.dependencies.AcquireEffect = func(
		ctx context.Context,
		securityContext domainsecurity.TurnSecurityContext,
	) (context.Context, func(), error) {
		if securityContext != fixture.securityContext {
			return nil, nil, ErrAuthorityInvalid
		}
		effectAcquires++
		return ctx, func() { effectReleases++ }, nil
	}
	sourceUses := 0
	service.dependencies.UseCurrentAccountIngressSource = func(
		ctx context.Context,
		securityContext domainsecurity.TurnSecurityContext,
		use func(context.Context, domainfundsquerysource.DescriptorV1, fundsquerysourceport.ExactReadLease) error,
	) error {
		if securityContext != fixture.securityContext {
			return ErrAuthorityInvalid
		}
		sourceUses++
		return use(ctx, fixture.descriptor, fixture.source)
	}

	result, descriptor, err := service.ResolveAccountIngress(
		context.Background(),
		NewResolveAccountIngressInputV1(fixture.securityContext, []domainnative.ResolveAccountIngressCandidateInputV1{first, second}),
	)
	if err != nil {
		t.Fatalf("resolve account ingress: %v", err)
	}
	if effectAcquires != 1 || effectReleases != 1 || sourceUses != 1 ||
		runner.calls != 1 || fixture.source.copies != 1 || descriptor != fixture.descriptor ||
		len(result.Resolutions) != 2 ||
		result.Resolutions[0].Ordinal != 2 || result.Resolutions[1].Ordinal != 7 ||
		result.Resolutions[0].BankInstitution != "中国银行" {
		t.Fatalf(
			"unexpected lease/result behavior: effect=%d/%d source=%d runner=%d copies=%d result=%#v",
			effectAcquires,
			effectReleases,
			sourceUses,
			runner.calls,
			fixture.source.copies,
			result,
		)
	}
	encoded := strings.ToLower(mustAccountIngressJSONV1(t, result))
	for _, forbidden := range []string{
		"6222021234567890123", "6217009876543210987", "databasepath", "cer1_",
	} {
		if strings.Contains(encoded, strings.ToLower(forbidden)) {
			t.Fatalf("safe account-ingress result exposed %q: %s", forbidden, encoded)
		}
	}
}

func TestResolveAccountIngressFailsClosedOnAmbiguityAndRepeatedSourceCallback(t *testing.T) {
	for _, testCase := range []struct {
		name        string
		disposition string
		repeat      bool
	}{
		{name: "ambiguous", disposition: domainnative.AccountIngressResolutionDispositionAmbiguousV1},
		{name: "integrity", disposition: domainnative.AccountIngressResolutionDispositionIntegrityFailureV1},
		{name: "repeated callback", disposition: domainnative.AccountIngressResolutionDispositionResolvedV1, repeat: true},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			fixture := newNativeFlowAdmissionFixture(t)
			candidate, err := domainnative.NewResolveAccountIngressCandidateInputV1(1, "6222021234567890123")
			if err != nil {
				t.Fatal(err)
			}
			resolution := domainnative.AccountIngressResolutionV1{
				Ordinal: 1, Disposition: testCase.disposition,
			}
			if testCase.disposition == domainnative.AccountIngressResolutionDispositionResolvedV1 {
				resolution.EntityType = "bank_account_number"
			}
			runner := &accountIngressRunnerStubV1{
				securityContext: fixture.securityContext,
				descriptor:      fixture.descriptor,
				resolutions:     []domainnative.AccountIngressResolutionV1{resolution},
			}
			service := fixture.service(&nativeRunnerStub{})
			service.dependencies.AccountIngressRunner = runner
			service.dependencies.UseCurrentAccountIngressSource = func(
				ctx context.Context,
				_ domainsecurity.TurnSecurityContext,
				use func(context.Context, domainfundsquerysource.DescriptorV1, fundsquerysourceport.ExactReadLease) error,
			) error {
				if err := use(ctx, fixture.descriptor, fixture.source); err != nil {
					return err
				}
				if testCase.repeat {
					return use(ctx, fixture.descriptor, newNativeExactSourceStub())
				}
				return nil
			}

			result, descriptor, err := service.ResolveAccountIngress(
				context.Background(),
				NewResolveAccountIngressInputV1(
					fixture.securityContext,
					[]domainnative.ResolveAccountIngressCandidateInputV1{candidate},
				),
			)
			if !reflect.DeepEqual(result, domainnative.ResolveAccountIngressResultV1{}) ||
				descriptor != (domainfundsquerysource.DescriptorV1{}) ||
				(!errors.Is(err, ErrAccountIngressResolutionExecutionFailed) && !errors.Is(err, ErrAuthorityInvalid)) {
				t.Fatalf("fail-closed result=%#v err=%v", result, err)
			}
		})
	}
}

func TestFundsQueryProfilesKeepAccountFlowCompatibleAndRestrictAccountIngress(t *testing.T) {
	fixture := newNativeFlowAdmissionFixture(t)
	if !accountFlowDescriptorMatches(fixture.descriptor, fixture.securityContext) ||
		!accountIngressDescriptorMatchesV1(fixture.descriptor, fixture.securityContext) {
		t.Fatal("combined fixed profile did not support both private operations")
	}

	legacyInput := fixture.descriptorInput
	legacyInput.QueryProfileDigest =
		domainfundsquerysource.FixedAccountFlowQueryProfileDigestV1()
	legacy, err := domainfundsquerysource.NewDescriptorV1(legacyInput)
	if err != nil {
		t.Fatal(err)
	}
	if !accountFlowDescriptorMatches(legacy, fixture.securityContext) {
		t.Fatal("legacy account-flow profile lost existing account-flow compatibility")
	}
	if accountIngressDescriptorMatchesV1(legacy, fixture.securityContext) {
		t.Fatal("legacy account-flow-only profile authorized account ingress")
	}
}

type accountIngressRunnerStubV1 struct {
	securityContext domainsecurity.TurnSecurityContext
	descriptor      domainfundsquerysource.DescriptorV1
	resolutions     []domainnative.AccountIngressResolutionV1
	calls           int
}

func (runner *accountIngressRunnerStubV1) ResolveAccountIngress(
	ctx context.Context,
	securityContext domainsecurity.TurnSecurityContext,
	arguments domainnative.ResolveAccountIngressArgumentsV1,
	descriptor domainfundsquerysource.DescriptorV1,
	source fundsquerysourceport.ExactReadLease,
) (domainnative.ResolveAccountIngressResultV1, error) {
	runner.calls++
	if securityContext != runner.securityContext || descriptor != runner.descriptor ||
		domainnative.ValidateResolveAccountIngressArgumentsAuthorityV1(arguments, securityContext, descriptor) != nil {
		return domainnative.ResolveAccountIngressResultV1{}, errors.New("authority mismatch")
	}
	destination, err := fundsquerysourcefixture.NewUnlinkedDestinationV1("")
	if err != nil {
		return domainnative.ResolveAccountIngressResultV1{}, err
	}
	defer destination.Close()
	if err := source.CopyExactTo(ctx, destination); err != nil {
		return domainnative.ResolveAccountIngressResultV1{}, err
	}
	return domainnative.ResolveAccountIngressResultV1{
		SchemaVersion: 1,
		Contract:      domainnative.AccountIngressResolutionResultContractV1,
		Resolutions:   append([]domainnative.AccountIngressResolutionV1(nil), runner.resolutions...),
		Provenance: domainnative.AccountIngressResolutionProvenanceV1{
			DatasetSnapshotID:              securityContext.DatasetSnapshotID,
			ContextEpoch:                   securityContext.ContextEpoch,
			ContextDigest:                  securityContext.ContextDigest,
			CaseBindingHash:                securityContext.CaseBindingHash,
			ExpectedProducerContentID:      descriptor.FundsProducerContentID,
			ExpectedProducerManifestSHA256: descriptor.FundsProducerContentManifestSHA256,
			DuckDBContentSnapshotDigest:    descriptor.DuckDBContentSnapshotDigest,
			DuckDBSnapshotManifestSHA256:   descriptor.DuckDBSnapshotManifestSHA256,
			MaterializationIdentity:        descriptor.MaterializationIdentity,
			SourceSignature: strings.TrimPrefix(
				descriptor.MaterializationIdentity,
				domainsecurity.FundsMaterializationIdentityPrefixV1,
			),
			ResultSignature:        strings.Repeat("9", 64),
			ProducerContentID:      descriptor.FundsProducerContentID,
			ProducerManifestSHA256: descriptor.FundsProducerContentManifestSHA256,
			QueryContract:          domainnative.AccountIngressResolutionQueryContractV1,
			QuerySQLHash:           domainnative.AccountIngressResolutionQuerySQLHashV1,
		},
	}, nil
}

func mustAccountIngressJSONV1(t *testing.T, value any) string {
	t.Helper()
	body, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}
