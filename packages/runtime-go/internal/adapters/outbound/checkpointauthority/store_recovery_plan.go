package checkpointauthority

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"reflect"
	"time"

	finalauthority "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	persistencefs "analytix.local/runtime-go/internal/adapters/outbound/persistencefs"
	domaincheckpoint "analytix.local/runtime-go/internal/domain/checkpointauthority"
	checkpointport "analytix.local/runtime-go/internal/ports/checkpointauthority"
)

var checkpointRecoveryLeafNamesV1 = []finalauthority.SecurePrivateCASOwnerLeafV1{
	{Name: "snapshot-intents", MaxBytes: maxCheckpointAuthorityCASBytes},
	{Name: "snapshot-completions", MaxBytes: maxCheckpointAuthorityCASBytes},
	{Name: "snapshot-dispositions", MaxBytes: maxCheckpointAuthorityCASBytes},
	{Name: "operation-group-intents-v2", MaxBytes: domaincheckpoint.MaxOperationGroupIntentRecordBytes},
	{Name: "operation-group-terminals-v2", MaxBytes: maxCheckpointAuthorityCASBytes},
}

type PreparedRecoveryV1 struct {
	current   *finalauthority.PreparedSecurePrivateCASOwnerRecoveryV1
	legacy    *preparedLegacyCheckpointRecoveryV1
	validated bool
}

type preparedLegacyCheckpointRecoveryV1 struct {
	checkpointRoot string
	dataDir        string
	quarantineRoot string
	auditRoot      string
	access         finalauthority.SecurePrivateCASRecoveryAccessAuthority
	topology       *finalauthority.PreparedSecurePrivateCASOwnerTopologyV1
	audit          *finalauthority.PreparedSecurePrivateCASRecoveryV1
	state          persistencefs.LegacyCheckpointSnapshotQuarantineStateV1
}

func PrepareRecoveryV1(
	ctx context.Context,
	root string,
	access finalauthority.SecurePrivateCASRecoveryAccessAuthority,
) (*PreparedRecoveryV1, error) {
	absolute, err := checkpointAuthorityRoot(root, access)
	if err != nil {
		return nil, err
	}
	current, err := finalauthority.PrepareSecurePrivateCASOwnerRecoveryV1(
		ctx, absolute, checkpointRecoveryLeafNamesV1, access,
	)
	if err != nil {
		return nil, errors.Join(checkpointport.ErrCorrupt, err)
	}
	legacy, err := prepareLegacyCheckpointRecoveryV1(ctx, absolute, access)
	if err != nil {
		return nil, err
	}
	prepared := &PreparedRecoveryV1{current: current, legacy: legacy}
	if err := prepared.Revalidate(ctx); err != nil {
		return nil, err
	}
	return prepared, nil
}

func (prepared *PreparedRecoveryV1) ValidateSemantics(ctx context.Context) error {
	if prepared == nil || prepared.current == nil || prepared.legacy == nil {
		return errors.New("checkpoint authority recovery plan is invalid")
	}
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return err
		}
	}
	if err := validatePreparedCurrentRecoveryV1(ctx, prepared.current); err != nil {
		return err
	}
	if err := validatePreparedLegacyCheckpointRecoveryV1(
		ctx, prepared.legacy.state, prepared.legacy.audit,
	); err != nil {
		return err
	}
	prepared.validated = true
	return nil
}

