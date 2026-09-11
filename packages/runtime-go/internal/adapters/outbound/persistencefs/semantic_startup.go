package persistencefs

import (
	privatecasport "analytix.local/runtime-go/internal/ports/privatecas"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainstartup "analytix.local/runtime-go/internal/domain/startup"
	startupport "analytix.local/runtime-go/internal/ports/startup"
)

const (
	semanticJournalSchemaVersion    = 4
	semanticPlanningMarkerVersion   = 2
	semanticPlanningMarkerPurpose   = "analytix.semantic-startup-planning/v2"
	semanticPlanningMarkerV1Purpose = "analytix.semantic-startup-planning/v1"
	semanticJournalPreparing        = "preparing"
	semanticJournalPrepared         = "prepared"
	semanticJournalApplying         = "applying"
	semanticJournalCommitted        = "committed"
	semanticJournalTempPrefix       = ".startup-replace-"
	semanticJournalTempSuffix       = ".tmp"
	maxSemanticJournalBytes         = 8 << 20
	maxSemanticPlanningBytes        = 64 << 10
)

var authenticatedSemanticRecoveryEmptyConfigurationDigest = domainsecurity.SHA256Hex(
	[]byte("analytix.semantic-startup-authenticated-recovery/empty-v1"),
)

type SemanticPlanBuilder struct {
	roots            RootSet
	rootAuthority    *RootAuthority
	journalAuthority *JournalNamespaceAuthority
	authorityErr     error
	mu               sync.Mutex
	fault            func(string, int) error
	restartPreserved SemanticRestartPreservationV1
	originalCreates  privatecasport.OriginalCreateResiduesV1
}

type preparedSemanticPlan struct {
	builder    *SemanticPlanBuilder
	plan       domainstartup.SemanticStartupPlanV1
	baseline   domainstartup.ReadOnlyStartupBaselineV1
	stageRoot  string
	stageRoots RootSet
	stageID    string
	planningID string
	closed     bool
	mu         sync.Mutex
}

type semanticJournalV1 struct {
	SchemaVersion        int                                 `json:"schemaVersion"`
	State                string                              `json:"state"`
	RootBindingDigest    string                              `json:"rootBindingDigest"`
	RootCapabilityDigest string                              `json:"rootCapabilityDigest"`
	RootCapabilities     []frozenRootCapability              `json:"rootCapabilities"`
	RootPromotionIntents []rootPromotionIntentV1             `json:"rootPromotionIntents,omitempty"`
	Plan                 domainstartup.SemanticStartupPlanV1 `json:"plan"`
	NextOperation        int                                 `json:"nextOperation"`
	JournalDigest        string                              `json:"journalDigest"`
	JournalRootIdentity  string                              `json:"journalRootIdentity"`
	JournalStageIdentity string                              `json:"journalStageIdentity,omitempty"`
	AuthorityKeyID       string                              `json:"authorityKeyId"`
	AuthoritySignature   string                              `json:"authoritySignature"`
}

type semanticPlanningMarkerV1 struct {
	SchemaVersion     int    `json:"schemaVersion"`
	Purpose           string `json:"purpose"`
	RootBindingDigest string `json:"rootBindingDigest"`
	PlanningIdentity  string `json:"planningIdentity"`
	StageName         string `json:"stageName"`
	StageIdentity     string `json:"stageIdentity"`
	DataIdentity      string `json:"dataIdentity"`
	DurableIdentity   string `json:"durableIdentity"`
	MarkerDigest      string `json:"markerDigest"`
}

type rootPromotionIntentV1 struct {
	Root           string `json:"root"`
	Anchor         string `json:"anchor"`
	AnchorIdentity string `json:"anchorIdentity"`
	MissingSuffix  string `json:"missingSuffix"`
	IntentDigest   string `json:"intentDigest"`
}

func NewSemanticPlanBuilder(roots RootSet) *SemanticPlanBuilder {
	authority, err := FreezeRootAuthority(roots)
	journalAuthority, journalErr := FreezeJournalNamespaceAuthorityForRoots(roots)
	return &SemanticPlanBuilder{roots: roots, rootAuthority: authority, journalAuthority: journalAuthority, authorityErr: errors.Join(err, journalErr)}
}

func NewSemanticPlanBuilderWithRootAuthority(roots RootSet, authority *RootAuthority) *SemanticPlanBuilder {
	journalAuthority, err := FreezeJournalNamespaceAuthorityForRoots(roots)
	builder := &SemanticPlanBuilder{roots: roots, rootAuthority: authority, journalAuthority: journalAuthority, authorityErr: err}
	if authorityRoots, ok := authority.Roots(); !ok || authorityRoots != roots {
		builder.authorityErr = errors.Join(builder.authorityErr, errors.New("semantic startup root authority does not match configured roots"))
	}
	return builder
}

func NewSemanticPlanBuilderWithAuthorities(roots RootSet, authority *RootAuthority, journalAuthority *JournalNamespaceAuthority) *SemanticPlanBuilder {
	builder := &SemanticPlanBuilder{roots: roots, rootAuthority: authority, journalAuthority: journalAuthority}
	if authorityRoots, ok := authority.Roots(); !ok || authorityRoots != roots {
		builder.authorityErr = errors.New("semantic startup root authority does not match configured roots")
	}
	if err := journalAuthority.Validate(); err != nil {
		builder.authorityErr = errors.Join(builder.authorityErr, err)
	}
	if !journalAuthority.matchesRoots(roots) {
		builder.authorityErr = errors.Join(builder.authorityErr, errors.New("semantic startup journal authority does not match configured roots"))
	}
	return builder
}

func (builder *SemanticPlanBuilder) Recover(ctx context.Context, configurationDigest string) error {
	if builder == nil || builder.authorityErr != nil || builder.rootAuthority == nil || builder.journalAuthority == nil ||
		!domainsecurity.IsSHA256Hex(strings.TrimSpace(configurationDigest)) {
		return errors.New("semantic startup plan builder is unavailable")
	}
	if err := contextError(ctx); err != nil {
		return err
	}
	builder.mu.Lock()
	defer builder.mu.Unlock()
	if err := contextError(ctx); err != nil {
		return err
	}
	if err := builder.rootAuthority.Validate(); err != nil {
		return err
	}
	if err := builder.journalAuthority.Validate(); err != nil {
		return err
	}
	journalRoot := filepath.Join(builder.journalAuthority.path(), "journal")
	if err := preflightCurrentSemanticJournalForCleanupWithPreservationV1(
		ctx, builder.roots, builder.journalAuthority, journalRoot, strings.TrimSpace(configurationDigest), builder.restartPreserved, builder.originalCreates); err != nil {
		return err
	}
	if _, err := preflightRetiredSemanticJournalsContext(ctx, builder.journalAuthority, builder.roots, builder.originalCreates); err != nil {
		return err
	}
	if err := recoverSemanticPlanningStageContext(ctx, builder.roots, builder.journalAuthority); err != nil {
		return err
	}
	if _, err := preflightRetiredSemanticJournalsContext(ctx, builder.journalAuthority, builder.roots, builder.originalCreates); err != nil {
		return err
	}
	return recoverSemanticJournalWithPreservationV1(ctx, builder.roots, builder.rootAuthority, builder.journalAuthority, strings.TrimSpace(configurationDigest), builder.fault, builder.restartPreserved, builder.originalCreates)
}

// RecoverAuthenticatedExisting recovers only an already-started semantic
// transaction. The transaction's installation signature, root binding, and
// signed configuration digest are the authority; a later process configuration
// cannot strand recovery or authorize a different plan. No caller-supplied
// digest is accepted by this path.
func (builder *SemanticPlanBuilder) RecoverAuthenticatedExisting(ctx context.Context) error {
	if builder == nil || builder.authorityErr != nil || builder.rootAuthority == nil || builder.journalAuthority == nil {
		return errors.New("semantic startup plan builder is unavailable")
	}
	if err := contextError(ctx); err != nil {
		return err
	}
	builder.mu.Lock()
	defer builder.mu.Unlock()
	if err := contextError(ctx); err != nil {
		return err
	}
	if err := builder.rootAuthority.Validate(); err != nil {
		return err
	}
	if err := builder.journalAuthority.Validate(); err != nil {
		return err
	}
	configurationDigest, err := authenticatedSemanticRecoveryConfigurationDigestWithPreservationV1(
		ctx, builder.roots, builder.journalAuthority, builder.restartPreserved, builder.originalCreates)
	if err != nil {
		return err
	}
	if _, err := preflightRetiredSemanticJournalsContext(ctx, builder.journalAuthority, builder.roots, builder.originalCreates); err != nil {
		return err
	}
	// The active signed journal is preflighted before any independent planning
	// residue is retired. A forged or foreign live journal therefore produces
	// zero recovery mutation.
	if err := recoverSemanticPlanningStageContext(ctx, builder.roots, builder.journalAuthority); err != nil {
		return err
	}
	revalidatedDigest, err := authenticatedSemanticRecoveryConfigurationDigestWithPreservationV1(
		ctx, builder.roots, builder.journalAuthority, builder.restartPreserved, builder.originalCreates)
	if err != nil {
		return err
	}
	if revalidatedDigest != configurationDigest {
		return errors.New("semantic startup authenticated recovery changed after planning cleanup")
	}
	if _, err := preflightRetiredSemanticJournalsContext(ctx, builder.journalAuthority, builder.roots, builder.originalCreates); err != nil {
		return err
	}
	return recoverSemanticJournalWithPreservationV1(
		ctx, builder.roots, builder.rootAuthority, builder.journalAuthority, configurationDigest, builder.fault, builder.restartPreserved, builder.originalCreates)
}

func authenticatedSemanticRecoveryConfigurationDigest(
	ctx context.Context,
	roots RootSet,
	journalAuthority *JournalNamespaceAuthority,
) (string, error) {
	return authenticatedSemanticRecoveryConfigurationDigestWithPreservationV1(ctx, roots, journalAuthority, nil)
}

