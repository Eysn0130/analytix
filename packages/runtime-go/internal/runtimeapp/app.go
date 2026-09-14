package runtimeapp

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	domainpendingwork "analytix.local/runtime-go/internal/domain/pendingwork"

	httpapi "analytix.local/runtime-go/internal/adapters/inbound/httpapi"
	authorityadvancefs "analytix.local/runtime-go/internal/adapters/outbound/authorityadvancefs"
	cachetelemetrystore "analytix.local/runtime-go/internal/adapters/outbound/cachetelemetrystore"
	casethreadauthority "analytix.local/runtime-go/internal/adapters/outbound/casethreadauthority"
	checkpointauthority "analytix.local/runtime-go/internal/adapters/outbound/checkpointauthority"
	continuationstore "analytix.local/runtime-go/internal/adapters/outbound/continuationstore"
	datasetsnapshotstore "analytix.local/runtime-go/internal/adapters/outbound/datasetsnapshot"
	evidenceregistry "analytix.local/runtime-go/internal/adapters/outbound/evidenceregistry"
	filestore "analytix.local/runtime-go/internal/adapters/outbound/filestore"
	finalauthority "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	fundscsvsourceadapter "analytix.local/runtime-go/internal/adapters/outbound/fundscsvsource"
	fundsquerysourceadapter "analytix.local/runtime-go/internal/adapters/outbound/fundsquerysource"
	mediaexecutiontransport "analytix.local/runtime-go/internal/adapters/outbound/mediaexecutiontransport"
	nativecomponenthost "analytix.local/runtime-go/internal/adapters/outbound/nativecomponenthost"
	pendingworkstore "analytix.local/runtime-go/internal/adapters/outbound/pendingworkstore"
	persistencefs "analytix.local/runtime-go/internal/adapters/outbound/persistencefs"
	piiauthorizationstore "analytix.local/runtime-go/internal/adapters/outbound/piiauthorization"
	processadapter "analytix.local/runtime-go/internal/adapters/outbound/process"
	providerclient "analytix.local/runtime-go/internal/adapters/outbound/provider/client"
	reportpublicationstore "analytix.local/runtime-go/internal/adapters/outbound/reportpublication"
	secretstore "analytix.local/runtime-go/internal/adapters/outbound/secretstore"
	threadriskpolicystore "analytix.local/runtime-go/internal/adapters/outbound/threadriskpolicy"
	turnterminalstore "analytix.local/runtime-go/internal/adapters/outbound/turnterminalstore"
	attachmentauthorityapp "analytix.local/runtime-go/internal/app/attachmentauthority"
	attachmentuseapp "analytix.local/runtime-go/internal/app/attachmentuse"
	authorityadvanceapp "analytix.local/runtime-go/internal/app/authorityadvance"
	cachetelemetryapp "analytix.local/runtime-go/internal/app/cachetelemetry"
	casethreadapp "analytix.local/runtime-go/internal/app/casethread"
	checkpointapp "analytix.local/runtime-go/internal/app/checkpoint"
	continuationapp "analytix.local/runtime-go/internal/app/continuation"
	controlapp "analytix.local/runtime-go/internal/app/control"
	evidenceapp "analytix.local/runtime-go/internal/app/evidence"
	executionpolicy "analytix.local/runtime-go/internal/app/executionpolicy"
	fundscleaningapp "analytix.local/runtime-go/internal/app/fundscleaning"
	fundscsvadmissionapp "analytix.local/runtime-go/internal/app/fundscsvadmission"
	gateprojection "analytix.local/runtime-go/internal/app/gateprojection"
	appidentity "analytix.local/runtime-go/internal/app/identity"
	localdisplayapp "analytix.local/runtime-go/internal/app/localdisplay"
	mediaexecutionapp "analytix.local/runtime-go/internal/app/mediaexecution"
	nativecomponentapp "analytix.local/runtime-go/internal/app/nativecomponent"
	pendingworkapp "analytix.local/runtime-go/internal/app/pendingwork"
	piiauthorizationapp "analytix.local/runtime-go/internal/app/piiauthorization"
	reportpublicationapp "analytix.local/runtime-go/internal/app/reportpublication"
	startupapp "analytix.local/runtime-go/internal/app/startup"
	steeringauthorityapp "analytix.local/runtime-go/internal/app/steeringauthority"
	subagentapp "analytix.local/runtime-go/internal/app/subagent"
	threadapp "analytix.local/runtime-go/internal/app/thread"
	appturn "analytix.local/runtime-go/internal/app/turn"
	turnsecurityapp "analytix.local/runtime-go/internal/app/turnsecurity"
	turnterminalapp "analytix.local/runtime-go/internal/app/turnterminal"
	domainevent "analytix.local/runtime-go/internal/domain/event"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainjob "analytix.local/runtime-go/internal/domain/job"
	domainnative "analytix.local/runtime-go/internal/domain/nativecomponent"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	jobs "analytix.local/runtime-go/internal/jobs"
	mcp "analytix.local/runtime-go/internal/mcp"
	authorityadvanceport "analytix.local/runtime-go/internal/ports/authorityadvance"
	datasetsnapshotport "analytix.local/runtime-go/internal/ports/datasetsnapshot"
	evidenceregistryport "analytix.local/runtime-go/internal/ports/evidenceregistry"
	finalauthorityport "analytix.local/runtime-go/internal/ports/finalauthority"
	nativecomponentport "analytix.local/runtime-go/internal/ports/nativecomponent"
	privatecasport "analytix.local/runtime-go/internal/ports/privatecas"
	startupport "analytix.local/runtime-go/internal/ports/startup"
	provider "analytix.local/runtime-go/internal/provider"
	research "analytix.local/runtime-go/internal/research"
	"analytix.local/runtime-go/internal/server"
)

type Config = server.RuntimeServerConfig

const DefaultRuntimeToken = server.DefaultRuntimeToken

type runtimeEvidenceRegistryAuthority interface {
	evidenceregistryport.Registry
	evidenceregistryport.LockedSnapshot
	evidenceregistryport.HistoricalReplay
	evidenceregistryport.Inventory
}

// Private composition-only observation for complete asynchronous operations.
// No configuration or public request can supply this callback.
type asyncTurnObservationContextKeyV1 struct{}
type asyncTurnPhaseObservationContextKeyV1 struct{}

func NewRuntimeServerHandler(config Config) http.Handler {
	handler, err := NewRuntimeServerHandlerE(config)
	if err != nil {
		panic(err)
	}
	return handler
}

func NewRuntimeServerHandlerE(config Config) (http.Handler, error) {
	return NewRuntimeServerHandlerWithOwnedPersistenceLeaseE(config)
}

func NewRuntimeServerHandlerWithPersistenceLeaseE(config Config, lease *persistencefs.CompositeLease) (http.Handler, error) {
	return newRuntimeServerHandlerWithPersistenceLeaseContextE(context.Background(), config, lease)
}

func newRuntimeServerHandlerWithPersistenceLeaseContextE(ctx context.Context, config Config, lease *persistencefs.CompositeLease) (http.Handler, error) {
	semanticValidation := persistencefs.NewStartupSemanticValidationAttemptV1()
	startupContext := semanticValidation.WithContext(ctx)
	config = normalizeConfig(config)
	if err := validateRuntimeProtectedRootTopology(config); err != nil {
		return nil, err
	}
	if err := validateRuntimeAuthentication(config); err != nil {
		return nil, err
	}
	currentRoots, err := resolveRuntimePersistenceRoots(config)
	if err != nil {
		return nil, err
	}
	frozenRoots, held := lease.FrozenRoots()
	if !held || currentRoots != frozenRoots {
		return nil, errors.New("runtime persistence roots changed after lease acquisition")
	}
	if err := lease.ValidateStartupUserData(config.UserDataDir); err != nil {
		return nil, err
	}
	rootAuthority, held := lease.FrozenAuthority()
	if !held {
		return nil, errors.New("runtime persistence root authority changed after lease acquisition")
	}
	originalCreates, err := prepareRuntimeOriginalCreateStartupV1(startupContext, currentRoots, lease)
	if err != nil {
		return nil, err
	}
	journalAuthority, err := ensureRuntimeJournalAuthorityAfterManagedPreflight(
		startupContext, currentRoots, lease, originalCreates,
	)
	if err != nil {
		return nil, err
	}
	runtimeInfoDataDir := config.DataDir
	config = applyRuntimePersistenceRoots(config, frozenRoots)
	activation, err := lease.BeginActivation()
	if err != nil {
		return nil, err
	}
	succeeded := false
	defer func() { activation.Complete(succeeded) }()
	handler, err := newRuntimeServerHandlerWithRootsModeE(
		startupContext, config, frozenRoots, rootAuthority, journalAuthority, lease, true, false, "", runtimeInfoDataDir, nil, true, nil, originalCreates,
	)
	if err != nil {
		return nil, err
	}
	succeeded = true
	return handler, nil
}

