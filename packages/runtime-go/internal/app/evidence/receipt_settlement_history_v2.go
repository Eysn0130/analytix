package evidence

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"reflect"

	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	registryport "analytix.local/runtime-go/internal/ports/evidenceregistry"
)

func cloneOriginalRegistryHistoryV2(history *registryport.OriginalHistoryV2) (*registryport.OriginalHistoryV2, error) {
	if history == nil {
		return nil, nil
	}
	body, err := json.Marshal(history)
	if err != nil {
		return nil, err
	}
	var frozen registryport.OriginalHistoryV2
	if err := json.Unmarshal(body, &frozen); err != nil {
		return nil, err
	}
	return &frozen, nil
}

// The producer proves the complete original graph and independent enrollment.
// This consumer binds every capsule's issues to exact signed prepared records
// and the durable markers whose full historical grant/result prefixes were
// validated above. Shared prefixes across capsules remain separate history;
// they never enter the current issue map or acquire repair authority.
func validateOriginalSettlementRegistryHistoryV2(ctx context.Context, issuer Issuer, history *registryport.OriginalHistoryV2,
	contexts map[string]domainsecurity.TurnSecurityContext, prepared map[string]domainevidence.PreparedEvidenceSettlement,
	markers map[string]settlementMarkerAuthority, markerOrder map[string][]string) error {
	if history == nil {
		return nil
	}
	if history.Indexes == nil || history.Capsules == nil {
		return errors.New("original registry V2 graph denominator is unavailable")
	}
	for digest, capsule := range history.Capsules {
		if err := ctx.Err(); err != nil {
			return err
		}
		frozen := capsule.SecurityContext
		if digest != capsule.RecordDigest || domainevidence.ValidateEvidenceRegistryAuthorityCapsule(capsule) != nil ||
			domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(frozen) != nil || !reflect.DeepEqual(contexts[frozen.ContextDigest], frozen) {
			return errors.New("original registry V2 capsule context or address is invalid")
		}
		seal := capsule.Seal
		publicKey, keyErr := base64.RawURLEncoding.DecodeString(seal.AuthorityPublicKey)
		signature, signatureErr := base64.RawURLEncoding.DecodeString(seal.AuthoritySignature)
		if err := errors.Join(keyErr, signatureErr); err != nil {
			return err
		}
		if err := issuer.Authority.VerifyTrusted(ctx, seal.AuthorityKeyID, publicKey, domainevidence.EvidenceRegistryAuthoritySealSigningBytes(seal), signature); err != nil {
			return errors.Join(errors.New("original registry V2 capsule is untrusted"), err)
		}
		order := 0
		for _, entry := range capsule.Registry.Entries {
			if entry.Operation != domainevidence.EvidenceRegistryIssue {
				continue
			}
			record, exists := prepared[entry.SettlementID]
			marker, marked := markers[entry.SettlementID]
			if !exists || !marked || !reflect.DeepEqual(record.SecurityContext, frozen) {
				return errors.New("original registry V2 issue lacks exact prepared and durable marker support")
			}
			expected, err := domainevidence.NewHostEvidenceSettlementMarker(record)
			if err != nil || marker.marker != expected {
				return errors.Join(errors.New("original registry V2 issue marker changed"), err)
			}
			if _, found, err := domainevidence.ResolvePreparedSettlementIssue(capsule.Registry, record); err != nil || !found {
				return errors.Join(errors.New("original registry V2 issue conflicts with prepared authority"), err)
			}
			sequence := markerOrder[frozen.ContextDigest]
			if order >= len(sequence) || sequence[order] != entry.SettlementID {
				return errors.New("original registry V2 issue is not a durable marker prefix")
			}
			order++
		}
	}
	return ctx.Err()
}