func validatePreparedCurrentRecoveryV1(
	ctx context.Context,
	owner *finalauthority.PreparedSecurePrivateCASOwnerRecoveryV1,
) error {
	if owner == nil || !owner.Present() {
		return nil
	}
	intents := make(map[string]domaincheckpoint.SnapshotIntentV1)
	if err := owner.VisitCommittedFiles(ctx, "snapshot-intents", func(file finalauthority.SecurePrivateCASFile) error {
		intent, err := domaincheckpoint.ParseSnapshotIntentV1(file.Body)
		canonical, canonicalErr := domaincheckpoint.SnapshotIntentV1Bytes(intent)
		if err != nil || canonicalErr != nil || intent.SnapshotIntentID != file.Digest || !bytes.Equal(canonical, file.Body) {
			return errors.Join(checkpointport.ErrCorrupt, errors.New("checkpoint prepared snapshot intent is non-canonical or content-address corrupt"), err, canonicalErr)
		}
		intents[file.Digest] = intent
		return nil
	}); err != nil {
		return err
	}
	completed := make(map[string]struct{})
	if err := owner.VisitCommittedFiles(ctx, "snapshot-completions", func(file finalauthority.SecurePrivateCASFile) error {
		intent, ok := intents[file.Digest]
		if !ok {
			return errors.Join(checkpointport.ErrCorrupt, errors.New("orphan checkpoint prepared snapshot completion"))
		}
		completion, err := domaincheckpoint.ParseSnapshotCompletionV1(file.Body, intent)
		canonical, canonicalErr := domaincheckpoint.SnapshotCompletionV1Bytes(completion, intent)
		if err != nil || canonicalErr != nil || !bytes.Equal(canonical, file.Body) {
			return errors.Join(checkpointport.ErrCorrupt, errors.New("checkpoint prepared snapshot completion is non-canonical or invalid"), err, canonicalErr)
		}
		completed[file.Digest] = struct{}{}
		return nil
	}); err != nil {
		return err
	}
	if err := owner.VisitCommittedFiles(ctx, "snapshot-dispositions", func(file finalauthority.SecurePrivateCASFile) error {
		intent, ok := intents[file.Digest]
		if !ok {
			return errors.Join(checkpointport.ErrCorrupt, errors.New("orphan checkpoint prepared snapshot disposition"))
		}
		if _, conflict := completed[file.Digest]; conflict {
			return errors.Join(checkpointport.ErrCorrupt, errors.New("checkpoint prepared snapshot has conflicting terminal records"))
		}
		disposition, err := domaincheckpoint.ParseSnapshotDispositionV1(file.Body, intent)
		canonical, canonicalErr := domaincheckpoint.SnapshotDispositionV1Bytes(disposition, intent)
		if err != nil || canonicalErr != nil || !bytes.Equal(canonical, file.Body) {
			return errors.Join(checkpointport.ErrCorrupt, errors.New("checkpoint prepared snapshot disposition is non-canonical or invalid"), err, canonicalErr)
		}
		return nil
	}); err != nil {
		return err
	}
	operationIntents := make(map[string]domaincheckpoint.OperationGroupIntentV2)
	if err := owner.VisitCommittedFiles(ctx, "operation-group-intents-v2", func(file finalauthority.SecurePrivateCASFile) error {
		intent, err := domaincheckpoint.ParseOperationGroupIntentV2(file.Body)
		canonical, canonicalErr := domaincheckpoint.OperationGroupIntentV2Bytes(intent)
		if err != nil || canonicalErr != nil || intent.OperationGroupID != file.Digest || !bytes.Equal(canonical, file.Body) {
			return errors.Join(checkpointport.ErrCorrupt, errors.New("checkpoint prepared operation intent is non-canonical or content-address corrupt"), err, canonicalErr)
		}
		operationIntents[file.Digest] = intent
		return nil
	}); err != nil {
		return err
	}
	return owner.VisitCommittedFiles(ctx, "operation-group-terminals-v2", func(file finalauthority.SecurePrivateCASFile) error {
		intent, ok := operationIntents[file.Digest]
		if !ok {
			return errors.Join(checkpointport.ErrCorrupt, errors.New("orphan checkpoint prepared operation terminal"))
		}
		terminal, err := domaincheckpoint.ParseOperationGroupTerminalV2(file.Body, intent)
		canonical, canonicalErr := domaincheckpoint.OperationGroupTerminalV2Bytes(terminal, intent)
		if err != nil || canonicalErr != nil || terminal.OperationGroupID != file.Digest || !bytes.Equal(canonical, file.Body) {
			return errors.Join(checkpointport.ErrCorrupt, errors.New("checkpoint prepared operation terminal is non-canonical or invalid"), err, canonicalErr)
		}
		return nil
	})
}

