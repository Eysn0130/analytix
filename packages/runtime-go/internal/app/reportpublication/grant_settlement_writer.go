package reportpublication

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"reflect"
	"strings"

	pendingworkapp "analytix.local/runtime-go/internal/app/pendingwork"
	domainpendingwork "analytix.local/runtime-go/internal/domain/pendingwork"
	domainpublication "analytix.local/runtime-go/internal/domain/reportpublication"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domaintoolresult "analytix.local/runtime-go/internal/domain/toolresult"
	finalauthorityport "analytix.local/runtime-go/internal/ports/finalauthority"
	publicationport "analytix.local/runtime-go/internal/ports/reportpublication"
)

type GrantSettlementPendingAuthorityV1 interface {
	WithTrustedSettledReportStageForDecisionV1(
		context.Context,
		string,
		string,
		string,
		func(pendingworkapp.TrustedSettledReportStageV1) error,
	) error
}

type GrantSettlementWriterConfigV1 struct {
	InstallationID string
	EnrollmentID   string
	Authority      finalauthorityport.Authority
	Pending        GrantSettlementPendingAuthorityV1
	Decisions      publicationport.DeliveryDecisionResolver
	Settlements    publicationport.ReportGrantSettlementStore
}

// GrantSettlementWriterV1 is the only decision -> grant-settlement writer.
// It cannot close pending work or publish a report; it merely binds one exact
// host-admitted durable tool result to the already-signed delivery decision.
type GrantSettlementWriterV1 struct {
	installationID string
	enrollmentID   string
	authority      finalauthorityport.Authority
	authorityKeyID string
	authorityKey   []byte
	pending        GrantSettlementPendingAuthorityV1
	decisions      publicationport.DeliveryDecisionResolver
	settlements    publicationport.ReportGrantSettlementStore
}

func NewGrantSettlementWriterV1(config GrantSettlementWriterConfigV1) (*GrantSettlementWriterV1, error) {
	publicKey := []byte(nil)
	keyID := ""
	if config.Authority != nil {
		publicKey = config.Authority.PublicKey()
		keyID = strings.TrimSpace(config.Authority.KeyID())
	}
	if !domainsecurity.IsSHA256Hex(strings.TrimSpace(config.InstallationID)) ||
		!domainsecurity.IsSHA256Hex(strings.TrimSpace(config.EnrollmentID)) || config.Authority == nil ||
		len(publicKey) != ed25519.PublicKeySize || keyID != domainsecurity.SHA256Hex(publicKey) ||
		config.Pending == nil || config.Decisions == nil || config.Settlements == nil {
		return nil, ErrPublicationUnavailable
	}
	return &GrantSettlementWriterV1{
		installationID: strings.TrimSpace(config.InstallationID), enrollmentID: strings.TrimSpace(config.EnrollmentID),
		authority: config.Authority, authorityKeyID: keyID, authorityKey: append([]byte(nil), publicKey...),
		pending: config.Pending, decisions: config.Decisions, settlements: config.Settlements,
	}, nil
}

