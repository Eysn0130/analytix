package runtimeapp

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"errors"

	evidenceregistrystore "analytix.local/runtime-go/internal/adapters/outbound/evidenceregistry"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

// This verifier has no storage or capability. The surrounding observation
// independently revalidates the original key and enrollment on both sides of
// the pure domain checks, including when those checks reject a record.
type runtimeOriginalRegistryVerifierV1 struct {
	keyID     string
	publicKey []byte
}

func (v runtimeOriginalRegistryVerifierV1) KeyID() string { return v.keyID }
func (v runtimeOriginalRegistryVerifierV1) PublicKey() []byte {
	return append([]byte(nil), v.publicKey...)
}
func (v runtimeOriginalRegistryVerifierV1) VerifyTrusted(ctx context.Context, key string, public, body, signature []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if key != v.keyID || !bytes.Equal(public, v.publicKey) || !ed25519.Verify(v.publicKey, body, signature) {
		return errors.New("original registry record is not authenticated by this installation")
	}
	return nil
}

// Physical reads and authenticated journal/primary observations belong to the
// caller and never enter this domain-only error classification. An unavailable
// result is usable only with the caller's complete immutable raw owner map.
func observeRuntimeOriginalRegistryDomainV1(ctx context.Context, core *runtimeChildIdentityStartupV1, files runtimeOriginalSemanticFilesV1, contexts []domainsecurity.TurnSecurityContext, complete bool) (inventory runtimeOriginalRegistryInventoryV1, resultErr error) {
	if ctx == nil || core == nil || core.verification == nil || core.revalidateKey == nil {
		return inventory, errors.New("original registry verification is unavailable")
	}
	if err := errors.Join(ctx.Err(), core.revalidateKey(ctx)); err != nil {
		return inventory, err
	}
	verifier := runtimeOriginalRegistryVerifierV1{keyID: core.verification.KeyID(), publicKey: append([]byte(nil), core.verification.PublicKey()...)}
	if len(verifier.publicKey) != ed25519.PublicKeySize || verifier.keyID != domainsecurity.SHA256Hex(verifier.publicKey) {
		return inventory, errors.New("original registry key observation is invalid")
	}
	v2 := evidenceregistrystore.OriginalFilesContainV2(files)
	if v2 {
		if err := core.originalRegistryTrust.Revalidate(ctx); err != nil {
			return inventory, err
		}
	}
	defer func() {
		resultErr = errors.Join(resultErr, core.revalidateKey(ctx), ctx.Err())
		if v2 || inventory.unavailable {
			resultErr = errors.Join(resultErr, core.originalRegistryTrust.Revalidate(ctx))
		}
		if resultErr != nil {
			inventory = runtimeOriginalRegistryInventoryV1{}
		}
	}()
	var domainErr error
	if v2 {
		trust := core.originalRegistryTrust.projection
		if complete {
			graph, err := evidenceregistrystore.ParseOriginalGraphV2(ctx, files, contexts, trust.InstallationID, trust.Enrollment.EnrollmentID, verifier)
			domainErr = err
			if err == nil {
				inventory.v2 = &graph
			}
		} else {
			domainErr = evidenceregistrystore.ValidateOriginalFilesV2(ctx, files, contexts, trust.InstallationID, trust.Enrollment.EnrollmentID, verifier)
		}
	} else if complete {
		inventory.legacy, domainErr = evidenceregistrystore.ParseOriginalLegacyInventoryV1(ctx, files, contexts, verifier)
	} else {
		domainErr = evidenceregistrystore.ValidateOriginalLegacyFilesV1(ctx, files, verifier)
	}
	if errors.Is(domainErr, context.Canceled) || errors.Is(domainErr, context.DeadlineExceeded) {
		return runtimeOriginalRegistryInventoryV1{}, domainErr
	}
	if domainErr != nil {
		// Even legacy failures require an independent installation. A local
		// self-consistent key alone cannot authorize this degradation.
		if err := core.originalRegistryTrust.Revalidate(ctx); err != nil {
			return runtimeOriginalRegistryInventoryV1{}, err
		}
		inventory = runtimeOriginalRegistryInventoryV1{unavailable: true}
	}
	return inventory, nil
}
