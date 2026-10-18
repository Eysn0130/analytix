package mcp

import (
	httpmcp "analytix.local/runtime-go/internal/adapters/outbound/mcp/http"
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"analytix.local/runtime-go/internal/adapters/outbound/managededitingfiles"
	managededitingapp "analytix.local/runtime-go/internal/app/managedediting"
	"analytix.local/runtime-go/internal/app/workspacemutation"
)

func allowMCPTestEffect(context.Context) error { return nil }
func bindTestEffectAdmission(m *ProductionManager) *ProductionManager {
	if err := m.BindEffectAdmission(allowMCPTestEffect); err != nil {
		panic(err)
	}
	return m
}
func newAdmissionTestManager(specs []ServerSpec) *ProductionManager {
	return bindTestEffectAdmission(NewProductionManager(specs))
}
func newAdmissionTestManagerWithCache(specs []ServerSpec, cache string) *ProductionManager {
	return bindTestEffectAdmission(NewProductionManagerWithCache(specs, cache))
}
func newAdmissionTestManagerWithOptions(specs []ServerSpec, options ProductionManagerOptions) *ProductionManager {
	return bindTestEffectAdmission(NewProductionManagerWithOptions(specs, options))
}

func editingAdmissionFixture(t *testing.T) (*managededitingapp.Registry, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "captured.txt")
	if err := os.WriteFile(path, []byte("synthetic baseline"), 0600); err != nil {
		t.Fatal(err)
	}
	canonical, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	return managededitingapp.New(workspacemutation.NewCoordinator(), managededitingfiles.New()), canonical
}

func TestMCPEffectAdmissionFailsClosedAndCannotRebind(t *testing.T) {
	m := NewProductionManager(nil)
	client := &connectionEpochClient{}
	if _, err := m.admittedToolCall(context.Background(), client, "lookup", nil, nil); !errors.Is(err, errMCPEffectAdmission) || client.calls != 0 {
		t.Fatal("unbound manager executed effect")
	}
	if m.BindEffectAdmission(allowMCPTestEffect) == nil {
		t.Fatal("manager bound after an attempted effect")
	}
	m = NewProductionManager(nil)
	if m.BindEffectAdmission(allowMCPTestEffect) != nil || m.BindEffectAdmission(allowMCPTestEffect) == nil {
		t.Fatal("binding was not single assignment")
	}
}

func TestMCPConfigurationWithoutConstructorDoesNotTaintCapture(t *testing.T) {
	registry, path := editingAdmissionFixture(t)
	m := NewProductionManager([]ServerSpec{{ID: "unknown", Transport: "stdio", Command: "unused", BackgroundStart: true, TrustScope: "user"}})
	if err := m.BindEffectAdmission(registry.BeginOpaque); err != nil {
		t.Fatal(err)
	}
	m.Connect()
	defer m.Disconnect()
	release, err := registry.Capture(context.Background(), "capture", path)
	if err != nil {
		t.Fatal(err)
	}
	release()
}

func TestMCPRealConstructorAdmissionPrecedesInitializeAndStaysSticky(t *testing.T) {
	registry, path := editingAdmissionFixture(t)
	m := NewProductionManager(nil)
	if err := m.BindEffectAdmission(registry.BeginOpaque); err != nil {
		t.Fatal(err)
	}
	release, err := registry.Capture(context.Background(), "capture", path)
	if err != nil {
		t.Fatal(err)
	}
	original := newHTTPMCPTransportClient
	defer func() { newHTTPMCPTransportClient = original }()
	constructors := 0
	newHTTPMCPTransportClient = func(ServerSpec, string) (*httpmcp.TransportClient, error) {
		constructors++
		return nil, errors.New("synthetic constructor failure")
	}
	spec := ServerSpec{ID: "unknown", Transport: "http", URL: "https://mcp.invalid"}
	if _, err = m.newAdmittedMCPTransport(context.Background(), spec, "", nil); !errors.Is(err, errMCPEffectAdmission) || constructors != 0 {
		t.Fatal("constructor crossed capture barrier")
	}
	release()
	if _, err = m.newAdmittedMCPTransport(context.Background(), spec, "", nil); errors.Is(err, errMCPEffectAdmission) || err == nil || constructors != 1 {
		t.Fatal("admitted constructor did not dispatch")
	}
	if _, err = registry.Capture(context.Background(), "later", path); !errors.Is(err, managededitingapp.ErrUncontainedExecution) {
		t.Fatal("successful opaque admission was reset")
	}
}

