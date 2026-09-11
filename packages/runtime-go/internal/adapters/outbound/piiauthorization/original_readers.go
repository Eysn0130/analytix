package piiauthorization

import (
	"context"
	"errors"
	"sort"

	finalauthority "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	domainpii "analytix.local/runtime-go/internal/domain/piiauthorization"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	piiport "analytix.local/runtime-go/internal/ports/piiauthorization"
)

// OriginalGrantReaderV1 has detached canonical records and no signing,
// mutation, recovery or current authorization capability. The caller owns
// the native physical and installation observation lifetime.
type OriginalGrantReaderV1 struct {
	grants originalPIIRecordReaderV1[domainpii.PIIProjectionGrantV1]
}

func ParseOriginalGrantReaderV1(ctx context.Context, files map[string]finalauthority.SecurePrivateCASOriginalEntryV1, authority *finalauthority.AnchoredFileAuthority, check func(string, string) error, creates *finalauthority.PreparedSecurePrivateCASOriginalCreateResiduesV1) (*OriginalGrantReaderV1, error) {
	records, err := captureOriginalPIIRecordsV1(ctx, files, originalGrantLeavesV1(), authority, check, creates, (&PreparedRecoveryV1{}).validateDomainSemantics)
	if err != nil {
		return nil, err
	}
	return &OriginalGrantReaderV1{grants: originalPIIRecordReaderV1[domainpii.PIIProjectionGrantV1]{records["grants"], domainpii.ParsePIIProjectionGrantV1}}, nil
}

func (reader *OriginalGrantReaderV1) ResolveGrant(ctx context.Context, id string) (domainpii.PIIProjectionGrantV1, error) {
	if reader == nil {
		return domainpii.PIIProjectionGrantV1{}, errors.New("original PII grant reader is unavailable")
	}
	return reader.grants.resolve(ctx, id)
}

func (reader *OriginalGrantReaderV1) VisitGrants(ctx context.Context, visit func(domainpii.PIIProjectionGrantV1) error) error {
	if reader == nil {
		return errors.New("original PII grant reader is unavailable")
	}
	return reader.grants.visit(ctx, visit)
}

// OriginalAccessReaderV2 retains the complete V2 receipt/disposition
// denominator. Closed records are not collapsed into an empty restart plan.
type OriginalAccessReaderV2 struct {
	receipts     originalPIIRecordReaderV1[domainpii.ControlledArtifactAccessReceiptV2]
	dispositions originalPIIRecordReaderV1[domainpii.ControlledArtifactAccessDispositionV2]
}

func ParseOriginalAccessReaderV2(ctx context.Context, files map[string]finalauthority.SecurePrivateCASOriginalEntryV1, authority *finalauthority.AnchoredFileAuthority, check func(string, string) error, creates *finalauthority.PreparedSecurePrivateCASOriginalCreateResiduesV1) (*OriginalAccessReaderV2, error) {
	records, err := captureOriginalPIIRecordsV1(ctx, files, originalAccessLeavesV2(), authority, check, creates, (&PreparedAccessRecoveryV2{}).validateDomainSemantics)
	if err != nil {
		return nil, err
	}
	return &OriginalAccessReaderV2{
		receipts:     originalPIIRecordReaderV1[domainpii.ControlledArtifactAccessReceiptV2]{records["access-receipts"], domainpii.ParseControlledArtifactAccessReceiptV2},
		dispositions: originalPIIRecordReaderV1[domainpii.ControlledArtifactAccessDispositionV2]{records["access-dispositions"], domainpii.ParseControlledArtifactAccessDispositionV2},
	}, nil
}

func (reader *OriginalAccessReaderV2) ResolveAccessReceiptV2(ctx context.Context, id string) (domainpii.ControlledArtifactAccessReceiptV2, error) {
	if reader == nil {
		return domainpii.ControlledArtifactAccessReceiptV2{}, errors.New("original V2 access reader is unavailable")
	}
	return reader.receipts.resolve(ctx, id)
}