func authenticatedSemanticRecoveryConfigurationDigestWithPreservationV1(ctx context.Context, roots RootSet, journalAuthority *JournalNamespaceAuthority, preserved SemanticRestartPreservationV1,
	originals ...privatecasport.OriginalCreateResiduesV1,
) (string, error) {
	if journalAuthority == nil || journalAuthority.Validate() != nil || !journalAuthority.matchesRoots(roots) {
		return "", errors.New("semantic startup authenticated recovery authority is invalid")
	}
	journalRoot := filepath.Join(journalAuthority.path(), "journal")
	if err := preflightBenignStartupAuthorityCreateResidue(journalAuthority, journalRoot); err != nil {
		return "", err
	}
	directory, err := secureStartupOpenDirectory(journalAuthority.root, filepath.Base(journalRoot), "")
	if errors.Is(err, os.ErrNotExist) {
		return authenticatedSemanticRecoveryEmptyConfigurationDigest, nil
	}
	if err != nil {
		return "", err
	}
	authority, err := loadStartupJournalAuthority(journalAuthority, journalRoot)
	if err != nil {
		_ = directory.Close()
		return "", errors.New("semantic startup journal authority is missing or invalid")
	}
	digest := ""
	bindDigest := func(candidate string) error {
		candidate = strings.TrimSpace(candidate)
		if !domainsecurity.IsSHA256Hex(candidate) {
			return errors.New("semantic startup authenticated recovery configuration is invalid")
		}
		if digest != "" && digest != candidate {
			return errors.New("semantic startup authenticated recovery configuration changed")
		}
		digest = candidate
		return nil
	}
	if retirementBody, _, retirementErr := directory.ReadFile(
		semanticJournalRetirementFile, maxSemanticRetirementBytes, false,
	); retirementErr == nil {
		record, _, decodeErr := decodeSemanticRetirementForDirectory(retirementBody, directory, authority, roots)
		if decodeErr != nil {
			err = decodeErr
		} else {
			err = bindDigest(record.ConfigurationDigest)
		}
	} else if !errors.Is(retirementErr, os.ErrNotExist) {
		err = retirementErr
	}
	if err == nil {
		body, _, readErr := directory.ReadFile("journal.json", maxSemanticJournalBytes, false)
		switch {
		case readErr == nil:
			journal, decodeErr := decodeSemanticJournalForRetirement(directory, body, authority)
			if decodeErr != nil {
				err = decodeErr
			} else {
				err = bindDigest(journal.Plan.ConfigurationDigest)
			}
		case errors.Is(readErr, os.ErrNotExist):
			entries, entriesErr := directory.ReadEntriesBoundedContext(ctx, maxSemanticJournalResidue)
			if entriesErr != nil {
				err = entriesErr
				break
			}
			visible := entries[:0]
			for _, entry := range entries {
				if entry.Name() != semanticJournalRetirementFile {
					visible = append(visible, entry)
				}
			}
			if len(visible) == 0 {
				break
			}
			if len(visible) != 1 || !validSemanticJournalTempName(visible[0]) {
				err = errors.New("semantic startup journal is missing with ambiguous residue")
				break
			}
			body, _, readErr = directory.ReadFile(visible[0].Name(), maxSemanticJournalBytes, false)
			if readErr != nil {
				err = errors.New("semantic startup journal temp is unreadable")
				break
			}
			candidate, decodeErr := decodeSemanticJournalForRetirement(directory, body, authority)
			if decodeErr != nil {
				err = decodeErr
			} else {
				err = bindDigest(candidate.Plan.ConfigurationDigest)
			}
		default:
			err = readErr
		}
	}
	closeErr := directory.Close()
	if err != nil || closeErr != nil {
		return "", errors.Join(err, closeErr)
	}
	if digest == "" {
		digest = authenticatedSemanticRecoveryEmptyConfigurationDigest
	}
	if err := preflightCurrentSemanticJournalForCleanupWithPreservationV1(ctx, roots, journalAuthority, journalRoot, digest, preserved, originals...); err != nil {
		return "", err
	}
	return digest, nil
}