func newRuntimeServerHandlerWithRootsModeE(
	ctx context.Context,
	config Config,
	persistenceRoots persistencefs.RootSet,
	rootAuthority *persistencefs.RootAuthority,
	journalAuthority *persistencefs.JournalNamespaceAuthority,
	privateCASAccessAuthority finalauthority.SecurePrivateCASRecoveryAccessAuthority,
	planStartup bool,
	simulation bool,
	expectedConfigurationDigest string,
	runtimeInfoDataDir string,
	planOutput *runtimeStartupPlanOutputV1,
	activateAfterPlan bool,
	configurationSnapshot *filestore.RuntimeConfigSnapshotV1,
	originalCreates *runtimeOriginalCreateStartupV1,
	inheritedChildFloors ...domainpendingwork.ChildIdentityFloorsV1,
) (_ http.Handler, resultErr error) {
	startupPhase := "configuration"
	if simulation {
		defer func() {
			if resultErr != nil {
				resultErr = fmt.Errorf("semantic startup phase %s: %w", startupPhase, resultErr)
			}
		}()
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := validateControlledArtifactHostConfigV2(config); err != nil {
		return nil, err
	}
	providerConfig := provider.NewRuntimeProviderConfigSet(provider.RuntimeProviderConfigInput{
		DefaultProviderID:     config.ProviderID,
		DefaultBaseURL:        config.BaseURL,
		DefaultAPIKey:         config.APIKey,
		DefaultEndpointFormat: config.EndpointFormat,
		DefaultModel:          config.Model,
		ModelProvidersJSON:    config.ModelProvidersJSON,
	})
	if err := providerConfig.ConfigurationError(); err != nil {
		return nil, err
	}
	if rootAuthority == nil || rootAuthority.Validate() != nil || journalAuthority == nil || journalAuthority.Validate() != nil ||
		privateCASAccessAuthority == nil {
		return nil, errors.New("runtime startup persistence authority is invalid")
	}
	if err := originalCreates.revalidateV1(ctx, persistenceRoots); err != nil {
		return nil, err
	}
	ctx = persistencefs.WithOriginalPlainResiduesV1(ctx, originalCreates.plainProofV1())
	var privateCASRecoveryJournal privatecasport.RecoveryJournalV1
	if planStartup {
		lease, ok := privateCASAccessAuthority.(*persistencefs.CompositeLease)
		if !ok {
			return nil, errors.New("runtime signed private CAS recovery requires the frozen composite lease")
		}
		constructedJournal, journalErr := persistencefs.NewPrivateCASRecoveryJournalV1(lease)
		if journalErr != nil {
			return nil, fmt.Errorf("construct runtime signed private CAS recovery journal: %w", journalErr)
		}
		privateCASRecoveryJournal = constructedJournal
	}
	authorityDataDir := config.DataDir
	var startupSession startupapp.ReadOnlyPlanningSessionV1
	var err error
	var snapshot filestore.RuntimeConfigSnapshotV1
	var mcpSpecs []mcp.ServerSpec
	var document map[string]any
	var hasRuntimeConfig bool
	if configurationSnapshot == nil {
		snapshot, mcpSpecs, document, hasRuntimeConfig, err = loadRuntimeConfiguration(config)
		if err != nil {
			return nil, err
		}
		configurationSnapshot = &snapshot
	} else {
		snapshot = *configurationSnapshot
		mcpSpecs, err = loadRuntimeMCPServerSpecs(snapshot, config.DataDir)
		if err == nil {
			document, hasRuntimeConfig, err = loadRuntimeConfigDocumentFromSnapshot(snapshot)
		}
		if err != nil {
			return nil, err
		}
	}
	mcpSpecs = mcp.BindHostScheduleMCPServerV1(mcpSpecs, config.HostScheduleMCPServer)
	mcpSearch := loadRuntimeMCPSearchSettings(document, hasRuntimeConfig)
	skillCatalog, err := loadRuntimeSkillCatalog(config, document, hasRuntimeConfig)
	if err != nil {
		return nil, err
	}
	subagentConfig, err := loadRuntimeSubagentConfig(document, hasRuntimeConfig)
	if err != nil {
		return nil, err
	}
	sandboxSettings := loadRuntimeSandboxSettings(config, document, hasRuntimeConfig)
	webConfig := loadRuntimeWebConfig(document, hasRuntimeConfig)
	visionBridgeConfig := loadRuntimeVisionBridgeConfig(document, hasRuntimeConfig)
	// Provider/model transport fields in the legacy Vision Bridge config are
	// compatibility input only. Production execution resolves the selected
	// media route and credential JIT from the Provider Registry.
	visionBridgeConfig.ProviderID = ""
	visionBridgeConfig.BaseURL = ""
	visionBridgeConfig.APIKey = ""
	visionBridgeConfig.Model = ""
	visionBridgeConfig.EndpointFormat = ""
	stepLimits := loadRuntimeStepLimitConfig(document, hasRuntimeConfig)
	streamIdleTimeout := providerclient.DefaultStreamIdleTimeout
	if configuredTimeout, configured := loadRuntimeStreamIdleTimeout(document, hasRuntimeConfig); configured {
		streamIdleTimeout = configuredTimeout
	}
	configurationDigest, err := runtimeStartupConfigurationDigest(
		runtimeStartupSecurityConfiguration(config), mcpSpecs, document, hasRuntimeConfig, mcpSearch, skillCatalog, subagentConfig, sandboxSettings,
		webConfig, visionBridgeConfig, stepLimits, streamIdleTimeout,
	)
	if err != nil {
		return nil, err
	}
	if expectedConfigurationDigest != "" && configurationDigest != expectedConfigurationDigest {
		return nil, errors.New("startup security configuration changed after semantic planning")
	}
	optionalInstallation, err := loadRuntimeOptionalDomainInstallation(ctx, config, persistenceRoots, rootAuthority)
	if err != nil {
		return nil, err
	}
	childIdentityStartup, err := prepareRuntimeChildIdentityStartupV1(ctx, persistenceRoots, rootAuthority, privateCASAccessAuthority, optionalInstallation, originalCreates)
	if err != nil {
		return nil, err
	}
	if childIdentityStartup.verification != nil {
		childIdentityStartup.originalRegistryTrust, err = prepareRuntimeOriginalRegistryTrustV2(ctx, config, childIdentityStartup.verification)
		if err != nil {
			return nil, err
		}
	}
	childFloors, err := domainpendingwork.MergeChildIdentityFloorsV1(append(inheritedChildFloors, childIdentityStartup.floors)...)
	if err != nil {
		return nil, err
	}
	authorityAdvanceStartup, err := prepareRuntimeAuthorityAdvanceStartupV2(ctx, persistenceRoots, rootAuthority, privateCASAccessAuthority, optionalInstallation)
	if err != nil {
		return nil, err
	}
	reportRestartPreservation, err := prepareRuntimeReportPreservationBeforeRecoveryV1(ctx, childIdentityStartup, privateCASAccessAuthority, optionalInstallation, authorityAdvanceStartup)
	if err != nil {
		return nil, err
	}
	ctx, reportRestartPreservation.finalHistory, err = bindRuntimeOriginalFinalHistoryV1(ctx, childIdentityStartup, reportRestartPreservation)
	if err != nil {
		return nil, err
	}
	retainedOriginalCreates, err := originalCreates.retainedV1(ctx, reportRestartPreservation)
	if err != nil {
		return nil, err
	}
	// The complete Original/Final classification is now settled. Healthy
	// owners may perform their authenticated cleanup; unavailable owners
	// retain the exact physical files through recovery and activation.
	ctx = persistencefs.WithOriginalPlainResiduesV1(ctx, retainedOriginalCreates.plainProofV1())
	if planOutput != nil {
		planOutput.configurationDigest = configurationDigest
		planOutput.originalCreates = retainedOriginalCreates
		planOutput.finalHistoryQualification, _ = ctx.Value(runtimeFinalHistoryQualificationKeyV1{}).(*runtimeFinalHistoryQualificationV1)
	}
	if planStartup {
		_, recoveryJournalErr := privateCASRecoveryJournal.Load(ctx)
		resumeSignedPrivateCASRecovery := recoveryJournalErr == nil
		if recoveryJournalErr != nil && !errors.Is(recoveryJournalErr, privatecasport.ErrJournalAbsent) {
			return nil, fmt.Errorf("load runtime signed private CAS recovery journal before cleanup: %w", recoveryJournalErr)
		}
		semanticBuilder := persistencefs.NewSemanticPlanBuilderWithRestartPreservationV1(persistenceRoots, rootAuthority, journalAuthority, reportRestartPreservation, retainedOriginalCreates.proofV1())
		// Recovery may revoke a live pre-recovery private-CAS generation before
		// applying an authenticated residue. Prove every non-private managed
		// surface first; the signed recovery transaction separately validates
		// the exact private authority topology and residue inventory.
		if err := childIdentityStartup.revalidate(ctx); err != nil {
			return nil, err
		}
		if err := authorityAdvanceStartup.revalidate(ctx); err != nil {
			return nil, err
		}
		if resumeSignedPrivateCASRecovery {
			if err := persistencefs.ValidateNoSemanticStartupStateForPrivateCASRecoveryV1(
				ctx, persistenceRoots, journalAuthority,
			); err != nil {
				return nil, err
			}
		} else {
			if err := recoverRuntimePrivateCASCreateResidues(ctx, config.DataDir, privateCASAccessAuthority, reportRestartPreservation); err != nil {
				return nil, err
			}
			// Healthy original creation residues have completed their native
			// cleanup. Subsequent observations retain only the selected owners.
			childIdentityStartup.originalCreates = retainedOriginalCreates
			if err := semanticBuilder.RecoverAuthenticatedExisting(ctx); err != nil {
				return nil, err
			}
			if err := recoverRuntimePrivateCASOrphanTopology(ctx, config.DataDir, privateCASAccessAuthority, reportRestartPreservation); err != nil {
				return nil, err
			}
		}
		if err := recoverRuntimePrivateCASOwnersWithPreservationV1(
			ctx, config.DataDir, privateCASAccessAuthority, optionalInstallation, reportRestartPreservation, privateCASRecoveryJournal,
		); err != nil {
			return nil, err
		}
		// Authenticated recovery of an already-started checkpoint transaction
		// is a distinct prebaseline phase. Its exact delta is verified before
		// the first semantic planning session is allowed to exist.
		snapshotReader := persistencefs.NewStartupSnapshotReaderWithOriginalCreateResiduesV1(persistenceRoots, retainedOriginalCreates.proofV1())
		checkpointRecovery, checkpointRecoveryDelta, err := recoverAuthenticatedCheckpointOperationsBeforeSemanticBaselineV1(
			ctx, snapshotReader, config.DataDir, privateCASAccessAuthority, sandboxSettings.AllowWriteRoots, time.Now().UTC(), reportRestartPreservation.report,
		)
		if err != nil {
			return nil, err
		}
		startupSession, err = startupapp.BeginReadOnlyStartupPlanV1(ctx, snapshotReader)
		if err != nil {
			return nil, err
		}
		baseline, err := startupSession.BindConfiguration(ctx, configurationDigest, time.Now().UTC())
		if err != nil {
			return nil, err
		}
		prepared, err := startupapp.BuildSemanticStartupPlanV1(
			ctx, semanticBuilder, baseline, configurationDigest,
			func(ctx context.Context, stage startupport.PersistenceRootsV1) (stageErr error) {
				stageRoots, err := persistencefs.ResolveRootSet(stage.DataDir, stage.DurableDir)
				if err != nil {
					return err
				}
				stageAuthority, authorityErr := persistencefs.FreezeRootAuthority(stageRoots)
				if authorityErr != nil {
					return authorityErr
				}
				stageAccessAuthority, authorityErr := persistencefs.NewSemanticStagePrivateCASAccessAuthority(stageRoots, stageAuthority)
				if authorityErr != nil {
					return authorityErr
				}
				defer func() {
					stageErr = errors.Join(stageErr, stageAccessAuthority.Close())
				}()
				stageOriginalCreates, err := retainedOriginalCreates.copiedV1(ctx, stageRoots, stageAccessAuthority)
				if err != nil {
					return err
				}
				_, stageErr = newRuntimeServerHandlerWithRootsModeE(ctx, config, stageRoots, stageAuthority, journalAuthority, stageAccessAuthority, false, true, configurationDigest, runtimeInfoDataDir, nil, true, configurationSnapshot, stageOriginalCreates, childFloors)
				return stageErr
			},
		)
		if err != nil {
			return nil, err
		}
		// The semantic plan may atomically add repaired private-CAS records.
		// Retire pre-plan in-process generations only after every read-only
		// preflight and simulation succeeds, so recursive activation opens
		// an exact post-apply inventory without reintroducing early recovery
		// side effects on rejected startup state.
		if err := withRetiredRuntimePrivateCASOwnerGenerationsForSemanticApply(
			ctx, config.DataDir, privateCASAccessAuthority, optionalInstallation, prepared.Plan(), prepared.Apply, reportRestartPreservation,
		); err != nil {
			return nil, errors.Join(err, prepared.Close())
		}
		if err := checkpointRecovery.Verify(ctx, checkpointRecoveryDelta); err != nil {
			return nil, errors.Join(err, prepared.Close())
		}
		if err := prepared.Close(); err != nil {
			return nil, err
		}
		if !activateAfterPlan {
			return nil, nil
		}
		return newRuntimeServerHandlerWithRootsModeE(ctx, config, persistenceRoots, rootAuthority, journalAuthority, privateCASAccessAuthority, false, false, configurationDigest, runtimeInfoDataDir, nil, true, configurationSnapshot, retainedOriginalCreates, childFloors)
	}
	config = applyRuntimePersistenceRoots(config, persistenceRoots)
	startupPhase = "durable-store-open"
	mcpProxyURL := strings.TrimSpace(config.MCPProxyURL)
	if mcpProxyURL == "" {
		mcpProxyURL = strings.TrimSpace(config.ModelProxyURL)
	}
	if err := childIdentityStartup.revalidate(ctx); err != nil {
		return nil, err
	}
	if err := authorityAdvanceStartup.revalidate(ctx); err != nil {
		return nil, err
	}
	store, err := server.NewRuntimeEventSessionStoreWithPreservationV1(config, simulation, reportRestartPreservation.durable, reportRestartPreservation.usage, childFloors)
	if err != nil {
		return nil, err
	}
	acceptedFinalCASReader, err := finalauthority.NewAcceptedFinalCASReader(persistenceRoots.DurableDir)
	if err != nil {
		return nil, err
	}
	if err := store.BindPrimaryThreadReaderV1(acceptedFinalCASReader); err != nil {
		return nil, err
	}
	if err := store.SeedFromG2Routes(config.Routes); err != nil {
		return nil, err
	}
	finalEventDelivery := store.AcceptedFinalEventDelivery()
	if finalEventDelivery == nil {
		return nil, errors.New("accepted final event delivery adapter is unavailable")
	}
	var finalPublicReader threadapp.AuthorityThreadReader = store
	finalEventIO := evidenceapp.FinalPublicationEventIO{
		// The production event-log adapter serializes each thread independently,
		// so read-only startup inventory replay can use the bounded app-layer pool.
		InventoryReadConcurrency: 8,
		ReadThread: func(_ context.Context, _ appturn.AcceptedFinalCompletionStore, privateRecord domainevidence.PrivateAcceptedFinalRecord) (map[string]any, error) {
			return finalPublicReader.GetThread(privateRecord.SecurityContext.ThreadID)
		},
		ReadCASObservation: func(ctx context.Context, _ appturn.AcceptedFinalCompletionStore, privateRecord domainevidence.PrivateAcceptedFinalRecord) (domainevidence.AcceptedFinalCASObservationV1, error) {
			return acceptedFinalCASReader.ReadAcceptedFinalCASObservation(ctx, privateRecord.SecurityContext.ThreadID, privateRecord.SecurityContext.TurnID)
		},
		LoadEvents: func(ctx context.Context, _ appturn.AcceptedFinalCompletionStore, threadID string) ([]map[string]any, error) {
			result, err := store.LoadEventsSinceContext(ctx, threadID, 0)
			if err != nil {
				return nil, err
			}
			if len(result.Diagnostics) != 0 {
				return nil, errors.New("accepted final event log contains invalid records")
			}
			return result.Events, nil
		},
		AppendEvents: func(ctx context.Context, _ appturn.AcceptedFinalCompletionStore, events []map[string]any) ([]map[string]any, error) {
			return finalEventDelivery.Stage(ctx, events)
		},
		Readback: finalEventDelivery,
		WithEventReservation: func(
			ctx context.Context,
			_ appturn.AcceptedFinalCompletionStore,
			threadID, commitID string,
			work evidenceapp.AcceptedFinalEventReservationWorkV1,
		) error {
			return finalEventDelivery.WithReservation(ctx, threadID, commitID, work)
		},
		ActivateAndPublishEvents: func(
			ctx context.Context,
			_ appturn.AcceptedFinalCompletionStore,
			events []map[string]any,
			seal domainevent.AcceptedFinalDeliverySealV1,
			activate func() error,
		) error {
			return finalEventDelivery.ActivateAndPublish(ctx, events, seal, activate)
		},
	}
	attachmentStore, err := filestore.NewPersistentAttachmentStore(config.DataDir)
	if err != nil {
		return nil, err
	}
	memoryStore, err := filestore.NewPersistentMemoryStore(config.DataDir)
	if err != nil {
		return nil, err
	}
	evidenceRegistryRoot := filepath.Join(config.DataDir, "private", "evidence-registry")
	settlementStore, err := reportRestartPreservation.openSettlementStoreV1(ctx, filepath.Join(config.DataDir, "private", "evidence-settlements"), privateCASAccessAuthority)
	if err != nil {
		return nil, err
	}
	privateFinalStore, err := finalauthority.NewPrivateStoreContext(ctx,
		filepath.Join(config.DataDir, "private", "accepted-finals"), privateCASAccessAuthority,
	)
	if err != nil {
		return nil, err
	}
	caseThreadStore, err := casethreadauthority.NewStoreContext(ctx,
		filepath.Join(config.DataDir, "private", "case-thread-authority"), privateCASAccessAuthority,
	)
	if err != nil {
		return nil, err
	}
	continuationRoot := filepath.Join(config.DataDir, "private", "gate-continuations")
	if simulation {
		stageAccess, ok := privateCASAccessAuthority.(continuationstore.SemanticMigrationAccessAuthority)
		if !ok || !stageAccess.IsSemanticStagePrivateCASAccessAuthority() {
			return nil, errors.New("semantic continuation migration authority is unavailable")
		}
		if err := continuationstore.MigrateLegacyV1ForSemanticStage(ctx, continuationRoot, stageAccess); err != nil {
			return nil, err
		}
	}
	continuationStore, err := continuationstore.NewStoreContext(ctx, continuationRoot, privateCASAccessAuthority)
	if err != nil {
		return nil, err
	}
	pendingWorkStore, err := pendingworkstore.NewStoreContext(ctx,
		filepath.Join(config.DataDir, "private", "pending-work"), privateCASAccessAuthority,
	)
	if err != nil {
		return nil, err
	}
	providerCacheTelemetryStore, err := cachetelemetrystore.NewStoreContext(ctx,
		filepath.Join(config.DataDir, "private", "provider-cache-telemetry"), privateCASAccessAuthority,
	)
	if err != nil {
		return nil, err
	}
	turnTerminalStore, err := turnterminalstore.NewStoreContext(ctx,
		filepath.Join(config.DataDir, "private", "turn-terminal-authority"), privateCASAccessAuthority,
	)
	if err != nil {
		return nil, err
	}
	attachmentAuthorityStore, err := reportRestartPreservation.OpenAttachmentRestartStoreV1(ctx,
		filepath.Join(config.DataDir, "private", "attachment-authority"), privateCASAccessAuthority,
	)
	if err != nil {
		return nil, err
	}
	var authorityAdvanceIntents authorityadvanceport.IntentInventoryStore = authorityAdvanceStartup.inventory
	var authorityAdvanceSettlements authorityadvanceport.SettlementInventoryStore = authorityAdvanceStartup.inventory
	privateAuthorityAdvanceExists := authorityAdvanceStartup.inventory.HasRecords()
	if !reportRestartPreservation.publication.recoveryDeferredV1() {
		authorityAdvanceStore, err := authorityadvancefs.NewStore(filepath.Join(config.DataDir, "private", "authority-advance"), privateCASAccessAuthority)
		if err != nil {
			return nil, err
		}
		privateAuthorityAdvanceExists, err = authorityAdvanceStore.HasRecords(ctx)
		if err != nil {
			return nil, err
		}
		authorityAdvanceIntents, authorityAdvanceSettlements = authorityAdvanceStore, authorityAdvanceStore
	}
	optionalDomains, err := prepareRuntimeOptionalDomainCapabilities(ctx, config.DataDir, privateCASAccessAuthority, optionalInstallation, reportRestartPreservation.publication)
	if err != nil {
		return nil, err
	}
	evidenceDomainAvailable := optionalDomains["evidence-authority"].available && optionalDomains["dataset-snapshot-authority"].available
	publicationDomainAvailable := optionalDomains["pii-authorization"].available && optionalDomains["report-publication"].available &&
		optionalDomains["controlled-artifact-access"].available && optionalDomains["controlled-artifact-access-v2"].available
	var datasetSnapshotStoresV2 *datasetsnapshotstore.StoresV2
	var evidenceAuthorityStoresV2 runtimeSharedEvidenceStoresV2
	evidenceResourcesOwnedByHandler := false
	defer func() {
		if !evidenceResourcesOwnedByHandler {
			resultErr = errors.Join(resultErr, evidenceAuthorityStoresV2.Close(), datasetSnapshotStoresV2.Close())
		}
	}()
	if evidenceDomainAvailable {
		datasetSnapshotStoresV2, err = datasetsnapshotstore.OpenStoresV2(
			filepath.Join(config.DataDir, "private", "dataset-snapshot-authority"), privateCASAccessAuthority,
		)
		if err != nil {
			return nil, err
		}
		evidenceAuthorityStoresV2, err = openRuntimeSharedEvidenceStoresV2(config.DataDir, privateCASAccessAuthority)
		if err != nil {
			return nil, err
		}
	}
	caseEntityCapability, err := openRuntimeCaseEntityCapabilityV1(
		ctx,
		filepath.Join(config.DataDir, "private", runtimePrivateCASCaseEntityOwnerV1),
		privateCASAccessAuthority,
	)
	if err != nil {
		return nil, err
	}
	caseEntityOwnedByHandler := false
	defer func() {
		if !caseEntityOwnedByHandler {
			resultErr = errors.Join(resultErr, caseEntityCapability.Close())
		}
	}()
	threadRiskPolicyStore, err := threadriskpolicystore.NewStore(
		filepath.Join(config.DataDir, "private", "thread-risk-policy"), privateCASAccessAuthority,
	)
	if err != nil {
		return nil, err
	}
	checkpointAuthorityStore, err := checkpointauthority.NewStoreContext(ctx,
		filepath.Join(config.DataDir, "private", "checkpoint-authority"), privateCASAccessAuthority,
	)
	if err != nil {
		return nil, err
	}
	sensitiveResourceOwner := &controlledPIIResourceOwnerV1{}
	defer func() {
		resultErr = errors.Join(resultErr, sensitiveResourceOwner.Close())
	}()
	var piiAuthorizationStore *piiauthorizationstore.Store
	var reportPublicationStores *reportpublicationstore.Stores
	var controlledAccessStore *piiauthorizationstore.AccessStore
	var controlledAccessStoreV2 *piiauthorizationstore.AccessStoreV2
	if publicationDomainAvailable {
		piiAuthorizationStore, err = piiauthorizationstore.NewStore(
			filepath.Join(config.DataDir, "private", "pii-authorization", "grants"), privateCASAccessAuthority,
		)
		if err != nil {
			return nil, err
		}
		if err := sensitiveResourceOwner.Add(piiAuthorizationStore); err != nil {
			return nil, errors.Join(err, piiAuthorizationStore.Close())
		}
		reportPublicationStores, err = reportpublicationstore.NewStores(
			filepath.Join(config.DataDir, "private", "report-publication"), privateCASAccessAuthority,
		)
		if err != nil {
			return nil, err
		}
		if err := sensitiveResourceOwner.Add(reportPublicationStores); err != nil {
			return nil, errors.Join(err, reportPublicationStores.Close())
		}
		controlledAccessStore, err = piiauthorizationstore.NewAccessStore(
			filepath.Join(config.DataDir, "private", "controlled-artifact-access"), privateCASAccessAuthority,
		)
		if err != nil {
			return nil, err
		}
		if err := sensitiveResourceOwner.Add(controlledAccessStore); err != nil {
			return nil, errors.Join(err, controlledAccessStore.Close())
		}
		controlledAccessStoreV2, err = piiauthorizationstore.NewAccessStoreV2(
			filepath.Join(config.DataDir, "private", "controlled-artifact-access-v2"), privateCASAccessAuthority,
		)
		if err != nil {
			return nil, err
		}
		if err := sensitiveResourceOwner.Add(controlledAccessStoreV2); err != nil {
			return nil, errors.Join(err, controlledAccessStoreV2.Close())
		}
	}
	attachmentAccess := &attachmentauthorityapp.Service{
		Threads: store, Bindings: filestore.CaseBindingReader{}, Owners: attachmentAuthorityStore, Uploads: attachmentAuthorityStore,
	}
	attachmentRecoveryObservedAt := time.Now().UTC()
	attachmentRecoveryPlan, err := attachmentStore.PreflightAuthorityReconciliation(
		ctx, attachmentAuthorityStore, attachmentAccess.ValidateUploadOwnerCurrent, attachmentRecoveryObservedAt,
	)
	if err != nil {
		return nil, err
	}
	privateAuthorityExists, err := privateFinalStore.HasRecords(ctx)
	if err != nil {
		return nil, err
	}
	caseThreadAuthorityExists, err := caseThreadStore.HasRecords(ctx)
	if err != nil {
		return nil, err
	}
	privateSettlementExists, err := settlementStore.HasRecords(ctx)
	if err != nil {
		return nil, err
	}
	privateRegistryExists, err := evidenceregistry.HasState(ctx, evidenceRegistryRoot, privateCASAccessAuthority)
	if err != nil {
		return nil, err
	}
	privateContinuationExists, err := continuationStore.HasRecords(ctx)
	if err != nil {
		return nil, err
	}
	privatePendingWorkExists, err := pendingWorkStore.HasRecords(ctx)
	if err != nil {
		return nil, err
	}
	privateProviderCacheTelemetryExists, err := providerCacheTelemetryStore.HasRecords(ctx)
	if err != nil {
		return nil, err
	}
	privateTurnTerminalExists, err := turnTerminalStore.HasRecords(ctx)
	if err != nil {
		return nil, err
	}
	privateAttachmentAuthorityExists, err := attachmentAuthorityStore.HasRecords(ctx)
	if err != nil {
		return nil, err
	}
	privateDatasetSnapshotExists := optionalDomains["dataset-snapshot-authority"].hasRecords
	privateEvidenceAuthorityExists := optionalDomains["evidence-authority"].hasRecords
	privateCaseEntityExists := caseEntityCapability.hasRecords
	privateThreadRiskPolicyExists, err := threadRiskPolicyStore.HasRecords(ctx)
	if err != nil {
		return nil, err
	}
	privateCheckpointAuthorityExists, err := checkpointAuthorityStore.HasRecords(ctx)
	if err != nil {
		return nil, err
	}
	privatePIIAuthorizationExists := optionalDomains["pii-authorization"].hasRecords
	privateReportPublicationExists := optionalDomains["report-publication"].hasRecords
	privateControlledAccessExists := optionalDomains["controlled-artifact-access"].hasRecords
	privateControlledAccessV2Exists := optionalDomains["controlled-artifact-access-v2"].hasRecords
	publicSteeringAuthorityExists, err := steeringauthorityapp.HasPublicAuthorityStateV1(store)
	if err != nil {
		return nil, err
	}
	privateAuthorityRequired := (privateAuthorityInventoryV1{
		FinalRecords: privateAuthorityExists, Settlements: privateSettlementExists,
		EvidenceRegistry: privateRegistryExists, Continuations: privateContinuationExists,
		PendingWork: privatePendingWorkExists, ProviderCacheTelemetry: privateProviderCacheTelemetryExists,
		TurnTerminal: privateTurnTerminalExists, AttachmentAuthority: privateAttachmentAuthorityExists,
		AuthorityAdvance: privateAuthorityAdvanceExists, EvidenceAuthority: privateEvidenceAuthorityExists,
		DatasetSnapshot:     privateDatasetSnapshotExists,
		ThreadRiskPolicy:    privateThreadRiskPolicyExists,
		CheckpointAuthority: privateCheckpointAuthorityExists,
		PIIAuthorization:    privatePIIAuthorizationExists, ReportPublication: privateReportPublicationExists,
		ControlledAccessV1: privateControlledAccessExists, ControlledAccessV2: privateControlledAccessV2Exists,
		CaseEntity:          privateCaseEntityExists,
		CaseThreadAuthority: caseThreadAuthorityExists, SteeringAuthority: publicSteeringAuthorityExists,
	}).Required()
	publicAuthorityExists, publicSettlementExists := false, false
	if !privateAuthorityRequired {
		publicAuthorityExists, err = evidenceapp.HasPublicAcceptedFinalAuthority(store)
		if err != nil {
			return nil, err
		}
		publicSettlementExists, err = evidenceapp.HasPublicEvidenceSettlementAuthority(store)
		if err != nil {
			return nil, err
		}
	}
	finalAuthority, err := finalauthority.OpenOrCreateFileAuthority(
		filepath.Join(config.DataDir, "private", "authority", "final-answer-ed25519-v1.json"),
		privateAuthorityRequired || publicAuthorityExists || publicSettlementExists || publicSteeringAuthorityExists,
	)
	if err != nil {
		return nil, err
	}
	if err := cachetelemetryapp.VerifyTrustedInventoryV1(ctx, providerCacheTelemetryStore, finalAuthority); err != nil {
		return nil, err
	}
	if err := turnterminalapp.VerifyTrustedInventoryV1(ctx, turnTerminalStore, finalAuthority); err != nil {
		return nil, err
	}
	if err := continuationapp.VerifyTrustedInventoryV1(ctx, continuationStore, finalAuthority); err != nil {
		return nil, err
	}
	if err := steeringauthorityapp.VerifyTrustedInventoryV1(ctx, store, steeringauthorityapp.NewService(finalAuthority)); err != nil {
		return nil, err
	}
	pendingWorkPreflight := pendingworkapp.NewService(finalAuthority, pendingWorkStore, nil)
	pendingWorkInventory, err := pendingWorkPreflight.TrustedInventoryV1(ctx)
	if err != nil {
		return nil, err
	}
	if err := authorityadvanceapp.VerifyTrustedInventoryV2(ctx, authorityAdvanceIntents, authorityAdvanceSettlements, finalAuthority); err != nil {
		return nil, err
	}
	if !publicationDomainAvailable {
		publication := reportRestartPreservation.publication
		if publication != nil && publication.deferred != nil {
			if err := publication.deferred.validateV1(ctx, reportRestartPreservation.core, publication, reportRestartPreservation.report, pendingWorkInventory); err != nil {
				return nil, err
			}
			if err := reportRestartPreservation.ValidateSemanticOperationsV1(ctx, nil, nil, ""); err != nil {
				return nil, err
			}
		}
		if publication != nil && publication.closed != nil {
			if err := publication.closed.semantic.withSemanticCandidateV1(ctx, nil, nil, "", func(candidate *runtimeOriginalReportHistoryV1) error {
				plan := candidate.plan
				if publication.closed.mixedClosed != nil {
					_, closed, ok := runtimeMixedClosedDeferredPlansV1(plan)
					if !ok {
						return errRuntimeReportRestartReconciliationRequired
					}
					plan = closed
				}
				return validateRuntimeClosedReportPendingV1(plan, pendingWorkInventory)
			}); err != nil {
				return nil, err
			}
		} else if publication == nil || publication.deferred == nil {
			// Recheck the complete fresh pending denominator against the original
			// denial scope before any generic restart consumer sees it.
			for _, receipt := range pendingWorkInventory.Receipts {
				if receipt.Kind == domainpendingwork.KindReportStage {
					if disposition, disposed := pendingWorkInventory.Dispositions[receipt.WorkID]; !disposed || disposition.Status == domainpendingwork.StatusOutcomeUnknown {
						if err := validateRuntimeUnavailableReportPreservationV1(ctx, reportRestartPreservation, pendingWorkInventory, optionalDomains); err != nil {
							return nil, err
						}
						break
					}
				}
			}
		}
	}
	if publicationDomainAvailable {
		if err := piiauthorizationapp.VerifyTrustedInventoryV1(ctx, piiAuthorizationStore, reportPublicationStores.Ledgers, finalAuthority); err != nil {
			return nil, err
		}
		if err := reportpublicationapp.VerifyTrustedInventoryV1(
			ctx, reportPublicationStores.Attempts, reportPublicationStores.Receipts, reportPublicationStores.Indexes,
			reportPublicationStores.Selections, reportPublicationStores.Commits, reportPublicationStores.Decisions,
			reportPublicationStores.GrantSettlements, reportPublicationStores.StageCompletions,
			reportPublicationStores.DeliveryOutcomes, finalAuthority,
		); err != nil {
			return nil, err
		}
		reportRestartPlan, err := reportpublicationapp.PreflightRestartV1(ctx, reportpublicationapp.RestartPreflightConfigV1{
			Pending: pendingWorkInventory, Attempts: reportPublicationStores.Attempts,
			Receipts: reportPublicationStores.Receipts, Indexes: reportPublicationStores.Indexes,
			Commits: reportPublicationStores.Commits, Selections: reportPublicationStores.Selections,
			Decisions: reportPublicationStores.Decisions, Ledgers: reportPublicationStores.Ledgers,
			GrantSettlements: reportPublicationStores.GrantSettlements, StageCompletions: reportPublicationStores.StageCompletions,
			DeliveryOutcomes: reportPublicationStores.DeliveryOutcomes, Threads: store,
			Projections: reportPublicationStores.PIIProjections, Inspections: reportPublicationStores.Inspections,
			Intents:     authorityAdvanceIntents,
			Settlements: authorityAdvanceSettlements, Authority: finalAuthority,
		})
		if err != nil {
			return nil, err
		}
		// Fresh stores must retain the complete independently audited Original
		// terminal graph. No partial attempt obtains generic restart authority.
		if (len(reportRestartPlan.Attempts) != 0 || reportRestartPreservation.history != nil) && !reportRestartPreservation.history.matchesCompletedPlanV1(reportRestartPlan) {
			return nil, errRuntimeReportRestartReconciliationRequired
		}
		historicalControlledAvailable := false
		if history := reportRestartPreservation.history; history != nil {
			// The early historical reader sealed Original primary bytes. Rebuild
			// its proof after semantic startup. Only the actual isolated stage
			// may record crash-open uncertainty; activation requires zero open.
			if err := history.semantic.withSemanticCandidateV1(ctx, nil, nil, "", func(candidate *runtimeOriginalReportHistoryV1) error {
				bridge, err := reportpublicationapp.NewHistoricalArtifactDeliveryBridgeV1(candidate.authority, reportPublicationStores.Commits, reportPublicationStores.Receipts, finalAuthority)
				if err != nil {
					return err
				}
				dependencies := piiauthorizationapp.ControlledAccessInventoryDependenciesV2{Access: controlledAccessStoreV2, Historical: bridge, Grants: piiAuthorizationStore, Artifacts: reportPublicationStores.Artifacts, Authority: finalAuthority}
				plan, err := piiauthorizationapp.VerifyControlledAccessInventoryV2(ctx, dependencies)
				if err != nil {
					return err
				}
				if plan.OpenReceiptCount() != 0 {
					if !simulation || !controlledAccessStoreV2.IsSemanticStageControlledAccessStoreV2() {
						return errRuntimeReportRestartReconciliationRequired
					}
					if err := piiauthorizationapp.ApplyControlledAccessRestartPlanV2(ctx, plan, controlledAccessStoreV2, finalAuthority, time.Now().UTC()); err != nil {
						return err
					}
					plan, err = piiauthorizationapp.VerifyControlledAccessInventoryV2(ctx, dependencies)
					if err != nil {
						return err
					}
				}
				if plan.OpenReceiptCount() != 0 {
					return errRuntimeReportRestartReconciliationRequired
				}
				return history.controlled.validateCurrentV2(ctx, history.semantic.core, history.semantic.publication, candidate, history.semantic.reports, nil, nil, "")
			}); err != nil {
				return nil, err
			}
			historicalControlledAvailable = history.controlled != nil && history.controlled.hasRecords
		}
		if err := validateControlledAccessCompositionV2(controlledAccessCompositionStateV2{
			HasDurableRecords: privateControlledAccessV2Exists, HistoricalAuthorityAvailable: historicalControlledAvailable,
		}); err != nil {
			return nil, err
		}
		if err := piiauthorizationapp.VerifyControlledPublicationInventoryV1(
			ctx, reportPublicationStores.Receipts, reportPublicationStores.PIIProjections,
			reportPublicationStores.Ledgers, piiAuthorizationStore, finalAuthority,
		); err != nil {
			return nil, err
		}
		controlledAccessDependencies := piiauthorizationapp.ControlledAccessInventoryDependenciesV1{
			Access: controlledAccessStore, Grants: piiAuthorizationStore,
			Receipts: reportPublicationStores.Receipts, Commits: reportPublicationStores.Commits,
			Indexes: reportPublicationStores.Indexes, Ledgers: reportPublicationStores.Ledgers,
			Projections: reportPublicationStores.PIIProjections, Inspections: reportPublicationStores.Inspections,
			Artifacts: reportPublicationStores.Artifacts, Authority: finalAuthority,
		}
		controlledAccessRestart, err := piiauthorizationapp.VerifyControlledAccessInventoryV1(ctx, controlledAccessDependencies)
		if err != nil {
			return nil, err
		}
		if err := piiauthorizationapp.ApplyControlledAccessRestartPlanV1(
			ctx, controlledAccessRestart, controlledAccessStore, finalAuthority, time.Now().UTC(),
		); err != nil {
			return nil, err
		}
		controlledAccessRestart, err = piiauthorizationapp.VerifyControlledAccessInventoryV1(ctx, controlledAccessDependencies)
		if err != nil || controlledAccessRestart.OpenReceiptCount() != 0 {
			return nil, errors.Join(errors.New("controlled artifact access restart recovery is incomplete"), err)
		}
	}
	if err := sensitiveResourceOwner.Close(); err != nil {
		return nil, err
	}
	providerTelemetry, err := cachetelemetryapp.NewDurableService(finalAuthority, providerCacheTelemetryStore)
	if err != nil {
		return nil, err
	}
	if scope := reportRestartPreservation.report; scope != nil && len(scope.ThreadIDs()) != 0 {
		if err := providerTelemetry.PreserveRestartContextsV1(ctx, scope.Contexts()); err != nil {
			return nil, err
		}
	}
	terminalCoordinator, err := turnterminalapp.NewCoordinator(
		finalAuthority, privateFinalStore, turnTerminalStore, providerTelemetry,
	)
	if err != nil {
		return nil, err
	}
	if scope := reportRestartPreservation.report; scope != nil && len(scope.ThreadIDs()) != 0 {
		if err := terminalCoordinator.PreserveRestartContextsV1(ctx, scope.Contexts()); err != nil {
			return nil, err
		}
	}
	providerRestartPlan, err := providerTelemetry.PlanRestartSettlements(ctx, time.Now().UTC())
	if err != nil {
		return nil, err
	}
	if err := providerTelemetry.ApplyRestartSettlements(ctx, providerRestartPlan); err != nil {
		return nil, err
	}
	providerClient, err := providerclient.NewProductionHTTPProviderClient(
		providerclient.NewDefaultHTTPClientWithProxy(config.ModelProxyURL), providerTelemetry,
	)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(config.ProviderAuditSocketPath) != "" {
		providerClient.ProviderBodyAuditor, err = providerclient.NewUnixProviderRequestBodyAuditorV1(
			config.ProviderAuditSocketPath,
		)
		if err != nil {
			return nil, err
		}
	}
	providerClient.StreamIdleTimeout = streamIdleTimeout
	threadRiskAuthority, err := newRuntimeThreadRiskAuthority(config, finalAuthority, threadRiskPolicyStore)
	if err != nil {
		return nil, err
	}
	datasetSnapshotAuthority, _, err := newContractDatasetSnapshotAuthority(config, finalAuthority)
	if err != nil {
		return nil, err
	}
	var sharedEvidenceDatasetSnapshotV2 runtimeSharedEvidenceDatasetSnapshotV2
	defer func() {
		if !evidenceResourcesOwnedByHandler && sharedEvidenceDatasetSnapshotV2.registryOwner != nil {
			resultErr = errors.Join(resultErr, sharedEvidenceDatasetSnapshotV2.registryOwner.Close())
		}
	}()
	var sharedEvidenceConfiguredV2 bool
	if evidenceDomainAvailable {
		sharedEvidenceDatasetSnapshotV2, sharedEvidenceConfiguredV2, err = newRuntimeSharedEvidenceDatasetSnapshotV2(
			ctx, config, finalAuthority, datasetSnapshotStoresV2, evidenceAuthorityStoresV2,
			privateCASAccessAuthority, filestore.CaseBindingReader{}, reportRestartPreservation.registry,
		)
		if err != nil {
			return nil, err
		}
	} else {
		// Domain unavailability cannot bypass the shared enrollment/key anchor.
		_, sharedEvidenceConfiguredV2, err = loadRuntimeSharedEvidenceEnrollmentV2(ctx, config, finalAuthority)
		if err != nil {
			return nil, err
		}
	}
	// Composition is local-only. Protected operations reach this shared
	// authority and perform their own fresh witness challenge; absent enrollment
	// leaves only the case effect fail-closed while the general Agent still runs.
	var datasetSnapshotAuthorityV2 datasetsnapshotport.AuthorityV2
	var datasetSnapshotCurrentAuthorityV2 datasetsnapshotport.CurrentAuthorityV2
	if sharedEvidenceDatasetSnapshotV2.snapshot != nil {
		datasetSnapshotAuthorityV2 = runtimeCaseDatasetSnapshotAuthorityV2{
			snapshot: sharedEvidenceDatasetSnapshotV2.snapshot, registry: sharedEvidenceDatasetSnapshotV2.registryOwner,
		}
		datasetSnapshotCurrentAuthorityV2 = sharedEvidenceDatasetSnapshotV2.snapshot
	}
	identityAuthority, err := newRuntimeHostIdentityAuthority(finalAuthority)
	if err != nil {
		return nil, err
	}
	attachmentUses := attachmentuseapp.NewService(
		finalAuthority,
		attachmentAuthorityStore,
		func(ctx context.Context, securityContext domainsecurity.TurnSecurityContext) error {
			return turnsecurityapp.ValidateCurrent(turnsecurityapp.CurrentValidationInput{
				OperationContext: ctx, Identity: identityAuthority,
				Observer: filestore.CaseBindingReader{}, RiskAuthority: threadRiskAuthority,
				SnapshotAuthority: datasetSnapshotAuthority, SnapshotAuthorityV2: datasetSnapshotAuthorityV2,
				Context: securityContext, Workspace: securityContext.WorkspaceRealPath,
			})
		},
	)
	attachmentRestartPlan, err := attachmentUses.PlanRestartDispositions(ctx)
	if err != nil {
		return nil, err
	}
	var evidenceStore runtimeEvidenceRegistryAuthority
	if !evidenceDomainAvailable || (sharedEvidenceConfiguredV2 && sharedEvidenceDatasetSnapshotV2.registryOwner == nil) ||
		(reportRestartPreservation.registry != nil && reportRestartPreservation.registry.unavailable) {
		evidenceStore = runtimeUnavailableEvidenceRegistryV1{}
	} else if sharedEvidenceDatasetSnapshotV2.registryOwner != nil {
		evidenceStore = sharedEvidenceDatasetSnapshotV2.registryOwner
		validatedRegistryExists, validateErr := evidenceregistry.HasState(ctx, evidenceRegistryRoot, privateCASAccessAuthority)
		if validateErr != nil || validatedRegistryExists != privateRegistryExists {
			if validateErr != nil {
				return nil, validateErr
			}
			return nil, errors.New("evidence registry authority inventory changed during startup")
		}
	} else {
		legacyPlan, prepareErr := evidenceregistry.PrepareRecoveryV2(ctx, evidenceRegistryRoot, privateCASAccessAuthority)
		if prepareErr != nil {
			return nil, prepareErr
		}
		if prepareErr := legacyPlan.RevalidatePhysicalV2(ctx); prepareErr != nil {
			return nil, prepareErr
		}
		if !legacyPlan.LegacyV1ActivationAllowed() {
			evidenceStore = runtimeUnavailableEvidenceRegistryV1{}
		} else {
			legacyEvidenceStore, openErr := evidenceregistry.NewStore(evidenceRegistryRoot, finalAuthority)
			if openErr != nil {
				return nil, openErr
			}
			validatedRegistryExists, validateErr := legacyEvidenceStore.HasRecords(ctx)
			if validateErr != nil {
				return nil, validateErr
			} else if validatedRegistryExists != privateRegistryExists {
				return nil, errors.New("evidence registry authority inventory changed during startup")
			} else {
				evidenceStore = legacyEvidenceStore
			}
		}
	}
	caseThreads, err := casethreadapp.NewRegistry(ctx, finalAuthority, caseThreadStore)
	if err != nil {
		return nil, err
	}
	if scope := reportRestartPreservation.report; scope != nil && len(scope.ThreadIDs()) != 0 {
		if err := caseThreads.PreserveRestartScopeV1(ctx, scope.DeniedThreadIDsV1()); err != nil {
			return nil, err
		}
	}
	var legacyTypeScriptLineageWitness *jobs.FrozenLegacyTypeScriptLineageWitnessV1
	startupPhase = "case-compaction-recovery"
	caseThreads.SetActiveInheritedHistoryIdentityValidatorV1(runtimeActiveHistoryIdentityValidatorV1(config.DataDir, pendingWorkPreflight, privateFinalStore, finalAuthority))
	// Authenticate active inherited prefixes before any context/compaction repair.
	// Original/held scopes retain their separately qualified observer.
	activeThreadIDs, err := store.AllThreadIDs()
	if err != nil {
		return nil, err
	}
	for _, threadID := range activeThreadIDs {
		if caseThreads.RestartPreservesThreadV1(threadID) {
			continue
		}
		snapshot, readErr := acceptedFinalCASReader.ReadPrimaryThreadSnapshotV1(ctx, threadID)
		if readErr != nil {
			return nil, readErr
		}
		if _, validateErr := threadapp.ValidateActiveInheritedHistoryV1(ctx, snapshot.Thread, caseThreads, acceptedFinalCASReader); validateErr != nil {
			return nil, validateErr
		}
	}
	store.SetCaseThreadAuthority(caseThreads)
	if err := threadapp.RecoverCommittedCaseCompactions(ctx, caseThreads, store); err != nil {
		return nil, err
	}
	startupPhase = "case-context-repair"
	if err := casethreadapp.RepairCommittedContexts(caseThreads, store); err != nil {
		return nil, err
	}
	if simulation {
		startupPhase = "legacy-lineage-witness"
		legacyTypeScriptLineageWitness, err = newFrozenLegacyTypeScriptChildLineageWitnessV1(
			filepath.Join(config.DataDir, "child-runs"), newLegacyTypeScriptRawThreadReaderV1(persistenceRoots.DurableDir),
		)
		if err != nil {
			return nil, err
		}
		startupPhase = "semantic-content-migration"
		if err := store.ApplySemanticStartupMigrationsAfterAuthorityRepair(); err != nil {
			return nil, err
		}
	}
	startupPhase = "final-authority-reconciliation"
	existingAuthorityReader := runtimeRestartFinalPublicReaderV1(ctx, threadapp.NewCaseThreadRestartAuthorityReaderV1(ctx, store, caseThreads), reportRestartPreservation.report)
	finalPublicReader = existingAuthorityReader
	evidenceIssuer := evidenceapp.Issuer{Registry: evidenceStore, SettlementStore: settlementStore, Authority: finalAuthority}
	finalHistoricalReader := runtimeFinalHistoricalReaderV1(evidenceStore, childIdentityStartup, reportRestartPreservation)
	authorityInventory, err := evidenceapp.PreflightFinalAuthorityInventory(ctx, existingAuthorityReader, acceptedFinalCASReader, finalHistoricalReader, finalAuthority, privateFinalStore)
	if err != nil {
		return nil, err
	}
	migrationRecords := append([]domainevidence.PrivateAcceptedFinalRecord{}, authorityInventory.Committed...)
	migrationRecords = append(migrationRecords, authorityInventory.NotCommitted...)
	migrationRecords = append(migrationRecords, authorityInventory.AuditOnlyPublicWinners...)
	migrationRecords = append(migrationRecords, authorityInventory.AuditOnlyNotCommitted...)
	migrationContexts := make([]domainsecurity.TurnSecurityContext, 0, len(migrationRecords))
	for _, record := range migrationRecords {
		migrationContexts = append(migrationContexts, record.SecurityContext)
	}
	for _, repair := range authorityInventory.PublicCommitRepairs {
		migrationContexts = append(migrationContexts, repair.PrivateRecord.SecurityContext)
	}
	caseMigrationPlan, err := caseThreads.PlanContexts(ctx, migrationContexts)
	if err != nil {
		return nil, err
	}
	caseThreads, err = caseThreads.WithPlan(ctx, caseMigrationPlan)
	if err != nil {
		return nil, err
	}
	authorityReader := runtimeRestartFinalPublicReaderV1(ctx, threadapp.NewCaseThreadRestartAuthorityReaderV1(ctx, store, caseThreads), reportRestartPreservation.report)
	finalPublicReader = authorityReader
	authorityInventory, err = evidenceapp.PreflightFinalAuthorityInventory(ctx, authorityReader, acceptedFinalCASReader, finalHistoricalReader, finalAuthority, privateFinalStore)
	if err != nil {
		return nil, err
	}
	if err := caseThreads.ApplyPlan(ctx, caseMigrationPlan); err != nil {
		return nil, err
	}
	store.SetCaseThreadAuthority(caseThreads)
	terminalCandidates := append([]domainevidence.PrivateAcceptedFinalRecord{}, authorityInventory.Committed...)
	terminalPrivateInventory := append([]domainevidence.PrivateAcceptedFinalRecord{}, authorityInventory.Committed...)
	terminalPrivateInventory = append(terminalPrivateInventory, authorityInventory.NotCommitted...)
	terminalAuditOnlyInventory := append([]domainevidence.PrivateAcceptedFinalRecord{}, authorityInventory.AuditOnlyPublicWinners...)
	terminalAuditOnlyInventory = append(terminalAuditOnlyInventory, authorityInventory.AuditOnlyNotCommitted...)
	for _, repair := range authorityInventory.PublicCommitRepairs {
		terminalCandidates = append(terminalCandidates, repair.PrivateRecord)
		terminalPrivateInventory = append(terminalPrivateInventory, repair.PrivateRecord)
	}
	terminalRecovery, err := terminalCoordinator.RecoverV1(ctx, turnterminalapp.RestartRecoveryInputV1{
		CompletionStore: store, CASReader: acceptedFinalCASReader,
		PrivateInventory: terminalPrivateInventory, AuditOnlyPrivateInventory: terminalAuditOnlyInventory,
		Candidates: terminalCandidates,
	})
	if err != nil {
		return nil, err
	}
	authorityInventory, err = evidenceapp.PreflightFinalAuthorityInventory(ctx, authorityReader, acceptedFinalCASReader, finalHistoricalReader, finalAuthority, privateFinalStore)
	if err != nil {
		return nil, err
	}
	terminalCommitted, _, err := terminalCompleteFinalAuthorityV1(authorityInventory, terminalRecovery)
	if err != nil {
		return nil, err
	}
	terminalProjectionAuthorities, err := terminalCompleteProjectionAuthoritiesV1(terminalCommitted, terminalRecovery)
	if err != nil {
		return nil, err
	}
	restartInventory, err := casethreadapp.PreflightRestartInventory(caseThreads, store)
	if err != nil {
		return nil, err
	}
	startupPhase = "case-restart-reconciliation"
	if err := casethreadapp.ApplyRestartInventory(caseThreads, restartInventory); err != nil {
		return nil, err
	}
	executableReader := threadapp.NewExecutableAuthorityReader(authorityReader, caseThreads)
	settlementPreservation := reportRestartPreservation.settlementObserverV1()
	settlementInventory, err := evidenceapp.PreflightEvidenceSettlementInventoryWithPreservationV1(ctx, executableReader, evidenceIssuer, settlementPreservation)
	if err != nil {
		return nil, err
	}
	finalEventPreservation := reportRestartPreservation.finalEventObserverV1(authorityInventory, terminalRecovery, authorityReader)
	eventReconciliationPlans, err := evidenceapp.PreflightAcceptedFinalEventsWithPreservationV1(
		ctx, finalEventIO, store, executableReader, terminalCommitted, terminalRecovery.LegacyQuarantined,
		authorityInventory.AuditOnlyPublicWinners, finalEventPreservation,
	)
	if err != nil {
		return nil, err
	}
	if err := attachmentStore.ApplyAuthorityReconciliation(
		ctx, attachmentAuthorityStore, attachmentAccess.ValidateUploadOwnerCurrent, attachmentRecoveryPlan,
	); err != nil {
		return nil, err
	}
	startupPhase = "evidence-final-event-reconciliation"
	if err := attachmentUses.ApplyRestartDispositions(ctx, attachmentRestartPlan, time.Now().UTC()); err != nil {
		return nil, err
	}
	store.SetCaseThreadAuthority(caseThreads)
	if err := evidenceapp.ApplyEvidenceSettlementReconciliationInventoryWithPreservationV1(ctx, executableReader, evidenceIssuer, settlementInventory, settlementPreservation); err != nil {
		return nil, err
	}
	if err := evidenceapp.ApplyAcceptedFinalEventsWithPreservationV1(
		ctx, finalEventIO, store, executableReader, eventReconciliationPlans,
		terminalCommitted, terminalRecovery.LegacyQuarantined, authorityInventory.AuditOnlyPublicWinners, finalEventPreservation,
	); err != nil {
		return nil, err
	}
	if err := reconcileCheckpointOperationAuditsWithPreservationV1(
		ctx,
		simulation,
		checkpointapp.SnapshotAuthority{Store: checkpointAuthorityStore},
		store,
		caseThreads,
		reportRestartPreservation,
	); err != nil {
		return nil, err
	}
	if err := appturn.RecoverGeneralTerminalPublicationsAtStartupV1(ctx, store); err != nil {
		return nil, err
	}
	trustedFinals := gateprojection.NewTrustedFinalProjectionIndexWithReadback(finalAuthority, finalEventDelivery)
	if err := trustedFinals.SeedTerminalComplete(ctx, terminalProjectionAuthorities); err != nil {
		return nil, err
	}
	currentCaseAuthority := threadapp.NewCurrentCaseThreadAuthorityValidator(caseThreads, filestore.CaseBindingReader{}, nil)
	publicProjector := threadapp.NewTrustedPublicProjectorWithPreservedHistoryV1(
		trustedFinals, caseThreads, currentCaseAuthority, acceptedFinalCASReader, reportRestartPreservation.report,
	)
	store.SetActiveHistorySourceAdmissionV1(func(source map[string]any) error {
		if _, err := publicProjector.ProjectThread(source); err != nil {
			return err
		}
		if _, present := source["securityState"]; present {
			threadID, _ := source["id"].(string)
			_, err := currentCaseAuthority.ValidateCurrent(threadID, source)
			return err
		}
		_, err := caseThreads.SourceAdmissionDigestV1(source)
		return err
	})
	validateCurrentChildContext := func(ctx context.Context, securityContext domainsecurity.TurnSecurityContext) error {
		return turnsecurityapp.ValidateCurrent(turnsecurityapp.CurrentValidationInput{
			OperationContext: ctx, Identity: identityAuthority,
			Observer: filestore.CaseBindingReader{}, RiskAuthority: threadRiskAuthority,
			SnapshotAuthority: datasetSnapshotAuthority, SnapshotAuthorityV2: datasetSnapshotAuthorityV2,
			Context: securityContext, Workspace: securityContext.WorkspaceRealPath,
		})
	}
	childCompletions := subagentapp.NewChildCompletionAuthority(
		func(threadID string, turnID string) (domainsecurity.TurnSecurityContext, bool) {
			committed, ok := caseThreads.CommittedContext(threadID, turnID)
			return committed.SecurityContext, ok
		},
		trustedFinals, finalAuthority, store, validateCurrentChildContext,
	)
	childPreservation, err := reportRestartPreservation.childConstructorInputV1(ctx)
	if err != nil {
		return nil, err
	}
	storedChildVerifier := reportRestartPreservation.childConstructorVerifierV1(childCompletions)
	var jobManager *jobs.Manager
	if simulation {
		startupPhase = "child-run-migration"
		jobManager, err = jobs.NewManagerForSemanticStartupWithRestartPreservationV1(
			ctx, filepath.Join(config.DataDir, "child-runs"), filepath.Join(authorityDataDir, "child-runs"), storedChildVerifier,
			legacyTypeScriptLineageWitness, childPreservation, childFloors,
		)
	} else {
		jobManager, err = jobs.NewManagerWithRestartPreservationV1(ctx, filepath.Join(config.DataDir, "child-runs"), storedChildVerifier, childPreservation, childFloors)
	}
	if err != nil {
		return nil, err
	}
	startupPhase = "runtime-composition"
	continuations := continuationapp.NewService(finalAuthority, continuationStore)
	pendingWork := pendingworkapp.NewService(finalAuthority, pendingWorkStore, store)
	if history := reportRestartPreservation.history; history != nil && len(history.closedResults) != 0 {
		workIDs := make([]string, 0, len(history.plan.Attempts))
		for _, entry := range history.plan.Attempts {
			if _, closed := history.closedResults[entry.Stage.WorkID]; closed && (reportRestartPreservation.report == nil || !reportRestartPreservation.report.OwnsThread(entry.Stage.Context.ThreadID)) {
				workIDs = append(workIDs, entry.Stage.WorkID)
			}
		}
		if err := pendingWork.BindClosedReportRestartV1(ctx, pendingWorkInventory, workIDs); err != nil {
			return nil, err
		}
	}
	if scope := reportRestartPreservation.report; scope != nil && len(scope.ThreadIDs()) != 0 {
		if err := pendingWork.PreserveReportRestartScopeV1(ctx, *scope); err != nil {
			return nil, err
		}
	}
	fundsAccountFlow := runtimeFundsAccountFlowCompositionV1{}
	var bundledFundsHostSpec *mcp.HostFundsServerSpecV1
	if !simulation && caseEntityCapability.semanticUse && caseEntityCapability.store != nil &&
		datasetSnapshotCurrentAuthorityV2 != nil {
		fundsAccountFlow = composeRuntimeFundsAccountFlowV1(
			config.UserDataDir,
			pendingWork,
			caseEntityCapability.store,
			datasetSnapshotCurrentAuthorityV2,
			filestore.CaseBindingReader{},
			datasetSnapshotStoresV2.Materials,
			validateCurrentChildContext,
		)
		if fundsAccountFlow.available() {
			bundledFundsHostSpec, err = admitBundledFundsHostForStartupV1(ctx, config)
			if err != nil {
				return nil, err
			}
		}
	}
	foregroundHandoffs := subagentapp.NewForegroundHandoffAuthorityWithCaseTyped(
		subagentapp.NewForegroundSubmissionRegistry(), store,
		func(threadID string, turnID string) (domainsecurity.TurnSecurityContext, bool) {
			thread, loadErr := store.GetThread(threadID)
			if loadErr != nil || thread == nil {
				return domainsecurity.TurnSecurityContext{}, false
			}
			securityContext, contextErr := appturn.FrozenSecurityContextForTurn(thread, turnID)
			return securityContext, contextErr == nil
		},
		validateCurrentChildContext,
		func(threadID string, turnID string) (subagentapp.ForegroundChildTerminalV1, bool) {
			resolved, terminalErr := appturn.ResolveCommittedGeneralTerminalV1(ctx, store, acceptedFinalCASReader, threadID, turnID)
			if terminalErr != nil {
				return subagentapp.ForegroundChildTerminalV1{}, false
			}
			return subagentapp.ForegroundChildTerminalV1{
				SecurityContext: resolved.SecurityContext, TerminalDigest: resolved.Commit.CommitDigest,
				TerminalStatus: resolved.Commit.TerminalStatus, TerminalReason: resolved.Commit.TerminalReason,
			}, true
		},
		childCompletions,
		func(
			operationContext context.Context,
			parent domainsecurity.TurnSecurityContext,
			result domainjob.CaseForegroundChildResultV1,
		) error {
			return evidenceapp.ValidateCurrentCaseForegroundChildResultV1(
				operationContext, evidenceStore, parent, result,
			)
		},
		func(
			operationContext context.Context,
			record domainjob.Record,
			parent domainsecurity.TurnSecurityContext,
		) error {
			_, bindErr := subagentapp.BindCaseDelegationV1(operationContext, subagentapp.BindCaseDelegationInputV1{
				SecurityContext: parent,
				SecurityBinding: record.SecurityBinding,
				CaseEntities:    fundsAccountFlow.caseEntities,
				Request: subagentapp.TaskRequest{
					Prompt: record.Prompt, CaseDelegation: domainjob.CloneCaseDelegationContextV1(record.CaseDelegation),
				},
				Expected: record.CaseDelegation,
			})
			if bindErr != nil {
				return bindErr
			}
			return nil
		},
	)
	subagentState := subagentapp.NewRuntimeState()
	nativeHealthDependencies := nativecomponentapp.HealthDependencies{
		Store: store, DurableAuthority: currentCaseAuthority,
		AcquireEffect: subagentState.AcquireContextEffect,
		Now:           time.Now,
	}
	var mcpManager *mcp.ProductionManager
	var nativeAuthority *nativecomponentapp.RuntimeAuthority
	var directSourcePreviewRunner nativecomponentport.DirectSourcePreviewRunner
	var nativeOwner *nativecomponenthost.Owner
	if simulation {
		nativeAuthority, err = nativecomponentapp.NewUnavailableRuntimeAuthority(nativeHealthDependencies)
	} else {
		var openErr error
		if strings.TrimSpace(config.UserDataDir) == "" {
			openErr = nativecomponenthost.ErrUnavailable
		} else if supplied, ok := ctx.Value(bundledFundsHostValidationContextKeyV1{}).(bundledFundsHostValidationDependenciesV1); ok && supplied.openNativeOwnerForTest != nil {
			nativeOwner, openErr = supplied.openNativeOwnerForTest(config.UserDataDir)
		} else {
			nativeOwner, openErr = nativecomponenthost.OpenProduction(config.UserDataDir)
		}
		if openErr != nil && !nativeAdmissionMayDisableCaseCapability(openErr) {
			return nil, openErr
		}
		if openErr == nil && nativeOwner == nil {
			return nil, errors.New("native component host returned no owner or unavailable status")
		}
		if openErr != nil {
			if nativeOwner != nil {
				return nil, errors.Join(errors.New("native component host returned owner with unavailable status"), nativeOwner.Close())
			}
			nativeAuthority, err = nativecomponentapp.NewUnavailableRuntimeAuthority(nativeHealthDependencies)
		} else {
			directSourcePreviewRunner = nativeOwner
			nativeDependencies := nativecomponentapp.RuntimeAuthorityDependencies{
				Health: nativeHealthDependencies,
				LiveAuthority: nativecomponentapp.LiveAuthorityFunc(func(
					operationContext context.Context,
					securityContext domainsecurity.TurnSecurityContext,
				) error {
					return turnsecurityapp.ValidateCurrent(turnsecurityapp.CurrentValidationInput{
						OperationContext: operationContext, Identity: identityAuthority,
						Observer:      filestore.CaseBindingReader{},
						RiskAuthority: threadRiskAuthority, SnapshotAuthority: datasetSnapshotAuthority,
						SnapshotAuthorityV2: datasetSnapshotAuthorityV2,
						Context:             securityContext, Workspace: securityContext.WorkspaceRealPath,
					})
				}),
				HealthOnlyAuthority: nativecomponentapp.NewHealthOnlyCurrentnessV1(
					nativecomponentapp.LiveAuthorityFunc(func(
						operationContext context.Context,
						securityContext domainsecurity.TurnSecurityContext,
					) error {
						return turnsecurityapp.ValidateCurrentOrdinaryEffect(
							turnsecurityapp.CurrentValidationInput{
								OperationContext: operationContext,
								Identity:         identityAuthority,
								Observer:         filestore.CaseBindingReader{},
								RiskAuthority:    threadRiskAuthority,
								Context:          securityContext,
								Workspace:        securityContext.WorkspaceRealPath,
							},
						)
					}),
					nativeOwner.ValidateCurrentDataEngine,
				),
				Owner: nativeOwner,
			}
			if bundledFundsHostSpec != nil && fundsAccountFlow.available() {
				if !fundsAccountFlow.applyToRuntimeAuthorityV1(
					&nativeDependencies,
					func(
						operationContext context.Context,
						securityContext domainsecurity.TurnSecurityContext,
					) error {
						return turnsecurityapp.ValidateCurrentInsideExactDatasetCapability(
							turnsecurityapp.CurrentValidationInput{
								OperationContext: operationContext, Identity: identityAuthority,
								Observer:      filestore.CaseBindingReader{},
								RiskAuthority: threadRiskAuthority,
								Context:       securityContext,
								Workspace:     securityContext.WorkspaceRealPath,
							},
						)
					},
					func(
						operationContext context.Context,
						securityContext domainsecurity.TurnSecurityContext,
						grant domainsecurity.ExecutionGrant,
					) error {
						if mcpManager == nil {
							return errors.New("host funds current grant validator is unavailable")
						}
						return mcpManager.ValidateCurrentAccountFlowOuterGrant(
							operationContext,
							securityContext,
							grant,
						)
					},
				) {
					return nil, errors.Join(
						errors.New("funds account-flow runtime composition is incomplete"),
						nativeOwner.Close(),
					)
				}
			}
			nativeAuthority, err = nativecomponentapp.NewReadyRuntimeAuthority(nativeDependencies)
			if err != nil {
				return nil, errors.Join(err, nativeOwner.Close())
			}
		}
	}
	if err != nil {
		return nil, err
	}
	var fundsCSVAdmission *fundscsvadmissionapp.ServiceV1
	var fundsCleaning *fundscleaningapp.ServiceV1
	var activateImportRegistry func(context.Context, domainsecurity.CaseBindingObservationV1, string) error
	if sharedEvidenceDatasetSnapshotV2.registryOwner != nil {
		activateImportRegistry = sharedEvidenceDatasetSnapshotV2.registryOwner.ActivateAfterImport
	}
	if nativeOwner != nil && sharedEvidenceDatasetSnapshotV2.evidence != nil &&
		sharedEvidenceDatasetSnapshotV2.snapshot != nil {
		immutableSource, sourceErr := fundsquerysourceadapter.NewHostExactSource(config.UserDataDir)
		if sourceErr != nil {
			return nil, errors.Join(sourceErr, nativeAuthority.Close())
		}
		fundsCSVAdmission, err = fundscsvadmissionapp.NewServiceV1(fundscsvadmissionapp.ConfigV1{
			Diagnostics: os.Stderr,
			Observer:    filestore.CaseBindingReader{}, Identity: identityAuthority,
			Evidence:  sharedEvidenceDatasetSnapshotV2.evidence,
			Snapshots: sharedEvidenceDatasetSnapshotV2.snapshot,
			Materials: datasetSnapshotStoresV2,
			Native:    nativeOwner, Source: immutableSource,
			ReadImportSource:         fundscsvsourceadapter.ReadImportExactV1,
			CaseCreator:              filestore.CaseBindingReader{},
			ActivateEvidenceRegistry: activateImportRegistry,
		})
		if err != nil {
			return nil, errors.Join(err, nativeAuthority.Close())
		}
		if fundsAccountFlow.localDisplaySource != nil {
			fundsCleaning, err = fundscleaningapp.NewServiceV1(fundscleaningapp.ConfigV1{
				Identity: identityAuthority, Source: fundsAccountFlow.localDisplaySource,
				Native: nativeOwner, Admission: fundsCSVAdmission,
			})
			if err != nil {
				return nil, errors.Join(err, nativeAuthority.Close())
			}
		}
	}
	processProtectedRoots := filestore.EffectiveProcessProtectedRoots(
		sandboxSettings.ProtectedReadDirs,
		config.SandboxMode,
	)
	var hostFundsNativeOwnerCurrent func(context.Context) error
	if nativeOwner != nil {
		hostFundsNativeOwnerCurrent = nativeOwner.ValidateCurrentDataEngine
	}
	mcpManager = mcp.NewProductionManagerWithOptions(mcpSpecs, mcp.ProductionManagerOptions{
		CacheDir:                    filepath.Join(config.DataDir, "mcp-schema-cache"),
		ProxyURL:                    mcpProxyURL,
		ProtectedReadDirs:           processProtectedRoots,
		DatasetAuthority:            datasetSnapshotCurrentAuthorityV2,
		HostFundsServer:             bundledFundsHostSpec,
		HostFundsNativeOwnerCurrent: hostFundsNativeOwnerCurrent,
		AccountFlowExecutor:         runtimeFundsAccountFlowExecutorV1(nativeAuthority),
	})
	if supplied, ok := ctx.Value(bundledFundsHostValidationContextKeyV1{}).(bundledFundsHostValidationDependenciesV1); !simulation && ok && supplied.observeHostSourceRead != nil {
		supplied.observeHostSourceRead(mcpManager.HostFundsSourceReadCapabilityEventsV1)
	}
	caseFinalizer := evidenceapp.WithToolEvidenceAuthority(
		evidenceapp.NewCasePublicationFinalizerWithHostEvidenceAuthority(
			evidenceStore, evidenceStore, finalAuthority, privateFinalStore, finalEventIO,
			terminalCoordinator, mcpManager, filestore.CaseBindingReader{}, trustedFinals,
		),
		evidenceapp.ToolEvidenceService{Issuer: evidenceIssuer, Reader: mcpManager},
	)
	var providerRegistryAuthority *providerRegistryAuthorityV1
	if config.DarwinSecretStoreKeychainDBPath == "" &&
		config.DarwinSecretStoreKeychainBindingDigest == "" &&
		config.DarwinSecretStoreKeychainSecurityDigest == "" {
		providerRegistryAuthority, err = openProviderRegistryAuthorityV1(ctx, config.DataDir)
	} else {
		providerRegistryAuthority, err = openProviderRegistryAuthorityV1(ctx, config.DataDir, secretstore.Options{
			DarwinKeychainDBPath:             config.DarwinSecretStoreKeychainDBPath,
			DarwinKeychainBindingDigest:      config.DarwinSecretStoreKeychainBindingDigest,
			DarwinKeychainSecurityDigest:     config.DarwinSecretStoreKeychainSecurityDigest,
			DarwinKeychainAuthorityStorePath: filepath.Join(authorityDataDir, "private", "provider-secrets", providerRegistrySecretStoreFileV1),
		})
	}
	if err != nil {
		return nil, errors.Join(err, nativeAuthority.Close())
	}
	mcpManager.SetAccountCredentialResolver(providerRegistryAuthority.Manager())
	providerRegistryAuthorityTransferred := false
	defer func() {
		if !providerRegistryAuthorityTransferred {
			resultErr = errors.Join(resultErr, providerRegistryAuthority.Close())
		}
	}()
	asyncObserver, _ := ctx.Value(asyncTurnObservationContextKeyV1{}).(func(server.AsyncTurnObservationV1))
	phaseObserver, _ := ctx.Value(asyncTurnPhaseObservationContextKeyV1{}).(func(string))
	handler, err := server.NewRuntimeServerHandlerFromComponents(config, server.RuntimeServerComponents{
		AsyncTurnObserverV1:      asyncObserver,
		AsyncTurnPhaseObserverV1: phaseObserver,

		ChildIdentityFloors: childFloors,
		Store:               store,
		Attachments:         attachmentStore,
		AttachmentAccess:    attachmentAccess,
		AttachmentUses:      attachmentUses,
		Memories:            memoryStore,
		CaseFinalizer:       caseFinalizer,
		CaseThreads:         caseThreads,
		TurnSecurity: turnsecurityapp.WorkspaceSecurityAuthority{
			Observer:            filestore.CaseBindingReader{},
			Identity:            identityAuthority,
			RiskAuthority:       threadRiskAuthority,
			SnapshotAuthority:   datasetSnapshotAuthority,
			SnapshotAuthorityV2: datasetSnapshotAuthorityV2,
		},
		Continuations:     continuations,
		PendingWork:       pendingWork,
		Checkpoints:       checkpointapp.SnapshotAuthority{Store: checkpointAuthorityStore},
		PublicProjector:   publicProjector,
		InfoDataDir:       runtimeInfoDataDir,
		Provider:          providerClient,
		ProviderConfig:    providerConfig,
		ProviderExecution: newProviderRegistryExecutionResolverWithPricingV1(providerRegistryAuthority.Manager(), config.ModelProvidersJSON),
		ProviderRegistry:  providerRegistryAuthority.Service(),
		MediaExecution:    mediaexecutionapp.New(providerRegistryAuthority.Manager(), mediaexecutiontransport.New),
		ModelProxyURL:     strings.TrimSpace(config.ModelProxyURL),
		ApprovalPolicy:    config.ApprovalPolicy,
		SandboxMode:       config.SandboxMode,
		Gate:              controlapp.NewApprovalUserInputManager(),
		MCP:               mcpManager,
		CommandProbe:      processadapter.NewCommandProbe(processProtectedRoots...),
		CommandHomeDir:    processadapter.HomeDir(),
		WorkspaceProbe:    processadapter.NewWorkspaceStatusProbe(processProtectedRoots...),
		ShellRunner:       processadapter.NewShellRunner(),
		GitStatusProbe:    processadapter.NewGitStatusProbe(processProtectedRoots...),
		WorktreeManager: processadapter.NewWorktreeManager(
			filepath.Join(config.DataDir, "subagent-worktrees"),
			processProtectedRoots...,
		),
		MCPSearch:          mcpSearch,
		AllowWriteRoots:    sandboxSettings.AllowWriteRoots,
		ProtectedReadDirs:  sandboxSettings.ProtectedReadDirs,
		Jobs:               jobManager,
		ChildCompletions:   childCompletions,
		ForegroundHandoffs: foregroundHandoffs,
		G6Readiness:        config.G6Readiness,
		AutoResearch:       research.NewAutoResearchProjectStore(nil),
		Skills:             skillCatalog,
		Subagents:          subagentConfig,
		SubagentState:      subagentState,
		NativeAuthority:    nativeAuthority,
		CaseEntities:       fundsAccountFlow.caseEntities,
		ResolveCaseIngress: runtimeAccountIngressResolverV1(nativeAuthority),
		CaseAnswerSlots: func(
			operationContext context.Context,
			parent domainsecurity.TurnSecurityContext,
			outputs []domainnative.AccountFlowProviderModelOutputV1,
		) ([]domainjob.CaseDelegatedAnswerSlotBindingV1, error) {
			return evidenceapp.ResolveCurrentCaseForegroundAnswerSlotsV1(
				operationContext, evidenceStore, parent, outputs,
			)
		},
		SteeringAuthority: steeringauthorityapp.NewService(finalAuthority),
		Web:               webConfig,
		VisionBridge:      visionBridgeConfig,
		StepLimits:        stepLimits,
		HeartbeatInterval: 15 * time.Second,
		SkipMCPConnect:    simulation,
	})
	if err != nil {
		return nil, errors.Join(err, nativeAuthority.Close())
	}
	handler, err = bindRuntimeOwnedResourceV1(handler, providerRegistryAuthority)
	if err != nil {
		return nil, errors.Join(err, nativeAuthority.Close())
	}
	if !simulation && evidenceDomainAvailable {
		if sharedEvidenceDatasetSnapshotV2.registryOwner != nil {
			handler, err = bindRuntimeOwnedResourceV1(handler, sharedEvidenceDatasetSnapshotV2.registryOwner)
			if err != nil {
				return nil, errors.Join(err, nativeAuthority.Close())
			}
		}
		handler, err = bindRuntimeOwnedResourceV1(handler, datasetSnapshotStoresV2)
		if err != nil {
			return nil, errors.Join(err, nativeAuthority.Close())
		}
		handler, err = bindRuntimeOwnedResourceV1(handler, evidenceAuthorityStoresV2)
		if err != nil {
			return nil, errors.Join(err, nativeAuthority.Close())
		}
	}
	if !simulation && caseEntityCapability.store != nil {
		handler, err = bindRuntimeOwnedResourceV1(handler, &caseEntityCapability)
		if err != nil {
			return nil, errors.Join(err, nativeAuthority.Close())
		}
	}
	localDisplayHandler := httpapi.LocalDisplayMuxV1{
		RuntimeToken: config.RuntimeToken,
		Insecure:     config.Insecure,
		Next:         handler,
		LocalDisplay: httpapi.LocalDisplayHandlerV1{
			ObjectEditing:     newObjectEditingHandler(config, identityAuthority, sandboxSettings.ProtectedReadDirs),
			FundsCSVAdmission: fundsCSVAdmission,
			FundsCleaning:     fundsCleaning,
			Service: localdisplayapp.NewServiceWithTypedLocalDataSurface(
				fundsAccountFlow.caseEntities,
				privateFinalStore,
				localdisplayapp.ImportMappingPreviewDependenciesV1{
					Identity: identityAuthority, Reader: fundsCSVAdmission,
				},
				localdisplayapp.CleaningDiffPreviewDependenciesV1{
					Identity: identityAuthority, Reader: fundsCleaning,
				},
				localdisplayapp.DirectSourcePreviewDependenciesV1{
					Identity:               identityAuthority,
					UseCurrentLocalDisplay: fundsAccountFlow.useCurrentLocalDisplay,
					Runner:                 directSourcePreviewRunner,
				},
				localdisplayapp.AcceptedSlotDisplayDependenciesV1{
					Identity: identityAuthority, Evidence: evidenceStore,
					UseRetainedSource: fundsAccountFlow.useRetainedAcceptedSlotDisplay,
				},
			),
			LoadFrozenSecurityContext: func(threadID, turnID string) (domainsecurity.TurnSecurityContext, error) {
				return appturn.LoadFrozenSecurityContext(store, threadID, turnID)
			},
			LoadCurrentSecurityContext: func(threadID string) (domainsecurity.TurnSecurityContext, error) {
				threadID = strings.TrimSpace(threadID)
				thread, loadErr := store.GetThread(threadID)
				storedThreadID, storedThreadIDOK := thread["id"].(string)
				if loadErr != nil || !storedThreadIDOK ||
					strings.TrimSpace(storedThreadID) != threadID {
					return domainsecurity.TurnSecurityContext{}, errors.New("current local display context is unavailable")
				}
				securityContext, parseErr := domainsecurity.ParseTurnSecurityContext(thread["securityState"])
				if parseErr != nil ||
					domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(securityContext) != nil ||
					securityContext.ThreadID != threadID {
					return domainsecurity.TurnSecurityContext{}, errors.New("current local display context is unavailable")
				}
				return securityContext, nil
			},
		},
	}
	boundHandler, err := bindFinalPublicationAuthorityIdentityV1(localDisplayHandler, finalAuthority)
	if err != nil {
		return nil, errors.Join(err, nativeAuthority.Close())
	}
	if !simulation && caseEntityCapability.store != nil {
		caseEntityOwnedByHandler = true
	}
	if !simulation && evidenceDomainAvailable {
		evidenceResourcesOwnedByHandler = true
	}
	// Semantic-stage handlers are discarded after planning, so their Registry
	// resources close through the defer above. Only a live handler owns them.
	providerRegistryAuthorityTransferred = !simulation
	return boundHandler, nil
}

// Native-component availability is an additive case capability. Missing or
// untrusted packaged native material must fail the protected funds lane
// closed, but it must not prevent the ordinary Agent runtime from starting.
// Lifecycle failures are deliberately excluded: an owner/child whose cleanup
// cannot be proved is a process-safety failure, not an optional capability.
func nativeAdmissionMayDisableCaseCapability(err error) bool {
	return !errors.Is(err, nativecomponenthost.ErrLifecycle) &&
		(errors.Is(err, nativecomponenthost.ErrUnavailable) ||
			errors.Is(err, nativecomponenthost.ErrTrust))
}

func newRuntimeHostIdentityAuthority(
	installationAuthority finalauthorityport.Authority,
) (*appidentity.InstallationLocalAuthority, error) {
	if installationAuthority == nil {
		return nil, errors.New("runtime host identity authority is unavailable")
	}
	authority, err := appidentity.NewInstallationLocalAuthority(installationAuthority.KeyID())
	if err != nil {
		return nil, errors.Join(errors.New("runtime host identity authority is unavailable"), err)
	}
	return authority, nil
}

func validateControlledArtifactHostConfigV2(config Config) error {
	values := []string{
		config.ControlledArtifactHostV2URL,
		config.ControlledArtifactHostV2Token,
		config.ControlledArtifactHostV2BackendGeneration,
		config.ControlledArtifactHostV2AllocationDigest,
		config.ControlledArtifactHostV2TLSRootCertDER,
		config.ControlledArtifactHostV2TLSLeafSPKISHA256,
	}
	nonEmpty := 0
	for _, value := range values {
		if value != "" {
			nonEmpty++
		}
		if value != strings.TrimSpace(value) {
			return errors.New("desktop controlled artifact host V2 configuration is invalid")
		}
	}
	if nonEmpty == 0 {
		return nil
	}
	if nonEmpty != len(values) {
		return errors.New("desktop controlled artifact host V2 configuration is invalid")
	}
	client, err := newControlledArtifactHostClientV2(config)
	if err != nil {
		return errors.New("desktop controlled artifact host V2 configuration is invalid")
	}
	client.Close()
	return nil
}

func newControlledArtifactHostClientV2(
	config Config,
) (*piiauthorizationstore.DesktopControlledHostClientV2, error) {
	generation, generationErr := strconv.ParseUint(config.ControlledArtifactHostV2BackendGeneration, 10, 64)
	rootDER, rootErr := base64.RawURLEncoding.Strict().DecodeString(config.ControlledArtifactHostV2TLSRootCertDER)
	defer clearRuntimePrivateBytesV2(rootDER)
	if generationErr != nil || generation == 0 || generation > 1<<53-1 ||
		strconv.FormatUint(generation, 10) != config.ControlledArtifactHostV2BackendGeneration ||
		!domainsecurity.IsSHA256Hex(config.ControlledArtifactHostV2AllocationDigest) ||
		rootErr != nil || base64.RawURLEncoding.EncodeToString(rootDER) != config.ControlledArtifactHostV2TLSRootCertDER {
		return nil, errors.New("desktop controlled artifact host V2 configuration is invalid")
	}
	return piiauthorizationstore.NewDesktopControlledHostClientV2(
		piiauthorizationstore.DesktopControlledHostConfigV2{
			Origin: config.ControlledArtifactHostV2URL, Secret: config.ControlledArtifactHostV2Token,
			BackendGeneration: generation, TLSRootCertificateDER: rootDER,
			TLSLeafSPKISHA256: config.ControlledArtifactHostV2TLSLeafSPKISHA256,
		},
	)
}

// ProbeControlledArtifactHostV2 proves a configured launch host is reachable
// through its exact TLS/token/generation transport while it remains quiescent.
// It is not evidence that ControlledAccessServiceV2 is composed or ready.
func ProbeControlledArtifactHostV2(ctx context.Context, config Config) error {
	if ctx == nil {
		return errors.New("desktop controlled artifact host V2 probe is unavailable")
	}
	if config.ControlledArtifactHostV2URL == "" {
		return ctx.Err()
	}
	if err := validateControlledArtifactHostConfigV2(config); err != nil {
		return err
	}
	client, err := newControlledArtifactHostClientV2(config)
	if err != nil {
		return errors.New("desktop controlled artifact host V2 probe is unavailable")
	}
	defer client.Close()
	if err := client.ProbeV2(ctx); err != nil {
		return errors.New("desktop controlled artifact host V2 probe is unavailable")
	}
	return nil
}

func clearRuntimePrivateBytesV2(value []byte) {
	for index := range value {
		value[index] = 0
	}
}

func AcquireRuntimePersistenceLease(config Config) (*persistencefs.CompositeLease, error) {
	config = normalizeConfig(config)
	if err := validateRuntimeProtectedRootTopology(config); err != nil {
		return nil, err
	}
	roots, err := resolveRuntimePersistenceRoots(config)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(config.UserDataDir) != "" {
		return persistencefs.AcquireCompositeLeaseWithStartupUserData(roots, config.UserDataDir)
	}
	return persistencefs.AcquireCompositeLease(roots)
}

// validateRuntimeProtectedRootTopology keeps runtime persistence outside
// app-wide mandatory deny roots. An overlapping layout would either expose
// protected host state through an exception or disable ordinary Agent child
// processes that need the runtime data root.
func validateRuntimeProtectedRootTopology(config Config) error {
	dataDir, err := filestore.WorkspaceRealPath(config.DataDir)
	if err != nil {
		return errors.New("runtime data root is invalid")
	}
	if userData := strings.TrimSpace(config.UserDataDir); userData != "" {
		userData, err = filestore.WorkspaceRealPath(userData)
		if err != nil {
			return errors.New("runtime user-data root is invalid")
		}
		if filestore.PathWithinRoot(dataDir, userData) ||
			filestore.PathWithinRoot(userData, dataDir) {
			return errors.New("runtime data and user-data roots must not overlap")
		}
	}
	mandatoryRoots := []string{
		config.AuthorityManifestRoot,
		config.AuthorityCredentialProfileRoot,
		config.AuthorityCredentialBundleRoot,
	}
	for _, protectedRoot := range filestore.NormalizeRealRoots(mandatoryRoots) {
		if filestore.PathWithinRoot(protectedRoot, dataDir) {
			return errors.New("runtime mandatory protected root must not contain the runtime data root")
		}
	}
	return nil
}

func normalizeConfig(config Config) Config {
	if strings.TrimSpace(config.StartedAt) == "" {
		config.StartedAt = time.Now().UTC().Format(time.RFC3339Nano)
	}
	if strings.TrimSpace(config.Host) == "" {
		config.Host = "127.0.0.1"
	}
	if strings.TrimSpace(config.DataDir) == "" {
		config.DataDir = "/tmp/analytix"
	}
	if config.G6Readiness.SchemaVersion == 0 {
		config.G6Readiness = server.DefaultRuntimeReadinessStatus()
	}
	if approvalPolicy := controlapp.NormalizeApprovalPolicy(config.ApprovalPolicy); approvalPolicy != "" {
		config.ApprovalPolicy = approvalPolicy
	} else {
		config.ApprovalPolicy = executionpolicy.DefaultApprovalPolicy
	}
	if sandboxMode := controlapp.NormalizeSandboxMode(config.SandboxMode); sandboxMode != "" {
		config.SandboxMode = sandboxMode
	} else {
		config.SandboxMode = executionpolicy.DefaultSandboxMode
	}
	return config
}

func validateRuntimeAuthentication(config Config) error {
	if !config.Insecure && strings.TrimSpace(config.RuntimeToken) == "" {
		return errors.New("runtime token is required unless insecure mode is explicitly enabled")
	}
	return nil
}
