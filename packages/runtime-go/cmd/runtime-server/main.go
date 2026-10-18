package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unicode"

	backendgenerationfs "analytix.local/runtime-go/internal/adapters/outbound/backendgenerationfs"
	persistencefs "analytix.local/runtime-go/internal/adapters/outbound/persistencefs"
	backendgenerationapp "analytix.local/runtime-go/internal/app/backendgeneration"
	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
	domainmcp "analytix.local/runtime-go/internal/domain/mcp"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainregistry "analytix.local/runtime-go/internal/domain/providerregistry"
	"analytix.local/runtime-go/internal/runtimeapp"
)

const defaultRuntimeServerShutdownTimeout = 30 * time.Second

const backendGenerationConsumedMarkerV1 = "ANALYTIX_BACKEND_GENERATION_CONSUMED "
const desktopPrivateHistoryMigrationReadyMarkerV2 = "ANALYTIX_DESKTOP_PRIVATE_HISTORY_MIGRATION_READY_V2"
const desktopInstallationKeyPreflightReadyMarkerV1 = "ANALYTIX_DESKTOP_INSTALLATION_KEY_PREFLIGHT_READY_V1"

type runtimeServerStartup interface {
	ActivateContext(context.Context, runtimeapp.Config) (http.Handler, error)
}

type runtimeServerDependencies struct {
	prepare       func(context.Context, runtimeapp.Config, *runtimeapp.PersistenceLease) (runtimeServerStartup, error)
	listen        func(string, string) (net.Listener, error)
	stdin         io.Reader
	unsetenv      func(string) error
	notifyContext func(context.Context, ...os.Signal) (context.Context, context.CancelFunc)
}

func defaultRuntimeServerDependencies() runtimeServerDependencies {
	return runtimeServerDependencies{
		prepare: func(ctx context.Context, config runtimeapp.Config, lease *runtimeapp.PersistenceLease) (runtimeServerStartup, error) {
			return runtimeapp.PrepareRuntimeServerStartupWithPersistenceLeaseContextE(ctx, config, lease)
		},
		listen: net.Listen, stdin: os.Stdin, unsetenv: os.Unsetenv, notifyContext: signal.NotifyContext,
	}
}

func main() {
	if err := runRuntimeServer(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "[analytix] event=ANALYTIX_RUNTIME_SERVER_FAILED")
		os.Exit(1)
	}
}

func runRuntimeServer(args []string) (runErr error) {
	return runRuntimeServerWithDependencies(args, defaultRuntimeServerDependencies())
}