func prepareLegacyCheckpointRecoveryV1(
	ctx context.Context,
	checkpointRoot string,
	access finalauthority.SecurePrivateCASRecoveryAccessAuthority,
) (*preparedLegacyCheckpointRecoveryV1, error) {
	dataDir, err := legacyCheckpointSnapshotDataDir(checkpointRoot)
	if err != nil {
		return nil, err
	}
	quarantineRoot, err := persistencefs.LegacyCheckpointSnapshotQuarantineRootV1(dataDir)
	if err != nil {
		return nil, err
	}
	auditRoot, err := persistencefs.LegacyCheckpointSnapshotAuditRootV1(dataDir)
	if err != nil {
		return nil, err
	}
	state, err := persistencefs.InspectLegacyCheckpointSnapshotQuarantineV1(ctx, dataDir, access)
	if err != nil {
		return nil, err
	}
	topology, err := finalauthority.PrepareSecurePrivateCASOwnerTopologyV1(
		ctx, quarantineRoot, legacyCheckpointTopologyEntriesV1(state), access,
	)
	if err != nil {
		return nil, err
	}
	audit, err := finalauthority.PrepareSecurePrivateCASRecoveryIfPresent(
		ctx, auditRoot, maxLegacyCheckpointSnapshotAuditBytesV1, access,
	)
	if err != nil {
		return nil, err
	}
	confirmed, err := persistencefs.InspectLegacyCheckpointSnapshotQuarantineV1(ctx, dataDir, access)
	if err != nil || !reflect.DeepEqual(confirmed, state) {
		return nil, errors.Join(errors.New("legacy checkpoint quarantine changed during preparation"), err)
	}
	if err := validateLegacyCheckpointTopologyMembershipV1(topology, audit, confirmed); err != nil {
		return nil, errors.Join(errors.New("legacy checkpoint prepared audit topology is inconsistent"), err)
	}
	prepared := &preparedLegacyCheckpointRecoveryV1{
		checkpointRoot: checkpointRoot, dataDir: dataDir, quarantineRoot: quarantineRoot, auditRoot: auditRoot,
		access: access, topology: topology, audit: audit, state: confirmed,
	}
	if err := prepared.Revalidate(ctx); err != nil {
		return nil, err
	}
	return prepared, nil
}

func legacyCheckpointTopologyEntriesV1(state persistencefs.LegacyCheckpointSnapshotQuarantineStateV1) []string {
	entries := make([]string, 0, 2)
	if state.AuditRecordsPresent {
		entries = append(entries, persistencefs.LegacyCheckpointSnapshotAuditRecordsV1)
	}
	if state.PayloadsPresent {
		entries = append(entries, persistencefs.LegacyCheckpointSnapshotPayloadsV1)
	}
	return entries
}

func validateLegacyCheckpointTopologyMembershipV1(
	topology *finalauthority.PreparedSecurePrivateCASOwnerTopologyV1,
	audit *finalauthority.PreparedSecurePrivateCASRecoveryV1,
	state persistencefs.LegacyCheckpointSnapshotQuarantineStateV1,
) error {
	if topology == nil || audit == nil ||
		topology.PresentV1() != state.QuarantinePresent ||
		topology.ContainsEntryV1(persistencefs.LegacyCheckpointSnapshotAuditRecordsV1) != state.AuditRecordsPresent ||
		topology.ContainsEntryV1(persistencefs.LegacyCheckpointSnapshotPayloadsV1) != state.PayloadsPresent ||
		audit.Present() != state.AuditRecordsPresent {
		return errors.New("legacy checkpoint prepared quarantine membership is inconsistent")
	}
	return nil
}

func validatePreparedLegacyCheckpointRecoveryV1(
	ctx context.Context,
	state persistencefs.LegacyCheckpointSnapshotQuarantineStateV1,
	auditPlan *finalauthority.PreparedSecurePrivateCASRecoveryV1,
) error {
	if err := preflightLegacyCheckpointSnapshotQuarantineState(state); err != nil {
		return err
	}
	if auditPlan == nil {
		return errors.New("legacy checkpoint recovery plan is invalid")
	}
	var (
		audit      legacyCheckpointSnapshotAuditV1
		auditCount int
	)
	if err := auditPlan.VisitCommittedFiles(ctx, func(file finalauthority.SecurePrivateCASFile) error {
		auditCount++
		if auditCount > 1 {
			return errors.New("legacy checkpoint prepared audit inventory is ambiguous")
		}
		parsed, err := parseLegacyCheckpointSnapshotAuditFileV1(file)
		if err != nil {
			return err
		}
		audit = parsed
		return nil
	}); err != nil {
		return err
	}
	if state.SourcePresent {
		if auditCount != 0 {
			return errors.New("legacy checkpoint prepared source has an inconsistent quarantine audit")
		}
		return nil
	}
	if state.Payload == nil {
		if auditCount != 0 {
			return errors.New("legacy checkpoint prepared audit has no payload")
		}
		return nil
	}
	if auditCount == 1 {
		return validateLegacyCheckpointSnapshotAudit(audit, *state.Payload)
	}
	return nil
}

