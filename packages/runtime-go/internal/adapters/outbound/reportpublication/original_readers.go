package reportpublication

import (
	"context"
	"errors"
	"sort"

	finalauthorityadapter "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	domainpii "analytix.local/runtime-go/internal/domain/piiauthorization"
	domainpublication "analytix.local/runtime-go/internal/domain/reportpublication"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	publicationport "analytix.local/runtime-go/internal/ports/reportpublication"
)

// OriginalReadersV1 exposes detached canonical Original records through only
// read interfaces. It has no CAS handles, mutation or recovery capability.
// Its caller owns the native physical/current-key observation lifetime.
type OriginalReadersV1 struct {
	Attempts         originalAttemptReaderV1
	Receipts         originalReceiptReaderV1
	Commits          originalCommitReaderV1
	Selections       originalSelectionReaderV1
	Decisions        originalDecisionReaderV1
	GrantSettlements originalGrantSettlementReaderV1
	StageCompletions originalStageCompletionReaderV1
	DeliveryOutcomes originalDeliveryOutcomeReaderV1
	Indexes          originalIndexReaderV1
	Ledgers          originalLedgerReaderV1
	PIIProjections   originalPIIProjectionReaderV1
	Inspections      originalInspectionReaderV1
	Artifacts        originalArtifactReaderV1
}

// ParseOriginalReadersV1 validates the complete owner with the same canonical,
// material and domain graph rules as normal recovery, then freezes its bytes.
// Authentication is supplied by an independently anchored installation or the
// caller's current-key check; embedded record keys alone are insufficient.
func ParseOriginalReadersV1(ctx context.Context, files map[string]finalauthorityadapter.SecurePrivateCASOriginalEntryV1, authority *finalauthorityadapter.AnchoredFileAuthority, localCheck func(string, string) error, creates *finalauthorityadapter.PreparedSecurePrivateCASOriginalCreateResiduesV1) (*OriginalReadersV1, error) {
	if ctx == nil || authority == nil && localCheck == nil {
		return nil, errors.New("original report reader authentication is unavailable")
	}
	bodies := map[string]map[string][]byte{}
	err := finalauthorityadapter.ValidateOriginalDomainEntriesV1(ctx, files, originalPublicationLeavesV1(), authority, localCheck, creates, func(visit finalauthorityadapter.PrivateCASDomainVisitor, materials finalauthorityadapter.PrivateCASDomainMaterialVisitor, check func(string, string) error) error {
		if err := (&PreparedRecoveryV1{}).validateDomainSemantics(ctx, visit, materials, check); err != nil {
			return err
		}
		for _, leaf := range originalPublicationLeavesV1() {
			records := map[string][]byte{}
			if err := visit(leaf.Name, func(file finalauthorityadapter.SecurePrivateCASFile) error {
				if _, exists := records[file.Digest]; exists {
					return errors.New("original report inventory repeats a record")
				}
				records[file.Digest] = append([]byte(nil), file.Body...)
				return nil
			}); err != nil {
				return err
			}
			bodies[leaf.Name] = records
		}
		return context.Cause(ctx)
	})
	if err != nil {
		return nil, err
	}
	return &OriginalReadersV1{
		Attempts:         originalAttemptReaderV1{originalRecordReaderV1[domainpublication.PublicationAttemptV1]{records: bodies["attempts"], parse: domainpublication.ParsePublicationAttemptV1}},
		Receipts:         originalReceiptReaderV1{originalRecordReaderV1[domainpublication.PublicationReceiptV1]{records: bodies["receipts"], parse: domainpublication.ParsePublicationReceiptV1}},
		Commits:          originalCommitReaderV1{originalRecordReaderV1[domainpublication.PublicationCommitReceiptV1]{records: bodies["commit-receipts"], parse: domainpublication.ParsePublicationCommitReceiptV1}},
		Selections:       originalSelectionReaderV1{originalRecordReaderV1[domainpublication.PublicationCommitSelectionV1]{records: bodies["commit-selections"], parse: domainpublication.ParsePublicationCommitSelectionV1}},
		Decisions:        originalDecisionReaderV1{originalRecordReaderV1[domainpublication.ReportDeliveryDecisionV1]{records: bodies["delivery-decisions"], parse: domainpublication.ParseReportDeliveryDecisionV1}},
		GrantSettlements: originalGrantSettlementReaderV1{originalRecordReaderV1[domainpublication.ReportGrantSettlementV1]{records: bodies["grant-settlements"], parse: domainpublication.ParseReportGrantSettlementV1}},
		StageCompletions: originalStageCompletionReaderV1{originalRecordReaderV1[domainpublication.ReportStageCompletionV1]{records: bodies["stage-completions"], parse: domainpublication.ParseReportStageCompletionV1}},
		DeliveryOutcomes: originalDeliveryOutcomeReaderV1{originalRecordReaderV1[domainpublication.ReportDeliveryOutcomeV1]{records: bodies["delivery-projections"], parse: domainpublication.ParseReportDeliveryOutcomeV1}},
		Indexes:          originalIndexReaderV1{originalRecordReaderV1[domainpublication.PublicationIndexV1]{records: bodies["indexes"], parse: domainpublication.ParsePublicationIndexV1}},
		Ledgers:          originalLedgerReaderV1{originalRecordReaderV1[domainpublication.ClaimLedgerV1]{records: bodies["claim-ledgers"], parse: domainpublication.ParseClaimLedgerV1}},
		PIIProjections:   originalPIIProjectionReaderV1{originalRecordReaderV1[domainpublication.PIIProjectionV1]{records: bodies["pii-projections"], parse: domainpublication.ParsePIIProjectionV1}},
		Inspections:      originalInspectionReaderV1{originalRecordReaderV1[domainpublication.RenderInspectionV1]{records: bodies["render-inspections"], parse: domainpublication.ParseRenderInspectionV1}},
		Artifacts:        originalArtifactReaderV1{records: bodies["artifacts"]},
	}, nil
}

