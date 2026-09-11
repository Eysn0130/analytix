package runtimeapp

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"

	finalauthority "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	piistore "analytix.local/runtime-go/internal/adapters/outbound/piiauthorization"
	publicationstore "analytix.local/runtime-go/internal/adapters/outbound/reportpublication"
	piiapp "analytix.local/runtime-go/internal/app/piiauthorization"
	publicationapp "analytix.local/runtime-go/internal/app/reportpublication"
	domainpii "analytix.local/runtime-go/internal/domain/piiauthorization"
	domainstartup "analytix.local/runtime-go/internal/domain/startup"
)

// This historical cut carries no release service or restart writer.
// All committed receipts/dispositions and grants survive, including records
// omitted from the open-only application restart plan.
type runtimeCompletedControlledAccessHistoryV2 struct {
	owners       map[string]runtimeOriginalSemanticFilesV1
	hasRecords   bool
	openReceipts map[string]domainpii.ControlledArtifactAccessReceiptV2
}

func prepareRuntimeCompletedControlledAccessHistoryV2(ctx context.Context, core *runtimeChildIdentityStartupV1, publication *runtimePublicationSemanticPreservationV1, originalHistory, finalHistory *runtimeOriginalReportHistoryV1, original, final map[string]runtimeOriginalSemanticFilesV1) (*runtimeCompletedControlledAccessHistoryV2, error) {
	preserved := &runtimeCompletedControlledAccessHistoryV2{owners: map[string]runtimeOriginalSemanticFilesV1{}, openReceipts: map[string]domainpii.ControlledArtifactAccessReceiptV2{}}
	for _, owner := range []string{"pii-authorization", "controlled-artifact-access-v2"} {
		before := runtimeReportCommittedFilesV1(original[owner])
		preserved.owners[owner] = before.cloneV1()
	}
	hasRecords, err := verifyRuntimeControlledAccessEndpointV2(ctx, core, publication, originalHistory, original)
	if err != nil {
		return nil, err
	}
	// Only receipts from the independently audited Original graph can receive
	// a new indeterminate disposition. Existing closed records stay immutable.
	for name, entry := range preserved.owners["controlled-artifact-access-v2"] {
		if !strings.HasPrefix(name, "access-receipts/") {
			continue
		}
		receipt, err := domainpii.ParseControlledArtifactAccessReceiptV2(entry.Body)
		if err != nil {
			return nil, err
		}
		disposition := "access-dispositions/" + receipt.AccessID[:2] + "/" + receipt.AccessID + ".json"
		if _, closed := preserved.owners["controlled-artifact-access-v2"][disposition]; !closed {
			preserved.openReceipts[receipt.AccessID] = receipt
		}
	}
	for _, owner := range []string{"pii-authorization", "controlled-artifact-access-v2"} {
		if err := preserved.validateOwnerDeltaV2(owner, final[owner]); err != nil {
			return nil, errors.Join(errors.New("completed controlled access history changed in signed Final"), err)
		}
	}
	finalHasRecords, err := verifyRuntimeControlledAccessEndpointV2(ctx, core, publication, finalHistory, final)
	if err != nil {
		return nil, err
	}
	if hasRecords != finalHasRecords {
		return nil, errors.New("completed controlled access inventory changed")
	}
	preserved.hasRecords = hasRecords
	return preserved, nil
}

func verifyRuntimeControlledAccessEndpointV2(ctx context.Context, core *runtimeChildIdentityStartupV1, publication *runtimePublicationSemanticPreservationV1, history *runtimeOriginalReportHistoryV1, files map[string]runtimeOriginalSemanticFilesV1) (bool, error) {
	if ctx == nil || core == nil || core.verification == nil || publication == nil || history == nil {
		return false, errRuntimeReportRestartReconciliationRequired
	}
	if _, _, mixed := runtimeMixedClosedDeferredPlansV1(history.plan); !mixed && !history.matchesCompletedPlanV1(history.plan) {
		return false, errRuntimeReportRestartReconciliationRequired
	}
	check := func(id, key string) error {
		if id != core.verification.KeyID() || key != base64.RawURLEncoding.EncodeToString(core.verification.PublicKey()) {
			return errors.New("controlled access history belongs to another current key")
		}
		return nil
	}
	reports, err := publicationstore.ParseOriginalReadersV1(ctx, files["report-publication"], publication.installation, check, publication.creationProofV1("report-publication"))
	if err != nil {
		return false, err
	}
	grants, err := piistore.ParseOriginalGrantReaderV1(ctx, files["pii-authorization"], publication.installation, check, publication.creationProofV1("pii-authorization"))
	if err != nil {
		return false, err
	}
	access, err := piistore.ParseOriginalAccessReaderV2(ctx, files["controlled-artifact-access-v2"], publication.installation, check, publication.creationProofV1("controlled-artifact-access-v2"))
	if err != nil {
		return false, err
	}
	bridge, err := publicationapp.NewHistoricalArtifactDeliveryBridgeV1(history.authority, reports.Commits, reports.Receipts, core.verification)
	if err != nil {
		return false, err
	}
	_, err = piiapp.VerifyControlledAccessInventoryV2(ctx, piiapp.ControlledAccessInventoryDependenciesV2{Access: access, Historical: bridge, Grants: grants, Artifacts: reports.Artifacts, Authority: core.verification})
	if err != nil {
		return false, err
	}
	hasRecords := false
	if err := access.VisitAccessReceiptsV2(ctx, func(domainpii.ControlledArtifactAccessReceiptV2) error { hasRecords = true; return nil }); err != nil {
		return false, err
	}
	return hasRecords, context.Cause(ctx)
}

