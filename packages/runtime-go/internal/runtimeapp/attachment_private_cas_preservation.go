package runtimeapp

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainstartup "analytix.local/runtime-go/internal/domain/startup"
)

// The complete owner was semantically verified before this participant was
// bound. Revalidation uses existing physical plans only: V4 invokes it while
// holding the recovery exclusion, where reopening a live CAS would deadlock.
type runtimeAttachmentPrivateCASPreservationV1 struct {
	runtimePreparedPrivateCASOwnerRecovery
	observation       *runtimeAttachmentSemanticObservationV1
	core              *runtimeChildIdentityStartupV1
	revalidatePrimary func(context.Context) error
	digest            string
}

func (preserved runtimeReportRestartPreservationV1) bindAttachmentPrivateCASPreservationV1(ctx context.Context, prepared runtimePreparedPrivateCASOwnerRecovery) (_ runtimePreparedPrivateCASOwnerRecovery, resultErr error) {
	if preserved.report == nil || len(preserved.report.ThreadIDs()) == 0 {
		return prepared, nil
	}
	if prepared == nil || preserved.attachments == nil || preserved.core == nil {
		return nil, errors.New("attachment private CAS preservation is unavailable")
	}
	if err := preserved.validateAttachmentSemanticOperationsV1(ctx, nil, nil, ""); err != nil {
		return nil, err
	}
	preserved.attachments.mu.Lock()
	defer preserved.attachments.mu.Unlock()
	original, _, contexts, observation, err := readRuntimeAttachmentSemanticInventoryV1(ctx, preserved.core, preserved.report, preserved.attachments.recoveryBefore, preserved.attachments.createResidues)
	if err != nil {
		return nil, err
	}
	defer func() {
		resultErr = errors.Join(resultErr, observation.Revalidate(ctx), prepared.Revalidate(ctx), preserved.core.revalidateKey(ctx), preserved.report.RevalidatePrimary(ctx))
	}()
	inventory, err := original.verifyV1(ctx, preserved.core, contexts, preserved.report, observation.prepared)
	if err != nil {
		return nil, err
	}
	if err := preserved.attachments.validateHeldV1(original, inventory, preserved.report); err != nil {
		return nil, err
	}
	body, err := json.Marshal(struct {
		CreateResidues string                                        `json:"createResidues"`
		Version        string                                        `json:"version"`
		KeyID          string                                        `json:"keyId"`
		DeniedThreads  []string                                      `json:"deniedThreads"`
		Held           map[string]domainstartup.SemanticEntryStateV1 `json:"held"`
		Modes          map[string]uint32                             `json:"modes"`
	}{preserved.attachments.createResidues.FingerprintV1(), "analytix.original-attachment-private-cas-preservation/v1", preserved.core.verification.KeyID(), preserved.report.DeniedThreadIDsV1(), preserved.attachments.held, preserved.attachments.sharedModes})
	if err != nil {
		return nil, err
	}
	return &runtimeAttachmentPrivateCASPreservationV1{runtimePreparedPrivateCASOwnerRecovery: prepared, observation: observation, core: preserved.core, revalidatePrimary: preserved.report.RevalidatePrimary, digest: domainsecurity.SHA256Hex(body)}, nil
}

func (prepared *runtimeAttachmentPrivateCASPreservationV1) Revalidate(ctx context.Context) error {
	if prepared == nil || prepared.runtimePreparedPrivateCASOwnerRecovery == nil || prepared.observation == nil || prepared.core == nil || prepared.revalidatePrimary == nil || !domainsecurity.IsSHA256Hex(prepared.digest) {
		return errors.New("attachment private CAS preservation binding is invalid")
	}
	if err := prepared.runtimePreparedPrivateCASOwnerRecovery.Revalidate(ctx); err != nil {
		return err
	}
	return errors.Join(prepared.observation.Revalidate(ctx), prepared.core.revalidateKey(ctx), prepared.revalidatePrimary(ctx))
}

func (prepared *runtimeAttachmentPrivateCASPreservationV1) ValidateSemantics(ctx context.Context) error {
	return prepared.Revalidate(ctx)
}
func (prepared *runtimeAttachmentPrivateCASPreservationV1) PrivateCASRecoveryAdditionalSemanticDigestV4() string {
	if prepared == nil {
		return ""
	}
	return prepared.digest
}
func (prepared *runtimeAttachmentPrivateCASPreservationV1) FreezeSecurePrivateCASRecoveryTargetsV4() bool {
	return prepared != nil
}

// Bind outside recovery exclusion. The returned closure only revalidates
// already prepared physical proofs and never re-enters a live CAS operation.
func (preserved runtimeReportRestartPreservationV1) prepareAttachmentDirectoryRevalidationV1(ctx context.Context, requireSettled bool) ([]string, func(context.Context) error, error) {
	if preserved.report == nil || len(preserved.report.ThreadIDs()) == 0 {
		return nil, nil, nil
	}
	if preserved.attachments == nil || preserved.core == nil {
		return nil, nil, errors.New("original attachment directory preservation is unavailable")
	}
	preserved.attachments.mu.Lock()
	defer preserved.attachments.mu.Unlock()
	original, _, contexts, observation, err := readRuntimeAttachmentSemanticInventoryV1(ctx, preserved.core, preserved.report, preserved.attachments.recoveryBefore, preserved.attachments.createResidues)
	if err != nil {
		return nil, nil, err
	}
	if requireSettled && observation.journal != nil {
		return nil, nil, errors.New("attachment orphan cleanup cannot interleave with semantic recovery")
	}
	inventory, err := original.verifyV1(ctx, preserved.core, contexts, preserved.report, observation.prepared)
	if err != nil {
		return nil, nil, err
	}
	if err := preserved.attachments.validateHeldV1(original, inventory, preserved.report); err != nil {
		return nil, nil, err
	}
	directories := make([]string, 0, len(preserved.attachments.sharedModes))
	creationNames := make(map[string]bool)
	for _, relative := range preserved.attachments.createResidues.RelativePathsV1() {
		creationNames["data/"+relative] = true
	}
	for name := range preserved.attachments.sharedModes {
		if creationNames[name] {
			continue // The typed proof supplies these physical names separately.
		}
		if name == runtimeAttachmentAuthorityRootV1 || strings.HasPrefix(name, runtimeAttachmentAuthorityRootV1+"/") {
			directories = append(directories, strings.TrimPrefix(name, "data/"))
		}
	}
	sort.Strings(directories)
	revalidate := func(ctx context.Context) error {
		return errors.Join(observation.Revalidate(ctx), preserved.core.revalidateKey(ctx), preserved.report.RevalidatePrimary(ctx))
	}
	if err := revalidate(ctx); err != nil {
		return nil, nil, err
	}
	return directories, revalidate, nil
}