func runRuntimeServerWithDependencies(args []string, dependencies runtimeServerDependencies) (runErr error) {
	if len(args) > 0 && args[0] == "bundled-plugin" {
		commandCtx, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stopSignals()
		return runBundledPluginCommandV1(
			commandCtx, args[1:], os.Stdout, defaultBundledFundsMaterializationDependenciesV1(),
		)
	}
	if len(args) > 0 && args[0] == "provider" {
		return runDevelopmentProviderVerify(args[1:], dependencies.stdin, os.Stdout)
	}
	if len(args) > 0 && args[0] == "authority" {
		return runRuntimeAuthorityCommand(args[1:], os.Stdout, rand.Reader)
	}
	if len(args) > 0 && args[0] == "migration" {
		return runRuntimeMigrationCommand(args[1:], os.Stdout)
	}
	started := time.Now()
	cli, err := parseRuntimeServerCLI(args)
	if err != nil {
		return fmt.Errorf("parse flags: %w", err)
	}

	effectiveRuntimeToken := cli.RuntimeToken
	if effectiveRuntimeToken == "" && !cli.Insecure {
		return errors.New("runtime token is required unless insecure mode is explicitly enabled")
	}
	if err := validateRuntimeListenSecurity(cli); err != nil {
		return err
	}
	if cli.PrivateStartupFrameV1 {
		frame, err := readRuntimeStartupPrivateFrameV1(dependencies.stdin)
		if err != nil {
			return err
		}
		cli.DevelopmentProviderAuthorityDir = frame.DevelopmentProviderAuthorityDir
		if frame.ProtectedAuthorityV1 != nil {
			authority, err := frame.ProtectedAuthorityV1.config()
			if err != nil {
				return err
			}
			cli.MainOwnedAuthorityV1 = &authority
			identity := frame.ProtectedAuthorityV1.AuthorityAnchorV1
			cli.MainOwnedAuthorityIdentityV1 = &identity
		}
		if frame.HostScheduleMCPBindingV1 != nil {
			hostScheduleSpec, err := frame.HostScheduleMCPBindingV1.spec()
			if err != nil {
				return err
			}
			cli.HostScheduleMCPServer = &hostScheduleSpec
		}
		if frame.DarwinSecretStoreKeychainBindingV1 != nil {
			binding, err := frame.DarwinSecretStoreKeychainBindingV1.config(cli.UserDataDir, cli.DataDir)
			if err != nil {
				return err
			}
			cli.DarwinSecretStoreKeychainV1 = &binding
		}
	}
	config := runtimeConfigFromCLI(cli, effectiveRuntimeToken, 0)
	runtimeCtx, stopSignals := dependencies.notifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()
	lease, err := runtimeapp.AcquireRuntimePersistenceLease(config)
	if err != nil {
		return err
	}
	emitStartupOwnerPhaseV1(os.Stderr, started, startupGoLeaseAcquiredV1)
	defer func() {
		runErr = errors.Join(runErr, lease.Close())
	}()
	prepared, err := dependencies.prepare(runtimeCtx, config, lease)
	if err != nil {
		if errors.Is(err, context.Canceled) && runtimeCtx.Err() != nil {
			return nil
		}
		emitStartupPrepareFailureV1(os.Stderr, started, err)
		return err
	}
	emitStartupOwnerPhaseV1(os.Stderr, started, startupGoSemanticPreparedV1)
	if err := runtimeCtx.Err(); err != nil {
		return nil
	}
	if err := clearRuntimeServerSecretEnvironment(dependencies.unsetenv); err != nil {
		return err
	}
	listener, err := dependencies.listen("tcp", cli.Addr)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	emitStartupOwnerPhaseV1(os.Stderr, started, startupGoListenerBoundV1)
	defer listener.Close()
	tcpAddr, _ := listener.Addr().(*net.TCPAddr)
	if tcpAddr != nil {
		config.Port = tcpAddr.Port
		config.Host = tcpAddr.IP.String()
	}
	handler, err := prepared.ActivateContext(runtimeCtx, config)
	if err != nil {
		if errors.Is(err, context.Canceled) && runtimeCtx.Err() != nil {
			return nil
		}
		return err
	}
	emitStartupOwnerPhaseV1(os.Stderr, started, startupGoActivatedV1)
	authoritySource, ok := handler.(runtimeapp.FinalPublicationAuthorityIdentitySourceV1)
	if !ok {
		return errors.New("runtime final publication authority identity is unavailable")
	}
	finalPublicationAuthority, err := authoritySource.FinalPublicationAuthorityIdentityV1()
	if err != nil {
		return err
	}
	if err := runtimeapp.ValidateFinalPublicationAuthorityIdentityV1(finalPublicationAuthority); err != nil {
		return err
	}
	server := &http.Server{Handler: handler}
	runtimeURL := "http://" + listener.Addr().String()
	if err := runtimeapp.ProbeControlledArtifactHostV2(runtimeCtx, config); err != nil {
		return err
	}
	emitStartupOwnerPhaseV1(os.Stderr, started, startupGoHostProbedV1)
	controlledArtifactLaunch, err := runtimeapp.ControlledArtifactSidecarLaunchBindingProofV2(
		config, runtimeURL, uint64(os.Getpid()), effectiveRuntimeToken != "", false,
	)
	if err != nil {
		return err
	}
	var witnessedAuthorityInstallationID string
	var witnessedAuthorityKeyID string
	var witnessedAuthorityManifestDigest string
	if cli.MainOwnedAuthorityIdentityV1 != nil {
		witnessedAuthorityInstallationID = cli.MainOwnedAuthorityIdentityV1.InstallationID
		witnessedAuthorityKeyID = cli.MainOwnedAuthorityIdentityV1.AuthorityKeyID
		witnessedAuthorityManifestDigest = cli.MainOwnedAuthorityIdentityV1.CurrentManifestDigest
	}
	ready := map[string]any{
		"url":                                             runtimeURL,
		"runtimePid":                                      os.Getpid(),
		"runtimeTokenConfigured":                          effectiveRuntimeToken != "",
		"persistenceRootsConfigured":                      true,
		"productionRuntime":                               true,
		"controlledArtifactHostV2Configured":              controlledArtifactLaunch.Configured,
		"controlledArtifactHostV2Ready":                   controlledArtifactLaunch.Ready,
		"controlledArtifactHostV2BackendGeneration":       controlledArtifactLaunch.BackendGeneration,
		"controlledArtifactHostV2LaunchBindingProof":      controlledArtifactLaunch.Proof,
		"finalPublicationAuthorityKeyId":                  finalPublicationAuthority.KeyID,
		"finalPublicationAuthorityPublicKey":              base64.RawURLEncoding.EncodeToString(finalPublicationAuthority.PublicKey),
		"witnessedAuthorityV2Configured":                  cli.MainOwnedAuthorityIdentityV1 != nil,
		"witnessedAuthorityInstallationId":                witnessedAuthorityInstallationID,
		"witnessedAuthorityKeyId":                         witnessedAuthorityKeyID,
		"witnessedAuthorityManifestDigest":                witnessedAuthorityManifestDigest,
		"datasetSnapshotSelectionV2Configured":            false,
		"datasetSnapshotAdmissionV2State":                 "absent",
		"datasetSnapshotAdmissionV2InstallationId":        "",
		"datasetSnapshotAdmissionV2RuntimeLaunchNonce":    "",
		"datasetSnapshotAdmissionV2StagingBindingDigest":  "",
		"datasetSnapshotAdmissionV2SelectionDigest":       "",
		"datasetSnapshotAdmissionV2SnapshotId":            "",
		"datasetSnapshotAdmissionV2AuthorityRecordDigest": "",
		"datasetSnapshotAdmissionV2AckHmacSha256":         "",
	}
	readyJSON, _ := json.Marshal(ready)
	emitStartupOwnerPhaseV1(os.Stderr, started, startupGoReadyBuiltV1)
	fmt.Printf("ANALYTIX_RUNTIME_SERVER_READY %s\n", readyJSON)

	errCh := make(chan error, 1)
	go func() {
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
		close(errCh)
	}()

	select {
	case <-runtimeCtx.Done():
	case err := <-errCh:
		if err != nil {
			return errors.Join(fmt.Errorf("serve: %w", err), shutdownRuntimeServerUntilDrained(server, handler))
		}
		return shutdownRuntimeServerUntilDrained(server, handler)
	}
	return shutdownRuntimeServerUntilDrained(server, handler)
}

