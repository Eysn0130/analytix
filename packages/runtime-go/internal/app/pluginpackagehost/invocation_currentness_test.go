package pluginpackagehost

import (
	"context"
	"strings"
	"testing"
	"time"

	domainplugin "analytix.local/runtime-go/internal/domain/pluginmaterialization"
	materializationport "analytix.local/runtime-go/internal/ports/pluginmaterialization"
)

// The Host serializes its own calls, but an independently committed store
// transition must also invalidate work held inside readiness or an adapter.
func TestInvokeRejectsAuthorityChangedAcrossAdapterBoundary(t *testing.T) {
	for _, boundary := range []string{"readiness", "completion"} {
		for _, change := range []string{"disable", "disable-enable", "generation", "corrupt-activation", "missing-activation"} {
			t.Run(boundary+"/"+change, func(t *testing.T) {
				f := fixture(t)
				f.enable(t)
				ctx := context.Background()
				request := f.invokeRequest()
				mutate := func() {
					switch change {
					case "disable", "disable-enable":
						for _, state := range []domainplugin.DesiredStateV1{domainplugin.DesiredDisabledV1, domainplugin.DesiredEnabledV1} {
							_, err := f.state.SetDesiredState(ctx, materializationport.SetDesiredStateRequestV1{GenerationID: request.GenerationID, ExpectedRevision: f.state.activation.Revision, DesiredState: state}, f.authority, f.now.Add(time.Second))
							if err != nil {
								t.Fatal(err)
							}
							if change == "disable" {
								break
							}
						}
					case "generation":
						old := f.state.current.Receipt
						intent, err := domainplugin.NewIntentV1(domainplugin.IntentInputV1{Origin: old.Origin, SourceRegistrationSHA256: old.SourceRegistrationSHA256, Target: old.Target, PluginName: old.PluginName, PluginVersion: old.PluginVersion, SourceRoot: "/private/synthetic/source", SourceTreeSHA256: old.SourceTreeSHA256, SourceTreeFileCount: old.SourceTreeFileCount, ManifestSHA256: old.ManifestSHA256, RequestedAt: f.now.Add(time.Second)})
						if err != nil {
							t.Fatal(err)
						}
						receipt, err := domainplugin.NewReceiptV1(intent, strings.Repeat("e", 64), old.ActiveRelativePath, f.now.Add(time.Second), f.authority.KeyID(), f.authority.PublicKey(), func(b []byte) ([]byte, error) { return f.authority.Sign(ctx, b) })
						if err != nil {
							t.Fatal(err)
						}
						index, err := domainplugin.NewIndexV1(receipt, f.now.Add(time.Second))
						if err != nil {
							t.Fatal(err)
						}
						f.state.current = materializationport.ResultV1{Receipt: receipt, Index: index}
					case "corrupt-activation":
						f.state.readErr = materializationport.ErrCorrupt
					case "missing-activation":
						f.state.activation = domainplugin.ActivationV1{}
					}
				}
				if boundary == "readiness" {
					f.adapter.onReadiness = mutate
				} else {
					f.adapter.onInvoke = mutate
				}
				result, err := f.host.Invoke(ctx, request)
				if err == nil || len(result.Output) != 0 {
					t.Fatalf("changed authority returned success/output: %v %s", err, result.Output)
				}
				calls := 0
				if boundary == "completion" {
					calls = 1
				}
				if len(f.adapter.calls) != calls {
					t.Fatalf("adapter calls = %d, want %d", len(f.adapter.calls), calls)
				}
			})
		}
	}
}
