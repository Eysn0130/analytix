package mcp

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	mcpprotocol "analytix.local/runtime-go/internal/adapters/outbound/mcp/protocol"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainmcp "analytix.local/runtime-go/internal/domain/mcp"
	domainplugincapability "analytix.local/runtime-go/internal/domain/plugincapability"
	domainreportpublication "analytix.local/runtime-go/internal/domain/reportpublication"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	datasetsnapshotport "analytix.local/runtime-go/internal/ports/datasetsnapshot"
	evidenceauthorityport "analytix.local/runtime-go/internal/ports/evidenceauthority"
	sourceprobeport "analytix.local/runtime-go/internal/ports/sourceprobe"
	datasetsnapshotv2fixture "analytix.local/runtime-go/internal/testsupport/datasetsnapshotv2fixture"
)

type exactCurrentDatasetAuthorityStub struct {
	mu        sync.Mutex
	selection datasetsnapshotport.CurrentSelectionV2
	binding   domainsecurity.CaseBindingObservationV1
	withCalls int
	useCalls  int
	leaked    *exactCurrentDatasetCapabilityStub
}

type exactCurrentDatasetCapabilityStub struct {
	mu        sync.RWMutex
	active    bool
	ctx       context.Context
	owner     *exactCurrentDatasetAuthorityStub
	selection datasetsnapshotport.CurrentSelectionV2
}

type blockingNativeEvidenceClientV1 struct {
	*nativeSourceProbeClient
	readStarted chan struct{}
	readRelease chan struct{}
}

func (client *blockingNativeEvidenceClientV1) CallNativeLosslessContext(
	ctx context.Context,
	method string,
	params map[string]any,
) (domainmcp.LosslessToolResult, error) {
	if method == fundsEvidenceReadMethod {
		client.readStarted <- struct{}{}
		select {
		case <-client.readRelease:
		case <-ctx.Done():
			return domainmcp.LosslessToolResult{}, ctx.Err()
		}
	}
	return client.nativeSourceProbeClient.CallNativeLosslessContext(ctx, method, params)
}

func (authority *exactCurrentDatasetAuthorityStub) WithCurrentSelectionV2(
	ctx context.Context,
	input datasetsnapshotport.ResolveInputV2,
	securityContext domainsecurity.TurnSecurityContext,
	callback func(datasetsnapshotport.CurrentSelectionV2, datasetsnapshotport.CurrentSelectionCapabilityV2) error,
) error {
	if authority == nil {
		return errors.New("exact current dataset authority stub is unavailable")
	}
	selection := authority.currentSelection()
	authority.mu.Lock()
	binding := authority.binding
	authority.mu.Unlock()
	if ctx == nil || callback == nil || ctx.Err() != nil ||
		domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(securityContext) != nil ||
		input.TenantID != securityContext.TenantID || input.UserID != securityContext.UserID ||
		input.ExpectedDatasetSnapshotID != securityContext.DatasetSnapshotID ||
		!reflect.DeepEqual(input.Observation, binding) ||
		input.Observation.ObservationDigest != securityContext.PublicationPolicy.BindingObservationDigest ||
		!datasetSelectionMatchesSourceContextV2(selection, securityContext) {
		return errors.New("exact current dataset authority stub rejected mismatched input")
	}
	capability := &exactCurrentDatasetCapabilityStub{
		active: true, ctx: ctx, owner: authority, selection: cloneCurrentDatasetSelectionV2(selection),
	}
	authority.mu.Lock()
	authority.withCalls++
	authority.leaked = capability
	authority.mu.Unlock()
	defer capability.close()
	return callback(selection, capability)
}

func (authority *exactCurrentDatasetAuthorityStub) currentSelection() datasetsnapshotport.CurrentSelectionV2 {
	if authority == nil {
		return datasetsnapshotport.CurrentSelectionV2{}
	}
	authority.mu.Lock()
	defer authority.mu.Unlock()
	return cloneCurrentDatasetSelectionV2(authority.selection)
}

func (authority *exactCurrentDatasetAuthorityStub) setSelection(
	selection datasetsnapshotport.CurrentSelectionV2,
) {
	authority.mu.Lock()
	authority.selection = cloneCurrentDatasetSelectionV2(selection)
	authority.mu.Unlock()
}

func (capability *exactCurrentDatasetCapabilityStub) UseExact(
	selection datasetsnapshotport.CurrentSelectionV2,
	securityContext domainsecurity.TurnSecurityContext,
	use func(context.Context) error,
) error {
	if capability == nil {
		return errors.New("exact current dataset capability stub is inactive")
	}
	capability.mu.RLock()
	defer capability.mu.RUnlock()
	if !capability.active || capability.ctx == nil || capability.ctx.Err() != nil || use == nil ||
		domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(securityContext) != nil ||
		!reflect.DeepEqual(selection, capability.selection) ||
		!datasetSelectionMatchesSourceContextV2(selection, securityContext) {
		return errors.New("exact current dataset capability stub rejected changed selection")
	}
	leaseContext, cancel := context.WithCancel(capability.ctx)
	defer cancel()
	capability.owner.mu.Lock()
	capability.owner.useCalls++
	capability.owner.mu.Unlock()
	if err := use(leaseContext); err != nil {
		return err
	}
	if leaseContext.Err() != nil || capability.ctx.Err() != nil ||
		!reflect.DeepEqual(selection, capability.selection) {
		return errors.New("exact current dataset capability stub changed during use")
	}
	return nil
}

func (capability *exactCurrentDatasetCapabilityStub) close() {
	if capability == nil {
		return
	}
	capability.mu.Lock()
	capability.active = false
	capability.mu.Unlock()
}

type hostEvidenceManagerFixture struct {
	manager         *ProductionManager
	authority       *exactCurrentDatasetAuthorityStub
	client          *nativeSourceProbeClient
	datasetSigning  hostEvidenceDatasetSigningMaterial
	spec            ServerSpec
	binding         domainsecurity.CaseBindingObservationV1
	securityContext domainsecurity.TurnSecurityContext
	probeResponse   map[string]any
}

type hostEvidenceDatasetSigningMaterial struct {
	installationID string
	enrollmentID   string
	keyID          string
	publicKey      ed25519.PublicKey
	privateKey     ed25519.PrivateKey
}

func (material hostEvidenceDatasetSigningMaterial) sign(message []byte) ([]byte, error) {
	return ed25519.Sign(material.privateKey, message), nil
}