func (builder *SemanticPlanBuilder) Prepare(
	ctx context.Context,
	baseline domainstartup.ReadOnlyStartupBaselineV1,
	configurationDigest string,
	simulate startupport.SemanticSimulationV1,
) (result startupport.PreparedSemanticPlanV1, resultErr error) {
	if builder == nil || builder.authorityErr != nil || builder.rootAuthority == nil || builder.journalAuthority == nil || simulate == nil || domainstartup.ValidateReadOnlyStartupBaselineV1(baseline) != nil ||
		baseline.ConfigurationDigest != strings.TrimSpace(configurationDigest) {
		return nil, errors.New("semantic startup preparation input is invalid")
	}
	if err := contextError(ctx); err != nil {
		return nil, err
	}
	builder.mu.Lock()
	defer builder.mu.Unlock()
	if err := builder.rootAuthority.Validate(); err != nil {
		return nil, err
	}
	if err := builder.journalAuthority.Validate(); err != nil {
		return nil, err
	}
	journalRoot, err := semanticJournalRootForAuthority(builder.roots, builder.journalAuthority)
	if err != nil {
		return nil, err
	}
	if _, err := os.Lstat(journalRoot); err == nil {
		return nil, errors.New("semantic startup journal must be recovered before planning")
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	current, err := captureStrictWithRootAuthority(ctx, builder.roots, builder.rootAuthority, builder.originalCreates)
	if err != nil {
		return nil, err
	}
	currentSnapshot, err := managedSnapshotFromRaw(current)
	if err != nil || currentSnapshot.SnapshotDigest != baseline.ManagedSnapshotDigest ||
		currentSnapshot.RootBindingDigest != baseline.RootBindingDigest || currentSnapshot.RawCaptureDigest != baseline.RawCaptureDigest {
		return nil, errors.New("semantic startup baseline changed before simulation")
	}
	stageRoot, stageRoots, stageID, planningID, err := createSemanticStage(builder.roots, builder.journalAuthority)
	if err != nil {
		return nil, err
	}
	removeStage := true
	defer func() {
		if removeStage {
			resultErr = errors.Join(resultErr, removeSemanticPlanningStage(builder.roots, builder.journalAuthority, stageRoot, stageID))
		}
	}()
	if err := validateSemanticPlanningStage(builder.roots, builder.journalAuthority, stageRoot, stageID, planningID); err != nil {
		return nil, err
	}
	if err := copyManagedSnapshotToStage(ctx, current, stageRoots); err != nil {
		return nil, err
	}
	if err := validateSemanticPlanningStage(builder.roots, builder.journalAuthority, stageRoot, stageID, planningID); err != nil {
		return nil, err
	}
	stageProof, closeStageProof, err := bindOriginalCreateStageProofV1(ctx, builder.originalCreates, stageRoots)
	if err != nil {
		return nil, err
	}
	defer func() { resultErr = errors.Join(resultErr, closeStageProof()) }()
	stageContext, closePlainProof, err := bindOriginalPlainStageContextV1(ctx, stageRoots)
	if err != nil {
		return nil, err
	}
	defer func() { resultErr = errors.Join(resultErr, closePlainProof()) }()
	if err := simulate(stageContext, startupport.PersistenceRootsV1{DataDir: stageRoots.DataDir, DurableDir: stageRoots.DurableDir}); err != nil {
		return nil, err
	}
	if err := validateSemanticPlanningStage(builder.roots, builder.journalAuthority, stageRoot, stageID, planningID); err != nil {
		return nil, err
	}
	final, err := CaptureStrictWithOriginalCreateResiduesV1(stageContext, stageRoots, stageProof)
	if err != nil {
		return nil, err
	}
	if err := validateSemanticPlanningStage(builder.roots, builder.journalAuthority, stageRoot, stageID, planningID); err != nil {
		return nil, err
	}
	operations, err := semanticOperations(current, final)
	if err != nil {
		return nil, err
	}
	plan, err := domainstartup.NewSemanticStartupPlanV1(
		baseline.ManagedSnapshotDigest,
		configurationDigest,
		semanticStateDigest(final.Entries),
		operations,
	)
	if err != nil {
		return nil, err
	}
	prepared := &preparedSemanticPlan{
		builder: builder, plan: plan, baseline: baseline, stageRoot: stageRoot, stageRoots: stageRoots, stageID: stageID, planningID: planningID,
	}
	removeStage = false
	return prepared, nil
}

func (prepared *preparedSemanticPlan) Plan() domainstartup.SemanticStartupPlanV1 {
	if prepared == nil {
		return domainstartup.SemanticStartupPlanV1{}
	}
	return prepared.plan
}

func (prepared *preparedSemanticPlan) Apply(ctx context.Context) error {
	if prepared == nil || prepared.builder == nil || domainstartup.ValidateSemanticStartupPlanV1(prepared.plan) != nil ||
		domainstartup.ValidateReadOnlyStartupBaselineV1(prepared.baseline) != nil {
		return errors.New("prepared semantic startup plan is invalid")
	}
	if err := contextError(ctx); err != nil {
		return err
	}
	prepared.mu.Lock()
	defer prepared.mu.Unlock()
	if prepared.closed {
		return errors.New("prepared semantic startup plan is closed")
	}
	prepared.builder.mu.Lock()
	defer prepared.builder.mu.Unlock()
	if err := prepared.builder.rootAuthority.Validate(); err != nil {
		return err
	}
	if err := validateSemanticPlanningStage(prepared.builder.roots, prepared.builder.journalAuthority, prepared.stageRoot, prepared.stageID, prepared.planningID); err != nil {
		return err
	}
	current, err := captureStrictWithRootAuthority(ctx, prepared.builder.roots, prepared.builder.rootAuthority, prepared.builder.originalCreates)
	if err != nil {
		return err
	}
	snapshot, err := managedSnapshotFromRaw(current)
	if err != nil || snapshot.SnapshotDigest != prepared.baseline.ManagedSnapshotDigest ||
		snapshot.RawCaptureDigest != prepared.baseline.RawCaptureDigest {
		return errors.New("managed persistence changed before semantic startup apply")
	}
	if err := validateSemanticRestartOperationsV1(ctx, prepared.builder.restartPreserved, prepared.plan, 0, func(operation domainstartup.SemanticStartupOperationV1) ([]byte, error) {
		return readSemanticPlanningManagedFile(ctx, prepared.builder.roots, prepared.builder.journalAuthority, prepared.stageRoot, prepared.stageID, prepared.planningID, prepared.stageRoots, operation.Path, operation.After)
	}, ""); err != nil {
		return err
	}
	journalRoot, err := semanticJournalRootForAuthority(prepared.builder.roots, prepared.builder.journalAuthority)
	if err != nil {
		return err
	}
	if err := prepareSemanticJournal(ctx, journalRoot, prepared.builder.roots, prepared.builder.rootAuthority, prepared.builder.journalAuthority, prepared.plan, prepared.stageRoot, prepared.stageRoots, prepared.stageID, prepared.planningID, prepared.builder.fault); err != nil {
		return err
	}
	if err := applySemanticJournalWithPreservationV1(ctx, journalRoot, prepared.builder.roots, prepared.builder.rootAuthority, prepared.builder.journalAuthority, prepared.plan.ConfigurationDigest, prepared.builder.fault, prepared.builder.restartPreserved, prepared.builder.originalCreates); err != nil {
		return err
	}
	return nil
}

func (prepared *preparedSemanticPlan) Close() error {
	if prepared == nil {
		return nil
	}
	prepared.mu.Lock()
	defer prepared.mu.Unlock()
	if prepared.closed {
		return nil
	}
	prepared.closed = true
	return removeSemanticPlanningStage(prepared.builder.roots, prepared.builder.journalAuthority, prepared.stageRoot, prepared.stageID)
}

func captureStrictWithRootAuthority(ctx context.Context, roots RootSet, authority *RootAuthority,
	originals ...privatecasport.OriginalCreateResiduesV1,
) (RawSnapshot, error) {
	return captureStrictWithRootAuthorityAndResidues(ctx, roots, authority, nil, originals...)
}

func captureStrictWithRootAuthorityAndResidues(
	ctx context.Context,
	roots RootSet,
	authority *RootAuthority,
	allowed map[string]struct{},
	originals ...privatecasport.OriginalCreateResiduesV1,
) (RawSnapshot, error) {
	if err := contextError(ctx); err != nil {
		return RawSnapshot{}, err
	}
	if err := authority.Validate(); err != nil {
		return RawSnapshot{}, err
	}
	snapshot, err := captureStrictWithAllowedPrivateResidues(ctx, roots, nil, allowed, originals...)
	if err != nil {
		return RawSnapshot{}, err
	}
	if err := authority.Validate(); err != nil {
		return RawSnapshot{}, err
	}
	return snapshot, nil
}

func managedSnapshotFromRaw(raw RawSnapshot) (domainstartup.ManagedSnapshotV1, error) {
	entries := make([]domainstartup.ManagedEntryStateV1, 0, len(raw.Entries))
	for _, entry := range raw.Entries {
		entries = append(entries, domainstartup.ManagedEntryStateV1{
			Path: entry.Path, Type: entry.Type, Mode: entry.Mode, Size: entry.Size,
			ModTimeUnixNano: entry.ModTimeUnixNano, SHA256: entry.SHA256, RecordCount: entry.RecordCount,
		})
	}
	return domainstartup.NewManagedSnapshotV1(CanonicalRoots(raw.Roots), raw.SHA256, entries)
}

func createSemanticStage(roots RootSet, authority *JournalNamespaceAuthority) (
	resultRoot string,
	resultRoots RootSet,
	resultStageID string,
	resultPlanningID string,
	resultErr error,
) {
	planningRoot, err := semanticPlanningRootForAuthority(roots, authority)
	if err != nil {
		return "", RootSet{}, "", "", err
	}
	if authority == nil || !authority.matchesRoots(roots) || filepath.Dir(planningRoot) != authority.planningPath() {
		return "", RootSet{}, "", "", errors.New("semantic startup planning authority is invalid")
	}
	planningName := filepath.Base(planningRoot)
	planning, err := secureStartupOpenDirectory(authority.planningRoot, planningName, "")
	if errors.Is(err, os.ErrNotExist) {
		planning, err = secureStartupCreateDirectory(authority.planningRoot, planningName)
	}
	if err != nil {
		return "", RootSet{}, "", "", err
	}
	var stage *startupPrivateDirectory
	cleanupStage := false
	stageRoot := ""
	stageIdentity := ""
	defer func() {
		if stage != nil {
			resultErr = errors.Join(resultErr, stage.Close())
		}
		if cleanupStage {
			resultErr = errors.Join(resultErr, removeSemanticPlanningStage(roots, authority, stageRoot, stageIdentity))
		}
		resultErr = errors.Join(resultErr, planning.Close())
	}()
	stageName, err := startupPrivateRandomName("stage-")
	if err != nil {
		return "", RootSet{}, "", "", err
	}
	stage, err = planning.CreateDirectory(stageName)
	if err != nil {
		return "", RootSet{}, "", "", err
	}
	stageRoot = filepath.Join(planningRoot, stageName)
	stageIdentity = stage.Identity()
	cleanupStage = true
	dataDirectory, err := stage.CreateDirectory("data")
	if err != nil {
		return "", RootSet{}, "", "", err
	}
	dataIdentity := dataDirectory.Identity()
	if err := dataDirectory.Close(); err != nil {
		return "", RootSet{}, "", "", err
	}
	durableIdentity := dataIdentity
	durableRoot := filepath.Join(stageRoot, "data")
	if canonicalPathKey(roots.DataDir) != canonicalPathKey(roots.DurableDir) {
		durableDirectory, err := stage.CreateDirectory("durable")
		if err != nil {
			return "", RootSet{}, "", "", err
		}
		durableIdentity = durableDirectory.Identity()
		if err := durableDirectory.Close(); err != nil {
			return "", RootSet{}, "", "", err
		}
		durableRoot = filepath.Join(stageRoot, "durable")
	}
	dataRoot := filepath.Join(stageRoot, "data")
	marker := semanticPlanningMarkerV1{
		SchemaVersion: semanticPlanningMarkerVersion, Purpose: semanticPlanningMarkerPurpose, RootBindingDigest: rootBindingDigest(roots),
		PlanningIdentity: planning.Identity(), StageName: stageName, StageIdentity: stage.Identity(),
		DataIdentity: dataIdentity, DurableIdentity: durableIdentity,
	}
	marker.MarkerDigest = semanticPlanningMarkerDigest(marker)
	body, _ := json.Marshal(marker)
	if err := stage.WriteExclusive("planning.json", body, maxSemanticPlanningBytes); err != nil {
		return "", RootSet{}, "", "", err
	}
	resolved, err := ResolveRootSet(dataRoot, durableRoot)
	if err != nil {
		return "", RootSet{}, "", "", err
	}
	if err := validateSemanticPlanningStage(roots, authority, stageRoot, stage.Identity(), planning.Identity()); err != nil {
		return "", RootSet{}, "", "", err
	}
	cleanupStage = false
	return stageRoot, resolved, stage.Identity(), planning.Identity(), nil
}

func semanticPlanningRootForAuthority(
	roots RootSet,
	authority *JournalNamespaceAuthority,
) (string, error) {
	if authority == nil || authority.Validate() != nil || !authority.matchesRoots(roots) ||
		authority.planningPath() == "" {
		return "", errors.New("semantic startup planning namespace authority is invalid")
	}
	return filepath.Join(authority.planningPath(), "planning-"+rootBindingDigest(roots)), nil
}

func recoverSemanticPlanningStage(roots RootSet, authority *JournalNamespaceAuthority) error {
	return recoverSemanticPlanningStageContext(context.Background(), roots, authority)
}

func recoverSemanticPlanningStageContext(ctx context.Context, roots RootSet, authority *JournalNamespaceAuthority) error {
	if err := contextError(ctx); err != nil {
		return err
	}
	planningRoot, err := semanticPlanningRootForAuthority(roots, authority)
	if err != nil {
		return err
	}
	if authority == nil || !authority.matchesRoots(roots) || filepath.Dir(planningRoot) != authority.planningPath() {
		return errors.New("semantic startup planning recovery authority is invalid")
	}
	planning, err := secureStartupOpenDirectory(authority.planningRoot, filepath.Base(planningRoot), "")
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer planning.Close()
	entries, err := planning.ReadEntriesBoundedContext(ctx, maxSemanticNamespaceEntries)
	if err != nil {
		return err
	}
	type planningRecoveryPlan struct {
		name     string
		identity string
		retired  bool
	}
	plans := make([]planningRecoveryPlan, 0, len(entries))
	for _, entry := range entries {
		if err := contextError(ctx); err != nil {
			return err
		}
		if !entry.IsDir() {
			return errors.New("semantic startup planning namespace contains unknown residue")
		}
		switch {
		case validSemanticPlanningDirectoryName(entry.Name(), ".retired-stage-"):
			retired, openErr := planning.OpenDirectory(entry.Name(), "")
			if openErr != nil {
				return openErr
			}
			identity := retired.Identity()
			preflightErr := retired.PreflightTreeContext(ctx)
			closeErr := retired.Close()
			if preflightErr != nil || closeErr != nil {
				return errors.Join(preflightErr, closeErr)
			}
			plans = append(plans, planningRecoveryPlan{name: entry.Name(), identity: identity, retired: true})
		case validSemanticPlanningDirectoryName(entry.Name(), "stage-"):
			identity, inspectErr := inspectOneSemanticPlanningStageContext(ctx, roots, planning, filepath.Join(planningRoot, entry.Name()))
			if inspectErr != nil {
				return inspectErr
			}
			plans = append(plans, planningRecoveryPlan{name: entry.Name(), identity: identity})
		default:
			return errors.New("semantic startup planning namespace contains unknown residue")
		}
	}
	for _, plan := range plans {
		if err := contextError(ctx); err != nil {
			return err
		}
		if plan.retired {
			retired, openErr := planning.OpenDirectory(plan.name, plan.identity)
			if openErr != nil {
				return openErr
			}
			if err := retired.PreflightTreeContext(ctx); err != nil {
				_ = retired.Close()
				return err
			}
			if err := planning.RemoveRetiredDirectoryContext(ctx, plan.name, retired); err != nil {
				_ = retired.Close()
				return err
			}
			if err := retired.Close(); err != nil {
				return err
			}
			continue
		}
		stageRoot := filepath.Join(planningRoot, plan.name)
		identity, inspectErr := inspectOneSemanticPlanningStageContext(ctx, roots, planning, stageRoot)
		if inspectErr != nil || identity != plan.identity {
			return errors.Join(inspectErr, errors.New("semantic startup planning stage changed after preflight"))
		}
		if err := removeSemanticPlanningStageContext(ctx, roots, authority, stageRoot, identity); err != nil {
			return err
		}
	}
	return nil
}

func recoverOneSemanticPlanningStage(roots RootSet, authority *JournalNamespaceAuthority, planning *startupPrivateDirectory, stageRoot string) error {
	stageIdentity, err := inspectOneSemanticPlanningStage(roots, planning, stageRoot)
	if err != nil {
		return err
	}
	return removeSemanticPlanningStage(roots, authority, stageRoot, stageIdentity)
}

func inspectOneSemanticPlanningStage(roots RootSet, planning *startupPrivateDirectory, stageRoot string) (string, error) {
	return inspectOneSemanticPlanningStageContext(context.Background(), roots, planning, stageRoot)
}

func inspectOneSemanticPlanningStageContext(ctx context.Context, roots RootSet, planning *startupPrivateDirectory, stageRoot string) (string, error) {
	if err := contextError(ctx); err != nil {
		return "", err
	}
	stage, err := planning.OpenDirectory(filepath.Base(stageRoot), "")
	if err != nil {
		return "", errors.New("semantic startup planning stage is unsafe")
	}
	defer stage.Close()
	body, _, err := stage.ReadFile("planning.json", maxSemanticPlanningBytes, false)
	if errors.Is(err, os.ErrNotExist) {
		entries, readErr := stage.ReadEntriesBoundedContext(ctx, 1)
		if readErr != nil || len(entries) > 1 {
			return "", errors.New("semantic startup planning stage has ambiguous pre-marker residue")
		}
		if len(entries) == 1 {
			return "", errors.New("semantic startup planning stage has unknown pre-marker residue")
		}
	} else if err != nil {
		return "", err
	} else {
		marker, markerErr := decodeSemanticPlanningMarker(body)
		if markerErr != nil {
			return "", markerErr
		}
		if marker.RootBindingDigest != rootBindingDigest(roots) || marker.PlanningIdentity != planning.Identity() ||
			marker.StageName != filepath.Base(stageRoot) || marker.StageIdentity != stage.Identity() {
			return "", errors.New("semantic startup planning marker is invalid")
		}
	}
	stageIdentity := stage.Identity()
	if err := stage.PreflightTreeContext(ctx); err != nil {
		return "", err
	}
	if err := stage.Close(); err != nil {
		return "", err
	}
	return stageIdentity, nil
}

func validSemanticPlanningDirectoryName(name, prefix string) bool {
	random := strings.TrimPrefix(name, prefix)
	return len(random) == 32 && isLowerHex(random) && name == prefix+random && startupAuthorityNamedComponent(name)
}

func semanticPlanningMarkerDigest(marker semanticPlanningMarkerV1) string {
	body, _ := json.Marshal(struct {
		SchemaVersion     int    `json:"schemaVersion"`
		Purpose           string `json:"purpose"`
		RootBindingDigest string `json:"rootBindingDigest"`
		PlanningIdentity  string `json:"planningIdentity"`
		StageName         string `json:"stageName"`
		StageIdentity     string `json:"stageIdentity"`
		DataIdentity      string `json:"dataIdentity"`
		DurableIdentity   string `json:"durableIdentity"`
	}{marker.SchemaVersion, marker.Purpose, marker.RootBindingDigest, marker.PlanningIdentity, marker.StageName, marker.StageIdentity, marker.DataIdentity, marker.DurableIdentity})
	return domainsecurity.SHA256Hex(body)
}

func removeSemanticPlanningStage(roots RootSet, authority *JournalNamespaceAuthority, stageRoot, expectedIdentity string) error {
	return removeSemanticPlanningStageContext(context.Background(), roots, authority, stageRoot, expectedIdentity)
}

func removeSemanticPlanningStageContext(ctx context.Context, roots RootSet, authority *JournalNamespaceAuthority, stageRoot, expectedIdentity string) error {
	if err := contextError(ctx); err != nil {
		return err
	}
	if strings.TrimSpace(stageRoot) == "" {
		return nil
	}
	planningRoot, err := semanticPlanningRootForAuthority(roots, authority)
	if err != nil || authority == nil || !authority.matchesRoots(roots) || filepath.Dir(stageRoot) != planningRoot {
		return errors.New("semantic startup planning stage removal authority is invalid")
	}
	planning, err := secureStartupOpenDirectory(authority.planningRoot, filepath.Base(planningRoot), "")
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer planning.Close()
	retiredName, retired, err := planning.RetireDirectory(filepath.Base(stageRoot), expectedIdentity, ".retired-stage-")
	if err != nil || retired == nil {
		return err
	}
	defer retired.Close()
	return planning.RemoveRetiredDirectoryContext(ctx, retiredName, retired)
}

func decodeSemanticPlanningMarker(body []byte) (semanticPlanningMarkerV1, error) {
	if len(body) == 0 || len(body) > maxSemanticPlanningBytes {
		return semanticPlanningMarkerV1{}, StartupResourceLimitError{Code: "planning_bytes", Limit: maxSemanticPlanningBytes}
	}
	if err := validateSemanticControlJSON(body, maxSemanticPlanningBytes); err != nil {
		return semanticPlanningMarkerV1{}, errors.New("semantic startup planning marker JSON is invalid")
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var marker semanticPlanningMarkerV1
	if err := decoder.Decode(&marker); err != nil || decoder.Decode(&struct{}{}) != io.EOF || marker.StageName == "" || marker.StageIdentity == "" ||
		marker.PlanningIdentity == "" || marker.DataIdentity == "" || marker.DurableIdentity == "" ||
		marker.MarkerDigest != semanticPlanningMarkerDigest(marker) {
		return semanticPlanningMarkerV1{}, errors.New("semantic startup planning marker is invalid")
	}
	if marker.SchemaVersion == 1 {
		if marker.Purpose != semanticPlanningMarkerV1Purpose {
			return semanticPlanningMarkerV1{}, errors.New("semantic startup V1 planning marker is invalid")
		}
		for _, identity := range []string{marker.PlanningIdentity, marker.StageIdentity, marker.DataIdentity, marker.DurableIdentity} {
			if !validLegacyObjectIdentityV3(identity) {
				return semanticPlanningMarkerV1{}, errors.New("semantic startup V1 planning identity is invalid")
			}
		}
		return migrateSemanticPlanningMarkerV1(marker)
	}
	if marker.SchemaVersion != semanticPlanningMarkerVersion || marker.Purpose != semanticPlanningMarkerPurpose {
		return semanticPlanningMarkerV1{}, errors.New("semantic startup planning marker version is invalid")
	}
	for _, identity := range []string{marker.PlanningIdentity, marker.StageIdentity, marker.DataIdentity, marker.DurableIdentity} {
		if _, err := parseStrongDirectoryIdentity(identity); err != nil {
			return semanticPlanningMarkerV1{}, errors.New("semantic startup planning marker identity is invalid")
		}
	}
	return marker, nil
}

func validateSemanticPlanningStage(roots RootSet, authority *JournalNamespaceAuthority, stageRoot, stageIdentity, planningIdentity string) error {
	planningRoot, err := semanticPlanningRootForAuthority(roots, authority)
	if err != nil || authority == nil || !authority.matchesRoots(roots) || filepath.Dir(stageRoot) != planningRoot {
		return errors.New("semantic startup planning stage authority is invalid")
	}
	planning, err := secureStartupOpenDirectory(authority.planningRoot, filepath.Base(planningRoot), planningIdentity)
	if err != nil {
		return err
	}
	defer planning.Close()
	stage, err := planning.OpenDirectory(filepath.Base(stageRoot), stageIdentity)
	if err != nil {
		return err
	}
	defer stage.Close()
	body, _, err := stage.ReadFile("planning.json", maxSemanticPlanningBytes, false)
	if err != nil {
		return err
	}
	marker, err := decodeSemanticPlanningMarker(body)
	if err != nil || marker.RootBindingDigest != rootBindingDigest(roots) || marker.PlanningIdentity != planningIdentity ||
		marker.StageName != filepath.Base(stageRoot) || marker.StageIdentity != stageIdentity {
		return errors.New("semantic startup planning stage marker changed")
	}
	data, err := stage.OpenDirectory("data", marker.DataIdentity)
	if err != nil {
		return errors.New("semantic startup planning data root changed")
	}
	_ = data.Close()
	if canonicalPathKey(roots.DataDir) != canonicalPathKey(roots.DurableDir) {
		durable, err := stage.OpenDirectory("durable", marker.DurableIdentity)
		if err != nil {
			return errors.New("semantic startup planning durable root changed")
		}
		_ = durable.Close()
	} else if marker.DurableIdentity != marker.DataIdentity {
		return errors.New("semantic startup planning root alias changed")
	}
	return nil
}

func copyManagedSnapshotToStage(ctx context.Context, raw RawSnapshot, stageRoots RootSet) error {
	entries := append([]EntryRecord(nil), raw.Entries...)
	sort.Slice(entries, func(left int, right int) bool {
		leftDepth := strings.Count(entries[left].Path, "/")
		rightDepth := strings.Count(entries[right].Path, "/")
		if leftDepth != rightDepth {
			return leftDepth < rightDepth
		}
		return entries[left].Path < entries[right].Path
	})
	for _, entry := range entries {
		if err := contextError(ctx); err != nil {
			return err
		}
		if entry.Type == domainstartup.ManagedEntryTypeAbsent {
			continue
		}
		source, err := managedPath(raw.Roots, entry.Path)
		if err != nil {
			return err
		}
		target, err := managedPath(stageRoots, entry.Path)
		if err != nil {
			return err
		}
		switch entry.Type {
		case domainstartup.ManagedEntryTypeDirectory:
			if err := os.MkdirAll(target, os.FileMode(entry.Mode).Perm()); err != nil {
				return err
			}
			if err := os.Chmod(target, os.FileMode(entry.Mode).Perm()); err != nil {
				return err
			}
		case domainstartup.ManagedEntryTypeFile:
			if err := copyVerifiedFile(ctx, source, target, entry); err != nil {
				return err
			}
		default:
			return errors.New("semantic startup snapshot contains an unknown entry")
		}
	}
	return nil
}

func copyVerifiedFile(ctx context.Context, source string, target string, expected EntryRecord) (resultErr error) {
	if expected.Size < 0 || expected.Size > domainstartup.MaxSemanticManagedFileBytesV1 {
		return StartupResourceLimitError{Code: "managed_file_bytes", Limit: domainstartup.MaxSemanticManagedFileBytesV1}
	}
	initial, err := os.Lstat(source)
	if err != nil || initial.Mode()&os.ModeSymlink != 0 || !initial.Mode().IsRegular() || initial.Size() != expected.Size ||
		uint32(initial.Mode()) != expected.Mode || initial.ModTime().UnixNano() != expected.ModTimeUnixNano {
		return errors.New("managed file changed while preparing semantic startup")
	}
	sourceFile, err := os.Open(source)
	if err != nil {
		return errors.New("managed file changed while preparing semantic startup")
	}
	defer func() { resultErr = errors.Join(resultErr, sourceFile.Close()) }()
	opened, err := sourceFile.Stat()
	if err != nil || !os.SameFile(initial, opened) || opened.Size() != expected.Size {
		return errors.New("managed file changed while preparing semantic startup")
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		return err
	}
	targetFile, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, os.FileMode(expected.Mode).Perm())
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		closeErr := targetFile.Close()
		resultErr = errors.Join(resultErr, closeErr)
		if !committed || closeErr != nil {
			_ = os.Remove(target)
		}
	}()
	hasher := sha256.New()
	buffer := make([]byte, 1<<20)
	written := int64(0)
	for {
		if err := contextError(ctx); err != nil {
			return err
		}
		count, readErr := sourceFile.Read(buffer)
		if count > 0 {
			written += int64(count)
			if written > domainstartup.MaxSemanticManagedFileBytesV1 || written > expected.Size {
				return errors.New("managed file changed while preparing semantic startup")
			}
			if _, err := hasher.Write(buffer[:count]); err != nil {
				return err
			}
			for offset := 0; offset < count; {
				writeCount, writeErr := targetFile.Write(buffer[offset:count])
				if writeErr != nil || writeCount <= 0 {
					return errors.Join(writeErr, io.ErrShortWrite)
				}
				offset += writeCount
			}
		}
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			return readErr
		}
	}
	if written != expected.Size || hex.EncodeToString(hasher.Sum(nil)) != expected.SHA256 {
		return errors.New("managed file changed while preparing semantic startup")
	}
	refreshed, statErr := sourceFile.Stat()
	current, lstatErr := os.Lstat(source)
	if statErr != nil || lstatErr != nil || current.Mode()&os.ModeSymlink != 0 || !os.SameFile(opened, refreshed) ||
		!os.SameFile(refreshed, current) || current.Size() != expected.Size || uint32(current.Mode()) != expected.Mode ||
		current.ModTime().UnixNano() != expected.ModTimeUnixNano {
		return errors.New("managed file changed while preparing semantic startup")
	}
	if err := targetFile.Sync(); err != nil {
		return err
	}
	if err := targetFile.Chmod(os.FileMode(expected.Mode).Perm()); err != nil {
		return err
	}
	committed = true
	return nil
}