func (writer *GrantSettlementWriterV1) SettleDecisionV1(
	ctx context.Context,
	decision domainpublication.ReportDeliveryDecisionV1,
) (domainpublication.ReportGrantSettlementV1, error) {
	if writer == nil || ctx == nil || writer.authority == nil || writer.pending == nil || writer.decisions == nil ||
		writer.settlements == nil || len(writer.authorityKey) != ed25519.PublicKeySize {
		return domainpublication.ReportGrantSettlementV1{}, ErrPublicationUnavailable
	}
	if err := ctx.Err(); err != nil {
		return domainpublication.ReportGrantSettlementV1{}, err
	}
	keyID, publicKey, signature, err := domainpublication.ReportDeliveryDecisionAuthorityMaterialV1(decision)
	if err != nil || decision.InstallationID != writer.installationID || decision.EnrollmentID != writer.enrollmentID ||
		keyID != writer.authorityKeyID || !bytes.Equal(publicKey, writer.authorityKey) ||
		writer.authority.VerifyTrusted(
			ctx, keyID, publicKey, domainpublication.ReportDeliveryDecisionSigningBytesV1(decision), signature,
		) != nil {
		return domainpublication.ReportGrantSettlementV1{}, ErrPublicationIntegrity
	}
	registered, err := writer.decisions.Resolve(ctx, decision.DecisionID)
	if err != nil {
		if ctx.Err() != nil {
			return domainpublication.ReportGrantSettlementV1{}, ctx.Err()
		}
		if errors.Is(err, publicationport.ErrNotFound) {
			return domainpublication.ReportGrantSettlementV1{}, errors.Join(ErrPublicationIntegrity, err)
		}
		return domainpublication.ReportGrantSettlementV1{}, errors.Join(ErrPublicationRestartUnresolved, err)
	}
	if !reflect.DeepEqual(registered, decision) {
		return domainpublication.ReportGrantSettlementV1{}, ErrPublicationIntegrity
	}
	var stored domainpublication.ReportGrantSettlementV1
	err = writer.pending.WithTrustedSettledReportStageForDecisionV1(
		ctx, decision.ReportStageWorkID, decision.DecisionID, decision.RecordDigest,
		func(settled pendingworkapp.TrustedSettledReportStageV1) error {
			if writer.validateSettledStageForDecisionV1(ctx, settled, decision) != nil {
				return ErrPublicationIntegrity
			}
			input := domainpublication.ReportGrantSettlementInputV1{
				Decision: decision, Grant: settled.Grant,
				ActiveRegistry: settled.ActiveRegistry, SettledRegistry: settled.SettledRegistry,
				ResultItemID: settled.ResultItemID, ResultItemDigest: settled.ResultItemDigest, SettledAt: settled.SettledAt,
				AuthorityKeyID: writer.authorityKeyID, AuthorityPublicKey: writer.authorityKey,
			}
			settlement, createErr := domainpublication.NewReportGrantSettlementV1(
				input, func(message []byte) ([]byte, error) { return writer.authority.Sign(ctx, message) },
			)
			if createErr != nil {
				return errors.Join(ErrPublicationIntegrity, createErr)
			}
			_, createErr = writer.settlements.CreateExclusive(ctx, settlement)
			readbackCtx, cancel := detachedPublicationEffectReadbackContextV1(ctx)
			defer cancel()
			readback, resolveErr := writer.settlements.Resolve(readbackCtx, settlement.SettlementID)
			if resolveErr != nil {
				return errors.Join(ErrPublicationRestartUnresolved, createErr, resolveErr)
			}
			if !reflect.DeepEqual(readback, settlement) ||
				domainpublication.ValidateReportGrantSettlementGraphV1(readback, input) != nil {
				return ErrPublicationIntegrity
			}
			stored = readback
			return nil
		},
	)
	if err != nil {
		if ctx.Err() != nil && !errors.Is(err, ErrPublicationRestartUnresolved) && !errors.Is(err, ErrPublicationIntegrity) {
			return domainpublication.ReportGrantSettlementV1{}, ctx.Err()
		}
		if errors.Is(err, ErrPublicationIntegrity) || errors.Is(err, ErrPublicationRestartUnresolved) {
			return domainpublication.ReportGrantSettlementV1{}, err
		}
		if errors.Is(err, pendingworkapp.ErrGrantAuthority) || errors.Is(err, pendingworkapp.ErrOperationMismatch) ||
			errors.Is(err, pendingworkapp.ErrWorkClosed) {
			return domainpublication.ReportGrantSettlementV1{}, errors.Join(ErrPublicationIntegrity, err)
		}
		return domainpublication.ReportGrantSettlementV1{}, errors.Join(ErrPublicationRestartUnresolved, err)
	}
	return stored, nil
}

