package piiauthorization

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"time"

	domainpii "analytix.local/runtime-go/internal/domain/piiauthorization"
	domainpublication "analytix.local/runtime-go/internal/domain/reportpublication"
	finalauthorityport "analytix.local/runtime-go/internal/ports/finalauthority"
	piiauthorizationport "analytix.local/runtime-go/internal/ports/piiauthorization"
	publicationport "analytix.local/runtime-go/internal/ports/reportpublication"
)

type ControlledAccessInventoryDependenciesV1 struct {
	Access      piiauthorizationport.AccessInventoryStore
	Grants      piiauthorizationport.Store
	Receipts    publicationport.ReceiptStore
	Commits     publicationport.CommitReceiptStore
	Indexes     publicationport.IndexStore
	Ledgers     publicationport.ClaimLedgerStore
	Projections publicationport.PIIProjectionStore
	Inspections publicationport.RenderInspectionStore
	Artifacts   publicationport.ControlledArtifactMetadataStore
	Authority   finalauthorityport.Authority
}

// ControlledAccessRestartPlanV1 contains only installation-trusted open
// reservations. Its fields are private so callers cannot manufacture restart
// terminalization authority.
type ControlledAccessRestartPlanV1 struct {
	verified     bool
	openReceipts []domainpii.ControlledArtifactAccessReceiptV1
}

func (plan ControlledAccessRestartPlanV1) OpenReceiptCount() int {
	if !plan.verified {
		return 0
	}
	return len(plan.openReceipts)
}

// VerifyControlledAccessInventoryV1 closes the private access journal over the
// exact PII grant and committed publication graph. It returns open crash cuts
// but does not mutate them; runtime startup must terminalize and then re-run
// this full verification before any controlled-access service is activated.
func VerifyControlledAccessInventoryV1(
	ctx context.Context,
	dependencies ControlledAccessInventoryDependenciesV1,
) (ControlledAccessRestartPlanV1, error) {
	if ctx == nil || dependencies.Access == nil || dependencies.Grants == nil || dependencies.Receipts == nil ||
		dependencies.Commits == nil || dependencies.Indexes == nil || dependencies.Ledgers == nil ||
		dependencies.Projections == nil || dependencies.Inspections == nil || dependencies.Artifacts == nil ||
		dependencies.Authority == nil {
		return ControlledAccessRestartPlanV1{}, errors.New("controlled access trusted inventory dependencies are required")
	}
	if err := ctx.Err(); err != nil {
		return ControlledAccessRestartPlanV1{}, err
	}
	receipts := make(map[string]domainpii.ControlledArtifactAccessReceiptV1)
	var ordered []domainpii.ControlledArtifactAccessReceiptV1
	if err := dependencies.Access.VisitAccessReceipts(ctx, func(receipt domainpii.ControlledArtifactAccessReceiptV1) error {
		if _, duplicate := receipts[receipt.AccessID]; duplicate {
			return errors.New("controlled access inventory repeats an access receipt")
		}

		receipts[receipt.AccessID] = receipt
		ordered = append(ordered, receipt)
		return nil
	}); err != nil {
		return ControlledAccessRestartPlanV1{}, fmt.Errorf("verify controlled access receipt inventory: %w", err)
	}
	// Resolver calls must run after the inventory releases its CAS access guard.
	for _, receipt := range ordered {
		if err := ctx.Err(); err != nil {
			return ControlledAccessRestartPlanV1{}, fmt.Errorf("verify controlled access receipt inventory: %w", err)
		}
		if err := verifyControlledAccessReceiptAuthorityV1(ctx, dependencies.Authority, receipt); err != nil {
			return ControlledAccessRestartPlanV1{}, fmt.Errorf("verify controlled access receipt inventory: %w", err)
		}
		if err := verifyControlledAccessPublicationGraphV1(ctx, dependencies, receipt); err != nil {
			return ControlledAccessRestartPlanV1{}, fmt.Errorf("verify controlled access receipt inventory: %w", err)
		}
	}
	closed := make(map[string]struct{}, len(receipts))
	if err := dependencies.Access.VisitAccessDispositions(ctx, func(disposition domainpii.ControlledArtifactAccessDispositionV1) error {
		receipt, found := receipts[disposition.AccessID]
		if !found {
			return errors.New("controlled access inventory contains an orphan disposition")
		}
		if _, duplicate := closed[disposition.AccessID]; duplicate {
			return errors.New("controlled access inventory repeats a terminal disposition")
		}
		if domainpii.ValidateControlledArtifactAccessDispositionForReceiptV1(disposition, receipt) != nil {
			return errors.New("controlled access disposition does not close its exact receipt")
		}
		if err := verifyControlledAccessDispositionAuthorityV1(ctx, dependencies.Authority, disposition); err != nil {
			return err
		}
		closed[disposition.AccessID] = struct{}{}
		return nil
	}); err != nil {
		return ControlledAccessRestartPlanV1{}, fmt.Errorf("verify controlled access disposition inventory: %w", err)
	}
	open := make([]domainpii.ControlledArtifactAccessReceiptV1, 0, len(receipts)-len(closed))
	for accessID, receipt := range receipts {
		if _, found := closed[accessID]; !found {
			open = append(open, receipt)
		}
	}
	sort.Slice(open, func(i, j int) bool { return open[i].AccessID < open[j].AccessID })
	return ControlledAccessRestartPlanV1{verified: true, openReceipts: open}, nil
}