func TestProductionManagerHostEvidenceAuthorityPositiveLifecycle(t *testing.T) {
	fixture := newHostEvidenceManagerFixture(t, true)
	input := fixture.probeInput(fixture.securityContext)
	if err := validateSourceProbeInputV2(input); err != nil {
		t.Fatalf("positive fixture has invalid source input: %v", err)
	}
	if !datasetSelectionMatchesSourceContextV2(fixture.authority.currentSelection(), fixture.securityContext) {
		t.Fatal("positive fixture has invalid dataset selection")
	}
	if err := fixture.authority.WithCurrentSelectionV2(
		context.Background(),
		datasetResolveInputForSourceProbeV2(fixture.securityContext, fixture.binding),
		fixture.securityContext,
		func(selection datasetsnapshotport.CurrentSelectionV2, capability datasetsnapshotport.CurrentSelectionCapabilityV2) error {
			return capability.UseExact(selection, fixture.securityContext, func(context.Context) error { return nil })
		},
	); err != nil {
		t.Fatalf("positive fixture dataset capability is invalid: %v", err)
	}
	probe, err := fixture.manager.ProbeCaseSource(context.Background(), input)
	if err != nil || !domainsecurity.SourceProbeEligibleForHostAuthorityV2(probe) ||
		fixture.client.listCalls != 1 || fixture.client.nativeCalls != 2 ||
		len(fixture.manager.sourceAdmissions) != 1 {
		t.Fatalf("exact DSV2 source admission failed: probe=%#v err=%v client=%#v admissions=%#v",
			probe, err, fixture.client, fixture.manager.sourceAdmissions)
	}
	if !reflect.DeepEqual(fixture.client.observedMethods, []string{"analytix/sourceProbe", "tools/call"}) ||
		len(fixture.client.observedParamHistory) != 2 {
		t.Fatalf("composite source probe native order mismatch: %#v", fixture.client.observedMethods)
	}
	probeProjection := assertFundsProjectionOnlyNativeParamsV2(
		t, "analytix/sourceProbe", fixture.client.observedParamHistory[0],
	)
	countProjection := assertFundsProjectionOnlyNativeParamsV2(
		t, "tools/call", fixture.client.observedParamHistory[1],
	)
	if probeProjection != countProjection {
		t.Fatalf("composite source probe projection mismatch: probe=%#v count=%#v", probeProjection, countProjection)
	}
	diagnostics := fixture.manager.ServerDiagnosticsForSecurityContext(fixture.securityContext)
	diagnosticBody, err := json.Marshal(diagnostics)
	if err != nil || len(diagnostics) != 1 {
		t.Fatalf("composite source diagnostics unavailable: diagnostics=%#v err=%v", diagnostics, err)
	}
	diagnostic := diagnostics[0].(map[string]any)
	if diagnostic["sourceProbeCount"] != float64(1) || diagnostic["sourceReady"] != true ||
		diagnostic["datasetSnapshotId"] != fixture.securityContext.DatasetSnapshotID ||
		diagnostic["connectionEpoch"] != float64(probe.ConnectionEpoch) {
		t.Fatalf("composite count admission was not reflected by value-free diagnostics: %#v", diagnostic)
	}
	for _, forbidden := range []string{"rowCount", "rawResult", "grantDigest", "sourcePath"} {
		if bytes.Contains(diagnosticBody, []byte(forbidden)) {
			t.Fatalf("composite diagnostics leaked %q: %s", forbidden, diagnosticBody)
		}
	}
	admittedSelection := fixture.authority.currentSelection()
	fixture.advanceRegistrySibling(t)
	advancedSelection := fixture.authority.currentSelection()
	if admittedSelection.Head.Bundle.RecordDigest == advancedSelection.Head.Bundle.RecordDigest ||
		admittedSelection.Head.Bundle.Generation >= advancedSelection.Head.Bundle.Generation ||
		admittedSelection.Head.Bundle.EvidenceRegistryIndexDigest == advancedSelection.Head.Bundle.EvidenceRegistryIndexDigest ||
		admittedSelection.Head.Bundle.DatasetSnapshotIndexDigest != advancedSelection.Head.Bundle.DatasetSnapshotIndexDigest ||
		admittedSelection.Head.Bundle.DatasetSnapshotCount != advancedSelection.Head.Bundle.DatasetSnapshotCount ||
		admittedSelection.SelectionDigest == advancedSelection.SelectionDigest ||
		!sameDatasetSelectionBindingV2(admittedSelection, advancedSelection) {
		t.Fatal("registry sibling advance was not isolated from the exact dataset child binding")
	}
	if tools := fixture.manager.LiveToolsForSecurityContext(fixture.securityContext); !reflect.DeepEqual(tools, []string{fundsCountEvidenceToolName}) {
		t.Fatalf("exact host admission did not expose the funds tool: %#v", tools)
	}
	if advertisements := fixture.manager.MCPToolAdvertisementSnapshotV1(fixture.securityContext); len(advertisements) != 1 ||
		advertisements[0].Name != fundsCountEvidenceToolName {
		t.Fatalf("exact host admission did not expose one grant-minting advertisement: %#v", advertisements)
	}

	var leaked sourceprobeport.HostEvidenceCapability
	err = fixture.manager.WithCurrentProbeAuthority(context.Background(), sourceprobeport.CurrentInput{
		ServerID: fixture.spec.ID, Context: fixture.securityContext, Binding: fixture.binding,
		ConnectionEpoch: probe.ConnectionEpoch,
	}, func(current domainsecurity.VerifiedSourceProbe, capability sourceprobeport.HostEvidenceCapability) error {
		leaked = capability
		if _, marshalErr := json.Marshal(capability); marshalErr == nil {
			return errors.New("host evidence capability was serializable")
		}
		selection, selectionErr := capability.DatasetSelection()
		if selectionErr != nil {
			return selectionErr
		}
		driftedContext := fixture.securityContext
		driftedContext.TurnID = "turn-host-evidence-drift"
		driftedProbe := current
		driftedProbe.ProbeContextDigest = domainsecurity.SHA256Hex([]byte("drifted-probe-context"))
		driftedSelection := selection
		driftedSelection.SelectionDigest = domainsecurity.SHA256Hex([]byte("drifted-dataset-selection"))
		for _, attempt := range []struct {
			name      string
			context   domainsecurity.TurnSecurityContext
			probe     domainsecurity.VerifiedSourceProbe
			selection datasetsnapshotport.CurrentSelectionV2
		}{
			{name: "context", context: driftedContext, probe: current, selection: selection},
			{name: "probe", context: fixture.securityContext, probe: driftedProbe, selection: selection},
			{name: "selection", context: fixture.securityContext, probe: current, selection: driftedSelection},
		} {
			executed := false
			if useErr := capability.UseExact(attempt.context, attempt.probe, attempt.selection, func(context.Context) error {
				executed = true
				return nil
			}); useErr == nil || executed {
				return errors.New("host evidence capability accepted drifted " + attempt.name)
			}
		}
		return capability.UseExact(fixture.securityContext, current, selection, func(leaseContext context.Context) error {
			if leaseContext == nil || leaseContext.Err() != nil {
				return errors.New("host evidence lease context is unavailable")
			}
			return nil
		})
	})
	if err != nil || leaked == nil {
		t.Fatalf("current host evidence callback failed: leaked=%T err=%v", leaked, err)
	}
	if _, err := leaked.DatasetSelection(); err == nil {
		t.Fatal("host evidence selection leaked after the issuing callback")
	}
	if err := leaked.UseExact(
		fixture.securityContext, probe, fixture.authority.currentSelection(),
		func(context.Context) error { t.Fatal("expired host evidence capability executed"); return nil },
	); err == nil {
		t.Fatal("host evidence capability remained active after callback return")
	}

	arguments := map[string]any{"table_name": "analysis_txn_detail_idx"}
	envelope := sourceProbeHostContext(
		t, fixture.manager, fixture.securityContext, fundsCountEvidenceToolName, probe.ConnectionEpoch, arguments,
	)
	callProjection, err := fundsCountProjectionForSelectionV2(
		fixture.securityContext,
		fixture.authority.currentSelection(),
	)
	if err != nil {
		t.Fatal(err)
	}
	callProjectionRecord, err := domainsecurity.FundsCountProjectionV2Record(callProjection)
	if err != nil {
		t.Fatal(err)
	}
	fixture.client.response = map[string]any{
		"content": []any{},
		"structuredContent": map[string]any{
			"schemaVersion": float64(2), "purpose": fundsCountToolOutcomePurposeV2,
			"semanticStatus": "success", "data": callProjectionRecord,
		},
	}
	result := fixture.manager.CallToolSecurityBoundContext(
		context.Background(), fundsCountEvidenceToolName, true, envelope, arguments,
	)
	if result["executed"] != true || fixture.client.toolCalls != 0 || fixture.client.nativeCalls != 3 {
		t.Fatalf("exact host evidence dispatch failed: result=%#v tool=%d native=%d",
			result, fixture.client.toolCalls, fixture.client.nativeCalls)
	}
	if observed := assertFundsProjectionOnlyNativeParamsV2(t, "tools/call", fixture.client.observedParams); observed != callProjection {
		t.Fatal("tools/call projection drifted from the callback-scoped DSV2/FPC selection")
	}

	_, grant, err := envelope.Authority()
	if err != nil {
		t.Fatal(err)
	}
	argumentBytes, _ := json.Marshal(arguments)
	fixture.client.response = map[string]any{
		"schemaVersion": float64(2), "purpose": "analytix.funds.count-case-rows-evidence-candidate/v2",
		"serverName": "analytix_funds", "serverVersion": hostFundsTestPackageVersionV1,
		"toolName": "count_case_rows", "projection": callProjectionRecord,
		"paginationComplete": true, "readOnly": true,
	}
	var evidenceCapability sourceprobeport.HostEvidenceCapability
	err = fixture.manager.WithCurrentEvidenceReadAuthority(context.Background(), sourceprobeport.EvidenceReadInput{
		Context: fixture.securityContext, Binding: fixture.binding, Grant: grant, Arguments: argumentBytes,
	}, func(
		current domainsecurity.VerifiedSourceProbe,
		raw domainmcp.LosslessToolResult,
		capability sourceprobeport.HostEvidenceCapability,
	) error {
		if !domainmcp.ValidLosslessToolResult(raw) {
			return errors.New("native evidence read lost its exact raw result")
		}
		evidenceCapability = capability
		selection, selectionErr := capability.DatasetSelection()
		if selectionErr != nil {
			return selectionErr
		}
		return capability.UseExact(fixture.securityContext, current, selection, func(context.Context) error { return nil })
	})
	if err != nil || evidenceCapability == nil || fixture.client.nativeCalls != 4 {
		t.Fatalf("exact native evidence read failed: capability=%T err=%v native=%d",
			evidenceCapability, err, fixture.client.nativeCalls)
	}
	if observed := assertFundsProjectionOnlyNativeParamsV2(t, fundsEvidenceReadMethod, fixture.client.observedParams); observed != callProjection {
		t.Fatal("evidenceRead projection drifted from tools/call")
	}
	if _, err := evidenceCapability.DatasetSelection(); err == nil {
		t.Fatal("evidence-read host authority leaked after callback return")
	}

	var publicationCapability sourceprobeport.HostEvidenceCapability
	err = fixture.manager.WithFreshPublicationSnapshotAuthority(context.Background(), sourceprobeport.PublicationInput{
		Context: fixture.securityContext, Binding: fixture.binding,
		Requirements: []sourceprobeport.PublicationSourceRequirement{{
			ReceiptID: "evr_host_evidence_positive", ServerID: fixture.spec.ID,
			ServerIdentity: probe.ServerIdentity, ServerVersion: hostFundsTestPackageVersionV1,
			ConnectionEpoch: probe.ConnectionEpoch, ToolName: fundsCountEvidenceToolName,
			DatasetSnapshotID: fixture.securityContext.DatasetSnapshotID,
		}},
	}, func(probes []domainsecurity.VerifiedSourceProbe, capability sourceprobeport.HostEvidenceCapability) error {
		if len(probes) != 1 {
			return errors.New("publication source re-probe count is invalid")
		}
		publicationCapability = capability
		selection, selectionErr := capability.DatasetSelection()
		if selectionErr != nil {
			return selectionErr
		}
		return capability.UseExact(fixture.securityContext, probes[0], selection, func(context.Context) error { return nil })
	})
	if err != nil || publicationCapability == nil || fixture.client.listCalls != 2 || fixture.client.nativeCalls != 6 {
		t.Fatalf("fresh publication source authority failed: capability=%T err=%v list=%d native=%d",
			publicationCapability, err, fixture.client.listCalls, fixture.client.nativeCalls)
	}
	if count := len(fixture.client.observedMethods); count < 2 ||
		!reflect.DeepEqual(fixture.client.observedMethods[count-2:], []string{"analytix/sourceProbe", "tools/call"}) {
		t.Fatalf("fresh publication composite probe order mismatch: %#v", fixture.client.observedMethods)
	}
	if _, err := publicationCapability.DatasetSelection(); err == nil {
		t.Fatal("publication host authority leaked after callback return")
	}
	fixture.authority.mu.Lock()
	datasetCapability := fixture.authority.leaked
	withCalls := fixture.authority.withCalls
	useCalls := fixture.authority.useCalls
	fixture.authority.mu.Unlock()
	if datasetCapability == nil || withCalls < 5 || useCalls < 8 {
		t.Fatalf("positive path did not traverse real callback-scoped dataset authority: with=%d use=%d", withCalls, useCalls)
	}
	if err := datasetCapability.UseExact(
		fixture.authority.currentSelection(), fixture.securityContext, func(context.Context) error {
			t.Fatal("expired dataset capability executed")
			return nil
		},
	); err == nil {
		t.Fatal("dataset capability remained active after callback return")
	}
}