func (writer *GrantSettlementWriterV1) validateSettledStageForDecisionV1(
	ctx context.Context,
	settled pendingworkapp.TrustedSettledReportStageV1,
	decision domainpublication.ReportDeliveryDecisionV1,
) error {
	expectedContext, err := domainpendingwork.ContextBindingFromSecurityContextV1(decision.Context)
	receiptBody, receiptBodyErr := domainpendingwork.PendingWorkReceiptV1Bytes(settled.Receipt)
	receiptKeyID, receiptPublicKey, receiptSignature, receiptAuthorityErr :=
		domainpendingwork.PendingWorkReceiptV1AuthorityMaterial(settled.Receipt)
	resultBody, resultBodyErr := json.Marshal(settled.ResultItem)
	activeEntry, activeEntryFound := domainsecurity.ExecutionGrantRegistryEntryByID(settled.ActiveRegistry, decision.GrantID)
	resultEpoch, resultEpochOK := reportResultItemUint64V1(settled.ResultItem["contextEpoch"])
	if err != nil || receiptBodyErr != nil || receiptAuthorityErr != nil || resultBodyErr != nil ||
		settled.Receipt.Context != expectedContext || settled.Receipt.WorkID != decision.ReportStageWorkID ||
		settled.Receipt.ReceiptID != decision.ReportStageReceiptID || len(settled.Receipt.GrantMembers) != 1 ||
		domainsecurity.SHA256Hex(receiptBody) != decision.ReportStageReceiptSHA256 ||
		receiptKeyID != writer.authorityKeyID || !bytes.Equal(receiptPublicKey, writer.authorityKey) ||
		writer.authority.VerifyTrusted(
			ctx, receiptKeyID, receiptPublicKey, domainpendingwork.PendingWorkReceiptV1SigningBytes(settled.Receipt), receiptSignature,
		) != nil ||
		settled.Receipt.GrantRegistrySequence != decision.GrantRegistrySequence ||
		settled.Receipt.GrantRegistryDigest != decision.GrantRegistryDigest ||
		settled.Receipt.GrantMembers[0].GrantID != decision.GrantID ||
		settled.Receipt.GrantMembers[0].RegistryEntryDigest != decision.GrantRegistryEntryDigest ||
		!activeEntryFound || activeEntry.Sequence != settled.Receipt.GrantMembers[0].RegistrySequence ||
		activeEntry.EntryDigest != settled.Receipt.GrantMembers[0].RegistryEntryDigest ||
		settled.Grant.GrantID != decision.GrantID || settled.Grant.ToolCallID != decision.ToolCallID ||
		settled.Grant.ToolName != decision.ToolName || settled.ResultItemID == "" ||
		!domainsecurity.IsSHA256Hex(settled.ResultItemDigest) ||
		domainsecurity.CanonicalJSONHash(resultBody) != settled.ResultItemDigest ||
		domaintoolresult.ValidatePrivateAdmittedReportResultItemV1(
			settled.ResultItem, decision.DecisionID, decision.RecordDigest,
		) != nil ||
		reportResultItemStringV1(settled.ResultItem, "id") != settled.ResultItemID ||
		reportResultItemStringV1(settled.ResultItem, "threadId") != decision.Context.ThreadID ||
		reportResultItemStringV1(settled.ResultItem, "turnId") != decision.Context.TurnID ||
		reportResultItemStringV1(settled.ResultItem, "contextDigest") != decision.Context.ContextDigest ||
		reportResultItemStringV1(settled.ResultItem, "executionGrantId") != decision.GrantID ||
		reportResultItemStringV1(settled.ResultItem, "callId") != decision.ToolCallID ||
		!resultEpochOK || resultEpoch != decision.Context.ContextEpoch || settled.SettledAt.IsZero() {
		return ErrPublicationIntegrity
	}
	return nil
}

func reportResultItemStringV1(item map[string]any, key string) string {
	value, _ := item[key].(string)
	return strings.TrimSpace(value)
}

func reportResultItemUint64V1(value any) (uint64, bool) {
	switch typed := value.(type) {
	case uint64:
		return typed, typed > 0
	case uint:
		return uint64(typed), typed > 0
	case int:
		if typed > 0 {
			return uint64(typed), true
		}
	case int64:
		if typed > 0 {
			return uint64(typed), true
		}
	case float64:
		if typed <= 0 || typed > 9007199254740991 {
			return 0, false
		}
		converted := uint64(typed)
		return converted, typed == float64(converted)
	default:
		return 0, false
	}
	return 0, false
}