// Reobserve physical bytes, before any prospective After can supply them.
// The report authority is rebuilt from the strict current/candidate primary
// by the semantic guard; its Original snapshot is never reused after mutation.
func (preserved *runtimeCompletedControlledAccessHistoryV2) validateCurrentV2(ctx context.Context, core *runtimeChildIdentityStartupV1, publication *runtimePublicationSemanticPreservationV1, history *runtimeOriginalReportHistoryV1, reports runtimeOriginalSemanticFilesV1, operations []domainstartup.SemanticStartupOperationV1, readAfter func(domainstartup.SemanticStartupOperationV1) ([]byte, error), noWriteOperationID string) (resultErr error) {
	if preserved == nil {
		return errRuntimeReportRestartReconciliationRequired
	}
	files := map[string]runtimeOriginalSemanticFilesV1{"report-publication": reports}
	for _, owner := range []string{"pii-authorization", "controlled-artifact-access-v2"} {
		root := filepath.Join(core.roots.DataDir, "private", owner)
		var observation *finalauthority.OriginalFixedOwnerObservationV1
		var err error
		if owner == "pii-authorization" {
			observation, err = piistore.PrepareOriginalObservationV1(ctx, root, core.access, publication.creationProofV1(owner))
		} else {
			observation, err = piistore.PrepareOriginalAccessObservationV2(ctx, root, core.access, publication.creationProofV1(owner))
		}
		if err != nil {
			return err
		}
		defer func() { resultErr = errors.Join(resultErr, observation.RevalidatePhysicalV1(ctx)) }()
		raw, err := observation.SnapshotOriginalFilesV1(ctx)
		if err != nil {
			return err
		}
		if err := preserved.validateOwnerDeltaV2(owner, raw); err != nil {
			return err
		}
		files[owner] = raw
	}
	// This observes and authenticates every journal operation, including its
	// applied prefix. Future records never supply a missing physical Original.
	original, final, observation, err := publication.readV1(ctx)
	if err != nil {
		return err
	}
	defer func() { resultErr = errors.Join(resultErr, observation.Revalidate(ctx)) }()
	for _, owner := range []string{"pii-authorization", "controlled-artifact-access-v2"} {
		if err := preserved.validateOwnerDeltaV2(owner, original[owner]); err != nil {
			return err
		}
		if err := preserved.validateOwnerDeltaV2(owner, final[owner]); err != nil {
			return err
		}
		files[owner] = final[owner].cloneV1()
		for _, operation := range operations {
			if noWriteOperationID != "" && operation.OperationID == noWriteOperationID {
				continue
			}
			name, owned := runtimeAssociatedRelativeV1(owner, operation)
			if owned {
				if err := files[owner].applyV1(operation, name, readAfter); err != nil {
					return err
				}
			}
		}
		if err := preserved.validateOwnerDeltaV2(owner, files[owner]); err != nil {
			return err
		}
	}
	hasRecords, err := verifyRuntimeControlledAccessEndpointV2(ctx, core, publication, history, files)
	if err != nil {
		return err
	}
	if hasRecords != preserved.hasRecords {
		return errors.New("completed controlled access denominator changed")
	}
	return nil
}

func (preserved *runtimeCompletedControlledAccessHistoryV2) validateOwnerDeltaV2(owner string, files runtimeOriginalSemanticFilesV1) error {
	// Native Windows File.Stat projects a writable CAS file as 0666. Its
	// DACL/attributes remain validated by the native physical observation.
	newRecordMode := uint32(0600)
	if runtime.GOOS == "windows" {
		newRecordMode = 0666
	}
	committed := runtimeReportCommittedFilesV1(files)
	before := preserved.owners[owner]
	for name, entry := range before {
		if current, found := committed[name]; !found || !reflect.DeepEqual(entry, current) {
			return errors.New("completed controlled access original bytes, mode or presence changed")
		}
	}
	for name, entry := range committed {
		if _, exists := before[name]; exists {
			continue
		}
		if owner != "controlled-artifact-access-v2" {
			return errors.New("controlled access history added an immutable grant")
		}
		disposition, err := domainpii.ParseControlledArtifactAccessDispositionV2(entry.Body)
		if err != nil {
			return err
		}
		receipt, open := preserved.openReceipts[disposition.AccessID]
		canonical, canonicalErr := domainpii.ControlledArtifactAccessDispositionV2Bytes(disposition)
		if !open || canonicalErr != nil || !bytes.Equal(canonical, entry.Body) || entry.Mode != newRecordMode ||
			name != "access-dispositions/"+receipt.AccessID[:2]+"/"+receipt.AccessID+".json" ||
			domainpii.ValidateControlledArtifactAccessDispositionForReceiptV2(disposition, receipt) != nil ||
			disposition.AuthorityKeyID != receipt.AuthorityKeyID || disposition.AuthorityPublicKey != receipt.AuthorityPublicKey ||
			disposition.Status != domainpii.ControlledArtifactAccessDispositionReleaseIndeterminateV1 ||
			disposition.ReasonCode != domainpii.ControlledArtifactAccessReasonReleaseIndeterminateV1 || disposition.ReleasedByteLength != receipt.ArtifactByteLength {
			return errors.New("controlled access restart addition is not exact Original-open indeterminate authority")
		}
	}
	return nil
}