func TestProductionManagerPublicationRequirementsAcceptOnlyKnownFundsEvidenceTools(t *testing.T) {
	for _, test := range []struct {
		name, tool string
		valid      bool
	}{
		{"count", fundsCountEvidenceToolName, true},
		{"account flow", CanonicalToolName("analytix_funds", hostFundsAccountFlowToolNameV1), true},
		{"unknown tool", "mcp__analytix_funds__query_anything", false},
		{"raw tool name", hostFundsAccountFlowToolNameV1, false},
		{"other server", "mcp__other__analyze_account_flows", false},
		{"wrong snapshot", CanonicalToolName("analytix_funds", hostFundsAccountFlowToolNameV1), false},
		{"wrong epoch", CanonicalToolName("analytix_funds", hostFundsAccountFlowToolNameV1), false},
		{"duplicate receipt", CanonicalToolName("analytix_funds", hostFundsAccountFlowToolNameV1), false},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newHostEvidenceManagerFixture(t, true)
			probe, err := fixture.manager.ProbeCaseSource(context.Background(), fixture.probeInput(fixture.securityContext))
			if err != nil {
				t.Fatal(err)
			}
			requirement := sourceprobeport.PublicationSourceRequirement{ReceiptID: "evr_source_requirement_fixture", ServerID: fixture.spec.ID, ServerIdentity: probe.ServerIdentity, ServerVersion: hostFundsTestPackageVersionV1, ConnectionEpoch: probe.ConnectionEpoch, ToolName: test.tool, DatasetSnapshotID: fixture.securityContext.DatasetSnapshotID}
			if test.name == "wrong snapshot" {
				requirement.DatasetSnapshotID += "-wrong"
			}
			if test.name == "wrong epoch" {
				requirement.ConnectionEpoch++
			}
			requirements := []sourceprobeport.PublicationSourceRequirement{requirement}
			if test.name == "duplicate receipt" {
				requirements = append(requirements, requirement)
			}
			before := fixture.client.nativeCalls
			called := false
			err = fixture.manager.WithFreshPublicationSnapshotAuthority(context.Background(), sourceprobeport.PublicationInput{Context: fixture.securityContext, Binding: fixture.binding, Requirements: requirements}, func(probes []domainsecurity.VerifiedSourceProbe, cap sourceprobeport.HostEvidenceCapability) error {
				called = true
				if len(probes) != 1 {
					return errors.New("fresh publication probe missing")
				}
				selection, err := cap.DatasetSelection()
				if err != nil {
					return err
				}
				return cap.UseExact(fixture.securityContext, probes[0], selection, func(context.Context) error { return nil })
			})
			if test.valid {
				if err != nil || !called || fixture.client.nativeCalls != before+2 {
					t.Fatalf("known receipt source was not freshly re-probed: %v", err)
				}
			} else if err == nil || called || fixture.client.nativeCalls != before {
				t.Fatal("invalid publication requirement acquired source authority or reached native I/O")
			}
		})
	}
}

func TestProductionManagerCompositeSourceProbeRejectsCountContractDamage(t *testing.T) {
	tests := []struct {
		name  string
		build func(domainsecurity.FundsCountProjectionV2) (domainmcp.LosslessToolResult, error)
	}{
		{
			name: "wrong row count",
			build: func(expected domainsecurity.FundsCountProjectionV2) (domainmcp.LosslessToolResult, error) {
				rowCount, err := strconv.ParseUint(expected.RowCount, 10, 64)
				if err != nil {
					return domainmcp.LosslessToolResult{}, err
				}
				observed, err := domainsecurity.NewFundsCountProjectionV2(domainsecurity.FundsCountProjectionInputV2{
					TurnSecurityContextDigest: expected.TurnSecurityContextDigest,
					DatasetSnapshotID:         expected.DatasetSnapshotID, DatasetSelectionDigest: expected.DatasetSelectionDigest,
					DatasetRecordDigest: expected.DatasetRecordDigest, DatasetManifestDigest: expected.DatasetManifestDigest,
					FundsProducerContentID:             expected.FundsProducerContentID,
					FundsProducerContentManifestSHA256: expected.FundsProducerContentManifestSHA256,
					DetailContentSHA256:                expected.DetailContentSHA256, RowCount: rowCount + 1,
				})
				if err != nil {
					return domainmcp.LosslessToolResult{}, err
				}
				return fundsCountLosslessResultForTestV2(observed, nil)
			},
		},
		{
			name: "projection mismatch",
			build: func(expected domainsecurity.FundsCountProjectionV2) (domainmcp.LosslessToolResult, error) {
				rowCount, err := strconv.ParseUint(expected.RowCount, 10, 64)
				if err != nil {
					return domainmcp.LosslessToolResult{}, err
				}
				observed, err := domainsecurity.NewFundsCountProjectionV2(domainsecurity.FundsCountProjectionInputV2{
					TurnSecurityContextDigest: expected.TurnSecurityContextDigest,
					DatasetSnapshotID:         expected.DatasetSnapshotID,
					DatasetSelectionDigest:    domainsecurity.SHA256Hex([]byte("mismatched-count-selection")),
					DatasetRecordDigest:       expected.DatasetRecordDigest, DatasetManifestDigest: expected.DatasetManifestDigest,
					FundsProducerContentID:             expected.FundsProducerContentID,
					FundsProducerContentManifestSHA256: expected.FundsProducerContentManifestSHA256,
					DetailContentSHA256:                expected.DetailContentSHA256, RowCount: rowCount,
				})
				if err != nil {
					return domainmcp.LosslessToolResult{}, err
				}
				return fundsCountLosslessResultForTestV2(observed, nil)
			},
		},
		{
			name: "extra source shaped field",
			build: func(expected domainsecurity.FundsCountProjectionV2) (domainmcp.LosslessToolResult, error) {
				return fundsCountLosslessResultForTestV2(expected, func(envelope map[string]any) {
					envelope["sourcePath"] = "/private/source.csv"
				})
			},
		},
		{
			name: "invalid lossless digest",
			build: func(expected domainsecurity.FundsCountProjectionV2) (domainmcp.LosslessToolResult, error) {
				result, err := fundsCountLosslessResultForTestV2(expected, nil)
				result.RawSHA256 = domainsecurity.SHA256Hex([]byte("forged-lossless-digest"))
				return result, err
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newHostEvidenceManagerFixture(t, true)
			fixture.client.losslessResponse = func(method string, params map[string]any) (domainmcp.LosslessToolResult, error) {
				if method != "tools/call" {
					return domainmcp.LosslessToolResult{}, errors.New("unexpected count canary method")
				}
				meta, _ := params["_meta"].(map[string]any)
				expected, err := domainsecurity.ParseFundsCountProjectionV2(
					meta[domainsecurity.FundsCountProjectionMetaKeyV2],
				)
				if err != nil {
					return domainmcp.LosslessToolResult{}, err
				}
				return test.build(expected)
			}
			if probe, err := fixture.manager.ProbeCaseSource(
				context.Background(), fixture.probeInput(fixture.securityContext),
			); err == nil || probe.ProbeDigest != "" {
				t.Fatalf("damaged count result created a probe: probe=%#v err=%v", probe, err)
			}
			assertCompositeSourceAdmissionAbsentV2(t, fixture, 2)
		})
	}
}

