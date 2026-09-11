// Package currentdataset provides deterministic test-only implementations of
// the existing current TSCV2 and DSV2 ports. Production packages must not
// import it.
package currentdataset

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"errors"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainfundsquerysource "analytix.local/runtime-go/internal/domain/fundsquerysource"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	datasetsnapshotport "analytix.local/runtime-go/internal/ports/datasetsnapshot"
	datasetsnapshotfixture "analytix.local/runtime-go/internal/testsupport/datasetsnapshotv2fixture"
	securitycontexttest "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

var errUnavailable = errors.New("test current dataset authority is unavailable")

var signedSelections sync.Map

type Harness struct {
	mu      sync.Mutex
	current domainsecurity.TurnSecurityContext
	revoked bool
}

func NewHarness() *Harness { return &Harness{} }

func (harness *Harness) NewCaseContext(
	input domainsecurity.TurnSecurityContextInput,
) (domainsecurity.TurnSecurityContext, error) {
	observation, err := observationForInput(input)
	if err != nil {
		return domainsecurity.TurnSecurityContext{}, err
	}
	policyDigest := domainsecurity.SHA256Hex([]byte(
		"test-current-dataset-risk:\x00" + input.ThreadID + "\x00" + input.WorkspaceRealPath,
	))
	policy, err := domainsecurity.NewTurnPublicationPolicyV1(domainsecurity.TurnPublicationPolicyInputV1{
		ThreadRiskPolicyDigest:   policyDigest,
		RiskClass:                domainsecurity.RiskClassCase,
		Disposition:              domainsecurity.PublicationDispositionCaseEvidenceGate,
		CaseBindingState:         domainsecurity.CaseBindingStateValid,
		BindingObservationDigest: observation.ObservationDigest,
		BlockerCode:              domainsecurity.PublicationBlockerNone,
	})
	if err != nil {
		return domainsecurity.TurnSecurityContext{}, err
	}
	riskBinding, err := securitycontexttest.WitnessedRiskBinding(
		input.ThreadID,
		input.WorkspaceRealPath,
		domainsecurity.RiskClassCase,
		policyDigest,
	)
	if err != nil {
		return domainsecurity.TurnSecurityContext{}, err
	}
	input.PublicationPolicy = policy
	input.RiskAuthorityBinding = riskBinding
	provisional, err := domainsecurity.NewTurnSecurityContextV2(input)
	if err != nil {
		return domainsecurity.TurnSecurityContext{}, err
	}
	selection, err := buildAccountIngressSelectionV1(provisional)
	if err != nil {
		return domainsecurity.TurnSecurityContext{}, err
	}
	input.DatasetSnapshotID = selection.Snapshot.Record.DatasetSnapshotID
	input.SourceManifestHash = selection.Snapshot.Record.SourceManifestHash
	securityContext, err := domainsecurity.NewTurnSecurityContextV2(input)
	if err != nil {
		return domainsecurity.TurnSecurityContext{}, err
	}
	signedSelections.Store(securityContext.ContextDigest, cloneSelectionV1(selection))
	return securityContext, nil
}