func semanticOperations(before RawSnapshot, after RawSnapshot) ([]domainstartup.SemanticStartupOperationV1, error) {
	beforeStates := semanticStates(before.Entries)
	afterStates := semanticStates(after.Entries)
	paths := make([]string, 0, len(beforeStates)+len(afterStates))
	seen := map[string]bool{}
	for path := range beforeStates {
		seen[path] = true
		paths = append(paths, path)
	}
	for path := range afterStates {
		if !seen[path] {
			paths = append(paths, path)
		}
	}
	sort.Strings(paths)
	absent := domainstartup.SemanticEntryStateV1{Type: domainstartup.ManagedEntryTypeAbsent}
	operations := make([]domainstartup.SemanticStartupOperationV1, 0)
	for _, path := range paths {
		left, ok := beforeStates[path]
		if !ok {
			left = absent
		}
		right, ok := afterStates[path]
		if !ok {
			right = absent
		}
		if left == right {
			continue
		}
		kind := ""
		switch {
		case left.Type == domainstartup.ManagedEntryTypeAbsent && right.Type == domainstartup.ManagedEntryTypeDirectory:
			kind = domainstartup.SemanticOperationCreateDirectory
		case left.Type == right.Type && left.Type != domainstartup.ManagedEntryTypeAbsent && left.SHA256 == right.SHA256 && left.Size == right.Size && left.Mode != right.Mode:
			kind = domainstartup.SemanticOperationSetMode
		case (left.Type == domainstartup.ManagedEntryTypeAbsent || left.Type == domainstartup.ManagedEntryTypeFile) && right.Type == domainstartup.ManagedEntryTypeFile:
			kind = domainstartup.SemanticOperationInstallFile
		case left.Type == domainstartup.ManagedEntryTypeFile && right.Type == domainstartup.ManagedEntryTypeAbsent:
			kind = domainstartup.SemanticOperationRemoveFile
		case left.Type == domainstartup.ManagedEntryTypeDirectory && right.Type == domainstartup.ManagedEntryTypeAbsent:
			kind = domainstartup.SemanticOperationRemoveDirectory
		default:
			return nil, fmt.Errorf("unsupported semantic startup transition at %s", path)
		}
		operations = append(operations, domainstartup.SemanticStartupOperationV1{Kind: kind, Path: path, Before: left, After: right})
	}
	return operations, nil
}

