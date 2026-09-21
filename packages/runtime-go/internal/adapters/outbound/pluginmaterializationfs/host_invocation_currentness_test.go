package pluginmaterializationfs

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	pluginapp "analytix.local/runtime-go/internal/app/pluginmaterialization"
	hostapp "analytix.local/runtime-go/internal/app/pluginpackagehost"
	identitydomain "analytix.local/runtime-go/internal/domain/identity"
	domainplugin "analytix.local/runtime-go/internal/domain/pluginmaterialization"
	domainpackage "analytix.local/runtime-go/internal/domain/pluginpackage"
	pluginport "analytix.local/runtime-go/internal/ports/pluginmaterialization"
	adapterport "analytix.local/runtime-go/internal/ports/pluginpackagehost"
)

type currentnessAdapter struct {
	onReady, onInvoke func()
	calls             int
}

func (a *currentnessAdapter) Readiness(context.Context, adapterport.Binding) (adapterport.Readiness, error) {
	if a.onReady != nil {
		a.onReady()
	}
	return adapterport.Readiness{Available: true, Operations: []string{"open"}}, nil
}
func (a *currentnessAdapter) Invoke(context.Context, adapterport.Call) (adapterport.Result, error) {
	a.calls++
	if a.onInvoke != nil {
		a.onInvoke()
	}
	return adapterport.Result{Output: json.RawMessage(`{"synthetic":"late-result"}`)}, nil
}

func TestInstalledHostRejectsIndependentActivationAndReopenABA(t *testing.T) {
	for _, boundary := range []string{"readiness", "completion"} {
		for _, reenable := range []bool{false, true} {
			name := boundary + "/disable"
			if reenable {
				name += "-enable"
			}
			t.Run(name, func(t *testing.T) {
				ctx := context.Background()
				source, _ := writeCanvasSkillSourceV1(t)
				home := realTempDir(t)
				store, err := NewPackageStoreV1(home, "analytix-canvas", nil)
				if err != nil {
					t.Fatal(err)
				}
				authority := newTestAuthority()
				now := time.Date(2026, 9, 21, 0, 0, 0, 0, time.UTC)
				binding := developmentBindingV1(t, source)
				service, err := pluginapp.NewDevelopmentSourceServiceV1(store, authority, binding, func() time.Time { return now })
				if err != nil {
					t.Fatal(err)
				}
				intent, err := binding.NewIntentV1(now)
				if err != nil {
					t.Fatal(err)
				}
				installed, err := service.Materialize(ctx, intent)
				if err != nil {
					t.Fatal(err)
				}
				principal, err := identitydomain.NewPrincipalV1(strings.Repeat("1", 64), "local", "local")
				if err != nil {
					t.Fatal(err)
				}
				adapter := &currentnessAdapter{}
				registration := hostapp.Registration{Identity: domainpackage.PackageIdentityV1{PackageID: "analytix-canvas", PackageVersion: "1.0.0"}, SourceRegistrationSHA256: intent.SourceRegistrationSHA256, Materialization: service, State: store, Adapter: adapter}
				host, err := hostapp.New(&canvasSkillIdentity{principal: principal}, authority, []hostapp.Registration{registration}, func() time.Time { return now })
				if err != nil {
					t.Fatal(err)
				}
				enabled, err := host.SetDesiredState(ctx, hostapp.SetDesiredStateRequest{PackageID: "analytix-canvas", GenerationID: installed.Receipt.GenerationID, DesiredState: domainplugin.DesiredEnabledV1})
				if err != nil {
					t.Fatal(err)
				}
				request := hostapp.InvokeRequest{PackageID: "analytix-canvas", GenerationID: enabled.GenerationID, ExpectedRevision: enabled.ActivationRevision, ContributionID: hostapp.WorkspaceEditorContributionID, Operation: "open", Input: json.RawMessage(`{}`)}
				if _, err := host.Invoke(ctx, request); err != nil || adapter.calls != 1 {
					t.Fatal("current installed binding rejected", err)
				}
				// Commit through a separately opened real store, without re-entering
				// the Host lock. This changes signed durable activation, not a mock.
				reopened, err := OpenExistingPackageStoreV1(home, "analytix-canvas")
				if err != nil {
					t.Fatal(err)
				}
				mutate := func() {
					revision := enabled.ActivationRevision
					for _, state := range []domainplugin.DesiredStateV1{domainplugin.DesiredDisabledV1, domainplugin.DesiredEnabledV1} {
						updated, err := reopened.SetDesiredState(ctx, pluginport.SetDesiredStateRequestV1{GenerationID: enabled.GenerationID, ExpectedRevision: revision, DesiredState: state}, authority, now.Add(time.Second))
						if err != nil {
							t.Fatal(err)
						}
						revision = updated.Revision
						if !reenable {
							break
						}
					}
				}
				if boundary == "readiness" {
					adapter.onReady = mutate
				} else {
					adapter.onInvoke = mutate
				}
				result, err := host.Invoke(ctx, request)
				want := hostapp.ErrDisabled
				if reenable {
					want = hostapp.ErrConflict
				}
				calls := 1
				if boundary == "completion" {
					calls = 2
				}
				if err != want || len(result.Output) != 0 || adapter.calls != calls {
					t.Fatalf("late authority accepted: error=%v calls=%d output=%s", err, adapter.calls, result.Output)
				}
				adapter.onReady, adapter.onInvoke = nil, nil
				restored, err := OpenExistingPackageStoreV1(home, "analytix-canvas")
				if err != nil {
					t.Fatal(err)
				}
				activation, err := restored.ReadActivation(ctx, authority)
				if err != nil || activation.Revision <= enabled.ActivationRevision {
					t.Fatal("transition did not persist", err)
				}
				if _, err := host.Invoke(ctx, request); err != want || adapter.calls != calls {
					t.Fatal("old invocation revived after reopen", err)
				}
				if reenable {
					request.ExpectedRevision = activation.Revision
					if _, err := host.Invoke(ctx, request); err != nil || adapter.calls != calls+1 {
						t.Fatal("fresh revision was not usable", err)
					}
				}
			})
		}
	}
}
