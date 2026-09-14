package pluginpackagehost

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	identitydomain "analytix.local/runtime-go/internal/domain/identity"
	domainplugin "analytix.local/runtime-go/internal/domain/pluginmaterialization"
	domainpackage "analytix.local/runtime-go/internal/domain/pluginpackage"
	identityport "analytix.local/runtime-go/internal/ports/identity"
	materializationport "analytix.local/runtime-go/internal/ports/pluginmaterialization"
	adapterport "analytix.local/runtime-go/internal/ports/pluginpackagehost"
)

type fakeIdentity struct {
	current identitydomain.PrincipalV1
	invalid bool
}

func (f *fakeIdentity) ResolveCurrent(context.Context) (identitydomain.PrincipalV1, error) {
	return f.current, nil
}
func (f *fakeIdentity) ValidateCurrent(_ context.Context, p identitydomain.PrincipalV1) error {
	if f.invalid || !identitydomain.SamePrincipalV1(f.current, p) {
		return identityport.ErrMismatch
	}
	return nil
}

type fakeAuthority struct{ private ed25519.PrivateKey }

func (f fakeAuthority) PublicKey() []byte { return f.private.Public().(ed25519.PublicKey) }
func (f fakeAuthority) KeyID() string {
	digest := sha256.Sum256(f.PublicKey())
	return hex.EncodeToString(digest[:])
}
func (f fakeAuthority) Sign(_ context.Context, body []byte) ([]byte, error) {
	return ed25519.Sign(f.private, body), nil
}

type fakeState struct {
	current    materializationport.ResultV1
	activation domainplugin.ActivationV1
	now        time.Time
	readErr    error
	setErr     error
	afterRead  func()
	afterSet   func()
	writes     int
}

func (f *fakeState) Materialize(context.Context, domainplugin.IntentV1, materializationport.InstallationAuthority, time.Time) (materializationport.ResultV1, error) {
	panic("Host must not materialize")
}
func (f *fakeState) ResolveActive(context.Context, materializationport.InstallationAuthority) (materializationport.ResultV1, error) {
	return f.current, nil
}
func (f *fakeState) ReadActivation(context.Context, materializationport.InstallationAuthority) (domainplugin.ActivationV1, error) {
	if f.afterRead != nil {
		f.afterRead()
	}
	if f.readErr != nil {
		return domainplugin.ActivationV1{}, f.readErr
	}
	if f.activation.Revision == 0 {
		return domainplugin.ActivationV1{}, errors.Join(materializationport.ErrUnavailable, materializationport.ErrNotFound)
	}
	return f.activation, nil
}
func (f *fakeState) SetDesiredState(ctx context.Context, request materializationport.SetDesiredStateRequestV1, authority materializationport.InstallationAuthority, now time.Time) (domainplugin.ActivationV1, error) {
	if request.GenerationID != f.current.Receipt.GenerationID || request.ExpectedRevision != f.activation.Revision {
		return domainplugin.ActivationV1{}, materializationport.ErrConflict
	}
	state, err := domainplugin.NewActivationV1(f.current.Receipt, request.ExpectedRevision+1, request.DesiredState, now, authority.KeyID(), authority.PublicKey(), func(body []byte) ([]byte, error) { return authority.Sign(ctx, body) })
	if err != nil {
		return domainplugin.ActivationV1{}, err
	}
	f.activation = state
	f.writes++
	if f.afterSet != nil {
		f.afterSet()
	}
	if f.setErr != nil {
		return domainplugin.ActivationV1{}, f.setErr
	}
	return state, nil
}

type fakeMaterialization struct {
	store *fakeState
	err   error
}

func (f *fakeMaterialization) ResolveActive(context.Context) (materializationport.ResultV1, error) {
	return f.store.current, f.err
}

type fakeAdapter struct {
	ready       bool
	operations  []string
	calls       []adapterport.Call
	onReadiness func()
	onInvoke    func()
	err         error
	output      json.RawMessage
}