func semanticStates(entries []EntryRecord) map[string]domainstartup.SemanticEntryStateV1 {
	states := make(map[string]domainstartup.SemanticEntryStateV1, len(entries))
	for _, entry := range entries {
		state := domainstartup.SemanticEntryStateV1{Type: entry.Type, Mode: entry.Mode, Size: entry.Size, SHA256: entry.SHA256}
		if entry.Type == domainstartup.ManagedEntryTypeDirectory {
			state.Size = 0
		}
		states[entry.Path] = state
	}
	return states
}

func semanticStateDigest(entries []EntryRecord) string {
	return semanticStateMapDigest(semanticStates(entries))
}

func semanticStateMapDigest(states map[string]domainstartup.SemanticEntryStateV1) string {
	type stateRecord struct {
		Path  string                             `json:"path"`
		State domainstartup.SemanticEntryStateV1 `json:"state"`
	}
	paths := make([]string, 0, len(states))
	for path := range states {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	records := make([]stateRecord, 0, len(paths))
	for _, path := range paths {
		records = append(records, stateRecord{Path: path, State: states[path]})
	}
	body, _ := json.Marshal(records)
	return domainsecurity.SHA256Hex(body)
}

func semanticJournalRootForAuthority(
	roots RootSet,
	authority *JournalNamespaceAuthority,
) (string, error) {
	if authority == nil || authority.Validate() != nil || !authority.matchesRoots(roots) || authority.path() == "" {
		return "", errors.New("semantic startup journal namespace authority is invalid")
	}
	return filepath.Join(authority.path(), "journal"), nil
}

func rootBindingDigest(roots RootSet) string {
	body, _ := json.Marshal(CanonicalRoots(roots))
	return domainsecurity.SHA256Hex(body)
}

func prepareSemanticJournal(
	ctx context.Context,
	journalRoot string,
	roots RootSet,
	rootAuthority *RootAuthority,
	journalAuthority *JournalNamespaceAuthority,
	plan domainstartup.SemanticStartupPlanV1,
	planningStageRoot string,
	stageRoots RootSet,
	planningStageIdentity string,
	planningIdentity string,
	fault func(string, int) error,
) error {
	if err := contextError(ctx); err != nil {
		return err
	}
	if domainstartup.ValidateSemanticStartupPlanV1(plan) != nil || rootAuthority == nil || rootAuthority.Validate() != nil {
		return errors.New("semantic startup journal plan is invalid")
	}
	authority, err := openOrCreateStartupJournalAuthority(journalAuthority, journalRoot)
	if err != nil {
		return err
	}
	if filepath.Dir(journalRoot) != journalAuthority.path() || filepath.Base(journalRoot) != "journal" {
		return errors.New("semantic startup journal root is outside persistent authority")
	}
	journalDirectory, err := secureStartupCreateDirectory(journalAuthority.root, filepath.Base(journalRoot))
	if err != nil {
		return err
	}
	defer journalDirectory.Close()
	journalRootIdentity := journalDirectory.Identity()
	if err := semanticFault(fault, "after_journal_root_create", -1); err != nil {
		return err
	}
	journal := semanticJournalV1{
		SchemaVersion: semanticJournalSchemaVersion, State: semanticJournalPreparing,
		RootBindingDigest: rootBindingDigest(roots), RootCapabilityDigest: rootAuthority.Digest(),
		RootCapabilities: rootAuthority.bindings(), RootPromotionIntents: rootPromotionIntents(rootAuthority.bindings()),
		Plan: plan, NextOperation: 0, JournalRootIdentity: journalRootIdentity, AuthorityKeyID: authority.keyID,
	}
	journal.JournalDigest = semanticJournalDigest(journal)
	if err := writeSemanticJournalWithHook(journalRoot, journal, authority, fault); err != nil {
		return err
	}
	if err := semanticFault(fault, "after_journal_prepare", -1); err != nil {
		return err
	}
	stageDirectory, err := journalDirectory.CreateDirectory("stage")
	if err != nil {
		return err
	}
	defer stageDirectory.Close()
	journal.JournalStageIdentity = stageDirectory.Identity()
	journal.JournalDigest = semanticJournalDigest(journal)
	if err := writeSemanticJournalWithHook(journalRoot, journal, authority, fault); err != nil {
		return err
	}
	for _, operation := range plan.Operations {
		if operation.Kind != domainstartup.SemanticOperationInstallFile {
			continue
		}
		data, err := readSemanticPlanningManagedFile(
			ctx, roots, journalAuthority, planningStageRoot, planningStageIdentity,
			planningIdentity, stageRoots, operation.Path, operation.After,
		)
		if err != nil {
			return fmt.Errorf("semantic startup staged file identity changed: operation_id=%s cause=read", operation.OperationID)
		}
		if int64(len(data)) != operation.After.Size {
			return fmt.Errorf("semantic startup staged file identity changed: operation_id=%s cause=size", operation.OperationID)
		}
		if domainsecurity.SHA256Hex(data) != operation.After.SHA256 {
			return fmt.Errorf("semantic startup staged file identity changed: operation_id=%s cause=digest", operation.OperationID)
		}
		if err := stageDirectory.WriteExclusive(operation.OperationID, data, semanticFileSizeLimit(operation.After.Size)); err != nil {
			return err
		}
	}
	if err := semanticFault(fault, "after_stage_sync", -1); err != nil {
		return err
	}
	journal.State = semanticJournalPrepared
	journal.JournalDigest = semanticJournalDigest(journal)
	if err := writeSemanticJournalWithHook(journalRoot, journal, authority, fault); err != nil {
		return err
	}
	return semanticFault(fault, "after_journal_prepared", -1)
}

func readSemanticPlanningManagedFile(
	ctx context.Context,
	roots RootSet,
	authority *JournalNamespaceAuthority,
	stageRoot string,
	stageIdentity string,
	planningIdentity string,
	stageRoots RootSet,
	label string,
	expected domainstartup.SemanticEntryStateV1,
) ([]byte, error) {
	if err := contextError(ctx); err != nil {
		return nil, err
	}
	if err := validateSemanticPlanningStage(roots, authority, stageRoot, stageIdentity, planningIdentity); err != nil {
		return nil, err
	}
	planningRoot, err := semanticPlanningRootForAuthority(roots, authority)
	if err != nil {
		return nil, err
	}
	planning, err := secureStartupOpenDirectory(authority.planningRoot, filepath.Base(planningRoot), planningIdentity)
	if err != nil {
		return nil, err
	}
	defer planning.Close()
	stage, err := planning.OpenDirectory(filepath.Base(stageRoot), stageIdentity)
	if err != nil {
		return nil, err
	}
	defer stage.Close()
	markerBody, _, err := stage.ReadFile("planning.json", maxSemanticPlanningBytes, false)
	if err != nil {
		return nil, err
	}
	marker, err := decodeSemanticPlanningMarker(markerBody)
	if err != nil {
		return nil, err
	}
	managedRoot, relative, err := managedRootRelative(stageRoots, label)
	if err != nil {
		return nil, err
	}
	rootName := "data"
	rootIdentity := marker.DataIdentity
	if canonicalPathKey(managedRoot) == canonicalPathKey(stageRoots.DurableDir) && canonicalPathKey(stageRoots.DataDir) != canonicalPathKey(stageRoots.DurableDir) {
		rootName = "durable"
		rootIdentity = marker.DurableIdentity
	}
	current, err := stage.OpenDirectory(rootName, rootIdentity)
	if err != nil {
		return nil, err
	}
	defer current.Close()
	body, err := readSemanticStageManagedFile(ctx, current, relative, expected)
	if err != nil {
		return nil, err
	}
	if err := validateSemanticPlanningStage(roots, authority, stageRoot, stageIdentity, planningIdentity); err != nil {
		return nil, err
	}
	return body, nil
}

func recoverSemanticJournal(ctx context.Context, roots RootSet, rootAuthority *RootAuthority, journalAuthority *JournalNamespaceAuthority, configurationDigest string) error {
	return recoverSemanticJournalWithHook(ctx, roots, rootAuthority, journalAuthority, configurationDigest, nil)
}

func recoverSemanticJournalWithHook(ctx context.Context, roots RootSet, rootAuthority *RootAuthority, journalAuthority *JournalNamespaceAuthority, configurationDigest string, fault func(string, int) error) error {
	return recoverSemanticJournalWithPreservationV1(ctx, roots, rootAuthority, journalAuthority, configurationDigest, fault, nil)
}

func recoverSemanticJournalWithPreservationV1(ctx context.Context, roots RootSet, rootAuthority *RootAuthority, journalAuthority *JournalNamespaceAuthority, configurationDigest string, fault func(string, int) error, preserved SemanticRestartPreservationV1,
	originals ...privatecasport.OriginalCreateResiduesV1,
) error {
	if rootAuthority == nil || rootAuthority.Validate() != nil || journalAuthority == nil || journalAuthority.Validate() != nil || !domainsecurity.IsSHA256Hex(configurationDigest) {
		return errors.New("semantic startup recovery authority is invalid")
	}
	if err := contextError(ctx); err != nil {
		return err
	}
	journalRoot, err := semanticJournalRootForAuthority(roots, journalAuthority)
	if err != nil {
		return err
	}
	if err := preflightCurrentSemanticJournalForCleanupWithPreservationV1(ctx, roots, journalAuthority, journalRoot, configurationDigest, preserved, originals...); err != nil {
		return err
	}
	if err := recoverRetiredSemanticJournalsContext(ctx, journalAuthority, roots, originals...); err != nil {
		return err
	}
	journalDirectory, err := secureStartupOpenDirectory(journalAuthority.root, filepath.Base(journalRoot), "")
	if errors.Is(err, os.ErrNotExist) {
		_, cleanupErr := cleanBenignStartupAuthorityCreateResidue(journalAuthority, journalRoot)
		return cleanupErr
	}
	if err != nil {
		return err
	}
	journalRootIdentity := journalDirectory.Identity()
	defer journalDirectory.Close()
	authority, authorityErr := loadStartupJournalAuthority(journalAuthority, journalRoot)
	if authorityErr != nil {
		return errors.New("semantic startup journal authority is missing or invalid")
	}
	if _, _, retirementErr := journalDirectory.ReadFile(semanticJournalRetirementFile, maxSemanticRetirementBytes, false); retirementErr == nil {
		if err := verifySemanticRetirementDirectoryContext(ctx, journalDirectory, authority, roots, originals...); err != nil {
			return err
		}
		if err := journalDirectory.Close(); err != nil {
			return err
		}
		return retireSemanticJournalContext(ctx, journalAuthority, journalRootIdentity, roots, configurationDigest, fault, originals...)
	} else if !errors.Is(retirementErr, os.ErrNotExist) {
		return retirementErr
	}
	journal, err := readSemanticJournalWithHook(journalRoot, authority, fault)
	if errors.Is(err, os.ErrNotExist) {
		entries, readErr := journalDirectory.ReadEntriesBoundedContext(ctx, maxSemanticJournalResidue)
		if readErr != nil {
			return readErr
		}
		journalEntries := entries[:0]
		for _, entry := range entries {
			if entry.Name() != semanticJournalRetirementFile {
				journalEntries = append(journalEntries, entry)
			}
		}
		if len(journalEntries) == 0 {
			if err := journalDirectory.Close(); err != nil {
				return err
			}
			return retireSemanticJournalContext(ctx, journalAuthority, journalRootIdentity, roots, configurationDigest, fault, originals...)
		}
		if len(journalEntries) != 1 {
			return errors.New("semantic startup journal is missing with ambiguous residue")
		}
		entry := journalEntries[0]
		if !validSemanticJournalTempName(entry) {
			return errors.New("semantic startup journal is missing with unknown residue")
		}
		body, _, readErr := journalDirectory.ReadFile(entry.Name(), maxSemanticJournalBytes, false)
		if readErr != nil {
			return errors.New("semantic startup journal temp is unreadable")
		}
		candidate, parseErr := decodeSemanticJournalForRetirement(journalDirectory, body, authority)
		if parseErr != nil {
			return parseErr
		}
		if candidate.State != semanticJournalPreparing || candidate.NextOperation != 0 ||
			candidate.RootBindingDigest != rootBindingDigest(roots) || candidate.Plan.ConfigurationDigest != configurationDigest ||
			validateRecordedRootCapabilities(roots, candidate.RootCapabilities, candidate.RootCapabilityDigest) != nil {
			return errors.New("semantic startup journal temp is not a rollback-safe signed prepare")
		}
		if journalRootIdentity != candidate.JournalRootIdentity {
			return errors.New("semantic startup journal temp root identity changed")
		}
		candidateAuthority, authorityErr := restoreRootAuthorityFromJournal(roots, candidate)
		if authorityErr != nil {
			return authorityErr
		}
		if err := validateSemanticJournalCurrentStateContext(ctx, roots, candidateAuthority, candidate, originals...); err != nil {
			return err
		}
		if err := journalDirectory.Close(); err != nil {
			return err
		}
		return retireSemanticJournalContext(ctx, journalAuthority, candidate.JournalRootIdentity, roots, configurationDigest, fault, originals...)
	}
	if err != nil {
		return err
	}
	if journal.RootBindingDigest != rootBindingDigest(roots) ||
		validateRootCapabilityInventory(roots, journal.RootCapabilities, journal.RootCapabilityDigest) != nil ||
		journal.Plan.ConfigurationDigest != configurationDigest {
		return errors.New("semantic startup journal belongs to different persistence roots")
	}
	if journalRootIdentity != journal.JournalRootIdentity {
		return errors.New("semantic startup journal root identity changed")
	}
	switch journal.State {
	case semanticJournalPreparing:
		if err := validateSemanticJournalCurrentStateContext(ctx, roots, rootAuthority, journal, originals...); err != nil {
			return err
		}
		if err := journalDirectory.Close(); err != nil {
			return err
		}
		return retireSemanticJournalContext(ctx, journalAuthority, journal.JournalRootIdentity, roots, configurationDigest, fault, originals...)
	case semanticJournalPrepared, semanticJournalApplying, semanticJournalCommitted:
		return applySemanticJournalWithPreservationV1(ctx, journalRoot, roots, rootAuthority, journalAuthority, configurationDigest, fault, preserved, originals...)
	default:
		return errors.New("semantic startup journal state is unknown")
	}
}

func preflightCurrentSemanticJournalForCleanup(
	ctx context.Context,
	roots RootSet,
	journalAuthority *JournalNamespaceAuthority,
	journalRoot string,
	configurationDigest string,
) error {
	return preflightCurrentSemanticJournalForCleanupWithPreservationV1(ctx, roots, journalAuthority, journalRoot, configurationDigest, nil)
}

func preflightCurrentSemanticJournalForCleanupWithPreservationV1(ctx context.Context, roots RootSet, journalAuthority *JournalNamespaceAuthority, journalRoot string, configurationDigest string, preserved SemanticRestartPreservationV1,
	originals ...privatecasport.OriginalCreateResiduesV1,
) error {
	if err := contextError(ctx); err != nil {
		return err
	}
	directory, err := secureStartupOpenDirectory(journalAuthority.root, filepath.Base(journalRoot), "")
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer directory.Close()
	authority, err := loadStartupJournalAuthority(journalAuthority, journalRoot)
	if err != nil {
		return errors.New("semantic startup journal authority is missing or invalid")
	}
	if retirementBody, _, retirementErr := directory.ReadFile(semanticJournalRetirementFile, maxSemanticRetirementBytes, false); retirementErr == nil {
		if _, _, err := decodeSemanticRetirementForDirectory(retirementBody, directory, authority, roots); err != nil {
			return err
		}
	} else if !errors.Is(retirementErr, os.ErrNotExist) {
		return retirementErr
	}
	body, _, err := directory.ReadFile("journal.json", maxSemanticJournalBytes, false)
	if errors.Is(err, os.ErrNotExist) {
		entries, entriesErr := directory.ReadEntriesBoundedContext(ctx, maxSemanticJournalResidue)
		if entriesErr != nil {
			return entriesErr
		}
		visible := entries[:0]
		for _, entry := range entries {
			if entry.Name() != semanticJournalRetirementFile {
				visible = append(visible, entry)
			}
		}
		if len(visible) == 0 {
			return nil
		}
		if len(visible) != 1 || !validSemanticJournalTempName(visible[0]) {
			return errors.New("semantic startup journal is missing with ambiguous residue")
		}
		body, _, err = directory.ReadFile(visible[0].Name(), maxSemanticJournalBytes, false)
		if err != nil {
			return errors.New("semantic startup journal temp is unreadable")
		}
		candidate, decodeErr := decodeSemanticJournalForRetirement(directory, body, authority)
		if decodeErr != nil {
			return decodeErr
		}
		if candidate.State != semanticJournalPreparing || candidate.NextOperation != 0 ||
			candidate.JournalRootIdentity != directory.Identity() || candidate.RootBindingDigest != rootBindingDigest(roots) ||
			candidate.Plan.ConfigurationDigest != configurationDigest ||
			validateRecordedRootCapabilities(roots, candidate.RootCapabilities, candidate.RootCapabilityDigest) != nil {
			return errors.New("semantic startup journal temp is not a rollback-safe signed prepare")
		}
		rootAuthority, restoreErr := restoreRootAuthorityFromJournal(roots, candidate)
		if restoreErr != nil {
			return restoreErr
		}
		return validateSemanticJournalCurrentStateContext(ctx, roots, rootAuthority, candidate, originals...)
	}
	if err != nil {
		return err
	}
	journal, err := decodeSemanticJournalForRetirement(directory, body, authority)
	if err != nil {
		return err
	}
	if journal.JournalRootIdentity != directory.Identity() || journal.RootBindingDigest != rootBindingDigest(roots) ||
		journal.Plan.ConfigurationDigest != configurationDigest ||
		validateRootCapabilityInventory(roots, journal.RootCapabilities, journal.RootCapabilityDigest) != nil {
		return errors.New("semantic startup journal belongs to different persistence roots")
	}
	rootAuthority, err := restoreRootAuthorityFromJournal(roots, journal)
	if err != nil {
		return err
	}
	if err := validateSemanticJournalCurrentStateContext(ctx, roots, rootAuthority, journal, originals...); err != nil {
		return err
	}
	if journal.State != semanticJournalPreparing {
		if err := validateSemanticJournalStageContext(ctx, journalRoot, journalAuthority, journal); err != nil {
			return err
		}
		_, err := validateSemanticJournalRestartPreservationV1(ctx, preserved, journalRoot, journalAuthority, roots, rootAuthority, journal)
		return err
	}
	return nil
}

func applySemanticJournal(ctx context.Context, journalRoot string, roots RootSet, rootAuthority *RootAuthority, journalAuthority *JournalNamespaceAuthority, configurationDigest string) error {
	return applySemanticJournalWithHook(ctx, journalRoot, roots, rootAuthority, journalAuthority, configurationDigest, nil)
}

func applySemanticJournalWithHook(ctx context.Context, journalRoot string, roots RootSet, rootAuthority *RootAuthority, journalAuthority *JournalNamespaceAuthority, configurationDigest string, fault func(string, int) error) error {
	return applySemanticJournalWithPreservationV1(ctx, journalRoot, roots, rootAuthority, journalAuthority, configurationDigest, fault, nil)
}

func applySemanticJournalWithPreservationV1(ctx context.Context, journalRoot string, roots RootSet, rootAuthority *RootAuthority, journalAuthority *JournalNamespaceAuthority, configurationDigest string, fault func(string, int) error, preserved SemanticRestartPreservationV1,
	originals ...privatecasport.OriginalCreateResiduesV1,
) error {
	if rootAuthority == nil || rootAuthority.Validate() != nil || journalAuthority == nil || journalAuthority.Validate() != nil || !domainsecurity.IsSHA256Hex(configurationDigest) {
		return errors.New("semantic startup apply authority is invalid")
	}
	authority, err := loadStartupJournalAuthority(journalAuthority, journalRoot)
	if err != nil {
		return errors.New("semantic startup journal authority is missing or invalid")
	}
	journal, err := readSemanticJournalWithHook(journalRoot, authority, fault)
	if err != nil {
		return err
	}
	if journal.RootBindingDigest != rootBindingDigest(roots) ||
		validateRootCapabilityInventory(roots, journal.RootCapabilities, journal.RootCapabilityDigest) != nil ||
		journal.Plan.ConfigurationDigest != configurationDigest || domainstartup.ValidateSemanticStartupPlanV1(journal.Plan) != nil {
		return errors.New("semantic startup journal authority is invalid")
	}
	incomingAuthority := rootAuthority
	restoredAuthority, err := restoreRootAuthorityFromJournal(roots, journal)
	if err != nil {
		return err
	}
	rootAuthority = restoredAuthority
	if incomingAuthority.Digest() == journal.RootCapabilityDigest {
		rootAuthority = incomingAuthority
	}
	if journal.State == semanticJournalPreparing {
		return errors.New("semantic startup journal is not fully staged")
	}
	if err := validateSemanticJournalStageContext(ctx, journalRoot, journalAuthority, journal); err != nil {
		return err
	}
	if err := validateSemanticJournalCurrentStateContext(ctx, roots, rootAuthority, journal, originals...); err != nil {
		return err
	}
	noWriteOperationID, err := validateSemanticJournalRestartPreservationV1(ctx, preserved, journalRoot, journalAuthority, roots, rootAuthority, journal)
	if err != nil {
		return err
	}
	if err := ensureSemanticRootsForOperations(roots, rootAuthority, journal.Plan.Operations); err != nil {
		return err
	}
	if err := semanticFault(fault, "after_cold_root_create_before_promotion", -1); err != nil {
		return err
	}
	promotedAuthority, err := FreezeRootAuthority(roots)
	if err != nil {
		return err
	}
	if promotedAuthority.Digest() != journal.RootCapabilityDigest {
		journal.RootCapabilityDigest = promotedAuthority.Digest()
		journal.RootCapabilities = promotedAuthority.bindings()
		journal.RootPromotionIntents = rootPromotionIntents(journal.RootCapabilities)
		journal.JournalDigest = semanticJournalDigest(journal)
		if err := writeSemanticJournalWithHook(journalRoot, journal, authority, fault); err != nil {
			return err
		}
	}
	rootAuthority = promotedAuthority
	if journal.State != semanticJournalCommitted {
		journal.State = semanticJournalApplying
		journal.JournalDigest = semanticJournalDigest(journal)
		if err := writeSemanticJournalWithHook(journalRoot, journal, authority, fault); err != nil {
			return err
		}
		for index := journal.NextOperation; index < len(journal.Plan.Operations); index++ {
			operation := journal.Plan.Operations[index]
			if err := contextError(ctx); err != nil {
				return err
			}
			if err := semanticFault(fault, "before_operation", index); err != nil {
				return err
			}
			if operation.OperationID == noWriteOperationID {
				root, relative, err := managedRootRelative(roots, operation.Path)
				if err != nil {
					return err
				}
				after, err := secureManagedTargetMatches(rootAuthority, root, relative, operation.After)
				if err != nil {
					return err
				}
				if !after {
					return errors.New("semantic preserved no-write cursor changed before execution")
				}
			} else {
				if err := applySemanticOperation(journalRoot, journalAuthority, roots, rootAuthority, journal.JournalRootIdentity, journal.JournalStageIdentity, operation); err != nil {
					return err
				}
			}
			if err := semanticFault(fault, "after_operation", index); err != nil {
				return err
			}
			journal.NextOperation = index + 1
			journal.JournalDigest = semanticJournalDigest(journal)
			if err := writeSemanticJournalWithHook(journalRoot, journal, authority, fault); err != nil {
				return err
			}
			if err := semanticFault(fault, "after_operation_journal", index); err != nil {
				return err
			}
		}
	}
	final, err := captureStrictWithRootAuthority(ctx, roots, rootAuthority, originals...)
	if err != nil || semanticStateDigest(final.Entries) != journal.Plan.FinalStateDigest {
		return errors.Join(errors.New("semantic startup journal final readback is inconsistent"), err)
	}
	journal.State = semanticJournalCommitted
	journal.NextOperation = len(journal.Plan.Operations)
	journal.JournalDigest = semanticJournalDigest(journal)
	if err := semanticFault(fault, "before_commit", len(journal.Plan.Operations)); err != nil {
		return err
	}
	if err := writeSemanticJournalWithHook(journalRoot, journal, authority, fault); err != nil {
		return err
	}
	if err := semanticFault(fault, "after_commit", len(journal.Plan.Operations)); err != nil {
		return err
	}
	return retireSemanticJournalContext(ctx, journalAuthority, journal.JournalRootIdentity, roots, configurationDigest, fault, originals...)
}

func validateSemanticJournalCurrentState(roots RootSet, rootAuthority *RootAuthority, journal semanticJournalV1) error {
	return validateSemanticJournalCurrentStateContext(context.Background(), roots, rootAuthority, journal)
}

func validateSemanticJournalCurrentStateContext(ctx context.Context, roots RootSet, rootAuthority *RootAuthority, journal semanticJournalV1,
	originals ...privatecasport.OriginalCreateResiduesV1,
) error {
	if domainstartup.ValidateSemanticStartupPlanV1(journal.Plan) != nil {
		return errors.New("semantic startup recovery plan is invalid")
	}
	current, err := captureStrictWithRootAuthorityAndResidues(
		ctx, roots, rootAuthority, semanticJournalPrivateResidueAllowlist(journal), originals...)
	if err != nil {
		return err
	}
	if journal.State == semanticJournalPreparing || journal.State == semanticJournalPrepared {
		snapshot, err := managedSnapshotFromRaw(current)
		if err != nil || snapshot.SnapshotDigest != journal.Plan.BaselineDigest {
			return errors.New("semantic startup recovery baseline drifted before apply")
		}
		return nil
	}
	currentStates := semanticStates(current.Entries)
	if journal.State == semanticJournalApplying && journal.NextOperation < len(journal.Plan.Operations) {
		operation := journal.Plan.Operations[journal.NextOperation]
		if operation.Kind == domainstartup.SemanticOperationInstallFile {
			tempLabel := semanticInstallTempLabel(operation)
			if temp, found := currentStates[tempLabel]; found {
				if !semanticStateMatches(currentStates, operation.Path, operation.Before) ||
					temp.Type != domainstartup.ManagedEntryTypeFile || temp.Size < 0 || temp.Size > operation.After.Size {
					return errors.New("semantic startup recovery install temp is unsafe")
				}
				delete(currentStates, tempLabel)
			}
		}
	}
	for index, operation := range journal.Plan.Operations {
		before := semanticStateMatches(currentStates, operation.Path, operation.Before)
		after := semanticStateMatches(currentStates, operation.Path, operation.After)
		requireAfter := journal.State == semanticJournalCommitted || journal.State == semanticJournalApplying && index < journal.NextOperation
		allowEither := journal.State == semanticJournalApplying && index == journal.NextOperation
		if (requireAfter && !after) || (allowEither && !before && !after) || (!requireAfter && !allowEither && !before) {
			return errors.New("semantic startup recovery operation cursor does not match managed state")
		}
	}
	projected := make(map[string]domainstartup.SemanticEntryStateV1, len(currentStates)+len(journal.Plan.Operations))
	for path, state := range currentStates {
		projected[path] = state
	}
	for _, operation := range journal.Plan.Operations {
		if operation.After.Type == domainstartup.ManagedEntryTypeAbsent && !semanticSnapshotAnchorPath(operation.Path) {
			delete(projected, operation.Path)
		} else {
			projected[operation.Path] = operation.After
		}
	}
	if semanticStateMapDigest(projected) != journal.Plan.FinalStateDigest {
		return errors.New("semantic startup recovery baseline drifted before apply")
	}
	return nil
}

func semanticJournalPrivateResidueAllowlist(journal semanticJournalV1) map[string]struct{} {
	allowed := map[string]struct{}{}
	for _, operation := range journal.Plan.Operations {
		if operation.Kind == domainstartup.SemanticOperationRemoveFile &&
			operation.Before.Type == domainstartup.ManagedEntryTypeFile &&
			operation.After.Type == domainstartup.ManagedEntryTypeAbsent &&
			privateAuthorityJournalRemovableResidue(operation.Path) {
			allowed[operation.Path] = struct{}{}
		}
	}
	return allowed
}

func validateSemanticJournalStage(journalRoot string, namespace *JournalNamespaceAuthority, journal semanticJournalV1) error {
	return validateSemanticJournalStageContext(context.Background(), journalRoot, namespace, journal)
}

func validateSemanticJournalStageContext(ctx context.Context, journalRoot string, namespace *JournalNamespaceAuthority, journal semanticJournalV1) error {
	if err := contextError(ctx); err != nil {
		return err
	}
	if namespace == nil || filepath.Dir(journalRoot) != namespace.path() {
		return errors.New("semantic startup journal namespace is invalid")
	}
	journalDirectory, err := secureStartupOpenDirectory(namespace.root, filepath.Base(journalRoot), journal.JournalRootIdentity)
	if err != nil {
		return errors.New("semantic startup journal root identity changed")
	}
	defer journalDirectory.Close()
	stageDirectory, err := journalDirectory.OpenDirectory("stage", journal.JournalStageIdentity)
	if err != nil {
		return errors.New("semantic startup journal stage is invalid")
	}
	defer stageDirectory.Close()
	expected := make(map[string]domainstartup.SemanticStartupOperationV1)
	for _, operation := range journal.Plan.Operations {
		if operation.Kind == domainstartup.SemanticOperationInstallFile {
			expected[operation.OperationID] = operation
		}
	}
	entries, err := stageDirectory.ReadEntriesBoundedContext(ctx, len(expected))
	if err != nil || len(entries) != len(expected) {
		return errors.New("semantic startup journal stage inventory is incomplete")
	}
	for _, entry := range entries {
		if err := contextError(ctx); err != nil {
			return err
		}
		operation, found := expected[entry.Name()]
		if !found || entry.IsDir() {
			return errors.New("semantic startup journal stage blob is invalid")
		}
		body, _, readErr := stageDirectory.ReadFile(entry.Name(), semanticFileSizeLimit(operation.After.Size), true)
		if readErr != nil || int64(len(body)) != operation.After.Size || domainsecurity.SHA256Hex(body) != operation.After.SHA256 {
			return errors.New("semantic startup journal stage blob lost integrity")
		}
	}
	return nil
}

func semanticStateMatches(states map[string]domainstartup.SemanticEntryStateV1, path string, expected domainstartup.SemanticEntryStateV1) bool {
	actual, found := states[path]
	if !found {
		actual = domainstartup.SemanticEntryStateV1{Type: domainstartup.ManagedEntryTypeAbsent}
	}
	return actual == expected
}

func semanticInstallTempLabel(operation domainstartup.SemanticStartupOperationV1) string {
	separator := strings.LastIndex(operation.Path, "/")
	if separator < 0 {
		return ""
	}
	return operation.Path[:separator+1] + "." + operation.Path[separator+1:] + ".startup-" + operation.OperationID + ".tmp"
}

func semanticSnapshotAnchorPath(path string) bool {
	switch path {
	case "durable/durable-meta.json", "durable/thread_summaries.jsonl", "durable/threads", "durable/runtime-go/threads", "durable/usage_events",
		"data/private", "data/memory", "data/child-runs", "data/attachments", "data/mcp-schema-cache":
		return true
	default:
		return false
	}
}

func semanticFault(hook func(string, int) error, stage string, operation int) error {
	if hook == nil {
		return nil
	}
	return hook(stage, operation)
}

func applySemanticOperation(journalRoot string, namespace *JournalNamespaceAuthority, roots RootSet, rootAuthority *RootAuthority, journalRootIdentity, journalStageIdentity string, operation domainstartup.SemanticStartupOperationV1) error {
	root, relative, err := managedRootRelative(roots, operation.Path)
	if err != nil {
		return err
	}
	if after, err := secureManagedTargetMatches(rootAuthority, root, relative, operation.After); err != nil {
		return err
	} else if after {
		return nil
	}
	before, err := secureManagedTargetMatches(rootAuthority, root, relative, operation.Before)
	if err != nil || !before {
		return errors.New("semantic startup target matches neither pre-state nor post-state")
	}
	switch operation.Kind {
	case domainstartup.SemanticOperationCreateDirectory:
		err = secureManagedCreateDirectory(rootAuthority, root, relative, os.FileMode(operation.After.Mode).Perm())
	case domainstartup.SemanticOperationInstallFile:
		if namespace == nil || filepath.Dir(journalRoot) != namespace.path() {
			return errors.New("semantic startup journal namespace is invalid")
		}
		journalDirectory, openErr := secureStartupOpenDirectory(namespace.root, filepath.Base(journalRoot), journalRootIdentity)
		if openErr != nil {
			return openErr
		}
		defer journalDirectory.Close()
		stageDirectory, openErr := journalDirectory.OpenDirectory("stage", journalStageIdentity)
		if openErr != nil {
			return openErr
		}
		defer stageDirectory.Close()
		data, _, err := stageDirectory.ReadFile(operation.OperationID, semanticFileSizeLimit(operation.After.Size), true)
		if err != nil || int64(len(data)) != operation.After.Size || domainsecurity.SHA256Hex(data) != operation.After.SHA256 {
			return errors.New("semantic startup journal blob is invalid")
		}
		err = secureManagedInstallFile(rootAuthority, root, relative, data, os.FileMode(operation.After.Mode).Perm(), operation.OperationID, operation.Before, operation.After)
	case domainstartup.SemanticOperationSetMode:
		err = secureManagedSetMode(rootAuthority, root, relative, operation.Before, os.FileMode(operation.After.Mode).Perm())
	case domainstartup.SemanticOperationRemoveFile, domainstartup.SemanticOperationRemoveDirectory:
		err = secureManagedRemove(rootAuthority, root, relative, operation.Before)
	default:
		return errors.New("semantic startup operation kind is unknown")
	}
	if err != nil {
		return err
	}
	matched, err := secureManagedTargetMatches(rootAuthority, root, relative, operation.After)
	if err != nil || !matched {
		return errors.New("semantic startup operation readback failed")
	}
	return nil
}

func semanticTargetMatches(roots RootSet, label string, expected domainstartup.SemanticEntryStateV1) (bool, error) {
	target, err := managedPath(roots, label)
	if err != nil {
		return false, err
	}
	return semanticPathMatches(target, expected)
}

func semanticPathMatches(target string, expected domainstartup.SemanticEntryStateV1) (bool, error) {
	info, err := os.Lstat(target)
	if errors.Is(err, os.ErrNotExist) {
		return expected.Type == domainstartup.ManagedEntryTypeAbsent, nil
	}
	if err != nil || info.Mode()&os.ModeSymlink != 0 {
		return false, err
	}
	if expected.Type == domainstartup.ManagedEntryTypeAbsent {
		return false, nil
	}
	if uint32(info.Mode()) != expected.Mode {
		return false, nil
	}
	switch expected.Type {
	case domainstartup.ManagedEntryTypeDirectory:
		return info.IsDir(), nil
	case domainstartup.ManagedEntryTypeFile:
		if !info.Mode().IsRegular() || info.Size() != expected.Size {
			return false, nil
		}
		file, err := os.Open(target)
		if err != nil {
			return false, err
		}
		hasher := sha256.New()
		written, copyErr := io.Copy(hasher, file)
		closeErr := file.Close()
		if copyErr != nil || closeErr != nil || written != expected.Size {
			return false, errors.Join(copyErr, closeErr)
		}
		return hex.EncodeToString(hasher.Sum(nil)) == expected.SHA256, nil
	default:
		return false, errors.New("semantic startup expected state is invalid")
	}
}

func managedPath(roots RootSet, label string) (string, error) {
	root, relative, err := managedRootRelative(roots, label)
	if err != nil {
		return "", err
	}
	target := filepath.Join(root, relative)
	rel, err := filepath.Rel(root, target)
	if err != nil || rel == "." || filepath.IsAbs(rel) || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", errors.New("semantic startup managed path is invalid")
	}
	return target, nil
}