func TestMCPUnknownWarmEffectRefusedBeforeCallAndStickyAfterAdmission(t *testing.T) {
	registry, path := editingAdmissionFixture(t)
	m := NewProductionManager(nil)
	client := &connectionEpochClient{}
	if err := m.BindEffectAdmission(registry.BeginOpaque); err != nil {
		t.Fatal(err)
	}
	release, err := registry.Capture(context.Background(), "capture", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = m.admittedToolCall(context.Background(), client, "lookup", nil, nil); !errors.Is(err, errMCPEffectAdmission) || client.calls != 0 || shouldRetryMCPCall(err) {
		t.Fatal("refusal dispatched or became retryable")
	}
	release()
	if _, err = m.admittedToolCall(context.Background(), client, "lookup", nil, nil); err != nil || client.calls != 1 {
		t.Fatal("admitted effect was not called exactly once")
	}
	if _, err = registry.Capture(context.Background(), "later", path); !errors.Is(err, managededitingapp.ErrUncontainedExecution) {
		t.Fatal("opaque call did not remain sticky")
	}
}

func TestMCPFundsPrivateClientStillRequiresOpaqueAdmission(t *testing.T) {
	fixture := newHostEvidenceManagerFixture(t, true)
	registry, path := editingAdmissionFixture(t)
	manager := NewProductionManagerWithOptions(nil, ProductionManagerOptions{
		DatasetAuthority: fixture.authority,
		HostFundsServer:  fixture.manager.hostFundsServer,
	})
	if err := manager.BindEffectAdmission(registry.BeginOpaque); err != nil {
		t.Fatal(err)
	}
	spec := manager.specs[0]
	client, err := manager.newMCPTransportClientNoLock(context.Background(), spec, SpecFingerprint(spec))
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if _, ok := client.(*hostFundsTransportClientV1); !ok {
		t.Fatal("fixture did not create the actual private Funds client")
	}
	release, err := registry.Capture(context.Background(), "funds-capture", path)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"", hostFundsCountToolNameV1, hostFundsAccountFlowToolNameV1} {
		if err := manager.admitMCPEffect(context.Background(), client, name); !errors.Is(err, errMCPEffectAdmission) {
			t.Fatal("private Funds type bypassed conservative admission")
		}
	}
	if result, err := manager.admittedToolCall(context.Background(), client, hostFundsCountToolNameV1, nil, nil); !errors.Is(err, errMCPEffectAdmission) || result != nil {
		t.Fatal("Funds tool dispatch crossed the admission barrier")
	}
	release()
	if err := manager.admitMCPEffect(context.Background(), client, ""); err != nil {
		t.Fatal("ordinary Funds metadata effect could not be admitted", err)
	}
	if _, err := client.ListTools(); err != nil {
		t.Fatal("admitted Funds catalog was unavailable", err)
	}
	client.Close()
	if _, err := registry.Capture(context.Background(), "funds-later", path); !errors.Is(err, managededitingapp.ErrUncontainedExecution) {
		t.Fatal("Funds opaque admission did not remain sticky after Close")
	}
}

type admissionCatalogClient struct {
	connectionEpochClient
	prompts, resources atomic.Int32
}

func (c *admissionCatalogClient) ListPrompts() ([]PromptSpec, error) {
	c.prompts.Add(1)
	return nil, nil
}
func (c *admissionCatalogClient) ListResources() ([]ResourceSpec, error) {
	c.resources.Add(1)
	return nil, nil
}
func TestMCPAsyncCatalogAdmissionRejectsBeforeEitherEffect(t *testing.T) {
	m := NewProductionManager(nil)
	var decisions atomic.Int32
	if err := m.BindEffectAdmission(func(context.Context) error { decisions.Add(1); return errors.New("refused") }); err != nil {
		t.Fatal(err)
	}
	client := &admissionCatalogClient{}
	m.clients["unknown"] = client
	m.refreshPromptResourceCatalogAsync(ServerSpec{ID: "unknown"}, client)
	deadline := time.After(2 * time.Second)
	for decisions.Load() < 2 {
		select {
		case <-deadline:
			t.Fatal("catalog admission missing")
		default:
			time.Sleep(time.Millisecond)
		}
	}
	if client.prompts.Load() != 0 || client.resources.Load() != 0 {
		t.Fatal("catalog effect ran after refusal")
	}
}

func TestMCPProbeCatalogAndNativeAdmissionPrecedeEffects(t *testing.T) {
	for _, denyAt := range []int{1, 2} {
		t.Run(string(rune('0'+denyAt)), func(t *testing.T) {
			fixture := newHostEvidenceManagerFixture(t, true)
			type probeKey struct{}
			decisions := 0
			fixture.manager.effectAdmission.mu.Lock()
			fixture.manager.effectAdmission.begin = func(ctx context.Context) error {
				if ctx.Value(probeKey{}) == true {
					decisions++
					if decisions == denyAt {
						return errors.New("refused")
					}
				}
				return nil
			}
			fixture.manager.effectAdmission.mu.Unlock()
			_, err := fixture.manager.ProbeCaseSource(context.WithValue(context.Background(), probeKey{}, true), fixture.probeInput(fixture.securityContext))
			if err == nil || decisions != denyAt || fixture.client.listCalls != denyAt-1 || fixture.client.nativeCalls != 0 {
				t.Fatalf("probe crossed admission: decisions=%d list=%d native=%d err=%v", decisions, fixture.client.listCalls, fixture.client.nativeCalls, err)
			}
		})
	}
}