func parseLegacyCheckpointSnapshotAuditFileV1(file finalauthority.SecurePrivateCASFile) (legacyCheckpointSnapshotAuditV1, error) {
	var audit legacyCheckpointSnapshotAuditV1
	decoder := jsonNewStrictDecoder(file.Body)
	if err := decoder.Decode(&audit); err != nil || file.Digest != audit.AuditDigest {
		return legacyCheckpointSnapshotAuditV1{}, errors.New("legacy checkpoint prepared audit record is invalid")
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return legacyCheckpointSnapshotAuditV1{}, errors.New("legacy checkpoint prepared audit record has trailing content")
	}
	canonical, err := legacyCheckpointSnapshotAuditBytes(audit)
	if err != nil || !bytes.Equal(canonical, file.Body) {
		return legacyCheckpointSnapshotAuditV1{}, errors.New("legacy checkpoint prepared audit record is non-canonical")
	}
	return audit, nil
}

func (prepared *preparedLegacyCheckpointRecoveryV1) Revalidate(ctx context.Context) error {
	if prepared == nil || prepared.topology == nil || prepared.audit == nil || prepared.access == nil {
		return errors.New("legacy checkpoint recovery plan is invalid")
	}
	if prepared.topology.PrivateCASRecoveryTopologyRootV3() != prepared.quarantineRoot {
		return errors.New("legacy checkpoint quarantine topology root changed")
	}
	if err := prepared.topology.RevalidatePrivateCASRecoveryTopologyV3(ctx); err != nil {
		return err
	}
	if err := prepared.audit.Revalidate(ctx); err != nil {
		return err
	}
	state, err := persistencefs.InspectLegacyCheckpointSnapshotQuarantineV1(ctx, prepared.dataDir, prepared.access)
	if err != nil || !reflect.DeepEqual(state, prepared.state) {
		return errors.Join(errors.New("legacy checkpoint quarantine changed after preparation"), err)
	}
	if err := validateLegacyCheckpointTopologyMembershipV1(prepared.topology, prepared.audit, state); err != nil {
		return err
	}
	if err := prepared.audit.Revalidate(ctx); err != nil {
		return err
	}
	return prepared.topology.RevalidatePrivateCASRecoveryTopologyV3(ctx)
}

func (prepared *preparedLegacyCheckpointRecoveryV1) applyBeforePrivateCASRecoveryV2(ctx context.Context) (bool, error) {
	if prepared == nil || prepared.topology == nil || prepared.audit == nil || prepared.access == nil {
		return false, errors.New("legacy checkpoint recovery plan is invalid")
	}
	if err := prepared.topology.RevalidatePrivateCASRecoveryTopologyV3(ctx); err != nil {
		return false, err
	}
	state, err := persistencefs.InspectLegacyCheckpointSnapshotQuarantineV1(ctx, prepared.dataDir, prepared.access)
	if err != nil || !reflect.DeepEqual(state, prepared.state) {
		return false, errors.Join(errors.New("legacy checkpoint quarantine changed before prepared mutation"), err)
	}
	if err := validateLegacyCheckpointTopologyMembershipV1(prepared.topology, prepared.audit, state); err != nil {
		return false, err
	}
	if err := prepared.topology.RevalidatePrivateCASRecoveryTopologyV3(ctx); err != nil {
		return false, err
	}
	if !state.SourcePresent && state.Payload == nil {
		return false, nil
	}
	changed := false
	var auditCAS *finalauthority.SecurePrivateCAS
	if state.SourcePresent {
		auditCAS, err = finalauthority.OpenSecurePrivateCASWithAccessAuthorityContext(
			ctx, prepared.auditRoot, maxLegacyCheckpointSnapshotAuditBytesV1, prepared.access,
		)
		if err != nil {
			return false, err
		}
		targetName, err := newLegacyCheckpointSnapshotPayloadName(state.Source.SHA256)
		if err != nil {
			return false, err
		}
		moved, err := persistencefs.MoveLegacyCheckpointSnapshotsToQuarantineV1(
			ctx, prepared.dataDir, targetName, state.Source, prepared.access,
		)
		if err != nil {
			return false, err
		}
		changed = true
		state.SourcePresent = false
		state.Source = persistencefs.LegacyCheckpointSnapshotTreeV1{}
		state.Payload = &persistencefs.LegacyCheckpointSnapshotPayloadV1{Name: targetName, Tree: moved}
	}
	if auditCAS == nil {
		auditCAS, err = finalauthority.OpenSecurePrivateCASWithAccessAuthorityContext(
			ctx, prepared.auditRoot, maxLegacyCheckpointSnapshotAuditBytesV1, prepared.access,
		)
		if err != nil {
			return false, err
		}
	}
	audits, err := loadLegacyCheckpointSnapshotAudits(ctx, auditCAS)
	if err != nil {
		return false, err
	}
	if len(audits) == 0 {
		// Re-preparation is required even when PutIfAbsent reports that a
		// concurrent writer won: the frozen empty audit inventory changed.
		changed = true
		audit, body, err := newLegacyCheckpointSnapshotAudit(*state.Payload, time.Now().UTC())
		if err != nil {
			return false, err
		}
		if err := auditCAS.PutIfAbsent(ctx, audit.AuditDigest, body); err != nil && !errors.Is(err, os.ErrExist) {
			return false, err
		}
		audits, err = loadLegacyCheckpointSnapshotAudits(ctx, auditCAS)
		if err != nil {
			return false, err
		}
	}
	if len(audits) != 1 {
		return false, errors.New("legacy checkpoint prepared quarantine audit inventory is ambiguous")
	}
	return changed, validateLegacyCheckpointSnapshotAudit(audits[0], *state.Payload)
}

