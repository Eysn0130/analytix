// Package privatecasrecovery provides signed-journal recovery helpers for
// adapter tests outside finalauthority. It is test support, not a production
// recovery coordinator.
package privatecasrecovery

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"

	finalauthority "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	persistencefs "analytix.local/runtime-go/internal/adapters/outbound/persistencefs"
	domainprivatecas "analytix.local/runtime-go/internal/domain/privatecastopology"
)

// ApplyV4 executes one already semantically validated authority through the
// production signed-journal engine. Root IDs are recovered from the typed
// runtime catalog, so a test-only or forgotten CAS root fails closed.
func ApplyV4(
	ctx context.Context,
	participantID string,
	authority finalauthority.PreparedSecurePrivateCASRecoveryAuthorityV3,
) (resultErr error) {
	if authority == nil || participantID == "" || participantID != strings.TrimSpace(participantID) {
		return errors.New("test private CAS recovery authority is invalid")
	}
	plans := authority.SecurePrivateCASRecoveryPlansV2()
	if len(plans) == 0 {
		return errors.New("test private CAS recovery authority has no plans")
	}
	bindings := make([]finalauthority.SecurePrivateCASRecoveryRootBindingV4, 0, len(plans))
	seen := make(map[string]struct{}, len(plans))
	for _, plan := range plans {
		if plan == nil {
			return errors.New("test private CAS recovery plan is nil")
		}
		rootID, err := catalogRootID(plan.RootPath())
		if err != nil {
			return err
		}
		if _, duplicate := seen[rootID]; duplicate {
			return errors.New("test private CAS recovery repeats a catalog root")
		}
		seen[rootID] = struct{}{}
		bindings = append(bindings, finalauthority.SecurePrivateCASRecoveryRootBindingV4{
			RootID: rootID, RootPath: plan.RootPath(),
		})
	}

	journalRoot, err := os.MkdirTemp("", "analytix-private-cas-v4-test-")
	if err != nil {
		return err
	}
	defer func() { resultErr = errors.Join(resultErr, os.RemoveAll(journalRoot)) }()
	roots, err := persistencefs.ResolveRootSet(journalRoot, journalRoot)
	if err != nil {
		return err
	}
	lease, err := persistencefs.AcquireCompositeLease(roots)
	if err != nil {
		return err
	}
	defer func() { resultErr = errors.Join(resultErr, lease.Close()) }()
	digest := sha256.Sum256([]byte("analytix.test-private-cas-recovery/v4\x00" + participantID))
	preflightOwner := &journalAuthorityTestPreflightV1{
		authority: authority, digest: hex.EncodeToString(digest[:]),
	}
	authorityPreflight, err := persistencefs.PrepareJournalAuthorityBootstrapV1(ctx, lease, preflightOwner)
	if err != nil {
		return err
	}
	if _, present, err := authorityPreflight.BindExistingV1(ctx); err != nil {
		return err
	} else if !present {
		if _, err := authorityPreflight.CreateV1(ctx); err != nil {
			return err
		}
	}
	journal, err := persistencefs.NewPrivateCASRecoveryJournalV1(lease)
	if err != nil {
		return err
	}
	return finalauthority.ApplyPreparedSecurePrivateCASRecoveryTransactionV4(
		ctx,
		[]finalauthority.PreparedSecurePrivateCASRecoveryParticipantV4{{
			ParticipantID: participantID, SemanticAuthorityDigest: hex.EncodeToString(digest[:]),
			Authority: authority, Roots: bindings,
		}},
		journal,
	)
}

type journalAuthorityTestPreflightV1 struct {
	authority finalauthority.PreparedSecurePrivateCASRecoveryAuthorityV3
	digest    string
}

func (owner *journalAuthorityTestPreflightV1) ObserveJournalAuthorityPreflightV1(
	ctx context.Context,
) (string, error) {
	if owner == nil || owner.authority == nil || owner.digest == "" {
		return "", errors.New("test journal-authority preflight is unavailable")
	}
	if err := owner.authority.Revalidate(ctx); err != nil {
		return "", err
	}
	return owner.digest, nil
}

func (owner *journalAuthorityTestPreflightV1) ValidateJournalAuthorityPreflightV1(
	ctx context.Context,
	expected string,
) error {
	if owner == nil || owner.authority == nil || expected == "" || expected != owner.digest {
		return errors.New("test journal-authority preflight changed")
	}
	return owner.authority.Revalidate(ctx)
}

func catalogRootID(root string) (string, error) {
	canonical := filepath.ToSlash(filepath.Clean(root))
	match := ""
	for _, spec := range domainprivatecas.RuntimeRootSpecsV1() {
		if canonical == spec.RelativeCASRoot || strings.HasSuffix(canonical, "/"+spec.RelativeCASRoot) {
			if match != "" {
				return "", errors.New("test private CAS recovery root aliases multiple catalog entries")
			}
			match = spec.RootID
		}
	}
	if match == "" {
		return "", errors.New("test private CAS recovery root is outside the runtime catalog")
	}
	return match, nil
}
