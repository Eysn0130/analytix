package runtimeapp

import (
	"context"
	"errors"
	"testing"

	nativecomponentapp "analytix.local/runtime-go/internal/app/nativecomponent"
	domaincaseentity "analytix.local/runtime-go/internal/domain/caseentity"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	datasetsnapshotport "analytix.local/runtime-go/internal/ports/datasetsnapshot"
)

func TestFundsAccountFlowCompositionIsAdditiveAndFailClosed(t *testing.T) {
	if composition := composeRuntimeFundsAccountFlowV1(
		"/protected/user-data",
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
	); composition.available() || composition.caseEntities != nil {
		t.Fatal("missing case dependencies advertised account-flow capability")
	}

	composition := composeRuntimeFundsAccountFlowV1(
		"/protected/user-data",
		runtimeFundsKeyedDigesterStubV1{},
		runtimeFundsCaseEntityStoreStubV1{},
		runtimeFundsDatasetAuthorityStubV1{},
		runtimeFundsCaseObserverStubV1{},
		runtimeFundsMaterialReaderStubV1{},
		func(context.Context, domainsecurity.TurnSecurityContext) error { return nil },
	)
	if !composition.available() {
		t.Fatal("complete host dependencies did not compose additive account-flow capability")
	}
	if composition.useCurrentSource == nil || composition.useCurrentIngressSource == nil ||
		composition.useCurrentLocalDisplay == nil ||
		composition.useRetainedAcceptedSlotDisplay == nil ||
		composition.resolveSubject == nil ||
		composition.resolveCounterparty == nil || composition.caseEntities == nil {
		t.Fatal("composed account-flow capability lost a production dependency")
	}
	dependencies := nativecomponentapp.RuntimeAuthorityDependencies{}
	if !composition.applyToRuntimeAuthorityV1(
		&dependencies,
		func(context.Context, domainsecurity.TurnSecurityContext) error {
			return nil
		},
		func(context.Context, domainsecurity.TurnSecurityContext, domainsecurity.ExecutionGrant) error {
			return nil
		},
	) || dependencies.UseCurrentAccountFlowSource == nil ||
		dependencies.UseCurrentAccountIngressSource == nil || dependencies.ResolveAccountFlowSubject == nil ||
		dependencies.ResolveAccountFlowCounterparty == nil ||
		dependencies.ValidateCurrentAccountFlowCallback == nil ||
		dependencies.ValidateCurrentAccountFlowOuterGrant == nil {
		t.Fatal("account-flow composition did not propagate into the production native authority dependencies")
	}
}

func TestFundsAccountFlowCompositionRejectsInvalidProtectedRootWithoutBlocking(t *testing.T) {
	composition := composeRuntimeFundsAccountFlowV1(
		"relative/user-data",
		runtimeFundsKeyedDigesterStubV1{},
		runtimeFundsCaseEntityStoreStubV1{},
		runtimeFundsDatasetAuthorityStubV1{},
		runtimeFundsCaseObserverStubV1{},
		runtimeFundsMaterialReaderStubV1{},
		func(context.Context, domainsecurity.TurnSecurityContext) error { return nil },
	)
	if composition.available() || composition.caseEntities != nil {
		t.Fatal("invalid protected root advertised account-flow capability")
	}
}

type runtimeFundsKeyedDigesterStubV1 struct{}

func (runtimeFundsKeyedDigesterStubV1) KeyedPayloadHash(context.Context, string, []byte) (string, error) {
	return "", errors.New("not invoked")
}

type runtimeFundsCaseEntityStoreStubV1 struct{}

func (runtimeFundsCaseEntityStoreStubV1) EnsureBinding(context.Context, domaincaseentity.CaseEntityBindingRecordInputV1) (domaincaseentity.CaseEntityBindingRecord, error) {
	return domaincaseentity.CaseEntityBindingRecord{}, errors.New("not invoked")
}

func (runtimeFundsCaseEntityStoreStubV1) ResolveBinding(context.Context, string) (domaincaseentity.CaseEntityBindingRecord, error) {
	return domaincaseentity.CaseEntityBindingRecord{}, errors.New("not invoked")
}

func (runtimeFundsCaseEntityStoreStubV1) ResolveBindingByStableOrdinal(context.Context, domainsecurity.TurnSecurityContext, string, uint32) (domaincaseentity.CaseEntityBindingRecord, error) {
	return domaincaseentity.CaseEntityBindingRecord{}, errors.New("not invoked")
}

func (runtimeFundsCaseEntityStoreStubV1) PutIngressIfAbsent(context.Context, domaincaseentity.CaseIngressRecord) error {
	return errors.New("not invoked")
}

func (runtimeFundsCaseEntityStoreStubV1) ResolveIngress(context.Context, string) (domaincaseentity.CaseIngressRecord, error) {
	return domaincaseentity.CaseIngressRecord{}, errors.New("not invoked")
}

func (runtimeFundsCaseEntityStoreStubV1) PutThreadContextIfAbsent(context.Context, domaincaseentity.ThreadCaseContextRecord) error {
	return errors.New("not invoked")
}

func (runtimeFundsCaseEntityStoreStubV1) ResolveThreadContext(context.Context, string) (domaincaseentity.ThreadCaseContextRecord, error) {
	return domaincaseentity.ThreadCaseContextRecord{}, errors.New("not invoked")
}

func (runtimeFundsCaseEntityStoreStubV1) ResolveLatestCaseLongitudinalContext(context.Context, domainsecurity.TurnSecurityContext) (domaincaseentity.ThreadCaseContextRecord, error) {
	return domaincaseentity.ThreadCaseContextRecord{}, errors.New("not invoked")
}

func (runtimeFundsCaseEntityStoreStubV1) ResolveLatestThreadContextForScope(context.Context, domainsecurity.TurnSecurityContext, string) (domaincaseentity.ThreadCaseContextRecord, error) {
	return domaincaseentity.ThreadCaseContextRecord{}, errors.New("not invoked")
}

type runtimeFundsDatasetAuthorityStubV1 struct{}

func (runtimeFundsDatasetAuthorityStubV1) ResolveWitnessedV2(
	context.Context,
	datasetsnapshotport.ResolveInputV2,
) (datasetsnapshotport.ResolvedSnapshotV2, error) {
	return datasetsnapshotport.ResolvedSnapshotV2{}, errors.New("not invoked")
}

func (runtimeFundsDatasetAuthorityStubV1) WithCurrentSelectionV2(
	context.Context,
	datasetsnapshotport.ResolveInputV2,
	domainsecurity.TurnSecurityContext,
	func(datasetsnapshotport.CurrentSelectionV2, datasetsnapshotport.CurrentSelectionCapabilityV2) error,
) error {
	return errors.New("not invoked")
}

type runtimeFundsCaseObserverStubV1 struct{}

func (runtimeFundsCaseObserverStubV1) Observe(string) (domainsecurity.CaseBindingObservationV1, error) {
	return domainsecurity.CaseBindingObservationV1{}, errors.New("not invoked")
}

type runtimeFundsMaterialReaderStubV1 struct{}

func (runtimeFundsMaterialReaderStubV1) ResolveExact(
	context.Context,
	datasetsnapshotport.MaterialKindV2,
	datasetsnapshotport.ExactMaterialReferenceV2,
) ([]byte, error) {
	return nil, errors.New("not invoked")
}