func (reader *OriginalAccessReaderV2) ResolveAccessDispositionV2(ctx context.Context, id string) (domainpii.ControlledArtifactAccessDispositionV2, error) {
	if reader == nil {
		return domainpii.ControlledArtifactAccessDispositionV2{}, errors.New("original V2 access reader is unavailable")
	}
	return reader.dispositions.resolve(ctx, id)
}

func (reader *OriginalAccessReaderV2) VisitAccessReceiptsV2(ctx context.Context, visit func(domainpii.ControlledArtifactAccessReceiptV2) error) error {
	if reader == nil {
		return errors.New("original V2 access reader is unavailable")
	}
	return reader.receipts.visit(ctx, visit)
}

func (reader *OriginalAccessReaderV2) VisitAccessDispositionsV2(ctx context.Context, visit func(domainpii.ControlledArtifactAccessDispositionV2) error) error {
	if reader == nil {
		return errors.New("original V2 access reader is unavailable")
	}
	return reader.dispositions.visit(ctx, visit)
}

func captureOriginalPIIRecordsV1(ctx context.Context, files map[string]finalauthority.SecurePrivateCASOriginalEntryV1, leaves []finalauthority.SecurePrivateCASOwnerLeafV1, authority *finalauthority.AnchoredFileAuthority, check func(string, string) error, creates *finalauthority.PreparedSecurePrivateCASOriginalCreateResiduesV1, validate func(context.Context, finalauthority.PrivateCASDomainVisitor, finalauthority.PrivateCASDomainMaterialVisitor, func(string, string) error) error) (map[string]map[string][]byte, error) {
	if ctx == nil || authority == nil && check == nil {
		return nil, errors.New("original PII reader authentication is unavailable")
	}
	records := map[string]map[string][]byte{}
	err := finalauthority.ValidateOriginalDomainEntriesV1(ctx, files, leaves, authority, check, creates, func(visit finalauthority.PrivateCASDomainVisitor, materials finalauthority.PrivateCASDomainMaterialVisitor, keyCheck func(string, string) error) error {
		if err := validate(ctx, visit, materials, keyCheck); err != nil {
			return err
		}
		for _, leaf := range leaves {
			values := map[string][]byte{}
			if err := visit(leaf.Name, func(file finalauthority.SecurePrivateCASFile) error {
				if _, duplicate := values[file.Digest]; duplicate {
					return errors.New("original PII inventory repeats a record")
				}
				values[file.Digest] = append([]byte(nil), file.Body...)
				return nil
			}); err != nil {
				return err
			}
			records[leaf.Name] = values
		}
		return context.Cause(ctx)
	})
	if err != nil {
		return nil, err
	}
	return records, nil
}

type originalPIIRecordReaderV1[T any] struct {
	records map[string][]byte
	parse   func([]byte) (T, error)
}

func (reader originalPIIRecordReaderV1[T]) resolve(ctx context.Context, id string) (result T, resultErr error) {
	if ctx == nil || reader.parse == nil || !domainsecurity.IsSHA256Hex(id) {
		return result, errors.New("original PII record selector is invalid")
	}
	if err := context.Cause(ctx); err != nil {
		return result, err
	}
	body, found := reader.records[id]
	if !found {
		return result, piiport.ErrNotFound
	}
	return reader.parse(body)
}

func (reader originalPIIRecordReaderV1[T]) visit(ctx context.Context, visit func(T) error) error {
	if ctx == nil || visit == nil || reader.parse == nil {
		return errors.New("original PII inventory visitor is unavailable")
	}
	ids := make([]string, 0, len(reader.records))
	for id := range reader.records {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		record, err := reader.resolve(ctx, id)
		if err != nil {
			return err
		}
		if err := visit(record); err != nil {
			return err
		}
	}
	return context.Cause(ctx)
}