func managedRootRelative(roots RootSet, label string) (string, string, error) {
	var root, relative string
	switch {
	case strings.HasPrefix(label, "data/"):
		root, relative = roots.DataDir, strings.TrimPrefix(label, "data/")
	case strings.HasPrefix(label, "durable/"):
		root, relative = roots.DurableDir, strings.TrimPrefix(label, "durable/")
	default:
		return "", "", errors.New("semantic startup path is outside managed roots")
	}
	relative = filepath.FromSlash(relative)
	if relative == "" || filepath.IsAbs(relative) || relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", "", errors.New("semantic startup managed path is invalid")
	}
	return root, relative, nil
}

func ensureSemanticRootsForOperations(roots RootSet, authority *RootAuthority, operations []domainstartup.SemanticStartupOperationV1) error {
	needed := map[string]bool{}
	for _, operation := range operations {
		root, _, err := managedRootRelative(roots, operation.Path)
		if err != nil {
			return err
		}
		needed[canonicalPathKey(root)] = true
	}
	for _, root := range CanonicalRoots(roots) {
		if !needed[canonicalPathKey(root)] {
			continue
		}
		if err := secureEnsureManagedRoot(authority, root); err != nil {
			return err
		}
	}
	return nil
}

func semanticJournalDigest(journal semanticJournalV1) string {
	body := struct {
		SchemaVersion        int                     `json:"schemaVersion"`
		State                string                  `json:"state"`
		RootBindingDigest    string                  `json:"rootBindingDigest"`
		RootCapabilityDigest string                  `json:"rootCapabilityDigest"`
		RootCapabilities     []frozenRootCapability  `json:"rootCapabilities"`
		RootPromotionIntents []rootPromotionIntentV1 `json:"rootPromotionIntents,omitempty"`
		PlanDigest           string                  `json:"planDigest"`
		NextOperation        int                     `json:"nextOperation"`
		JournalRootIdentity  string                  `json:"journalRootIdentity"`
		JournalStageIdentity string                  `json:"journalStageIdentity,omitempty"`
		AuthorityKeyID       string                  `json:"authorityKeyId"`
	}{
		journal.SchemaVersion, journal.State, journal.RootBindingDigest, journal.RootCapabilityDigest, journal.RootCapabilities, journal.RootPromotionIntents,
		journal.Plan.PlanDigest, journal.NextOperation, journal.JournalRootIdentity, journal.JournalStageIdentity, journal.AuthorityKeyID,
	}
	encoded, _ := json.Marshal(body)
	return domainsecurity.SHA256Hex(encoded)
}