func (harness *Harness) ValidateCurrent(
	ctx context.Context,
	securityContext domainsecurity.TurnSecurityContext,
) error {
	if harness == nil || ctx == nil || domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(securityContext) != nil {
		return errUnavailable
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	harness.mu.Lock()
	defer harness.mu.Unlock()
	if harness.revoked {
		return errUnavailable
	}
	harness.current = securityContext
	return nil
}

func (harness *Harness) Observe(workspace string) (domainsecurity.CaseBindingObservationV1, error) {
	if harness == nil {
		return domainsecurity.CaseBindingObservationV1{}, errUnavailable
	}
	harness.mu.Lock()
	securityContext := harness.current
	revoked := harness.revoked
	harness.mu.Unlock()
	if revoked || securityContext.WorkspaceRealPath != workspace {
		return domainsecurity.CaseBindingObservationV1{}, errUnavailable
	}
	return observationForContext(securityContext)
}

func (harness *Harness) WithCurrentSelectionV2(
	ctx context.Context,
	input datasetsnapshotport.ResolveInputV2,
	securityContext domainsecurity.TurnSecurityContext,
	callback func(datasetsnapshotport.CurrentSelectionV2, datasetsnapshotport.CurrentSelectionCapabilityV2) error,
) error {
	if harness == nil || ctx == nil || callback == nil || ctx.Err() != nil ||
		!harness.matchesCurrent(securityContext) || input.TenantID != securityContext.TenantID ||
		input.UserID != securityContext.UserID || input.ExpectedDatasetSnapshotID != securityContext.DatasetSnapshotID {
		return errUnavailable
	}
	observation, err := observationForContext(securityContext)
	if err != nil || input.Observation != observation {
		return errUnavailable
	}
	selection, err := accountIngressSelectionV1(securityContext)
	if err != nil {
		return errUnavailable
	}
	capability := &capability{
		harness: harness, expected: securityContext, selection: cloneSelectionV1(selection), ctx: ctx,
	}
	capability.active.Store(true)
	defer capability.active.Store(false)
	return callback(cloneSelectionV1(selection), capability)
}

func (harness *Harness) SetRevoked(revoked bool) {
	if harness == nil {
		return
	}
	harness.mu.Lock()
	harness.revoked = revoked
	harness.mu.Unlock()
}

func (harness *Harness) matchesCurrent(securityContext domainsecurity.TurnSecurityContext) bool {
	harness.mu.Lock()
	defer harness.mu.Unlock()
	return !harness.revoked && harness.current == securityContext
}

type capability struct {
	harness   *Harness
	expected  domainsecurity.TurnSecurityContext
	selection datasetsnapshotport.CurrentSelectionV2
	ctx       context.Context
	active    atomic.Bool
	used      atomic.Bool
}

func (capability *capability) UseExact(
	selection datasetsnapshotport.CurrentSelectionV2,
	securityContext domainsecurity.TurnSecurityContext,
	use func(context.Context) error,
) error {
	if capability == nil || use == nil || !capability.active.Load() ||
		!capability.used.CompareAndSwap(false, true) ||
		securityContext != capability.expected ||
		datasetsnapshotport.ValidateCurrentSelectionDigestV2(selection) != nil ||
		selection.SelectionDigest != capability.selection.SelectionDigest ||
		selection.Snapshot.Record.RecordDigest != capability.selection.Snapshot.Record.RecordDigest ||
		!capability.harness.matchesCurrent(securityContext) {
		return errUnavailable
	}
	leaseContext, cancel := context.WithCancel(capability.ctx)
	defer cancel()
	useErr := use(leaseContext)
	if !capability.harness.matchesCurrent(securityContext) {
		return errors.Join(useErr, errUnavailable)
	}
	return useErr
}

func (capability *capability) CommitExact(
	selection datasetsnapshotport.CurrentSelectionV2,
	securityContext domainsecurity.TurnSecurityContext,
	commit func(context.Context) error,
) error {
	if capability == nil || commit == nil || !capability.active.Load() ||
		!capability.used.CompareAndSwap(false, true) ||
		securityContext != capability.expected ||
		datasetsnapshotport.ValidateCurrentSelectionDigestV2(selection) != nil ||
		selection.SelectionDigest != capability.selection.SelectionDigest ||
		selection.Snapshot.Record.RecordDigest != capability.selection.Snapshot.Record.RecordDigest ||
		!capability.harness.matchesCurrent(securityContext) {
		return errUnavailable
	}
	leaseContext, cancel := context.WithCancel(capability.ctx)
	defer cancel()
	return commit(leaseContext)
}

// AccountIngressDescriptorV1 returns the path-free first-source provenance
// paired with the signed CurrentSelectionV2 used by this test harness.
// Production code must obtain this descriptor from fundsquerysource.Service
// while its exact DSV2 source lease is active.
func AccountIngressDescriptorV1(
	securityContext domainsecurity.TurnSecurityContext,
) (domainfundsquerysource.DescriptorV1, error) {
	selection, err := accountIngressSelectionV1(securityContext)
	if err != nil {
		return domainfundsquerysource.DescriptorV1{}, err
	}
	analytical, err := selection.Snapshot.Manifest.AnalyticalDuckDB.Values()
	if err != nil {
		return domainfundsquerysource.DescriptorV1{}, err
	}
	binding := selection.Snapshot.Record.Binding
	return domainfundsquerysource.NewDescriptorV1(domainfundsquerysource.DescriptorInputV1{
		SnapshotRecordDigest:     selection.Snapshot.Record.RecordDigest,
		DatasetSnapshotID:        selection.Snapshot.Record.DatasetSnapshotID,
		SourceManifestHash:       selection.Snapshot.Manifest.SourceManifestHash,
		CaseID:                   binding.CaseID,
		CaseBindingHash:          binding.CaseBindingHash,
		DatasetBindingDigest:     binding.BindingKeyDigest,
		BindingObservationDigest: binding.BindingObservationDigest,

		FundsProducerContentID: selection.Snapshot.Manifest.ProducerContentID,
		FundsProducerContentManifestSHA256: selection.Snapshot.Manifest.
			ProducerContentManifestSHA256,
		FundsProducerContentManifestByteLength: selection.Snapshot.Manifest.
			ProducerContentManifestByteLength,

		DuckDBSHA256:                 analytical.DuckDBSHA256,
		DuckDBByteLength:             analytical.DuckDBByteLength,
		DuckDBContentSnapshotDigest:  analytical.DuckDBContentSnapshotDigest,
		DuckDBSnapshotManifestSHA256: analytical.DuckDBSnapshotManifestSHA256,
		MaterializationIdentity:      analytical.MaterializationIdentity,
		SchemaDigest:                 analytical.SchemaDigest,
		DatasetUTCOffsetMinutes:      analytical.DatasetUTCOffsetMinutes,
		ExpectedCurrency:             analytical.ExpectedCurrency,
		MinorUnitScale:               analytical.MinorUnitScale,
		QueryProfileDigest:           analytical.QueryProfileDigest,
	})
}

func accountIngressSelectionV1(
	securityContext domainsecurity.TurnSecurityContext,
) (datasetsnapshotport.CurrentSelectionV2, error) {
	if domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(securityContext) != nil {
		return datasetsnapshotport.CurrentSelectionV2{}, errUnavailable
	}
	stored, found := signedSelections.Load(securityContext.ContextDigest)
	selection, valid := stored.(datasetsnapshotport.CurrentSelectionV2)
	if !found || !valid ||
		selection.Snapshot.Record.DatasetSnapshotID != securityContext.DatasetSnapshotID ||
		selection.Snapshot.Record.SourceManifestHash != securityContext.SourceManifestHash {
		return datasetsnapshotport.CurrentSelectionV2{}, errUnavailable
	}
	return cloneSelectionV1(selection), nil
}

func buildAccountIngressSelectionV1(
	securityContext domainsecurity.TurnSecurityContext,
) (datasetsnapshotport.CurrentSelectionV2, error) {
	observation, err := observationForContext(securityContext)
	if err != nil {
		return datasetsnapshotport.CurrentSelectionV2{}, err
	}
	acceptedAt := time.Unix(1_700_000_000+int64(securityContext.ContextEpoch), 0).UTC()
	material := strings.Join([]string{
		securityContext.WorkspaceRealPath,
		securityContext.TenantID,
		securityContext.UserID,
		securityContext.CaseID,
		securityContext.CaseBindingHash,
		securityContext.PublicationPolicy.BindingObservationDigest,
		strconv.FormatUint(securityContext.ContextEpoch, 10),
		securityContext.DatasetSnapshotID,
		securityContext.SourceManifestHash,
	}, "\x00")
	digest := func(kind string) string {
		return domainsecurity.SHA256Hex([]byte(
			"test-current-dataset:" + kind + ":\x00" + material,
		))
	}
	privateKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{0x43}, ed25519.SeedSize))
	publicKey := privateKey.Public().(ed25519.PublicKey)
	base, err := datasetsnapshotfixture.NewResolvedSnapshotV2(
		datasetsnapshotfixture.ResolvedInput{
			TenantID:           securityContext.TenantID,
			UserID:             securityContext.UserID,
			Observation:        observation,
			Material:           material,
			InstallationID:     digest("installation"),
			AcceptedAt:         acceptedAt,
			AuthorityKeyID:     domainsecurity.SHA256Hex(publicKey),
			AuthorityPublicKey: publicKey,
			Sign: func(message []byte) ([]byte, error) {
				return ed25519.Sign(privateKey, message), nil
			},
		},
	)
	if err != nil {
		return datasetsnapshotport.CurrentSelectionV2{}, err
	}
	analytical, err := domainsecurity.NewDatasetSnapshotAnalyticalDuckDBBindingV2(
		domainsecurity.DatasetSnapshotAnalyticalDuckDBBindingInputV2{
			DuckDBSHA256:                 digest("duckdb"),
			DuckDBByteLength:             8192,
			DuckDBContentSnapshotDigest:  digest("duckdb-content-snapshot"),
			DuckDBSnapshotManifestSHA256: digest("duckdb-snapshot-manifest"),
			MaterializationIdentity: domainsecurity.FundsMaterializationIdentityPrefixV1 +
				digest("source-signature"),
			SchemaDigest:            domainfundsquerysource.FixedFundsAnalyticalSchemaDigestV1(),
			QueryProfileDigest:      domainfundsquerysource.FixedFundsQueryProfileDigestV1(),
			DatasetUTCOffsetMinutes: 0,
			ExpectedCurrency:        "CNY",
			MinorUnitScale:          domainfundsquerysource.AccountFlowMinorUnitScaleV1,
		},
	)
	if err != nil {
		return datasetsnapshotport.CurrentSelectionV2{}, err
	}
	manifestInput, err := datasetsnapshotfixture.CloneManifestInputV2(base)
	if err != nil {
		return datasetsnapshotport.CurrentSelectionV2{}, err
	}
	policy, found := domainevidence.ResolveSourceRowProducerPolicyV1(
		domainevidence.FundsTransactionSourceRowPolicyIDV1,
	)
	if !found || domainevidence.ValidateSourceRowProducerPolicyV1(policy) != nil {
		return datasetsnapshotport.CurrentSelectionV2{}, errUnavailable
	}
	manifestInput.SourceType = policy.SourceType
	manifestInput.ProducerPolicyID = policy.PolicyID
	manifestInput.ProducerPolicyDigest = policy.PolicyDigest
	manifestInput.ProducerComponentID = policy.ProducerComponentID
	manifestInput.ProducerComponentVersion = policy.ProducerComponentVersion
	manifestInput.ProducerOperation = policy.Operation
	manifestInput.ProducerOperationSchemaHash = policy.OperationSchemaHash
	manifestInput.ParserID = policy.ParserID
	manifestInput.ParserVersion = policy.ParserVersion
	manifestInput.AnalyticalDuckDB = analytical
	manifest, err := domainsecurity.NewDatasetSnapshotManifestV2(manifestInput)
	if err != nil {
		return datasetsnapshotport.CurrentSelectionV2{}, err
	}
	record := base.Record
	err = manifest.WithExactFundsProducerAuthorityAdmissionV2(
		base.FundsProducerContent,
		func(issuer domainsecurity.DatasetSnapshotAuthoritySealedAdmissionV2) error {
			issued, issueErr := issuer.Issue(
				domainsecurity.DatasetSnapshotAuthoritySealedIssueInputV2{
					InstallationID:     base.Record.InstallationID,
					AcceptedAt:         acceptedAt,
					AuthorityKeyID:     domainsecurity.SHA256Hex(publicKey),
					AuthorityPublicKey: publicKey,
				},
				func(message []byte) ([]byte, error) {
					return ed25519.Sign(privateKey, message), nil
				},
			)
			record = issued
			return issueErr
		},
	)
	if err != nil {
		return datasetsnapshotport.CurrentSelectionV2{}, err
	}
	index, err := domainsecurity.NewDatasetSnapshotIndexV1(
		domainsecurity.DatasetSnapshotIndexInputV1{
			InstallationID:       record.InstallationID,
			EnrollmentID:         digest("enrollment"),
			Generation:           1,
			PreviousIndexDigest:  domainsecurity.DatasetSnapshotIndexGenesisDigestV1(),
			MutationID:           digest("index-mutation"),
			Binding:              record.Binding,
			SnapshotRecordDigest: record.RecordDigest,
			AuthorityKeyID:       domainsecurity.SHA256Hex(publicKey),
			AuthorityPublicKey:   publicKey,
		},
		func(message []byte) ([]byte, error) {
			return ed25519.Sign(privateKey, message), nil
		},
	)
	if err != nil {
		return datasetsnapshotport.CurrentSelectionV2{}, err
	}
	selection := datasetsnapshotport.CurrentSelectionV2{
		DatasetIndexPath: []domainsecurity.DatasetSnapshotIndexV1{index},
		SelectedIndex:    index,
		Snapshot: datasetsnapshotport.ResolvedSnapshotV2{
			Record: record, Manifest: manifest,
			FundsProducerContent: base.FundsProducerContent,
		},
	}
	selection.SelectionDigest, err = datasetsnapshotport.CanonicalCurrentSelectionDigestV2(selection)
	if err != nil || datasetsnapshotport.ValidateCurrentSelectionDigestV2(selection) != nil ||
		datasetsnapshotport.ValidateResolvedSnapshotV2(selection.Snapshot) != nil ||
		domainsecurity.ValidateDatasetSnapshotIndexRecordV2(index, record) != nil {
		return datasetsnapshotport.CurrentSelectionV2{}, errUnavailable
	}
	return selection, nil
}

