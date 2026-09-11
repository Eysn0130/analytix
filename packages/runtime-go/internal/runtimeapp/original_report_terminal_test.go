//go:build darwin || linux

package runtimeapp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	publicationapp "analytix.local/runtime-go/internal/app/reportpublication"
)

func TestRuntimeOriginalReportHistoryVerifiesPersistedTerminals(t *testing.T) {
	for _, rejected := range []bool{false, true} {
		name := "projected"
		if rejected {
			name = "rejected"
		}
		t.Run(name, func(t *testing.T) {
			for _, fault := range []string{"complete", "missing-stored-witness", "missing-primary-result"} {
				t.Run(fault, func(t *testing.T) {
					ctx := context.Background()
					fixture := newRuntimeOriginalCompletedReportHistoryFixtureV1(t, rejected)
					history, err := prepareRuntimeOriginalReportHistoryV1(ctx, fixture.core, fixture.publication, fixture.advance, fixture.scope)
					if err != nil {
						t.Fatal(err)
					}
					expected := publicationapp.RestartAttemptDeliveryProjectionV1
					if rejected {
						expected = publicationapp.RestartAttemptDeliveryRejectionV1
					}
					if history == nil || len(history.plan.Attempts) != 1 || history.plan.Attempts[0].State != expected {
						t.Fatal("persisted terminal was not verified")
					}
					switch fault {
					case "missing-stored-witness":
						digest := history.plan.Attempts[0].Selection.ObservationDigest
						if err := os.Remove(filepath.Join(fixture.core.roots.DataDir, "private", "evidence-authority", "observations", digest[:2], digest+".json")); err != nil {
							t.Fatal(err)
						}
					case "missing-primary-result":
						body, err := os.ReadFile(fixture.primary)
						if err != nil {
							t.Fatal(err)
						}
						var primary map[string]any
						if err := json.Unmarshal(body, &primary); err != nil {
							t.Fatal(err)
						}
						removeRuntimeOriginalReportResultForTestV1(t, primary, history.plan.Attempts[0].GrantSettlement.ResultItemID)
						body, err = json.Marshal(primary)
						if err != nil {
							t.Fatal(err)
						}
						if err := os.WriteFile(fixture.primary, body, 0600); err != nil {
							t.Fatal(err)
						}
					}
					before := startupWholeTreeRecordMapForTest(t, fixture.core.roots.DataDir, fixture.core.roots.DurableDir)
					history = nil
					err = fixture.refreshE()
					if err == nil {
						history, err = prepareRuntimeOriginalReportHistoryV1(ctx, fixture.core, fixture.publication, fixture.advance, fixture.scope)
					}
					if fault == "complete" {
						if err != nil || history == nil {
							t.Fatalf("complete terminal failed: %v", err)
						}
						if err := history.revalidate(ctx); err != nil {
							t.Fatal(err)
						}
					} else if err == nil || history != nil {
						t.Fatalf("%s returned historical authority", fault)
					}
					if !reflect.DeepEqual(before, startupWholeTreeRecordMapForTest(t, fixture.core.roots.DataDir, fixture.core.roots.DurableDir)) {
						t.Fatal("terminal history observation changed managed files")
					}
				})
			}
		})
	}
}

func removeRuntimeOriginalReportResultForTestV1(t *testing.T, primary map[string]any, resultID string) {
	t.Helper()
	removed := 0
	for _, rawTurn := range primary["turns"].([]any) {
		turn := rawTurn.(map[string]any)
		items := []any{}
		for _, rawItem := range turn["items"].([]any) {
			item := rawItem.(map[string]any)
			if item["id"] == resultID && item["kind"] == "tool_result" {
				removed++
				continue
			}
			items = append(items, rawItem)
		}
		turn["items"] = items
	}
	if removed != 1 {
		t.Fatalf("expected one exact report result, removed %d", removed)
	}
}

func TestRuntimeOriginalProjectedTerminalAllowsRuntimeActivation(t *testing.T) {
	testRuntimeOriginalCompletedTerminalActivationV1(t, false)
}

func TestRuntimeOriginalRejectedTerminalAllowsRuntimeActivation(t *testing.T) {
	testRuntimeOriginalCompletedTerminalActivationV1(t, true)
}

func testRuntimeOriginalCompletedTerminalActivationV1(t *testing.T, rejected bool) {
	t.Helper()
	fixture := newRuntimeOriginalCompletedReportHistoryFixtureV1(t, rejected)
	before := startupWholeTreeRecordMapForTest(t, filepath.Join(fixture.core.roots.DataDir, "private", "report-publication"))
	for restart := 0; restart < 2; restart++ {
		handler, err := NewRuntimeServerHandlerE(fixture.config)
		if err != nil {
			t.Fatalf("verified historical terminal blocked runtime activation %d: %v", restart, err)
		}
		shutdownOwnedRuntimeHandler(t, handler)
		after := startupWholeTreeRecordMapForTest(t, filepath.Join(fixture.core.roots.DataDir, "private", "report-publication"))
		if !reflect.DeepEqual(before, after) {
			t.Fatal("runtime activation changed immutable report history")
		}
	}
}