func validateSemanticJournal(journal semanticJournalV1, authority *startupJournalAuthority) error {
	if journal.SchemaVersion != semanticJournalSchemaVersion || !domainsecurity.IsSHA256Hex(journal.RootBindingDigest) ||
		!domainsecurity.IsSHA256Hex(journal.RootCapabilityDigest) || strings.TrimSpace(journal.JournalRootIdentity) == "" ||
		!domainsecurity.IsSHA256Hex(journal.AuthorityKeyID) || strings.TrimSpace(journal.AuthoritySignature) == "" ||
		domainstartup.ValidateSemanticStartupPlanV1(journal.Plan) != nil || journal.NextOperation < 0 ||
		journal.NextOperation > len(journal.Plan.Operations) || !domainsecurity.IsSHA256Hex(journal.JournalDigest) ||
		semanticJournalDigest(journal) != journal.JournalDigest || validateRootPromotionIntents(journal.RootCapabilities, journal.RootPromotionIntents) != nil || authority == nil ||
		authority.verify(journal.AuthorityKeyID, semanticJournalSigningBytes(journal), journal.AuthoritySignature) != nil {
		return errors.New("semantic startup journal integrity is invalid")
	}
	if _, err := parseStrongDirectoryIdentity(journal.JournalRootIdentity); err != nil {
		return errors.New("semantic startup journal root identity is invalid")
	}
	if err := validateSemanticJournalState(journal); err != nil {
		return err
	}
	if journal.JournalStageIdentity != "" {
		if _, err := parseStrongDirectoryIdentity(journal.JournalStageIdentity); err != nil {
			return errors.New("semantic startup journal stage identity is invalid")
		}
	}
	return nil
}