// ApplyControlledAccessRestartPlanV1 closes every unresolved reservation as
// release_indeterminate with the complete artifact length as the conservative
// exposure upper bound. It never claims that a crash released zero bytes.
func ApplyControlledAccessRestartPlanV1(
	ctx context.Context,
	plan ControlledAccessRestartPlanV1,
	store piiauthorizationport.RestartAccessStore,
	authority finalauthorityport.Authority,
	disposedAt time.Time,
) error {
	if ctx == nil || !plan.verified || store == nil || authority == nil || disposedAt.IsZero() {
		return errors.New("controlled access restart plan is invalid")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(plan.openReceipts) != 0 && !store.IsSemanticStageControlledAccessStore() {
		return errors.New("controlled access restart writes require an isolated semantic stage")
	}
	disposedAt = disposedAt.UTC()
	dispositions := make([]domainpii.ControlledArtifactAccessDispositionV1, 0, len(plan.openReceipts))
	for _, planned := range plan.openReceipts {
		if err := verifyControlledAccessReceiptAuthorityV1(ctx, authority, planned); err != nil {
			return err
		}
		current, err := store.ResolveAccessReceipt(ctx, planned.AccessID)
		if err != nil || !reflect.DeepEqual(current, planned) {
			return errors.Join(errors.New("controlled access restart receipt changed"), err)
		}
		if _, err := store.ResolveAccessDisposition(ctx, planned.AccessID); err == nil {
			return errors.New("controlled access restart plan became stale")
		} else if !errors.Is(err, piiauthorizationport.ErrNotFound) {
			return err
		}
		requestedAt, err := time.Parse(time.RFC3339Nano, planned.RequestedAt)
		if err != nil || disposedAt.Before(requestedAt) {
			return errors.New("controlled access restart time precedes the reservation")
		}
		disposition, err := domainpii.NewControlledArtifactAccessDispositionV1(
			planned,
			domainpii.ControlledArtifactAccessDispositionReleaseIndeterminateV1,
			domainpii.ControlledArtifactAccessReasonReleaseIndeterminateV1,
			planned.ArtifactByteLength,
			disposedAt,
			authority.KeyID(),
			authority.PublicKey(),
			func(message []byte) ([]byte, error) { return authority.Sign(ctx, message) },
		)
		if err != nil {
			return err
		}
		if err := verifyControlledAccessDispositionAuthorityV1(ctx, authority, disposition); err != nil {
			return err
		}
		dispositions = append(dispositions, disposition)
	}
	for _, disposition := range dispositions {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := store.PutAccessDispositionIfAbsent(ctx, disposition); err != nil {
			return err
		}
		stored, err := store.ResolveAccessDisposition(ctx, disposition.AccessID)
		if err != nil || !reflect.DeepEqual(stored, disposition) {
			return errors.Join(errors.New("controlled access restart disposition readback failed"), err)
		}
		if err := verifyControlledAccessDispositionAuthorityV1(ctx, authority, stored); err != nil {
			return err
		}
	}
	return nil
}

func verifyControlledAccessPublicationGraphV1(
	ctx context.Context,
	dependencies ControlledAccessInventoryDependenciesV1,
	access domainpii.ControlledArtifactAccessReceiptV1,
) error {
	grant, err := dependencies.Grants.ResolveGrant(ctx, access.PIIAuthorizationDigest)
	if err != nil || verifyPIIGrantAuthorityV1(ctx, dependencies.Authority, grant) != nil ||
		domainpii.ValidateControlledArtifactAccessReceiptForGrantV1(access, grant) != nil {
		return errors.New("controlled access receipt lost its trusted PII grant")
	}
	receipt, err := dependencies.Receipts.Resolve(ctx, access.PublicationReceiptDigest)
	if err != nil || verifyPublicationReceiptAuthorityV1(ctx, dependencies.Authority, receipt) != nil {
		return errors.New("controlled access receipt lost its trusted publication receipt")
	}
	commit, err := dependencies.Commits.Resolve(ctx, access.PublicationCommitDigest)
	if err != nil || verifyPublicationCommitAuthorityV1(ctx, dependencies.Authority, commit) != nil {
		return errors.New("controlled access receipt lost its trusted publication commit")
	}
	index, err := dependencies.Indexes.Resolve(ctx, commit.PublicationIndexDigest)
	if err != nil || verifyPublicationIndexAuthorityV1(ctx, dependencies.Authority, index) != nil ||
		domainpublication.ValidatePublicationCommitReceiptMaterialsV1(commit, receipt, index) != nil {
		return errors.New("controlled access receipt lost its committed publication index")
	}
	ledger, err := dependencies.Ledgers.Resolve(ctx, access.ClaimLedgerDigest)
	if err != nil || ValidateStoredGrantClaimLedgerV1(grant, ledger) != nil {
		return errors.New("controlled access receipt lost its exact claim ledger")
	}
	projection, err := dependencies.Projections.Resolve(ctx, access.PIIProjectionDigest)
	if err != nil || !controlledPublicationReceiptMatchesGrantV1(receipt, projection, grant) {
		return errors.New("controlled access receipt lost its exact PII projection")
	}
	inspection, err := dependencies.Inspections.Resolve(ctx, receipt.RenderInspectionDigest)
	if err != nil || domainpublication.ValidatePublicationReceiptMaterialsV1(receipt, ledger, projection, inspection) != nil {
		return errors.New("controlled access receipt lost its render inspection")
	}
	artifact, err := dependencies.Artifacts.ResolveControlledMetadata(ctx, access.TargetIdentityDigest)
	if err != nil {
		return errors.New("controlled access receipt lost its protected artifact")
	}
	return ValidateStoredControlledAccessPublicationV1(access, StoredControlledPublicationMaterialsV1{
		Grant: grant, Receipt: receipt, Commit: commit, Index: index,
		Ledger: ledger, Projection: projection, Inspection: inspection, Artifact: artifact,
	})
}

func verifyPIIGrantAuthorityV1(
	ctx context.Context,
	authority finalauthorityport.Authority,
	grant domainpii.PIIProjectionGrantV1,
) error {
	keyID, publicKey, signature, err := domainpii.PIIProjectionGrantAuthorityMaterialV1(grant)
	if err != nil || authority.VerifyTrusted(ctx, keyID, publicKey, domainpii.PIIProjectionGrantSigningBytesV1(grant), signature) != nil {
		return errors.New("PII projection grant lacks trusted installation authority")
	}
	return nil
}

func verifyControlledAccessReceiptAuthorityV1(
	ctx context.Context,
	authority finalauthorityport.Authority,
	receipt domainpii.ControlledArtifactAccessReceiptV1,
) error {
	keyID, publicKey, signature, err := domainpii.ControlledArtifactAccessReceiptAuthorityMaterialV1(receipt)
	if err != nil || authority.VerifyTrusted(
		ctx, keyID, publicKey, domainpii.ControlledArtifactAccessReceiptSigningBytesV1(receipt), signature,
	) != nil {
		return errors.New("controlled access receipt lacks trusted installation authority")
	}
	return nil
}

func verifyControlledAccessDispositionAuthorityV1(
	ctx context.Context,
	authority finalauthorityport.Authority,
	disposition domainpii.ControlledArtifactAccessDispositionV1,
) error {
	keyID, publicKey, signature, err := domainpii.ControlledArtifactAccessDispositionAuthorityMaterialV1(disposition)
	if err != nil || authority.VerifyTrusted(
		ctx, keyID, publicKey, domainpii.ControlledArtifactAccessDispositionSigningBytesV1(disposition), signature,
	) != nil {
		return errors.New("controlled access disposition lacks trusted installation authority")
	}
	return nil
}

func verifyPublicationReceiptAuthorityV1(
	ctx context.Context,
	authority finalauthorityport.Authority,
	receipt domainpublication.PublicationReceiptV1,
) error {
	keyID, publicKey, signature, err := domainpublication.PublicationReceiptAuthorityMaterialV1(receipt)
	if err != nil || authority.VerifyTrusted(ctx, keyID, publicKey, domainpublication.PublicationReceiptSigningBytesV1(receipt), signature) != nil {
		return errors.New("publication receipt lacks trusted installation authority")
	}
	return nil
}

func verifyPublicationCommitAuthorityV1(
	ctx context.Context,
	authority finalauthorityport.Authority,
	commit domainpublication.PublicationCommitReceiptV1,
) error {
	keyID, publicKey, signature, err := domainpublication.PublicationCommitReceiptAuthorityMaterialV1(commit)
	if err != nil || authority.VerifyTrusted(ctx, keyID, publicKey, domainpublication.PublicationCommitReceiptSigningBytesV1(commit), signature) != nil {
		return errors.New("publication commit lacks trusted installation authority")
	}
	return nil
}

func verifyPublicationIndexAuthorityV1(
	ctx context.Context,
	authority finalauthorityport.Authority,
	index domainpublication.PublicationIndexV1,
) error {
	keyID, publicKey, signature, err := domainpublication.PublicationIndexAuthorityMaterialV1(index)
	if err != nil || authority.VerifyTrusted(ctx, keyID, publicKey, domainpublication.PublicationIndexSigningBytesV1(index), signature) != nil {
		return errors.New("publication index lacks trusted installation authority")
	}
	return nil
}