func cloneSelectionV1(
	selection datasetsnapshotport.CurrentSelectionV2,
) datasetsnapshotport.CurrentSelectionV2 {
	selection.DatasetIndexPath = append(
		[]domainsecurity.DatasetSnapshotIndexV1(nil),
		selection.DatasetIndexPath...,
	)
	return selection
}

func observationForInput(
	input domainsecurity.TurnSecurityContextInput,
) (domainsecurity.CaseBindingObservationV1, error) {
	return domainsecurity.NewCaseBindingObservationV1(domainsecurity.CaseBindingObservationInputV1{
		WorkspaceRealPath: input.WorkspaceRealPath,
		State:             domainsecurity.CaseBindingStateValid,
		CaseID:            input.CaseID,
		BindingSHA256: domainsecurity.SHA256Hex([]byte(
			"test-current-dataset-binding:\x00" + input.CaseBindingHash,
		)),
		CaseBindingHash: input.CaseBindingHash,
	})
}

func observationForContext(
	securityContext domainsecurity.TurnSecurityContext,
) (domainsecurity.CaseBindingObservationV1, error) {
	return observationForInput(domainsecurity.TurnSecurityContextInput{
		WorkspaceRealPath: securityContext.WorkspaceRealPath,
		CaseID:            securityContext.CaseID,
		CaseBindingHash:   securityContext.CaseBindingHash,
	})
}

var _ datasetsnapshotport.CurrentAuthorityV2 = (*Harness)(nil)