func (f *fakeAdapter) Readiness(context.Context, adapterport.Binding) (adapterport.Readiness, error) {
	if f.onReadiness != nil {
		f.onReadiness()
	}
	return adapterport.Readiness{Available: f.ready, Operations: f.operations}, nil
}
func (f *fakeAdapter) Invoke(_ context.Context, call adapterport.Call) (adapterport.Result, error) {
	f.calls = append(f.calls, call)
	if f.onInvoke != nil {
		f.onInvoke()
	}
	return adapterport.Result{Output: f.output}, f.err
}

type hostFixture struct {
	host         *Service
	state        *fakeState
	identity     *fakeIdentity
	authority    fakeAuthority
	adapter      *fakeAdapter
	registration Registration
	now          time.Time
}

func fixture(t *testing.T) hostFixture {
	t.Helper()
	now := time.Date(2026, 9, 14, 0, 0, 0, 0, time.UTC)
	seed := sha256.Sum256([]byte("plugin-host-synthetic-authority"))
	authority := fakeAuthority{ed25519.NewKeyFromSeed(seed[:])}
	principal, err := identitydomain.NewPrincipalV1(strings.Repeat("1", 64), "local", "local")
	if err != nil {
		t.Fatal(err)
	}
	identity := &fakeIdentity{current: principal}
	input := domainplugin.IntentInputV1{Origin: domainplugin.DevelopmentSourceOriginV1, SourceRegistrationSHA256: strings.Repeat("a", 64), Target: domainplugin.TargetV1{Platform: "darwin", Arch: "arm64"}, PluginName: "analytix-documents", PluginVersion: "1.0.0", SourceRoot: "/private/synthetic/source", SourceTreeSHA256: strings.Repeat("b", 64), SourceTreeFileCount: 4, ManifestSHA256: strings.Repeat("c", 64), RequestedAt: now}
	intent, err := domainplugin.NewIntentV1(input)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := domainplugin.NewReceiptV1(intent, strings.Repeat("d", 64), "plugins/cache/analytix-hub/analytix-documents/1.0.0", now, authority.KeyID(), authority.PublicKey(), func(body []byte) ([]byte, error) { return authority.Sign(context.Background(), body) })
	if err != nil {
		t.Fatal(err)
	}
	index, err := domainplugin.NewIndexV1(receipt, now)
	if err != nil {
		t.Fatal(err)
	}
	state := &fakeState{current: materializationport.ResultV1{Receipt: receipt, Index: index}, now: now}
	adapter := &fakeAdapter{ready: true, operations: []string{"open", "export", "close"}, output: json.RawMessage(`{"objectId":"synthetic-object"}`)}
	registration := Registration{Identity: domainpackage.PackageIdentityV1{PackageID: intent.PluginName, PackageVersion: intent.PluginVersion}, SourceRegistrationSHA256: intent.SourceRegistrationSHA256, Materialization: &fakeMaterialization{store: state}, State: state, Adapter: adapter}
	host, err := New(identity, authority, []Registration{registration}, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	return hostFixture{host, state, identity, authority, adapter, registration, now}
}
func (f hostFixture) setRequest(state domainplugin.DesiredStateV1) SetDesiredStateRequest {
	return SetDesiredStateRequest{PackageID: f.registration.Identity.PackageID, GenerationID: f.state.current.Receipt.GenerationID, ExpectedRevision: f.state.activation.Revision, DesiredState: state}
}
func (f hostFixture) invokeRequest() InvokeRequest {
	return InvokeRequest{PackageID: f.registration.Identity.PackageID, GenerationID: f.state.current.Receipt.GenerationID, ExpectedRevision: f.state.activation.Revision, ContributionID: WorkspaceEditorContributionID, Operation: "open", Input: json.RawMessage(`{"objectId":"synthetic-object"}`)}
}
func (f hostFixture) enable(t *testing.T) {
	t.Helper()
	if _, err := f.host.SetDesiredState(context.Background(), f.setRequest(domainplugin.DesiredEnabledV1)); err != nil {
		t.Fatal(err)
	}
}

func TestListDistinguishesUnsetDisabledAndAdapterUnavailable(t *testing.T) {
	f := fixture(t)
	ctx := context.Background()
	views, err := f.host.List(ctx)
	if err != nil || len(views) != 1 {
		t.Fatal(err)
	}
	view := views[0]
	if !view.Materialized || view.ActivationState != "unset" || view.DesiredState != "" || view.ActivationID != "" || view.Available || view.Publishable {
		t.Fatalf("unset state invented a receipt: %+v", view)
	}
	view, err = f.host.SetDesiredState(ctx, f.setRequest(domainplugin.DesiredDisabledV1))
	if err != nil || view.DesiredState != domainplugin.DesiredDisabledV1 || view.ActivationState != "recorded" || view.Available {
		t.Fatal("disabled projection failed", err)
	}
	f.adapter.ready = false
	view, err = f.host.SetDesiredState(ctx, f.setRequest(domainplugin.DesiredEnabledV1))
	if err != nil || view.DesiredState != domainplugin.DesiredEnabledV1 || view.Available || view.UnavailableReason != "adapter_unavailable" {
		t.Fatal("installation claimed adapter readiness", err)
	}
	if _, err := f.host.Invoke(ctx, f.invokeRequest()); err != ErrAdapterUnavailable || len(f.adapter.calls) != 0 {
		t.Fatal("not-ready adapter was invoked", err)
	}
	f.adapter.ready = true
	views, err = f.host.List(ctx)
	if err != nil || !views[0].Available || len(views[0].Operations) != 3 {
		t.Fatal("ready adapter did not become available", err)
	}
	body, _ := json.Marshal(views)
	for _, private := range []string{"/private", f.authority.KeyID(), "authorityPublicKey", "sourceRegistrationSha256", "sourceRegistrationJson", "principal"} {
		if strings.Contains(string(body), private) {
			t.Fatal("private metadata leaked", private)
		}
	}
}

func TestInvokeBindsGenerationActivationContributionAndCurrentPrincipal(t *testing.T) {
	f := fixture(t)
	f.enable(t)
	ctx := context.Background()
	request := f.invokeRequest()
	for _, mutate := range []func(*InvokeRequest){func(r *InvokeRequest) { r.GenerationID = strings.Repeat("e", 64) }, func(r *InvokeRequest) { r.ExpectedRevision++ }, func(r *InvokeRequest) { r.ContributionID = "editor-adapter" }, func(r *InvokeRequest) { r.Operation = "arbitrary_uno_command" }, func(r *InvokeRequest) { r.Input = json.RawMessage(`{"path":"/private/file"}`) }, func(r *InvokeRequest) { r.Input = json.RawMessage(`{"objectId":"a","objectId":"b"}`) }} {
		changed := request
		mutate(&changed)
		if _, err := f.host.Invoke(ctx, changed); err == nil {
			t.Fatal("invalid invocation admitted")
		}
	}
	if len(f.adapter.calls) != 0 {
		t.Fatal("rejected invocations reached adapter")
	}
	result, err := f.host.Invoke(ctx, request)
	if err != nil || string(result.Output) != string(f.adapter.output) || len(f.adapter.calls) != 1 {
		t.Fatal("authorized invocation failed", err)
	}
	call := f.adapter.calls[0]
	if call.Binding.PackageID != request.PackageID || call.Binding.GenerationID != request.GenerationID || call.Binding.ActivationRevision != request.ExpectedRevision || call.Binding.SourceRegistrationSHA256 != f.registration.SourceRegistrationSHA256 || !identitydomain.SamePrincipalV1(call.Principal, f.identity.current) {
		t.Fatal("adapter lost Core authority binding")
	}
	f.state.afterRead = func() { f.identity.invalid = true }
	if _, err := f.host.Invoke(ctx, request); err != ErrIdentity || len(f.adapter.calls) != 1 {
		t.Fatal("identity revoked during authorization still invoked", err)
	}
}

func TestDisableAndInvokeShareOneExecutionBoundary(t *testing.T) {
	f := fixture(t)
	f.enable(t)
	ctx := context.Background()
	request := f.invokeRequest()
	disable := f.setRequest(domainplugin.DesiredDisabledV1)
	entered := make(chan struct{})
	release := make(chan struct{})
	invoked := make(chan error, 1)
	disabled := make(chan error, 1)
	disableStarted := make(chan struct{})
	f.adapter.onInvoke = func() { close(entered); <-release }
	go func() { _, err := f.host.Invoke(ctx, request); invoked <- err }()
	<-entered
	if f.host.mu.TryLock() {
		f.host.mu.Unlock()
		close(release)
		<-invoked
		t.Fatal("Host released serialization while adapter was executing")
	}
	go func() { close(disableStarted); _, err := f.host.SetDesiredState(ctx, disable); disabled <- err }()
	<-disableStarted
	close(release)
	if err := <-invoked; err != nil {
		t.Fatal(err)
	}
	if err := <-disabled; err != nil {
		t.Fatal(err)
	}
	if f.state.activation.DesiredState != domainplugin.DesiredDisabledV1 {
		t.Fatal("disable did not commit")
	}
	if _, err := f.host.Invoke(ctx, request); err != ErrDisabled || len(f.adapter.calls) != 1 {
		t.Fatal("adapter started after disable completed", err)
	}
}

func TestHostRejectsUntrustedOriginAndStaleGenerationState(t *testing.T) {
	f := fixture(t)
	f.enable(t)
	ctx := context.Background()
	request := f.invokeRequest()
	// Re-sign a structurally valid formal envelope with the same installation key.
	// Its valid signature does not grant this development Host a matching origin.
	receipt := f.state.current.Receipt
	formalIntent, err := domainplugin.NewIntentV1(domainplugin.IntentInputV1{PackageAuthoritySHA256: f.registration.SourceRegistrationSHA256, Target: receipt.Target, PluginName: receipt.PluginName, PluginVersion: receipt.PluginVersion, SourceRoot: "/formal/source", SourceTreeSHA256: receipt.SourceTreeSHA256, SourceTreeFileCount: receipt.SourceTreeFileCount, ManifestSHA256: receipt.ManifestSHA256, EntrypointSHA256: strings.Repeat("f", 64), RequestedAt: f.now})
	if err != nil {
		t.Fatal(err)
	}
	formal, err := domainplugin.NewReceiptV1(formalIntent, receipt.GenerationID, receipt.ActiveRelativePath, f.now, f.authority.KeyID(), f.authority.PublicKey(), func(b []byte) ([]byte, error) { return f.authority.Sign(ctx, b) })
	if err != nil {
		t.Fatal(err)
	}
	index, err := domainplugin.NewIndexV1(formal, f.now)
	if err != nil {
		t.Fatal(err)
	}
	f.state.current = materializationport.ResultV1{Receipt: formal, Index: index}
	if _, err := f.host.Invoke(ctx, request); err != ErrUnavailable {
		t.Fatal("formal receipt was accepted", err)
	}
	views, err := f.host.List(ctx)
	if err != nil || views[0].Materialized || views[0].Available {
		t.Fatal("formal source shown available", err)
	}
	f = fixture(t)
	f.enable(t)
	request = f.invokeRequest()
	oldActivation := f.state.activation
	f.state.current.Receipt.GenerationID = strings.Repeat("e", 64)
	// An invalid signature is rejected before stale activation can authorize it.
	if _, err := f.host.Invoke(ctx, request); err != ErrUnavailable {
		t.Fatal("modified generation accepted", err)
	}
	f = fixture(t)
	f.state.activation = oldActivation
	current := f.state.current.Receipt
	nextIntent, err := domainplugin.NewIntentV1(domainplugin.IntentInputV1{Origin: domainplugin.DevelopmentSourceOriginV1, SourceRegistrationSHA256: current.SourceRegistrationSHA256, Target: current.Target, PluginName: current.PluginName, PluginVersion: current.PluginVersion, SourceRoot: "/private/synthetic/source", SourceTreeSHA256: current.SourceTreeSHA256, SourceTreeFileCount: current.SourceTreeFileCount, ManifestSHA256: current.ManifestSHA256, RequestedAt: f.now.Add(time.Second)})
	if err != nil {
		t.Fatal(err)
	}
	nextReceipt, err := domainplugin.NewReceiptV1(nextIntent, strings.Repeat("e", 64), current.ActiveRelativePath, f.now, f.authority.KeyID(), f.authority.PublicKey(), func(b []byte) ([]byte, error) { return f.authority.Sign(ctx, b) })
	if err != nil {
		t.Fatal(err)
	}
	nextIndex, err := domainplugin.NewIndexV1(nextReceipt, f.now)
	if err != nil {
		t.Fatal(err)
	}
	f.state.current = materializationport.ResultV1{Receipt: nextReceipt, Index: nextIndex}
	views, err = f.host.List(ctx)
	if err != nil || !views[0].Materialized || views[0].ActivationState != "unavailable" || views[0].Available {
		t.Fatal("new signed generation inherited prior activation", err)
	}
	if _, err := f.host.Invoke(ctx, f.invokeRequest()); err != ErrUnavailable {
		t.Fatal("old signed activation authorized new generation", err)
	}
	f.state.activation = domainplugin.ActivationV1{}
	views, err = f.host.List(ctx)
	if err != nil || views[0].ActivationState != "unset" || views[0].DesiredState != "" {
		t.Fatal("new generation without activation was not unset", err)
	}

	if len(f.adapter.calls) != 0 {
		t.Fatal("rejected source reached adapter")
	}
}

func TestSetFailureDoesNotAssumeRollbackAndErrorsStayClosed(t *testing.T) {
	f := fixture(t)
	ctx := context.Background()
	f.state.setErr = errors.New("private root /private/installation key must never escape")
	if _, err := f.host.SetDesiredState(ctx, f.setRequest(domainplugin.DesiredEnabledV1)); err != ErrPersistence {
		t.Fatal("raw persistence error escaped", err)
	}
	views, err := f.host.List(ctx)
	if err != nil || views[0].DesiredState != domainplugin.DesiredEnabledV1 || views[0].ActivationRevision != 1 {
		t.Fatal("Host assumed rollback after persisted failure", err)
	}
	f.adapter.err = errors.New("private adapter root")
	if _, err := f.host.Invoke(ctx, f.invokeRequest()); err != ErrAdapterUnavailable {
		t.Fatal("raw adapter error escaped", err)
	}
	f.state.readErr = errors.Join(materializationport.ErrCorrupt, materializationport.ErrNotFound)
	views, err = f.host.List(ctx)
	if err != nil || views[0].ActivationState != "unavailable" {
		t.Fatal("corrupt storage mislabeled unset", err)
	}
}

func TestActivationAbsenceUsesPortErrorWithoutHidingPersistenceFailures(t *testing.T) {
	for _, test := range []struct {
		name  string
		err   error
		state string
		want  error
	}{
		{"missing", materializationport.ErrNotFound, "unset", ErrDisabled},
		{"unavailable", materializationport.ErrUnavailable, "unavailable", ErrUnavailable},
		{"corrupt-missing", errors.Join(materializationport.ErrCorrupt, materializationport.ErrNotFound), "unavailable", ErrUnavailable},
		{"storage-failure", errors.New("private storage failure"), "unavailable", ErrUnavailable},
	} {
		t.Run(test.name, func(t *testing.T) {
			f := fixture(t)
			f.state.readErr = test.err
			views, err := f.host.List(context.Background())
			if err != nil || len(views) != 1 || views[0].ActivationState != test.state || views[0].Available {
				t.Fatalf("unexpected activation projection: %+v, %v", views, err)
			}
			request := f.invokeRequest()
			request.ExpectedRevision = 1
			if _, err := f.host.Invoke(context.Background(), request); err != test.want || len(f.adapter.calls) != 0 {
				t.Fatalf("activation error authorized adapter or escaped vocabulary: %v", err)
			}
		})
	}
}

func TestHostRejectsInvalidRegistrationAndStalePrincipalForEveryMethod(t *testing.T) {
	f := fixture(t)
	registrations := []Registration{f.registration}
	host, err := New(f.identity, f.authority, registrations, func() time.Time { return f.now })
	if err != nil {
		t.Fatal(err)
	}
	registrations[0].Identity.PackageID = "modified-after-construction"
	views, err := host.List(context.Background())
	if err != nil || views[0].PackageID != "analytix-documents" {
		t.Fatal("composition registry was caller-mutable")
	}
	bad := f.registration
	bad.Identity.PackageID = "analytix-fund-analysis"
	if _, err := New(f.identity, f.authority, []Registration{bad}, func() time.Time { return f.now }); err != ErrInvalid {
		t.Fatal("foreign registration accepted", err)
	}
	if _, err := New(f.identity, f.authority, []Registration{f.registration, f.registration}, func() time.Time { return f.now }); err != ErrInvalid {
		t.Fatal("duplicate registration accepted", err)
	}
	f.identity.invalid = true
	if _, err := f.host.List(context.Background()); err != ErrIdentity {
		t.Fatal(err)
	}
	if _, err := f.host.SetDesiredState(context.Background(), f.setRequest(domainplugin.DesiredEnabledV1)); err != ErrIdentity {
		t.Fatal(err)
	}
	if _, err := f.host.Invoke(context.Background(), f.invokeRequest()); err != ErrIdentity {
		t.Fatal(err)
	}
	if f.state.writes != 0 || len(f.adapter.calls) != 0 {
		t.Fatal("stale principal reached effect")
	}
}

func TestHostBoundsInputAndAdapterProtocol(t *testing.T) {
	f := fixture(t)
	f.enable(t)
	ctx := context.Background()
	for _, input := range []json.RawMessage{json.RawMessage(`{"nested":{"root":"/private"}}`), json.RawMessage(`{"filePath":"/private/file"}`), json.RawMessage(`{"command":"execute"}`), json.RawMessage(`[]`), json.RawMessage(`{"text":"` + strings.Repeat("x", MaxInputBytes) + `"}`)} {
		request := f.invokeRequest()
		request.Input = input
		if _, err := f.host.Invoke(ctx, request); err != ErrInvalid {
			t.Fatal("unsafe or unbounded input accepted", err)
		}
	}
	f.adapter.operations = []string{"open", "open"}
	if _, err := f.host.Invoke(ctx, f.invokeRequest()); err != ErrAdapterUnavailable {
		t.Fatal("ambiguous readiness allowlist accepted", err)
	}
	f.adapter.operations = []string{"open"}
	f.adapter.output = json.RawMessage(`{"objectId":"a","objectId":"b"}`)
	if _, err := f.host.Invoke(ctx, f.invokeRequest()); err != ErrAdapterUnavailable {
		t.Fatal("ambiguous adapter output accepted", err)
	}
}

func TestHostObjectSelectorIsLimitedToNamedOperation(t *testing.T) {
	valid := json.RawMessage(`{"object":{"workspace":"/selected/workspace","path":"report.docx"}}`)
	if !validInput(valid, "open-object") || validInput(valid, "open") {
		t.Fatal("object selector did not stay operation scoped")
	}
	for _, body := range []string{
		`{"object":{"workspace":"/selected","path":"report.docx","root":"/private"}}`,
		`{"object":{"workspace":"/selected","path":"report.docx"},"nested":{"args":[]}}`,
		`{"object":{"workspace":"/selected","path":null}}`,
		`{"object":{"workspace":"/selected","path":"report.docx","path":"other.docx"}}`,
	} {
		if validInput(json.RawMessage(body), "open-object") {
			t.Fatal("unsafe selector accepted")
		}
	}
}