func TestProductionManagerCompositeSourceProbeRequiresCurrentLifecycleGrant(t *testing.T) {
	fixture := newHostEvidenceManagerFixture(t, true)
	fixture.manager.mu.Lock()
	fixture.manager.revokeHostFundsSourceReadNoLock(
		domainplugincapability.FundsSourceReadStateRevokedV1,
		domainplugincapability.FundsSourceReadReasonAuthorityRevokedV1,
	)
	fixture.manager.mu.Unlock()
	if probe, err := fixture.manager.ProbeCaseSource(
		context.Background(), fixture.probeInput(fixture.securityContext),
	); err == nil || probe.ProbeDigest != "" {
		t.Fatalf("missing lifecycle grant created a probe: probe=%#v err=%v", probe, err)
	}
	assertCompositeSourceAdmissionAbsentV2(t, fixture, 1)
}

func TestProductionManagerCompositeSourceProbeRequiresCurrentNativeOwner(t *testing.T) {
	tests := []struct {
		name                string
		current             func(context.Context) error
		expectedChecks      int
		expectedNativeCalls int
	}{
		{
			name:                "missing callback",
			expectedNativeCalls: 1,
		},
		{
			name: "pre-count failure",
			current: func(context.Context) error {
				return errors.New("test native owner is stale")
			},
			expectedChecks:      1,
			expectedNativeCalls: 1,
		},
		{
			name: "post-count failure",
			current: func() func(context.Context) error {
				calls := 0
				return func(context.Context) error {
					calls++
					if calls == 2 {
						return errors.New("test native owner changed during count")
					}
					return nil
				}
			}(),
			expectedChecks:      2,
			expectedNativeCalls: 2,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			checks := 0
			current := test.current
			if current != nil {
				current = func(delegate func(context.Context) error) func(context.Context) error {
					return func(ctx context.Context) error {
						checks++
						return delegate(ctx)
					}
				}(current)
			}
			fixture := newHostEvidenceManagerFixtureWithNativeCurrentness(t, true, current)
			before := sourceProbeClientCallCountsForTest(fixture.client)
			if probe, err := fixture.manager.ProbeCaseSource(
				context.Background(), fixture.probeInput(fixture.securityContext),
			); err == nil || probe.ProbeDigest != "" {
				t.Fatalf("non-current native owner created a probe: probe=%#v err=%v", probe, err)
			}
			after := sourceProbeClientCallCountsForTest(fixture.client)
			expectedMethods := []string{"analytix/sourceProbe"}
			if test.expectedNativeCalls == 2 {
				expectedMethods = append(expectedMethods, "tools/call")
			}
			observedMethods := fixture.client.observedMethods[before.native:after.native]
			if checks != test.expectedChecks || after.native-before.native != test.expectedNativeCalls ||
				after.tool-before.tool != 0 || !reflect.DeepEqual(observedMethods, expectedMethods) {
				t.Fatalf("native-owner currentness boundary mismatch: checks=%d before=%#v after=%#v", checks, before, after)
			}
			assertCompositeSourceAdmissionAbsentV2(t, fixture, test.expectedNativeCalls)
			fixture.manager.mu.Lock()
			_, grantCurrent := fixture.manager.hostFundsSourceReadLifecycle.CurrentGrant()
			fixture.manager.mu.Unlock()
			if grantCurrent {
				t.Fatal("native-owner currentness failure retained lifecycle authority")
			}
		})
	}

	t.Run("healthy pre and post currentness", func(t *testing.T) {
		checks := 0
		fixture := newHostEvidenceManagerFixtureWithNativeCurrentness(t, true, func(context.Context) error {
			checks++
			return nil
		})
		before := sourceProbeClientCallCountsForTest(fixture.client)
		probe, err := fixture.manager.ProbeCaseSource(
			context.Background(), fixture.probeInput(fixture.securityContext),
		)
		if err != nil || probe.ProbeDigest == "" {
			t.Fatalf("current native owner did not admit composite probe: probe=%#v err=%v", probe, err)
		}
		after := sourceProbeClientCallCountsForTest(fixture.client)
		observedMethods := fixture.client.observedMethods[before.native:after.native]
		if checks != 2 || after.native-before.native != 2 || after.tool-before.tool != 0 ||
			!reflect.DeepEqual(observedMethods, []string{"analytix/sourceProbe", "tools/call"}) ||
			len(fixture.manager.sourceAdmissions) != 1 {
			t.Fatalf("healthy native-owner currentness boundary mismatch: checks=%d before=%#v after=%#v admissions=%d",
				checks, before, after, len(fixture.manager.sourceAdmissions))
		}
	})
}

func TestProductionManagerSettledNativeFailureInvalidatesCompositeSourceAdmission(t *testing.T) {
	fixture := newHostEvidenceManagerFixture(t, true)
	if _, err := fixture.manager.ProbeCaseSource(
		context.Background(), fixture.probeInput(fixture.securityContext),
	); err != nil {
		t.Fatal(err)
	}
	const ordinaryToolName = "mcp__docs__lookup"
	ordinaryTool := managedTool{ServerID: "docs", Name: ordinaryToolName, RawName: "lookup"}
	fixture.manager.mu.Lock()
	fixture.manager.tools[ordinaryToolName] = ordinaryTool
	beforeCatalogHash := fixture.manager.catalogHash
	beforeToolCount := len(fixture.manager.tools)
	fixture.manager.mu.Unlock()
	beforeNativeCalls := sourceProbeClientCallCountsForTest(fixture.client)

	fixture.manager.InvalidateHostFundsSourceAfterSettledNativeFailure()

	if afterNativeCalls := sourceProbeClientCallCountsForTest(fixture.client); afterNativeCalls != beforeNativeCalls {
		t.Fatalf("settled native invalidation re-executed MCP: before=%#v after=%#v", beforeNativeCalls, afterNativeCalls)
	}
	fixture.manager.mu.Lock()
	_, grantCurrent := fixture.manager.hostFundsSourceReadLifecycle.CurrentGrant()
	retainedOrdinary, ordinaryCurrent := fixture.manager.tools[ordinaryToolName]
	afterCatalogHash := fixture.manager.catalogHash
	afterToolCount := len(fixture.manager.tools)
	admissionCount := len(fixture.manager.sourceAdmissions)
	probeCount := len(fixture.manager.sourceProbes)
	fixture.manager.mu.Unlock()
	if grantCurrent || admissionCount != 0 || probeCount != 0 {
		t.Fatalf("settled native failure retained Funds authority: grant=%v admissions=%d probes=%d",
			grantCurrent, admissionCount, probeCount)
	}
	if !ordinaryCurrent || !reflect.DeepEqual(retainedOrdinary, ordinaryTool) ||
		afterCatalogHash != beforeCatalogHash || afterToolCount != beforeToolCount {
		t.Fatal("settled native failure changed ordinary/non-Funds catalog state")
	}
	if tools := fixture.manager.Tools(); !reflect.DeepEqual(tools, []string{ordinaryToolName}) {
		t.Fatalf("settled native failure did not isolate Funds tool authority: %#v", tools)
	}
	diagnostics := fixture.manager.ServerDiagnosticsForSecurityContext(fixture.securityContext)
	if len(diagnostics) != 1 {
		t.Fatalf("settled native invalidation diagnostics unavailable: %#v", diagnostics)
	}
	diagnostic := diagnostics[0].(map[string]any)
	if diagnostic["sourceProbeCount"] != float64(0) || diagnostic["sourceReady"] != false ||
		diagnostic["sourceProbeDigest"] != "" {
		t.Fatalf("settled native failure remained source-ready: %#v", diagnostic)
	}
}