type originalRecordReaderV1[T any] struct {
	records map[string][]byte
	parse   func([]byte) (T, error)
}

func (reader originalRecordReaderV1[T]) Resolve(ctx context.Context, id string) (T, error) {
	var empty T
	if ctx == nil || reader.parse == nil || !domainsecurity.IsSHA256Hex(id) {
		return empty, errors.New("original report selector is invalid")
	}
	if err := context.Cause(ctx); err != nil {
		return empty, err
	}
	body, found := reader.records[id]
	if !found {
		return empty, publicationport.ErrNotFound
	}
	// Parsing on each read also detaches nested slices/maps from every caller.
	return reader.parse(body)
}

func (reader originalRecordReaderV1[T]) visit(ctx context.Context, visit func(T) error) error {
	if ctx == nil || visit == nil || reader.parse == nil {
		return errors.New("original report visitor is unavailable")
	}
	ids := make([]string, 0, len(reader.records))
	for id := range reader.records {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		record, err := reader.Resolve(ctx, id)
		if err != nil {
			return err
		}
		if err := visit(record); err != nil {
			return err
		}
	}
	return context.Cause(ctx)
}

type originalAttemptReaderV1 struct {
	originalRecordReaderV1[domainpublication.PublicationAttemptV1]
}

func (reader originalAttemptReaderV1) VisitAttempts(ctx context.Context, visit func(domainpublication.PublicationAttemptV1) error) error {
	return reader.visit(ctx, visit)
}

type originalReceiptReaderV1 struct {
	originalRecordReaderV1[domainpublication.PublicationReceiptV1]
}

func (reader originalReceiptReaderV1) VisitReceipts(ctx context.Context, visit func(domainpublication.PublicationReceiptV1) error) error {
	return reader.visit(ctx, visit)
}