func runRuntimeMigrationCommand(args []string, output io.Writer) error {
	if len(args) > 0 && args[0] == "prepare-desktop-installation-key-v1" {
		return runRuntimeInstallationKeyPreflightCommand(args[1:], output)
	}
	if len(args) == 0 || args[0] != "migrate-desktop-private-history-v2" || output == nil {
		return errors.New("runtime migration command is invalid")
	}
	flags := flag.NewFlagSet("migrate-desktop-private-history-v2", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	dataDir := flags.String("data-dir", "", "Analytix runtime data root")
	durableDir := flags.String("durable-root", "", "Analytix runtime durable root")
	userDataDir := flags.String("user-data-dir", "", "Electron userData root")
	if err := flags.Parse(args[1:]); err != nil || flags.NArg() != 0 ||
		!exactAbsoluteCLIPath(*dataDir) || !exactAbsoluteCLIPath(*durableDir) ||
		!exactAbsoluteCLIPath(*userDataDir) {
		return errors.New("runtime migration command is invalid")
	}
	migrationCtx, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()
	if err := runtimeapp.RunDesktopPrivateHistoryMigrationV2(
		migrationCtx, *dataDir, *durableDir, *userDataDir,
	); err != nil {
		// The desktop barrier needs only a fail-closed outcome. Do not reflect a
		// persistence path, digest, file body, credential, or task content.
		return errors.New("desktop private history migration failed")
	}
	if _, err := fmt.Fprintln(output, desktopPrivateHistoryMigrationReadyMarkerV2); err != nil {
		return errors.New("runtime migration marker write failed")
	}
	return nil
}

func runRuntimeInstallationKeyPreflightCommand(args []string, output io.Writer) error {
	if output == nil || !exactlyOneCLIFlagV1(args, "--data-dir") ||
		!exactlyOneCLIFlagV1(args, "--user-data-dir") ||
		(exactlyOneCLIFlagV1(args, "--durable-root") == exactlyOneCLIFlagV1(args, "--runtime-durable-root")) ||
		!exactlyZeroCLIFlagV1(args, "--durable-temp-dir") {
		return errors.New("desktop installation key preflight command is invalid")
	}
	cli, err := parseRuntimeServerCLI(args)
	if err != nil || cli.PrivateStartupFrameV1 || cli.UserDataDir == "" ||
		!exactAbsoluteCLIPath(firstNonEmpty(cli.ProductionDurableRoot, cli.RuntimeDurableRoot)) ||
		cli.RuntimeToken == "" || cli.Insecure {
		return errors.New("desktop installation key preflight command is invalid")
	}
	preflightCtx, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()
	if err := runtimeapp.RunDesktopInstallationKeyPreflightV1(
		preflightCtx, runtimeConfigFromCLI(cli, cli.RuntimeToken, 0),
	); err != nil {
		return errors.New("desktop installation key preflight failed")
	}
	if _, err := fmt.Fprintln(output, desktopInstallationKeyPreflightReadyMarkerV1); err != nil {
		return errors.New("desktop installation key preflight marker write failed")
	}
	return nil
}

func exactlyOneCLIFlagV1(args []string, name string) bool {
	count := 0
	for _, argument := range args {
		if argument == name || strings.HasPrefix(argument, name+"=") {
			count++
		}
	}
	return count == 1
}

func exactlyZeroCLIFlagV1(args []string, name string) bool {
	for _, argument := range args {
		if argument == name || strings.HasPrefix(argument, name+"=") {
			return false
		}
	}
	return true
}

func exactAbsoluteCLIPath(value string) bool {
	return value != "" && value == strings.TrimSpace(value) && filepath.IsAbs(value) && filepath.Clean(value) == value
}

type backendGenerationConsumedMarkerPayloadV1 struct {
	SchemaVersion          int    `json:"schemaVersion"`
	Generation             uint64 `json:"generation"`
	AllocationRecordDigest string `json:"allocationRecordDigest"`
}

func runRuntimeAuthorityCommand(args []string, output io.Writer, random io.Reader) (resultErr error) {
	if len(args) == 0 || args[0] != "consume-backend-generation-v1" || output == nil || random == nil {
		return errors.New("runtime authority command is invalid")
	}
	flags := flag.NewFlagSet("consume-backend-generation-v1", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	userData := flags.String("user-data-dir", "", "final Electron userData authority root")
	if err := flags.Parse(args[1:]); err != nil || flags.NArg() != 0 {
		return errors.New("runtime authority command is invalid")
	}
	trimmedUserData := strings.TrimSpace(*userData)
	if trimmedUserData == "" || trimmedUserData != *userData || !filepath.IsAbs(trimmedUserData) ||
		filepath.Clean(trimmedUserData) != trimmedUserData {
		return errors.New("runtime authority user data directory is invalid")
	}
	roots, err := persistencefs.ResolveRootSet(trimmedUserData, trimmedUserData)
	if err != nil {
		return err
	}
	lease, err := persistencefs.AcquireCompositeLease(roots)
	if err != nil {
		return err
	}
	leaseClosed := false
	defer func() {
		if !leaseClosed {
			resultErr = errors.Join(resultErr, lease.Close())
		}
	}()
	allocationRoot, err := backendgenerationfs.AllocationRootForUserDataV1(trimmedUserData)
	if err != nil {
		return err
	}
	prepared, err := backendgenerationfs.PrepareRecoveryV1(context.Background(), allocationRoot, lease)
	if err != nil {
		return err
	}
	if err := prepared.ValidateSemantics(context.Background()); err != nil {
		return err
	}
	if err := prepared.Revalidate(context.Background()); err != nil {
		return err
	}
	authorityPreflight, err := persistencefs.PrepareJournalAuthorityBootstrapV1(
		context.Background(), lease, prepared,
	)
	if err != nil {
		return err
	}
	if _, present, err := authorityPreflight.BindExistingV1(context.Background()); err != nil {
		return err
	} else if !present {
		if _, err := authorityPreflight.CreateV1(context.Background()); err != nil {
			return err
		}
	}
	journal, err := persistencefs.NewPrivateCASRecoveryJournalV1(lease)
	if err != nil {
		return err
	}
	if err := prepared.ApplyV4(context.Background(), journal); err != nil {
		return err
	}
	store, err := backendgenerationfs.OpenStoreV1(context.Background(), allocationRoot, lease)
	if err != nil {
		return err
	}
	allocator, err := backendgenerationapp.NewAllocatorV1(store, random)
	if err != nil {
		return errors.Join(err, store.Close())
	}
	consumed, err := allocator.Consume(context.Background())
	if err != nil {
		return errors.Join(err, store.Close())
	}
	if err := store.Close(); err != nil {
		return err
	}
	if err := lease.Close(); err != nil {
		return err
	}
	leaseClosed = true
	payload, err := json.Marshal(backendGenerationConsumedMarkerPayloadV1{
		SchemaVersion: 1, Generation: consumed.Record.Generation,
		AllocationRecordDigest: consumed.RecordDigest,
	})
	if err != nil {
		return errors.New("runtime authority marker encoding failed")
	}
	if _, err := fmt.Fprintf(output, "%s%s\n", backendGenerationConsumedMarkerV1, payload); err != nil {
		return errors.New("runtime authority marker write failed")
	}
	return nil
}

func validateRuntimeListenSecurity(cli runtimeServerCLIConfig) error {
	if !cli.Insecure {
		return nil
	}
	host, _, err := net.SplitHostPort(cli.Addr)
	if err != nil {
		return errors.New("insecure runtime listen address is invalid")
	}
	host = strings.TrimSpace(strings.Trim(host, "[]"))
	if strings.EqualFold(host, "localhost") {
		return nil
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return errors.New("insecure runtime must listen on an explicit loopback address")
	}
	return nil
}

func shutdownRuntimeServerWithTimeout(server *http.Server, handler http.Handler) error {
	shutdownCtx, cancel := context.WithTimeout(context.Background(), runtimeServerShutdownTimeout())
	defer cancel()
	return shutdownRuntimeServer(shutdownCtx, server, handler)
}

// shutdownRuntimeServerUntilDrained keeps the process-owned persistence lease
// live until every runtime writer and child has acknowledged termination. A
// failed bounded attempt closes admissions but is not permission to return to
// main's lease defer: returning could let a second runtime acquire the roots
// while an old child process still mutates them.
func shutdownRuntimeServerUntilDrained(server *http.Server, handler http.Handler) error {
	return retryRuntimeServerShutdown(
		func() error { return shutdownRuntimeServerWithTimeout(server, handler) },
		func(error) {
			fmt.Fprintln(os.Stderr, "[analytix] event=ANALYTIX_RUNTIME_SHUTDOWN_DRAIN_RETRY")
			time.Sleep(100 * time.Millisecond)
		},
	)
}

func retryRuntimeServerShutdown(attempt func() error, wait func(error)) error {
	if attempt == nil || wait == nil {
		return errors.New("runtime shutdown retry dependencies are unavailable")
	}
	for {
		err := attempt()
		if err == nil {
			return nil
		}
		wait(err)
	}
}

func runtimeConfigFromCLI(cli runtimeServerCLIConfig, runtimeToken string, port int) runtimeapp.Config {
	var authority runtimeMainOwnedAuthorityConfigV1
	if cli.MainOwnedAuthorityV1 != nil {
		authority = *cli.MainOwnedAuthorityV1
	}
	var darwinKeychain runtimeDarwinSecretStoreKeychainConfigV1
	if cli.DarwinSecretStoreKeychainV1 != nil {
		darwinKeychain = *cli.DarwinSecretStoreKeychainV1
	}
	return runtimeapp.Config{
		DevelopmentOfficeAssetRoot:              os.Getenv("ANALYTIX_DEVELOPMENT_OFFICE_ASSET_ROOT"),
		DocumentCodecExecutable:                 os.Getenv("ANALYTIX_DOCUMENT_CODEC_EXECUTABLE"),
		DocumentCodecEntry:                      os.Getenv("ANALYTIX_DOCUMENT_CODEC_ENTRY"),
		DevelopmentPluginSourceRoot:             os.Getenv("ANALYTIX_DEVELOPMENT_PLUGIN_SOURCE_ROOT"),
		RuntimeToken:                            runtimeToken,
		Insecure:                                cli.Insecure,
		DurableTempDir:                          firstNonEmpty(cli.RuntimeDurableRoot, cli.DurableTempDir),
		ProductionDurableRoot:                   cli.ProductionDurableRoot,
		Host:                                    cli.Host,
		Port:                                    port,
		DataDir:                                 cli.DataDir,
		UserDataDir:                             cli.UserDataDir,
		ProviderID:                              "",
		BaseURL:                                 "",
		APIKey:                                  "",
		Model:                                   "",
		EndpointFormat:                          "",
		ModelProvidersJSON:                      cli.ModelProvidersJSON,
		ModelProxyURL:                           "",
		ProviderAuditSocketPath:                 formalProviderAuditSocketPathV1(),
		MCPProxyURL:                             cli.MCPProxyURL,
		ApprovalPolicy:                          cli.ApprovalPolicy,
		SandboxMode:                             cli.SandboxMode,
		MCPConfigPath:                           firstNonEmpty(cli.MCPConfigPath, os.Getenv("ANALYTIX_MCP_CONFIG_PATH")),
		MCPConfigJSON:                           firstNonEmpty(cli.MCPConfigJSON, os.Getenv("ANALYTIX_MCP_CONFIG_JSON")),
		HostScheduleMCPServer:                   cloneRuntimeMCPServerSpecV1(cli.HostScheduleMCPServer),
		AllowWriteRoots:                         splitPathList(os.Getenv("ANALYTIX_ALLOW_WRITE_ROOTS")),
		ProtectedReadDirs:                       splitPathList(os.Getenv("ANALYTIX_PROTECTED_READ_DIRS")),
		AuthorityAnchorV1:                       authority.AnchorJSON,
		AuthorityManifestRoot:                   authority.ManifestRoot,
		AuthorityCredentialProfileRoot:          authority.CredentialProfileRoot,
		AuthorityCredentialBundleRoot:           authority.CredentialBundleRoot,
		DevelopmentProviderAuthorityDir:         cli.DevelopmentProviderAuthorityDir,
		DarwinSecretStoreKeychainDBPath:         darwinKeychain.DBPath,
		DarwinSecretStoreKeychainBindingDigest:  darwinKeychain.BindingDigest,
		DarwinSecretStoreKeychainSecurityDigest: darwinKeychain.SecurityDigest,
		ControlledArtifactHostV2URL:             os.Getenv("ANALYTIX_CONTROLLED_ARTIFACT_HOST_V2_URL"),
		ControlledArtifactHostV2Token:           os.Getenv("ANALYTIX_CONTROLLED_ARTIFACT_HOST_V2_TOKEN"),
		ControlledArtifactHostV2BackendGeneration: os.Getenv("ANALYTIX_CONTROLLED_ARTIFACT_HOST_V2_BACKEND_GENERATION"),
		ControlledArtifactHostV2AllocationDigest:  os.Getenv("ANALYTIX_CONTROLLED_ARTIFACT_HOST_V2_ALLOCATION_RECORD_DIGEST"),
		ControlledArtifactHostV2TLSRootCertDER:    os.Getenv("ANALYTIX_CONTROLLED_ARTIFACT_HOST_V2_TLS_ROOT_CERT_DER"),
		ControlledArtifactHostV2TLSLeafSPKISHA256: os.Getenv("ANALYTIX_CONTROLLED_ARTIFACT_HOST_V2_TLS_LEAF_SPKI_SHA256"),
	}
}

func clearRuntimeServerSecretEnvironment(unsetenv func(string) error) error {
	if unsetenv == nil {
		return errors.New("clear runtime secret environment")
	}
	var errs []error
	for _, name := range []string{
		"ANALYTIX_API_KEY",
		"ANALYTIX_MODEL_PROVIDERS",
		"ANALYTIX_HUB_TEST_GATEWAY_TOKEN",
		"ANALYTIX_HUB_TEST_DESKTOP_AUTH_TOKEN",
		"ANALYTIX_RUNTIME_TOKEN",
		"ANALYTIX_PROVIDER_AUDIT_SOCKET_PATH",
		"ANALYTIX_RUNTIME_GO_PACKAGED_MILESTONE_B",
		"ANALYTIX_AUTHORITY_ANCHOR_V1",
		"ANALYTIX_AUTHORITY_MANIFEST_ROOT",
		"ANALYTIX_AUTHORITY_CREDENTIAL_PROFILE_ROOT",
		"ANALYTIX_AUTHORITY_CREDENTIAL_BUNDLE_ROOT",
		// V1 is permanently retired but still scrubbed so an ambient legacy
		// authority can never survive into the runtime process.
		"ANALYTIX_CONTROLLED_ARTIFACT_HOST_URL",
		"ANALYTIX_CONTROLLED_ARTIFACT_HOST_TOKEN",
		"ANALYTIX_CONTROLLED_ARTIFACT_HOST_V2_URL",
		"ANALYTIX_CONTROLLED_ARTIFACT_HOST_V2_TOKEN",
		"ANALYTIX_CONTROLLED_ARTIFACT_HOST_V2_BACKEND_GENERATION",
		"ANALYTIX_CONTROLLED_ARTIFACT_HOST_V2_ALLOCATION_RECORD_DIGEST",
		"ANALYTIX_CONTROLLED_ARTIFACT_HOST_V2_TLS_ROOT_CERT_DER",
		"ANALYTIX_CONTROLLED_ARTIFACT_HOST_V2_TLS_LEAF_SPKI_SHA256",
	} {
		if err := unsetenv(name); err != nil {
			errs = append(errs, errors.New("clear runtime secret environment"))
		}
	}
	return errors.Join(errs...)
}

func formalProviderAuditSocketPathV1() string {
	if os.Getenv("ANALYTIX_RUNTIME_GO_PACKAGED_MILESTONE_B") != "1" {
		return ""
	}
	return strings.TrimSpace(os.Getenv("ANALYTIX_PROVIDER_AUDIT_SOCKET_PATH"))
}

type runtimeServerCLIConfig struct {
	Addr                            string
	Host                            string
	DurableTempDir                  string
	RuntimeDurableRoot              string
	ProductionDurableRoot           string
	RuntimeToken                    string
	Insecure                        bool
	DataDir                         string
	UserDataDir                     string
	ProviderID                      string
	BaseURL                         string
	Model                           string
	EndpointFormat                  string
	ModelProvidersJSON              string
	MCPConfigPath                   string
	MCPConfigJSON                   string
	ModelProxyURL                   string
	MCPProxyURL                     string
	ApprovalPolicy                  string
	SandboxMode                     string
	TokenEconomyMode                string
	HostScheduleMCPServer           *domainmcp.ServerSpec
	PrivateStartupFrameV1           bool
	MainOwnedAuthorityV1            *runtimeMainOwnedAuthorityConfigV1
	MainOwnedAuthorityIdentityV1    *authorityAnchorEnvelopeV1
	DevelopmentProviderAuthorityDir string
	DarwinSecretStoreKeychainV1     *runtimeDarwinSecretStoreKeychainConfigV1
}

type runtimeServerLifecycleHandler interface {
	Shutdown(context.Context) error
}

func shutdownRuntimeServer(ctx context.Context, server *http.Server, handler http.Handler) error {
	var errs []error
	quiesceCtx, cancelAdmissions := context.WithCancel(context.Background())
	cancelAdmissions()
	if err := server.Shutdown(quiesceCtx); err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, http.ErrServerClosed) {
		errs = append(errs, err)
	}
	if lifecycle, ok := handler.(runtimeServerLifecycleHandler); ok {
		if err := lifecycle.Shutdown(ctx); err != nil {
			errs = append(errs, err)
		}
	}
	if err := server.Shutdown(ctx); err != nil {
		errs = append(errs, err)
		_ = server.Close()
	}
	return errors.Join(errs...)
}

func runtimeServerShutdownTimeout() time.Duration {
	raw := strings.TrimSpace(os.Getenv("ANALYTIX_RUNTIME_SERVER_SHUTDOWN_TIMEOUT_MS"))
	if raw == "" {
		return defaultRuntimeServerShutdownTimeout
	}
	ms, err := strconv.Atoi(raw)
	if err != nil || ms <= 0 {
		return defaultRuntimeServerShutdownTimeout
	}
	return time.Duration(ms) * time.Millisecond
}

func parseRuntimeServerCLI(args []string) (runtimeServerCLIConfig, error) {
	const privateStartupFrameFlagV1 = "--private-startup-frame-v1"
	privateStartupFrameCount := 0
	for _, argument := range args {
		if argument == privateStartupFrameFlagV1 || strings.HasPrefix(argument, privateStartupFrameFlagV1+"=") {
			privateStartupFrameCount++
		}
	}
	if privateStartupFrameCount > 1 {
		return runtimeServerCLIConfig{}, errors.New("runtime private startup frame flag is duplicated")
	}
	fs := flag.NewFlagSet("runtime-server", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	addr := fs.String("addr", "", "listen address")
	host := fs.String("host", "127.0.0.1", "listen host; accepted for Electron analytix serve parity")
	port := fs.Int("port", 0, "listen port; accepted for Electron analytix serve parity")
	durableTempDir := fs.String("durable-temp-dir", "", "explicit temporary durable runtime store")
	runtimeDurableRoot := fs.String("runtime-durable-root", "", "explicit durable root for runtime validation")
	productionDurableRoot := fs.String("durable-root", "", "production durable root for the default Go runtime")
	insecure := fs.Bool("insecure", false, "allow local runtime requests without bearer-token auth")
	dataDir := fs.String("data-dir", "/tmp/analytix", "reported runtime data directory")
	userDataDir := fs.String("user-data-dir", "", "exact Electron userData startup authority root")
	providerID := fs.String("provider-id", "", "default analytix model provider id")
	baseURL := fs.String("base-url", "", "default provider base URL")
	model := fs.String("model", "", "default model id")
	endpointFormat := fs.String("endpoint-format", "chat_completions", "default provider endpoint format")
	modelProvidersJSON := fs.String("model-providers-json", "", "serialized analytix model provider config")
	mcpConfigPath := fs.String("mcp-config-path", "", "path to an Analytix/Claude-compatible .mcp.json file")
	mcpConfigJSON := fs.String("mcp-config-json", "", "serialized Analytix MCP config JSON")
	modelProxyURL := fs.String("model-proxy-url", "", "accepted for Electron analytix serve parity")
	mcpProxyURL := fs.String("mcp-proxy-url", "", "accepted for Electron analytix serve MCP parity")
	approvalPolicy := fs.String("approval-policy", "", "accepted for Electron analytix serve parity")
	sandboxMode := fs.String("sandbox-mode", "", "accepted for Electron analytix serve parity")
	tokenEconomyMode := fs.String("token-economy-mode", "", "accepted for Electron analytix serve parity")
	privateStartupFrameV1 := fs.Bool(
		"private-startup-frame-v1", false,
		"read one closed main-owned runtime authority document from private stdin",
	)
	if err := fs.Parse(args); err != nil {
		return runtimeServerCLIConfig{}, err
	}
	if fs.NArg() != 0 {
		return runtimeServerCLIConfig{}, errors.New("runtime-server does not accept positional arguments")
	}
	projectedModelPreferences, err := projectRuntimeModelPreferencesJSONV1(*modelProvidersJSON)
	if err != nil {
		return runtimeServerCLIConfig{}, err
	}
	if !exactAbsoluteCLIPath(*dataDir) {
		return runtimeServerCLIConfig{}, errors.New("runtime data directory is invalid")
	}
	if *userDataDir != "" && !exactAbsoluteCLIPath(*userDataDir) {
		return runtimeServerCLIConfig{}, errors.New("runtime user data directory is invalid")
	}
	effectiveAddr := strings.TrimSpace(*addr)
	if effectiveAddr == "" {
		effectiveAddr = net.JoinHostPort(firstNonEmpty(strings.TrimSpace(*host), "127.0.0.1"), strconv.Itoa(*port))
	}
	effectiveProductionDurableRoot := strings.TrimSpace(*productionDurableRoot)
	if effectiveProductionDurableRoot == "" &&
		strings.TrimSpace(*runtimeDurableRoot) == "" &&
		strings.TrimSpace(*durableTempDir) == "" {
		effectiveProductionDurableRoot = filepath.Clean(*dataDir)
	}
	return runtimeServerCLIConfig{
		Addr:                  effectiveAddr,
		Host:                  firstNonEmpty(strings.TrimSpace(*host), "127.0.0.1"),
		DurableTempDir:        *durableTempDir,
		RuntimeDurableRoot:    *runtimeDurableRoot,
		ProductionDurableRoot: effectiveProductionDurableRoot,
		RuntimeToken:          os.Getenv("ANALYTIX_RUNTIME_TOKEN"),
		Insecure:              *insecure,
		DataDir:               *dataDir,
		UserDataDir:           *userDataDir,
		ProviderID:            *providerID,
		BaseURL:               *baseURL,
		Model:                 *model,
		EndpointFormat:        *endpointFormat,
		ModelProvidersJSON:    projectedModelPreferences,
		MCPConfigPath:         *mcpConfigPath,
		MCPConfigJSON:         *mcpConfigJSON,
		ModelProxyURL:         *modelProxyURL,
		MCPProxyURL:           firstNonEmpty(*mcpProxyURL, os.Getenv("ANALYTIX_MCP_PROXY_URL")),
		ApprovalPolicy:        *approvalPolicy,
		SandboxMode:           *sandboxMode,
		TokenEconomyMode:      *tokenEconomyMode,
		PrivateStartupFrameV1: *privateStartupFrameV1,
	}, nil
}

// Runtime model preferences carry no Provider selection, route, catalog or
// credential authority. The Registry remains the only execution authority.
type runtimeModelPreferencesProviderV1 struct {
	ID             string                                      `json:"id"`
	BaseURL        string                                      `json:"baseUrl"`
	EndpointFormat string                                      `json:"endpointFormat"`
	ModelProfiles  map[string]runtimeModelPreferencesProfileV1 `json:"modelProfiles"`
}

type runtimeModelPreferencesProfileV1 struct {
	Aliases   []string                            `json:"aliases,omitempty"`
	Reasoning *domainmodel.ModelProviderReasoning `json:"reasoning,omitempty"`
}

func projectRuntimeModelPreferencesJSONV1(raw string) (string, error) {
	if len(raw) > 1<<20 {
		return "", errors.New("runtime model provider metadata is invalid")
	}
	if strings.TrimSpace(raw) == "" {
		return "", nil
	}
	invalid := errors.New("runtime model provider metadata is invalid")
	document, err := domainjsonstrict.DecodeObject([]byte(raw), domainjsonstrict.Options{
		MaxBytes: 1 << 20, MaxDepth: 16, MaxTokens: 1 << 20, MaxStringBytes: 3 * 2048,
	})
	if err != nil {
		return "", invalid
	}
	providersValue, present := document["providers"]
	if !present {
		return "", nil // Legacy route-only configuration cannot supply metadata.
	}
	providers, valid := providersValue.([]any)
	if !valid || len(providers) > 128 {
		return "", invalid
	}
	seenIDs := make(map[string]struct{}, len(providers))
	hasProfiles := false
	for _, value := range providers {
		provider, valid := value.(map[string]any)
		if !valid {
			return "", invalid
		}
		if _, present := provider["apiKey"]; present {
			return "", errors.New("runtime model provider metadata must be key-free")
		}
		if idValue, present := provider["id"]; present {
			id, valid := idValue.(string)
			if !valid || !domainregistry.ValidProviderID(id) {
				return "", invalid
			}
			if _, duplicate := seenIDs[id]; duplicate {
				return "", invalid
			}
			seenIDs[id] = struct{}{}
		}
		if _, present := provider["modelProfiles"]; present {
			hasProfiles = true
		}
	}
	if !hasProfiles {
		return "", nil // Preserve legacy authority rejection without enabling a route.
	}
	if !runtimeModelPreferencesFieldsV1(document, "providers") {
		return "", invalid
	}
	projected := struct {
		Providers []runtimeModelPreferencesProviderV1 `json:"providers"`
	}{Providers: make([]runtimeModelPreferencesProviderV1, 0, len(providers))}
	for _, value := range providers {
		provider := value.(map[string]any)
		if !runtimeModelPreferencesFieldsV1(provider, "id", "baseUrl", "endpointFormat", "modelProfiles") {
			return "", invalid
		}
		id, idValid := provider["id"].(string)
		endpoint, endpointValid := provider["baseUrl"].(string)
		format, formatValid := provider["endpointFormat"].(string)
		if !idValid || !domainregistry.ValidProviderID(id) || !endpointValid ||
			!runtimeModelPreferencesEndpointV1(endpoint) || !formatValid {
			return "", invalid
		}
		switch format {
		case "chat_completions", "messages", "responses", "custom_endpoint":
		default:
			return "", invalid
		}
		profilesValue, present := provider["modelProfiles"]
		if !present {
			continue // A closed route-only record still cannot supply a profile.
		}
		profiles, valid := profilesValue.(map[string]any)
		if !valid || len(profiles) > 200 {
			return "", invalid
		}
		entry := runtimeModelPreferencesProviderV1{
			ID: id, BaseURL: endpoint, EndpointFormat: format,
			ModelProfiles: make(map[string]runtimeModelPreferencesProfileV1),
		}
		seenModelKeys := make(map[string]struct{}, len(profiles))
		for model, value := range profiles {
			if !runtimeModelPreferencesStringV1(model, 128) {
				return "", invalid
			}
			modelKey := strings.ToLower(model)
			if _, duplicate := seenModelKeys[modelKey]; duplicate {
				return "", invalid
			}
			seenModelKeys[modelKey] = struct{}{}
			profile, valid := value.(map[string]any)
			if !valid || !runtimeModelPreferencesFieldsV1(profile, "aliases", "reasoning") {
				return "", invalid
			}
			projectedProfile := runtimeModelPreferencesProfileV1{}
			if aliasesValue, present := profile["aliases"]; present {
				aliases, valid := aliasesValue.([]any)
				if !valid || len(aliases) > 50 {
					return "", invalid
				}
				seenAliases := make(map[string]struct{}, len(aliases))
				for _, aliasValue := range aliases {
					alias, valid := aliasValue.(string)
					if !valid || !runtimeModelPreferencesStringV1(alias, 128) {
						return "", invalid
					}
					if _, duplicate := seenAliases[alias]; duplicate {
						return "", invalid
					}
					seenAliases[alias] = struct{}{}
					projectedProfile.Aliases = append(projectedProfile.Aliases, alias)
				}
			}
			if reasoningValue, present := profile["reasoning"]; present {
				// Main projects explicit persisted null to the existing non-nil
				// none protocol. Null here would reactivate official defaults.
				reasoning, valid := reasoningValue.(map[string]any)
				if !valid || !runtimeModelPreferencesFieldsV1(reasoning, "supportedEfforts", "defaultEffort", "requestProtocol") {
					return "", invalid
				}
				efforts, valid := reasoning["supportedEfforts"].([]any)
				defaultEffort, defaultValid := reasoning["defaultEffort"].(string)
				protocol, protocolValid := reasoning["requestProtocol"].(string)
				if !valid || len(efforts) == 0 || len(efforts) > 6 || !defaultValid || !protocolValid {
					return "", invalid
				}
				switch protocol {
				case "none", "deepseek-chat-completions", "deepseek-messages", "glm-chat-completions", "mimo-chat-completions", "openai-responses", "anthropic-thinking":
				default:
					return "", invalid
				}
				projectedReasoning := &domainmodel.ModelProviderReasoning{
					DefaultEffort: defaultEffort, RequestProtocol: protocol,
				}
				seenEfforts := make(map[string]struct{}, len(efforts))
				for _, effortValue := range efforts {
					effort, valid := effortValue.(string)
					if !valid || effort == "" || domainmodel.ValidateReasoningEffortV1(effort) != nil {
						return "", invalid
					}
					if _, duplicate := seenEfforts[effort]; duplicate {
						return "", invalid
					}
					seenEfforts[effort] = struct{}{}
					projectedReasoning.SupportedEfforts = append(projectedReasoning.SupportedEfforts, effort)
				}
				if _, supported := seenEfforts[defaultEffort]; !supported {
					return "", invalid
				}
				projectedProfile.Reasoning = projectedReasoning
			}
			if len(projectedProfile.Aliases) != 0 || projectedProfile.Reasoning != nil {
				entry.ModelProfiles[model] = projectedProfile
			}
		}
		if len(entry.ModelProfiles) != 0 {
			projected.Providers = append(projected.Providers, entry)
		}
	}
	if len(projected.Providers) == 0 {
		return "", nil
	}
	encoded, err := json.Marshal(projected)
	if err != nil || len(encoded) > 1<<20 {
		return "", invalid
	}
	return string(encoded), nil
}

func runtimeModelPreferencesFieldsV1(record map[string]any, allowed ...string) bool {
	for key := range record {
		found := false
		for _, candidate := range allowed {
			found = found || key == candidate
		}
		if !found {
			return false
		}
	}
	return true
}

func runtimeModelPreferencesStringV1(value string, maximum int) bool {
	if value == "" || value != strings.TrimSpace(value) || strings.HasPrefix(value, "\ufeff") || strings.HasSuffix(value, "\ufeff") {
		return false
	}
	units := 0
	for _, current := range value {
		if current < 0x20 || current == 0x7f {
			return false
		}
		units++
		if current > 0xffff {
			units++ // Match the persisted JavaScript string-length boundary.
		}
	}
	return units <= maximum
}

func runtimeModelPreferencesEndpointV1(value string) bool {
	if !runtimeModelPreferencesStringV1(value, 2048) || strings.ContainsAny(value, "?#\\") {
		return false
	}
	for _, current := range value {
		if unicode.IsSpace(current) {
			return false
		}
	}
	parsed, err := url.Parse(value)
	return err == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") &&
		parsed.Host != "" && parsed.User == nil && parsed.RawQuery == "" &&
		!parsed.ForceQuery && parsed.Fragment == "" && parsed.RawFragment == ""
}

func cloneRuntimeMCPServerSpecV1(spec *domainmcp.ServerSpec) *domainmcp.ServerSpec {
	if spec == nil {
		return nil
	}
	cloned := *spec
	cloned.Args = append([]string(nil), spec.Args...)
	cloned.Env = cloneRuntimeStringMapV1(spec.Env)
	return &cloned
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func splitPathList(value string) []string {
	if value == "" {
		return nil
	}
	parts := []string{}
	for _, part := range filepath.SplitList(value) {
		if strings.TrimSpace(part) != "" {
			parts = append(parts, strings.TrimSpace(part))
		}
	}
	return parts
}