func TestProductionManagerCompositeSourceProbeRechecksAuthorityAfterCount(t *testing.T) {
	tests := []struct {
		name  string
		drift func(*hostEvidenceManagerFixture)
	}{
		{
			name: "grant generation revoked",
			drift: func(fixture *hostEvidenceManagerFixture) {
				fixture.manager.revokeHostFundsSourceReadNoLock(
					domainplugincapability.FundsSourceReadStateRevokedV1,
					domainplugincapability.FundsSourceReadReasonAuthorityRevokedV1,
				)
			},
		},
		{
			name: "connection epoch drift",
			drift: func(fixture *hostEvidenceManagerFixture) {
				fixture.manager.connectionEpochs[fixture.spec.ID]++
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newHostEvidenceManagerFixture(t, true)
			fixture.client.losslessStarted = make(chan struct{})
			fixture.client.losslessRelease = make(chan struct{})
			probeDone := make(chan struct {
				probe domainsecurity.VerifiedSourceProbe
				err   error
			}, 1)
			go func() {
				probe, err := fixture.manager.ProbeCaseSource(
					context.Background(), fixture.probeInput(fixture.securityContext),
				)
				probeDone <- struct {
					probe domainsecurity.VerifiedSourceProbe
					err   error
				}{probe: probe, err: err}
			}()
			select {
			case <-fixture.client.losslessStarted:
			case <-time.After(5 * time.Second):
				t.Fatal("composite count call did not reach blocking seam")
			}
			fixture.manager.mu.Lock()
			test.drift(fixture)
			fixture.manager.mu.Unlock()
			close(fixture.client.losslessRelease)
			select {
			case result := <-probeDone:
				if result.err == nil || result.probe.ProbeDigest != "" {
					t.Fatalf("authority drift created a probe: probe=%#v err=%v", result.probe, result.err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("composite count call did not finish after release")
			}
			assertCompositeSourceAdmissionAbsentV2(t, fixture, 2)
		})
	}
}

func TestProductionManagerCountExecutionRequiresCurrentTypedSourceReadGrant(t *testing.T) {
	fixture := newHostEvidenceManagerFixture(t, true)
	probe, err := fixture.manager.ProbeCaseSource(
		context.Background(), fixture.probeInput(fixture.securityContext),
	)
	if err != nil {
		t.Fatal(err)
	}
	arguments := map[string]any{"table_name": domainsecurity.FundsCountProjectionTableV2}
	envelope := sourceProbeHostContext(
		t,
		fixture.manager,
		fixture.securityContext,
		fundsCountEvidenceToolName,
		probe.ConnectionEpoch,
		arguments,
	)
	before := sourceProbeClientCallCountsForTest(fixture.client)
	fixture.manager.mu.Lock()
	fixture.manager.revokeHostFundsSourceReadNoLock(
		domainplugincapability.FundsSourceReadStateRevokedV1,
		domainplugincapability.FundsSourceReadReasonAuthorityRevokedV1,
	)
	fixture.manager.mu.Unlock()
	result := fixture.manager.CallToolSecurityBoundContext(
		context.Background(), fundsCountEvidenceToolName, true, envelope, arguments,
	)
	if result["executed"] != false || result["code"] != "mcp_host_context_invalid" {
		t.Fatalf("revoked typed source-read grant reached production count execution: %#v", result)
	}
	if after := sourceProbeClientCallCountsForTest(fixture.client); after != before {
		t.Fatalf("revoked typed source-read grant reached MCP I/O: before=%#v after=%#v", before, after)
	}
	if tools := fixture.manager.LiveToolsForSecurityContext(fixture.securityContext); len(tools) != 0 {
		t.Fatalf("revoked typed source-read grant remained advertised: %#v", tools)
	}
}

func TestProductionManagerEvidenceReadRechecksTypedSourceReadGrantAfterNativeRead(t *testing.T) {
	fixture := newHostEvidenceManagerFixture(t, true)
	probe, err := fixture.manager.ProbeCaseSource(
		context.Background(), fixture.probeInput(fixture.securityContext),
	)
	if err != nil {
		t.Fatal(err)
	}
	arguments := map[string]any{"table_name": domainsecurity.FundsCountProjectionTableV2}
	envelope := sourceProbeHostContext(
		t,
		fixture.manager,
		fixture.securityContext,
		fundsCountEvidenceToolName,
		probe.ConnectionEpoch,
		arguments,
	)
	_, grant, err := envelope.Authority()
	if err != nil {
		t.Fatal(err)
	}
	argumentBytes, err := json.Marshal(arguments)
	if err != nil {
		t.Fatal(err)
	}
	projection, err := fundsCountProjectionForSelectionV2(
		fixture.securityContext,
		fixture.authority.currentSelection(),
	)
	if err != nil {
		t.Fatal(err)
	}
	projectionRecord, err := domainsecurity.FundsCountProjectionV2Record(projection)
	if err != nil {
		t.Fatal(err)
	}
	fixture.client.response = map[string]any{
		"schemaVersion": float64(2), "purpose": "analytix.funds.count-case-rows-evidence-candidate/v2",
		"serverName": "analytix_funds", "serverVersion": hostFundsTestPackageVersionV1,
		"toolName": "count_case_rows", "projection": projectionRecord,
		"paginationComplete": true, "readOnly": true,
	}
	blocking := &blockingNativeEvidenceClientV1{
		nativeSourceProbeClient: fixture.client,
		readStarted:             make(chan struct{}),
		readRelease:             make(chan struct{}),
	}
	var releaseOnce sync.Once
	releaseRead := func() {
		releaseOnce.Do(func() { close(blocking.readRelease) })
	}
	defer releaseRead()
	fixture.manager.mu.Lock()
	fixture.manager.clients[fixture.spec.ID] = blocking
	fixture.manager.mu.Unlock()

	callbackCalled := make(chan struct{}, 1)
	readDone := make(chan error, 1)
	go func() {
		readDone <- fixture.manager.WithCurrentEvidenceReadAuthority(
			context.Background(),
			sourceprobeport.EvidenceReadInput{
				Context: fixture.securityContext, Binding: fixture.binding,
				Grant: grant, Arguments: argumentBytes,
			},
			func(
				domainsecurity.VerifiedSourceProbe,
				domainmcp.LosslessToolResult,
				sourceprobeport.HostEvidenceCapability,
			) error {
				callbackCalled <- struct{}{}
				return nil
			},
		)
	}()
	select {
	case <-blocking.readStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("native evidence read did not reach the blocking production seam")
	}

	entrypointBody, err := os.ReadFile(fixture.spec.EntrypointPath)
	if err != nil {
		t.Fatal(err)
	}
	entrypointInfo, err := os.Stat(fixture.spec.EntrypointPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.WriteFile(fixture.spec.EntrypointPath, entrypointBody, entrypointInfo.Mode().Perm())
	})
	if err := os.WriteFile(fixture.spec.EntrypointPath, []byte("host provenance drift\n"), entrypointInfo.Mode().Perm()); err != nil {
		t.Fatal(err)
	}
	if tools := fixture.manager.LiveTools(); len(tools) != 0 {
		t.Fatalf("current provenance drift retained Funds source-read advertisement: %#v", tools)
	}
	if err := os.WriteFile(fixture.spec.EntrypointPath, entrypointBody, entrypointInfo.Mode().Perm()); err != nil {
		t.Fatal(err)
	}
	releaseRead()

	select {
	case err = <-readDone:
	case <-time.After(5 * time.Second):
		t.Fatal("native evidence read did not finish after release")
	}
	select {
	case <-callbackCalled:
		t.Fatalf("revoked typed source-read authority reached the post-native callback: %v", err)
	default:
	}
	if err == nil {
		t.Fatal("revoked typed source-read authority was not rejected after native read")
	}
}

func assertFundsProjectionOnlyNativeParamsV2(
	t *testing.T,
	method string,
	params map[string]any,
) domainsecurity.FundsCountProjectionV2 {
	t.Helper()
	meta, ok := params["_meta"].(map[string]any)
	if !ok || len(meta) != 1 {
		t.Fatalf("%s did not carry one independent funds projection: %#v", method, params)
	}
	projection, err := domainsecurity.ParseFundsCountProjectionV2(
		meta[domainsecurity.FundsCountProjectionMetaKeyV2],
	)
	if err != nil {
		t.Fatalf("%s carried an invalid funds projection: %v", method, err)
	}
	body, err := json.Marshal(params)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{
		"analytixRuntimeContext", "workspaceRealPath", "workspace", "caseId",
		"tenantId", "userId", "account", "card", "database", "duckdb", "pluginRoot",
	} {
		if bytes.Contains(bytes.ToLower(body), []byte(strings.ToLower(forbidden))) {
			t.Fatalf("%s leaked forbidden material %q: %s", method, forbidden, body)
		}
	}
	return projection
}

func fundsCountLosslessResultForTestV2(
	projection domainsecurity.FundsCountProjectionV2,
	mutate func(map[string]any),
) (domainmcp.LosslessToolResult, error) {
	record, err := domainsecurity.FundsCountProjectionV2Record(projection)
	if err != nil {
		return domainmcp.LosslessToolResult{}, err
	}
	envelope := map[string]any{
		"content": []any{},
		"structuredContent": map[string]any{
			"schemaVersion": float64(2), "purpose": fundsCountToolOutcomePurposeV2,
			"semanticStatus": "success", "data": record,
		},
	}
	if mutate != nil {
		mutate(envelope)
	}
	body, err := json.Marshal(envelope)
	if err != nil {
		return domainmcp.LosslessToolResult{}, err
	}
	return mcpprotocol.DecodeLosslessJSONResult(body), nil
}

func assertCompositeSourceAdmissionAbsentV2(
	t *testing.T,
	fixture *hostEvidenceManagerFixture,
	expectedNativeCalls int,
) {
	t.Helper()
	if fixture.client.nativeCalls != expectedNativeCalls ||
		len(fixture.manager.sourceAdmissions) != 0 || len(fixture.manager.sourceProbes) != 0 {
		t.Fatalf("failed composite probe retained authority: client=%#v admissions=%#v probes=%#v",
			fixture.client, fixture.manager.sourceAdmissions, fixture.manager.sourceProbes)
	}
	if expectedNativeCalls == 1 {
		if !reflect.DeepEqual(fixture.client.observedMethods, []string{"analytix/sourceProbe"}) {
			t.Fatalf("grant rejection crossed into count call: %#v", fixture.client.observedMethods)
		}
	} else if expectedNativeCalls == 2 &&
		!reflect.DeepEqual(fixture.client.observedMethods, []string{"analytix/sourceProbe", "tools/call"}) {
		t.Fatalf("composite native order mismatch: %#v", fixture.client.observedMethods)
	}
	diagnostics := fixture.manager.ServerDiagnosticsForSecurityContext(fixture.securityContext)
	if len(diagnostics) != 1 {
		t.Fatalf("failed composite diagnostics unavailable: %#v", diagnostics)
	}
	diagnostic := diagnostics[0].(map[string]any)
	if diagnostic["sourceProbeCount"] != float64(0) || diagnostic["sourceReady"] != false ||
		diagnostic["sourceProbeDigest"] != "" {
		t.Fatalf("failed composite probe remained diagnostically ready: %#v", diagnostic)
	}
}

func TestProductionManagerHostEvidenceAdmissionClearsOnContextSwitch(t *testing.T) {
	fixture := newHostEvidenceManagerFixture(t, true)
	contextA := fixture.securityContext
	if _, err := fixture.manager.ProbeCaseSource(context.Background(), fixture.probeInput(contextA)); err != nil {
		t.Fatal(err)
	}
	contextB := newHostEvidenceTurnSecurityContext(
		t, fixture.binding, fixture.authority.currentSelection().Snapshot, "turn-host-evidence-b",
	)
	if _, err := fixture.manager.ProbeCaseSource(context.Background(), fixture.probeInput(contextB)); err != nil {
		t.Fatal(err)
	}
	if tools := fixture.manager.LiveToolsForSecurityContext(contextA); len(tools) != 0 {
		t.Fatalf("context A retained factual tools after context B admission: %#v", tools)
	}
	if tools := fixture.manager.LiveToolsForSecurityContext(contextB); !reflect.DeepEqual(tools, []string{fundsCountEvidenceToolName}) {
		t.Fatalf("context B did not receive the sole current admission: %#v", tools)
	}
	if len(fixture.manager.sourceAdmissions) != 1 || len(fixture.manager.sourceProbes) != 1 {
		t.Fatalf("context switch retained parallel source authority: admissions=%#v probes=%#v",
			fixture.manager.sourceAdmissions, fixture.manager.sourceProbes)
	}
	called := false
	err := fixture.manager.WithCurrentProbeAuthority(context.Background(), sourceprobeport.CurrentInput{
		ServerID: fixture.spec.ID, Context: contextA, Binding: fixture.binding, ConnectionEpoch: 31,
	}, func(domainsecurity.VerifiedSourceProbe, sourceprobeport.HostEvidenceCapability) error {
		called = true
		return nil
	})
	if err == nil || called {
		t.Fatalf("changed context reached stale host authority callback: called=%v err=%v", called, err)
	}
	fixture.manager.Disconnect()
	if len(fixture.manager.sourceAdmissions) != 0 || len(fixture.manager.sourceProbes) != 0 ||
		len(fixture.manager.LiveToolsForSecurityContext(contextB)) != 0 {
		t.Fatal("disconnect retained manager-owned host evidence admission")
	}
}

func TestProductionManagerDatasetChildDriftFailsBeforeHostCallback(t *testing.T) {
	fixture := newHostEvidenceManagerFixture(t, true)
	probe, err := fixture.manager.ProbeCaseSource(
		context.Background(), fixture.probeInput(fixture.securityContext),
	)
	if err != nil {
		t.Fatal(err)
	}
	fixture.advanceDatasetChild(t)
	drifted := fixture.authority.currentSelection()
	if !datasetSelectionMatchesSourceContextV2(drifted, fixture.securityContext) ||
		sameDatasetSelectionBindingV2(fixture.manager.sourceAdmissions[sourceProbeRegistryKey(
			fixture.spec.ID, fixture.securityContext.ThreadID, fixture.securityContext.TurnID,
		)].Selection, drifted) {
		t.Fatal("dataset child drift fixture did not produce a distinct valid exact selection")
	}
	before := sourceProbeClientCallCountsForTest(fixture.client)
	called := false
	err = fixture.manager.WithCurrentProbeAuthority(context.Background(), sourceprobeport.CurrentInput{
		ServerID: fixture.spec.ID, Context: fixture.securityContext, Binding: fixture.binding,
		ConnectionEpoch: probe.ConnectionEpoch,
	}, func(domainsecurity.VerifiedSourceProbe, sourceprobeport.HostEvidenceCapability) error {
		called = true
		return nil
	})
	if err == nil || called {
		t.Fatalf("dataset child drift reached host callback: called=%v err=%v", called, err)
	}
	if after := sourceProbeClientCallCountsForTest(fixture.client); after != before {
		t.Fatalf("dataset child drift reached MCP I/O: before=%#v after=%#v", before, after)
	}
	if len(fixture.manager.sourceAdmissions) != 0 || len(fixture.manager.sourceProbes) != 0 ||
		len(fixture.manager.LiveToolsForSecurityContext(fixture.securityContext)) != 0 {
		t.Fatal("dataset child drift retained manager-owned host evidence authority")
	}
}

func TestProductionManagerEvidenceReadRequiresOpaqueHostFundsFingerprintBeforeNativeIO(t *testing.T) {
	fixture := newHostEvidenceManagerFixture(t, true)
	probe, err := fixture.manager.ProbeCaseSource(
		context.Background(), fixture.probeInput(fixture.securityContext),
	)
	if err != nil {
		t.Fatal(err)
	}
	arguments := map[string]any{"table_name": domainsecurity.FundsCountProjectionTableV2}
	envelope := sourceProbeHostContext(
		t,
		fixture.manager,
		fixture.securityContext,
		fundsCountEvidenceToolName,
		probe.ConnectionEpoch,
		arguments,
	)
	_, grant, err := envelope.Authority()
	if err != nil {
		t.Fatal(err)
	}
	argumentBytes, err := json.Marshal(arguments)
	if err != nil {
		t.Fatal(err)
	}
	before := sourceProbeClientCallCountsForTest(fixture.client)
	fixture.manager.mu.Lock()
	fixture.manager.hostFundsFingerprint = ""
	fixture.manager.mu.Unlock()
	called := false
	err = fixture.manager.WithCurrentEvidenceReadAuthority(
		context.Background(),
		sourceprobeport.EvidenceReadInput{
			Context:   fixture.securityContext,
			Binding:   fixture.binding,
			Grant:     grant,
			Arguments: argumentBytes,
		},
		func(
			domainsecurity.VerifiedSourceProbe,
			domainmcp.LosslessToolResult,
			sourceprobeport.HostEvidenceCapability,
		) error {
			called = true
			return nil
		},
	)
	if err == nil || called {
		t.Fatalf("non-opaque funds spec reached evidence callback: called=%v err=%v", called, err)
	}
	if after := sourceProbeClientCallCountsForTest(fixture.client); after != before {
		t.Fatalf("non-opaque funds spec reached native I/O: before=%#v after=%#v", before, after)
	}
}

func TestProductionManagerMissingAndCompatibilityAuthoritiesAreZeroIO(t *testing.T) {
	fixture := newHostEvidenceManagerFixture(t, false)
	before := sourceProbeClientCallCountsForTest(fixture.client)
	if _, err := fixture.manager.ProbeCaseSource(
		context.Background(), fixture.probeInput(fixture.securityContext),
	); err == nil || err.Error() != domainsecurity.SourceProbeBlockerDatasetSnapshotAuthorityUnavailable {
		t.Fatalf("missing DSV2 authority did not return the fixed blocker: %v", err)
	}
	currentCalled := false
	if err := fixture.manager.WithCurrentProbe(context.Background(), sourceprobeport.CurrentInput{
		ServerID: fixture.spec.ID, Context: fixture.securityContext, Binding: fixture.binding, ConnectionEpoch: 31,
	}, func(domainsecurity.VerifiedSourceProbe) error {
		currentCalled = true
		return nil
	}); err == nil {
		t.Fatal("compatibility current-probe surface did not fail closed")
	}
	evidenceCalled := false
	if err := fixture.manager.WithCurrentEvidenceRead(context.Background(), sourceprobeport.EvidenceReadInput{
		Context: fixture.securityContext, Binding: fixture.binding,
	}, func(domainsecurity.VerifiedSourceProbe, domainmcp.LosslessToolResult) error {
		evidenceCalled = true
		return nil
	}); err == nil {
		t.Fatal("compatibility evidence-read surface did not fail closed")
	}
	publicationCalled := false
	if err := fixture.manager.WithFreshPublicationSnapshot(context.Background(), sourceprobeport.PublicationInput{
		Context: fixture.securityContext, Binding: fixture.binding,
	}, func([]domainsecurity.VerifiedSourceProbe) error {
		publicationCalled = true
		return nil
	}); err == nil {
		t.Fatal("compatibility publication surface did not fail closed")
	}
	if currentCalled || evidenceCalled || publicationCalled {
		t.Fatalf("compatibility callback executed: current=%v evidence=%v publication=%v",
			currentCalled, evidenceCalled, publicationCalled)
	}
	if after := sourceProbeClientCallCountsForTest(fixture.client); after != before {
		t.Fatalf("missing/compatibility authority reached MCP I/O: before=%#v after=%#v", before, after)
	}
	if len(fixture.manager.sourceAdmissions) != 0 || len(fixture.manager.sourceProbes) != 0 {
		t.Fatalf("missing/compatibility authority registered private admission: admissions=%#v probes=%#v",
			fixture.manager.sourceAdmissions, fixture.manager.sourceProbes)
	}
}

func newHostEvidenceManagerFixture(t *testing.T, withAuthority bool) *hostEvidenceManagerFixture {
	return newHostEvidenceManagerFixtureWithNativeCurrentness(t, withAuthority, func(context.Context) error {
		return nil
	})
}

func newHostEvidenceManagerFixtureWithNativeCurrentness(
	t *testing.T,
	withAuthority bool,
	nativeOwnerCurrent func(context.Context) error,
) *hostEvidenceManagerFixture {
	t.Helper()
	binding, err := domainsecurity.NewCaseBindingObservationV1(domainsecurity.CaseBindingObservationInputV1{
		WorkspaceRealPath: "/workspace/host-evidence", State: domainsecurity.CaseBindingStateValid,
		CaseID: "case-host-evidence", BindingSHA256: domainsecurity.SHA256Hex([]byte("host-evidence-binding-file")),
		CaseBindingHash: domainsecurity.SHA256Hex([]byte("host-evidence-binding")),
	})
	if err != nil {
		t.Fatal(err)
	}
	selection, datasetSigning := newHostEvidenceDatasetSelection(t, binding)
	securityContext := newHostEvidenceTurnSecurityContext(
		t, binding, selection.Snapshot, "turn-host-evidence-a",
	)
	authority := &exactCurrentDatasetAuthorityStub{selection: selection, binding: binding}
	spec := hostFundsServerSpecFixtureV1(t)
	hostFundsServer, err := NewHostFundsServerSpecV1(spec, hostFundsStaticAdmissionInputFixtureV1(t, spec.ExpectedServerVersion))
	if err != nil {
		t.Fatal(err)
	}
	options := ProductionManagerOptions{
		HostFundsServer:             hostFundsServer,
		HostFundsNativeOwnerCurrent: nativeOwnerCurrent,
	}
	if withAuthority {
		options.DatasetAuthority = authority
	}
	manager := NewProductionManagerWithOptions(nil, options)
	client := &nativeSourceProbeClient{tools: hostFundsRuntimeToolCatalogV1(true, false)}
	if withAuthority {
		manager.Connect()
		t.Cleanup(manager.Disconnect)
		manager.mu.Lock()
		connectionEpoch := manager.connectionEpochs[spec.ID]
		clientReady := connectionEpoch != 0 && manager.clients[spec.ID] != nil &&
			manager.hostFundsSourceReadGrantedNoLock()
		if clientReady {
			manager.clients[spec.ID].Close()
			manager.clients[spec.ID] = client
		}
		manager.mu.Unlock()
		if !clientReady {
			t.Fatal("production Host Funds setup did not mint source-read authority")
		}
	} else {
		const connectionEpoch = uint64(31)
		manager.specs = []ServerSpec{spec}
		manager.clients[spec.ID] = client
		manager.connectionEpochs[spec.ID] = connectionEpoch
		manager.serverIdentities[spec.ID] = mcpTestVerifiedIdentity(
			t, spec.ID, spec.ExpectedServerName, spec.ExpectedServerVersion, connectionEpoch,
		)
		manager.specFingerprints[spec.ID] = SpecFingerprint(spec)
	}
	probeResponse := map[string]any{}
	client.nativeResponse = func(method string, params map[string]any) (any, error) {
		if method != "analytix/sourceProbe" {
			return client.response, nil
		}
		meta, _ := params["_meta"].(map[string]any)
		projection, err := domainsecurity.ParseFundsCountProjectionV2(
			meta[domainsecurity.FundsCountProjectionMetaKeyV2],
		)
		if err != nil {
			return nil, err
		}
		return map[string]any{
			"schemaVersion": float64(2), "purpose": fundsSourceProbePurposeV2,
			"serverName": spec.ExpectedServerName, "serverVersion": spec.ExpectedServerVersion,
			"projectionDigest": projection.ProjectionDigest, "ready": true, "readOnly": true,
		}, nil
	}
	client.losslessResponse = func(method string, params map[string]any) (domainmcp.LosslessToolResult, error) {
		if method != "tools/call" {
			body, err := json.Marshal(client.response)
			if err != nil {
				return domainmcp.LosslessToolResult{}, err
			}
			return mcpprotocol.DecodeLosslessJSONResult(body), nil
		}
		meta, _ := params["_meta"].(map[string]any)
		projection, err := domainsecurity.ParseFundsCountProjectionV2(
			meta[domainsecurity.FundsCountProjectionMetaKeyV2],
		)
		if err != nil {
			return domainmcp.LosslessToolResult{}, err
		}
		record, err := domainsecurity.FundsCountProjectionV2Record(projection)
		if err != nil {
			return domainmcp.LosslessToolResult{}, err
		}
		body, err := json.Marshal(map[string]any{
			"content": []any{},
			"structuredContent": map[string]any{
				"schemaVersion": float64(2), "purpose": fundsCountToolOutcomePurposeV2,
				"semanticStatus": "success", "data": record,
			},
		})
		if err != nil {
			return domainmcp.LosslessToolResult{}, err
		}
		return mcpprotocol.DecodeLosslessJSONResult(body), nil
	}
	return &hostEvidenceManagerFixture{
		manager: manager, authority: authority, client: client, spec: spec,
		datasetSigning: datasetSigning, binding: binding,
		securityContext: securityContext, probeResponse: probeResponse,
	}
}

func (fixture *hostEvidenceManagerFixture) probeInput(
	securityContext domainsecurity.TurnSecurityContext,
) sourceprobeport.Input {
	return sourceprobeport.Input{
		ServerID: fixture.spec.ID, Context: securityContext, Binding: fixture.binding,
		WorkspaceRealPath: securityContext.WorkspaceRealPath, ThreadID: securityContext.ThreadID,
		TurnID: securityContext.TurnID, CaseID: securityContext.CaseID,
		CaseBindingHash: securityContext.CaseBindingHash, DatasetSnapshotID: securityContext.DatasetSnapshotID,
		ContextEpoch: securityContext.ContextEpoch, ContextDigest: securityContext.ContextDigest,
	}
}

func (fixture *hostEvidenceManagerFixture) advanceRegistrySibling(t *testing.T) {
	t.Helper()
	selection := fixture.authority.currentSelection()
	previous := selection.Head.Bundle
	next, err := domainevidence.NewEvidenceAuthorityBundleV1(domainevidence.EvidenceAuthorityBundleInputV1{
		InstallationID:              fixture.datasetSigning.installationID,
		EnrollmentID:                fixture.datasetSigning.enrollmentID,
		Generation:                  previous.Generation + 1,
		PreviousBundleDigest:        previous.RecordDigest,
		MutationID:                  domainsecurity.SHA256Hex([]byte("host-evidence-registry-sibling-advance")),
		DatasetSnapshotIndexDigest:  previous.DatasetSnapshotIndexDigest,
		DatasetSnapshotCount:        previous.DatasetSnapshotCount,
		EvidenceRegistryIndexDigest: domainsecurity.SHA256Hex([]byte("host-evidence-registry-index-1")),
		EvidenceRegistryCount:       previous.EvidenceRegistryCount + 1,
		PublicationIndexDigest:      previous.PublicationIndexDigest,
		PublicationCount:            previous.PublicationCount,
		AuthorityKeyID:              fixture.datasetSigning.keyID,
		AuthorityPublicKey:          fixture.datasetSigning.publicKey,
	}, fixture.datasetSigning.sign)
	if err != nil {
		t.Fatal(err)
	}
	selection.Head.Bundle = next
	selection.SelectionDigest, err = datasetsnapshotport.CanonicalCurrentSelectionDigestV2(selection)
	if err != nil {
		t.Fatal(err)
	}
	fixture.authority.setSelection(selection)
}

func (fixture *hostEvidenceManagerFixture) advanceDatasetChild(t *testing.T) {
	t.Helper()
	selection := fixture.authority.currentSelection()
	previousBundle := selection.Head.Bundle
	previousIndex := selection.DatasetIndexPath[0]
	nextIndex, err := domainsecurity.NewDatasetSnapshotIndexV1(domainsecurity.DatasetSnapshotIndexInputV1{
		InstallationID:       fixture.datasetSigning.installationID,
		EnrollmentID:         fixture.datasetSigning.enrollmentID,
		Generation:           previousIndex.Generation + 1,
		PreviousIndexDigest:  previousIndex.IndexDigest,
		MutationID:           domainsecurity.SHA256Hex([]byte("host-evidence-dataset-child-advance")),
		Binding:              selection.Snapshot.Record.Binding,
		SnapshotRecordDigest: selection.Snapshot.Record.RecordDigest,
		AuthorityKeyID:       fixture.datasetSigning.keyID,
		AuthorityPublicKey:   fixture.datasetSigning.publicKey,
	}, fixture.datasetSigning.sign)
	if err != nil {
		t.Fatal(err)
	}
	nextBundle, err := domainevidence.NewEvidenceAuthorityBundleV1(domainevidence.EvidenceAuthorityBundleInputV1{
		InstallationID:              fixture.datasetSigning.installationID,
		EnrollmentID:                fixture.datasetSigning.enrollmentID,
		Generation:                  previousBundle.Generation + 1,
		PreviousBundleDigest:        previousBundle.RecordDigest,
		MutationID:                  domainsecurity.SHA256Hex([]byte("host-evidence-dataset-bundle-advance")),
		DatasetSnapshotIndexDigest:  nextIndex.IndexDigest,
		DatasetSnapshotCount:        previousBundle.DatasetSnapshotCount + 1,
		EvidenceRegistryIndexDigest: previousBundle.EvidenceRegistryIndexDigest,
		EvidenceRegistryCount:       previousBundle.EvidenceRegistryCount,
		PublicationIndexDigest:      previousBundle.PublicationIndexDigest,
		PublicationCount:            previousBundle.PublicationCount,
		AuthorityKeyID:              fixture.datasetSigning.keyID,
		AuthorityPublicKey:          fixture.datasetSigning.publicKey,
	}, fixture.datasetSigning.sign)
	if err != nil {
		t.Fatal(err)
	}
	selection.Head.Bundle = nextBundle
	selection.DatasetIndexPath = append(
		[]domainsecurity.DatasetSnapshotIndexV1{nextIndex},
		selection.DatasetIndexPath...,
	)
	selection.SelectedIndex = nextIndex
	selection.SelectionDigest, err = datasetsnapshotport.CanonicalCurrentSelectionDigestV2(selection)
	if err != nil {
		t.Fatal(err)
	}
	fixture.authority.setSelection(selection)
}

func newHostEvidenceTurnSecurityContext(
	t *testing.T,
	binding domainsecurity.CaseBindingObservationV1,
	snapshot datasetsnapshotport.ResolvedSnapshotV2,
	turnID string,
) domainsecurity.TurnSecurityContext {
	t.Helper()
	policy, err := domainsecurity.NewTurnPublicationPolicyV1(domainsecurity.TurnPublicationPolicyInputV1{
		ThreadRiskPolicyDigest: domainsecurity.SHA256Hex([]byte("host-evidence-thread-risk")),
		RiskClass:              domainsecurity.RiskClassCase, Disposition: domainsecurity.PublicationDispositionCaseEvidenceGate,
		CaseBindingState: binding.State, BindingObservationDigest: binding.ObservationDigest,
		BlockerCode: domainsecurity.PublicationBlockerNone,
	})
	if err != nil {
		t.Fatal(err)
	}
	securityContext, err := domainsecurity.NewTurnSecurityContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-host-evidence", TurnID: turnID, WorkspaceRealPath: binding.WorkspaceRealPath,
		TenantID: domainsecurity.LocalTenantID, UserID: domainsecurity.LocalUserID,
		CaseID: binding.CaseID, CaseBindingHash: binding.CaseBindingHash,
		DatasetSnapshotID:  snapshot.Record.DatasetSnapshotID,
		SourceManifestHash: snapshot.Manifest.SourceManifestHash, ContextEpoch: 9,
		IssuedAt:          time.Date(2026, 7, 26, 8, 0, 0, 0, time.UTC),
		PublicationPolicy: policy,
		RiskAuthorityBinding: domainsecurity.RiskAuthorityBindingV1{
			SchemaVersion:     domainsecurity.RiskAuthorityBindingSchemaVersion,
			Purpose:           domainsecurity.RiskAuthorityBindingPurpose,
			State:             domainsecurity.RiskAuthorityBindingStateWitnessed,
			IndexDigest:       domainsecurity.SHA256Hex([]byte("host-evidence-risk-index")),
			Generation:        1,
			CheckpointDigest:  domainsecurity.SHA256Hex([]byte("host-evidence-risk-checkpoint")),
			ObservationDigest: domainsecurity.SHA256Hex([]byte("host-evidence-risk-observation")),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return securityContext
}

func newHostEvidenceDatasetSelection(
	t *testing.T,
	binding domainsecurity.CaseBindingObservationV1,
) (datasetsnapshotport.CurrentSelectionV2, hostEvidenceDatasetSigningMaterial) {
	t.Helper()
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	sign := func(message []byte) ([]byte, error) { return ed25519.Sign(privateKey, message), nil }
	installationID := domainsecurity.SHA256Hex([]byte("host-evidence-installation"))
	enrollmentID := domainsecurity.SHA256Hex([]byte("host-evidence-enrollment"))
	keyID := domainsecurity.SHA256Hex(publicKey)
	resolvedSnapshot, err := datasetsnapshotv2fixture.NewResolvedSnapshotV2(
		datasetsnapshotv2fixture.ResolvedInput{
			TenantID: domainsecurity.LocalTenantID, UserID: domainsecurity.LocalUserID,
			Observation: binding, Material: "host-evidence",
			InstallationID: installationID,
			AcceptedAt:     time.Date(2026, 7, 26, 7, 30, 0, 0, time.UTC),
			AuthorityKeyID: keyID, AuthorityPublicKey: publicKey, Sign: sign,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	record := resolvedSnapshot.Record
	index, err := domainsecurity.NewDatasetSnapshotIndexV1(domainsecurity.DatasetSnapshotIndexInputV1{
		InstallationID: installationID, EnrollmentID: enrollmentID, Generation: 1,
		PreviousIndexDigest: domainsecurity.DatasetSnapshotIndexGenesisDigestV1(),
		MutationID:          domainsecurity.SHA256Hex([]byte("host-evidence-dataset-index-mutation")),
		Binding:             record.Binding, SnapshotRecordDigest: record.RecordDigest,
		AuthorityKeyID: keyID, AuthorityPublicKey: publicKey,
	}, sign)
	if err != nil {
		t.Fatal(err)
	}
	bundle, err := domainevidence.NewEvidenceAuthorityBundleV1(domainevidence.EvidenceAuthorityBundleInputV1{
		InstallationID: installationID, EnrollmentID: enrollmentID, Generation: 1,
		MutationID:                 domainsecurity.SHA256Hex([]byte("host-evidence-shared-bundle-mutation")),
		DatasetSnapshotIndexDigest: index.IndexDigest, DatasetSnapshotCount: 1,
		EvidenceRegistryIndexDigest: domainevidence.EvidenceRegistryAuthorityIndexGenesisDigestV2(),
		EvidenceRegistryCount:       0,
		PublicationIndexDigest:      domainreportpublication.PublicationIndexGenesisDigestV1(),
		PublicationCount:            0, AuthorityKeyID: keyID, AuthorityPublicKey: publicKey,
	}, sign)
	if err != nil {
		t.Fatal(err)
	}
	selection := datasetsnapshotport.CurrentSelectionV2{
		Head:             evidenceauthorityport.FreshHead{HasBundle: true, Bundle: bundle},
		DatasetIndexPath: []domainsecurity.DatasetSnapshotIndexV1{index},
		SelectedIndex:    index,
		Snapshot:         resolvedSnapshot,
	}
	selection.SelectionDigest, err = datasetsnapshotport.CanonicalCurrentSelectionDigestV2(selection)
	if err != nil {
		t.Fatal(err)
	}
	return selection, hostEvidenceDatasetSigningMaterial{
		installationID: installationID, enrollmentID: enrollmentID, keyID: keyID,
		publicKey: publicKey, privateKey: privateKey,
	}
}
