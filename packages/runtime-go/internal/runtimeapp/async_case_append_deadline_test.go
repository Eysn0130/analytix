//go:build darwin || linux

package runtimeapp

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	finalauthority "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	nativecomponenthost "analytix.local/runtime-go/internal/adapters/outbound/nativecomponenthost"
	packagedauthorityfs "analytix.local/runtime-go/internal/adapters/outbound/packagedbuildauthorityfs"
	"analytix.local/runtime-go/internal/contracts"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	"analytix.local/runtime-go/internal/server"
)

// This fixture explicitly withholds the native capability. It exercises the
// actual fixed-final/longitudinal/failure-record chain, not native acceptance.
// The delayed branch characterizes a secondary failure after an authentic
// committed winner; its intentional wait is not performance evidence.
func TestRuntimeSourceUnavailableAppendDeadline(t *testing.T) {
	for _, delayed := range []bool{false, true} {
		name := "natural_append"
		if delayed {
			name = "expired_after_fixed_winner"
		}
		t.Run(name, func(t *testing.T) {
			config, workspace := newAsyncCaseClosureConfigV1(t)
			var providerCalls atomic.Int64
			provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				providerCalls.Add(1)
				w.WriteHeader(http.StatusServiceUnavailable)
			}))
			defer provider.Close()
			config.BaseURL = provider.URL + "/v1"
			seedProviderRegistryExecutionAuthorityV1(t, config.DataDir, config.ProviderID, config.BaseURL, []string{config.Model}, config.Model, "test-only")
			dependencies := defaultBundledFundsHostValidationDependenciesV1()
			dependencies.inspectPackage = func(context.Context) (packagedauthorityfs.InspectionV2, error) {
				return packagedauthorityfs.InspectionV2{}, packagedauthorityfs.ErrNotPackagedRuntimeV2
			}
			dependencies.openNativeOwnerForTest = func(string) (*nativecomponenthost.Owner, error) {
				return nil, nativecomponenthost.ErrUnavailable
			}
			entered, release := make(chan struct{}), make(chan struct{})
			var releaseOnce sync.Once
			releaseAppend := func() { releaseOnce.Do(func() { close(release) }) }
			defer releaseAppend()
			type timedObservation struct {
				value server.AsyncTurnObservationV1
				at    time.Time
			}
			observations := make(chan timedObservation, 8)
			var fixedAt, appendAt time.Time
			ctx := context.WithValue(context.Background(), bundledFundsHostValidationContextKeyV1{}, dependencies)
			ctx = context.WithValue(ctx, asyncTurnObservationContextKeyV1{}, func(o server.AsyncTurnObservationV1) {
				observations <- timedObservation{value: o, at: time.Now()}
			})
			ctx = context.WithValue(ctx, asyncTurnPhaseObservationContextKeyV1{}, func(phase string) {
				switch phase {
				case "case_fixed_persist":
					fixedAt = time.Now()
				case "case_longitudinal_append":
					appendAt = time.Now()
					close(entered)
					<-release
				}
			})
			lease, err := AcquireRuntimePersistenceLease(config)
			if err != nil {
				t.Fatal(err)
			}
			inner, err := newRuntimeServerHandlerWithPersistenceLeaseContextE(ctx, config, lease)
			if err != nil {
				_ = lease.Close()
				t.Fatal(err)
			}
			handler := &ownedPersistenceLeaseHandler{Handler: inner, lease: lease}
			defer func() {
				releaseAppend()
				shutdownOwnedRuntimeHandler(t, handler)
			}()
			baseline := readAsyncCaseAuthorityLeavesV1(t, config, lease)
			thread := asyncCaseClosureJSONV1(t, handler, http.MethodPost, "/v1/threads", map[string]any{"workspace": workspace, "providerId": config.ProviderID, "model": config.Model}, http.StatusCreated)
			threadID := contracts.StringField(thread, "id")
			started := asyncCaseClosureJSONV1(t, handler, http.MethodPost, "/v1/threads/"+threadID+"/turns", map[string]any{
				"prompt": "请分析当前案件资金流向并生成资金报告。", "riskIntent": "case", "async": true,
				"maxModelSteps": 1, "approvalPolicy": "never",
			}, http.StatusAccepted)
			turnID := contracts.StringField(started, "turnId")
			for waiting := true; waiting; {
				select {
				case <-entered:
					waiting = false
				case timed := <-observations:
					o := timed.value
					if o.Stage == "finished" {
						t.Fatalf("append not reached: phase=%s completion=%s fallback=%s", o.CompletionPhase, o.CompletionErrorClass, o.FailureRecordErrorClass)
					}
				case <-time.After(20 * time.Second):
					t.Fatal("source-unavailable append was not entered")
				}
			}
			path := filepath.Join(config.ProductionDurableRoot, "threads", threadID, "thread.json")
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var raw map[string]any
			if json.Unmarshal(before, &raw) != nil {
				t.Fatal("invalid source-unavailable archive")
			}
			turn := raw["turns"].([]any)[0].(map[string]any)
			frozen, err := domainsecurity.ParseTurnSecurityContext(turn["securityContext"])
			if err != nil || domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(frozen) != nil || contracts.StringField(turn, "status") != "completed" {
				t.Fatal("append preceded the authentic fact-publication winner")
			}
			winner, err := domainevidence.ParseAcceptedFinalRecord(turn["acceptedFinal"])
			if err != nil || winner.Variant != domainevidence.SourceUnavailableAnswer || winner.TerminalReason != "source_unavailable" {
				t.Fatal("fixture did not commit the source-unavailable boundary")
			}
			// The natural branch does no CAS inventory reads while the shared
			// terminal budget runs. Bind its post-finish private record to this
			// already committed public winner instead.
			var authorityBefore map[string][]finalauthority.SecurePrivateCASFile
			if delayed {
				authorityBefore = readAsyncCaseAuthorityLeavesV1(t, config, lease)
				// Keep the production 15-second budget. Waiting from the fixed
				// phase also accounts for time already spent publishing it.
				if remaining := time.Until(fixedAt.Add(16 * time.Second)); remaining > 0 {
					timer := time.NewTimer(remaining)
					<-timer.C
				}
			}
			releasedAt := time.Now()
			releaseAppend()
			var finished server.AsyncTurnObservationV1
			var finishedAt time.Time
			for finished.Stage != "finished" {
				select {
				case timed := <-observations:
					finished, finishedAt = timed.value, timed.at
				case <-time.After(20 * time.Second):
					t.Fatal("source-unavailable append did not finish")
				}
			}
			wantCompletion, wantFallback := "none", "none"
			if delayed {
				wantCompletion, wantFallback = "deadline_exceeded", "unclassified"
			}
			if finished.ThreadID != threadID || finished.TurnID != turnID || finished.CompletionPhase != "case_longitudinal_append" || finished.CompletionErrorClass != wantCompletion || finished.FailureRecordErrorClass != wantFallback || finished.TerminalStatus != "completed" {
				t.Fatalf("phase=%s completion=%s fallback=%s status=%s", finished.CompletionPhase, finished.CompletionErrorClass, finished.FailureRecordErrorClass, finished.TerminalStatus)
			}
			after, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(before, after) {
				t.Fatal("append/fallback changed the already committed winner")
			}
			if delayed && !reflect.DeepEqual(authorityBefore, readAsyncCaseAuthorityLeavesV1(t, config, lease)) {
				t.Fatal("deadline fallback changed private terminal authority")
			}
			original := readAsyncCasePrivateFinalV1(t, config, lease, threadID, turnID)
			if original.AcceptedFinal.RecordDigest != winner.RecordDigest || original.AcceptedFinal.TerminalReason != "source_unavailable" {
				t.Fatal("private preparation differs from the authentic public winner")
			}
			assertAsyncCaseCommittedAuthorityV1(t, config, lease, original, baseline)
			if providerCalls.Load() != 0 {
				t.Fatal("source-unavailable fixture reached the Provider")
			}
			t.Logf("fixed_to_append_ms=%d barrier_ms=%d released_to_finished_ms=%d deliberate_delay=%t completion=%s fallback=%s provider_calls=0", appendAt.Sub(fixedAt).Milliseconds(), releasedAt.Sub(appendAt).Milliseconds(), finishedAt.Sub(releasedAt).Milliseconds(), delayed, finished.CompletionErrorClass, finished.FailureRecordErrorClass)
		})
	}
}