func (prepared *PreparedRecoveryV1) Revalidate(ctx context.Context) error {
	if prepared == nil || prepared.current == nil || prepared.legacy == nil {
		return errors.New("checkpoint authority recovery plan is invalid")
	}
	if err := prepared.current.Revalidate(ctx); err != nil {
		return err
	}
	if err := prepared.legacy.Revalidate(ctx); err != nil {
		return err
	}
	return prepared.current.Revalidate(ctx)
}

func (prepared *PreparedRecoveryV1) PrivateCASRecoveryTopologiesV3() []finalauthority.SecurePrivateCASRecoveryTopologyAuthorityV3 {
	if prepared == nil || prepared.current == nil || prepared.legacy == nil || prepared.legacy.topology == nil {
		return nil
	}
	return []finalauthority.SecurePrivateCASRecoveryTopologyAuthorityV3{prepared.current, prepared.legacy.topology}
}

// PrivateCASRecoveryAdditionalSemanticDigestV4 binds the non-CAS legacy
// quarantine payload inventory approved by the checkpoint semantic validator.
// A different but syntactically valid payload therefore cannot consume an
// older signed cleanup witness after restart.
func (prepared *PreparedRecoveryV1) PrivateCASRecoveryAdditionalSemanticDigestV4() string {
	if prepared == nil || prepared.legacy == nil || !prepared.validated {
		return ""
	}
	body, err := json.Marshal(prepared.legacy.state)
	if err != nil {
		return ""
	}
	digest := sha256.New()
	_, _ = digest.Write([]byte("analytix.checkpoint-private-cas-recovery-semantic/v4\x00"))
	_, _ = digest.Write(body)
	return hex.EncodeToString(digest.Sum(nil))
}

func (prepared *PreparedRecoveryV1) SecurePrivateCASRecoveryPlansV2() []*finalauthority.PreparedSecurePrivateCASRecoveryV1 {
	if prepared == nil || prepared.current == nil || prepared.legacy == nil || prepared.legacy.audit == nil {
		return nil
	}
	plans := prepared.current.SecurePrivateCASRecoveryPlansV2()
	return append(plans, prepared.legacy.audit)
}

// ApplyLegacyCheckpointQuarantineMigrationV1 performs the sole pre-V4
// namespace migration and reports whether the frozen owner inventory changed.
// Runtime startup re-prepares every owner only after a true result.
func (prepared *PreparedRecoveryV1) ApplyLegacyCheckpointQuarantineMigrationV1(ctx context.Context) (bool, error) {
	if prepared == nil || prepared.current == nil || prepared.legacy == nil || !prepared.validated {
		return false, errors.New("checkpoint authority recovery plan has not passed semantic validation")
	}
	return prepared.legacy.applyBeforePrivateCASRecoveryV2(ctx)
}

// jsonNewStrictDecoder is kept local so recovery parsing stays byte-for-byte
// equivalent to the normal legacy audit loader without opening a CAS store.
func jsonNewStrictDecoder(body []byte) *json.Decoder {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	return decoder
}