type originalCommitReaderV1 struct {
	originalRecordReaderV1[domainpublication.PublicationCommitReceiptV1]
}

func (reader originalCommitReaderV1) VisitCommitReceipts(ctx context.Context, visit func(domainpublication.PublicationCommitReceiptV1) error) error {
	return reader.visit(ctx, visit)
}

type originalSelectionReaderV1 struct {
	originalRecordReaderV1[domainpublication.PublicationCommitSelectionV1]
}

func (reader originalSelectionReaderV1) VisitCommitSelections(ctx context.Context, visit func(domainpublication.PublicationCommitSelectionV1) error) error {
	return reader.visit(ctx, visit)
}

type originalDecisionReaderV1 struct {
	originalRecordReaderV1[domainpublication.ReportDeliveryDecisionV1]
}

func (reader originalDecisionReaderV1) VisitDeliveryDecisions(ctx context.Context, visit func(domainpublication.ReportDeliveryDecisionV1) error) error {
	return reader.visit(ctx, visit)
}

type originalGrantSettlementReaderV1 struct {
	originalRecordReaderV1[domainpublication.ReportGrantSettlementV1]
}

func (reader originalGrantSettlementReaderV1) VisitGrantSettlements(ctx context.Context, visit func(domainpublication.ReportGrantSettlementV1) error) error {
	return reader.visit(ctx, visit)
}

type originalStageCompletionReaderV1 struct {
	originalRecordReaderV1[domainpublication.ReportStageCompletionV1]
}

func (reader originalStageCompletionReaderV1) VisitStageCompletions(ctx context.Context, visit func(domainpublication.ReportStageCompletionV1) error) error {
	return reader.visit(ctx, visit)
}

type originalDeliveryOutcomeReaderV1 struct {
	originalRecordReaderV1[domainpublication.ReportDeliveryOutcomeV1]
}

func (reader originalDeliveryOutcomeReaderV1) VisitDeliveryOutcomes(ctx context.Context, visit func(domainpublication.ReportDeliveryOutcomeV1) error) error {
	return reader.visit(ctx, visit)
}

type originalIndexReaderV1 struct {
	originalRecordReaderV1[domainpublication.PublicationIndexV1]
}

func (reader originalIndexReaderV1) VisitIndexes(ctx context.Context, visit func(domainpublication.PublicationIndexV1) error) error {
	return reader.visit(ctx, visit)
}

type originalLedgerReaderV1 struct {
	originalRecordReaderV1[domainpublication.ClaimLedgerV1]
}

type originalPIIProjectionReaderV1 struct {
	originalRecordReaderV1[domainpublication.PIIProjectionV1]
}

type originalInspectionReaderV1 struct {
	originalRecordReaderV1[domainpublication.RenderInspectionV1]
}

func (reader originalDeliveryOutcomeReaderV1) ResolveOutcome(ctx context.Context, id string) (domainpublication.ReportDeliveryOutcomeV1, error) {
	return reader.Resolve(ctx, id)
}

type originalArtifactReaderV1 struct{ records map[string][]byte }

func (reader originalArtifactReaderV1) ResolveExact(ctx context.Context, id string) ([]byte, error) {
	if ctx == nil || !domainsecurity.IsSHA256Hex(id) {
		return nil, errors.New("original report artifact selector is invalid")
	}
	if err := context.Cause(ctx); err != nil {
		return nil, err
	}
	body, found := reader.records[id]
	if !found {
		return nil, publicationport.ErrNotFound
	}
	return append([]byte(nil), body...), nil
}
func (reader originalArtifactReaderV1) ResolveControlledMetadata(ctx context.Context, id string) (domainpii.ControlledPIIArtifactMetadataV1, error) {
	body, err := reader.ResolveExact(ctx, id)
	if err != nil {
		return domainpii.ControlledPIIArtifactMetadataV1{}, err
	}
	defer clear(body)
	return domainpii.ControlledPIIArtifactMetadataFromBytesV1(body)
}