func semanticJournalSigningBytes(journal semanticJournalV1) []byte {
	journal.AuthoritySignature = ""
	body, _ := json.Marshal(journal)
	return body
}

func readSemanticJournal(root string, authority *startupJournalAuthority) (semanticJournalV1, error) {
	return readSemanticJournalWithHook(root, authority, nil)
}

func readSemanticJournalWithHook(root string, authority *startupJournalAuthority, fault func(string, int) error) (semanticJournalV1, error) {
	if authority == nil || authority.namespace == nil || filepath.Dir(root) != authority.namespace.path() {
		return semanticJournalV1{}, errors.New("semantic startup journal read authority is invalid")
	}
	directory, err := secureStartupOpenDirectory(authority.namespace.root, filepath.Base(root), "")
	if err != nil {
		return semanticJournalV1{}, err
	}
	defer directory.Close()
	body, _, err := directory.ReadFile("journal.json", maxSemanticJournalBytes, false)
	if err != nil {
		return semanticJournalV1{}, err
	}
	journal, err := decodeSemanticJournal(body, authority)
	var legacy authenticatedSemanticJournalV3
	if errors.As(err, &legacy) {
		journal, err = migrateAuthenticatedSemanticJournalV3(directory, legacy.journal, authority, func(stage string) error {
			return semanticFault(fault, stage, -1)
		})
	}
	if err != nil {
		return semanticJournalV1{}, err
	}
	if journal.JournalRootIdentity != directory.Identity() {
		return semanticJournalV1{}, errors.New("semantic startup journal root changed during signed read")
	}
	return journal, nil
}

func decodeSemanticJournal(body []byte, authority *startupJournalAuthority) (semanticJournalV1, error) {
	if len(body) == 0 || len(body) > maxSemanticJournalBytes {
		return semanticJournalV1{}, StartupResourceLimitError{Code: "journal_bytes", Limit: maxSemanticJournalBytes}
	}
	if err := validateSemanticControlJSON(body, maxSemanticJournalBytes); err != nil {
		return semanticJournalV1{}, errors.New("semantic startup journal JSON is invalid")
	}
	var envelope struct {
		SchemaVersion int `json:"schemaVersion"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return semanticJournalV1{}, errors.New("semantic startup journal version is invalid")
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var journal semanticJournalV1
	if err := decoder.Decode(&journal); err != nil {
		return semanticJournalV1{}, err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return semanticJournalV1{}, errors.New("semantic startup journal contains trailing JSON")
	}
	switch envelope.SchemaVersion {
	case 3:
		if err := validateSemanticJournalV3(journal, authority); err != nil {
			return semanticJournalV1{}, err
		}
		return semanticJournalV1{}, authenticatedSemanticJournalV3{journal: journal, blocker: semanticJournalV3Blocker()}
	case semanticJournalSchemaVersion:
		if err := validateSemanticJournal(journal, authority); err != nil {
			return semanticJournalV1{}, err
		}
	default:
		return semanticJournalV1{}, errors.New("semantic startup journal schema version is unsupported")
	}
	return journal, nil
}

func writeSemanticJournal(root string, journal semanticJournalV1, authority *startupJournalAuthority) error {
	return writeSemanticJournalWithHook(root, journal, authority, nil)
}

func writeSemanticJournalWithHook(root string, journal semanticJournalV1, authority *startupJournalAuthority, fault func(string, int) error) error {
	journal.JournalDigest = semanticJournalDigest(journal)
	journal.AuthoritySignature = ""
	signature, err := authority.sign(semanticJournalSigningBytes(journal))
	if err != nil {
		return err
	}
	journal.AuthoritySignature = signature
	if err := validateSemanticJournal(journal, authority); err != nil {
		return err
	}
	body, err := json.Marshal(journal)
	if err != nil {
		return err
	}
	if authority == nil || authority.namespace == nil || filepath.Dir(root) != authority.namespace.path() {
		return errors.New("semantic startup journal write authority is invalid")
	}
	directory, err := secureStartupOpenDirectory(authority.namespace.root, filepath.Base(root), journal.JournalRootIdentity)
	if err != nil {
		return err
	}
	defer directory.Close()
	if err := directory.WriteReplace("journal.json", body, maxSemanticJournalBytes, func(stage string) error {
		return semanticFault(fault, stage, journal.NextOperation)
	}); err != nil {
		return err
	}
	if err := authority.revalidate(); err != nil {
		return err
	}
	return nil
}

func validSemanticJournalTempName(entry os.DirEntry) bool {
	name := entry.Name()
	random := strings.TrimSuffix(strings.TrimPrefix(name, semanticJournalTempPrefix), semanticJournalTempSuffix)
	if random == name || len(random) != 32 || !isLowerHex(random) || entry.IsDir() ||
		!strings.HasPrefix(name, semanticJournalTempPrefix) || !strings.HasSuffix(name, semanticJournalTempSuffix) {
		return false
	}
	return name == semanticJournalTempPrefix+random+semanticJournalTempSuffix && startupAuthorityNamedComponent(name)
}

func semanticFileSizeLimit(expected int64) int64 {
	if expected < 1 {
		return 1
	}
	return expected
}

func contextError(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	return ctx.Err()
}
