//go:build darwin || linux

package runtimeapp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	casestore "analytix.local/runtime-go/internal/adapters/outbound/casethreadauthority"
	finaladapter "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	pendingstore "analytix.local/runtime-go/internal/adapters/outbound/pendingworkstore"
	casethreadapp "analytix.local/runtime-go/internal/app/casethread"
	contextepochapp "analytix.local/runtime-go/internal/app/contextepoch"
	pendingapp "analytix.local/runtime-go/internal/app/pendingwork"
	threadapp "analytix.local/runtime-go/internal/app/thread"
	turnapp "analytix.local/runtime-go/internal/app/turn"
	domaincontextepoch "analytix.local/runtime-go/internal/domain/contextepoch"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	authorityport "analytix.local/runtime-go/internal/ports/finalauthority"
	"analytix.local/runtime-go/internal/server"
	privatecastest "analytix.local/runtime-go/internal/testsupport/privatecas"
	securitytest "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

// Counting retains the real installation signer; it only observes whether a
// cancelled derivation crossed the signing boundary.
type runtimeActiveHistoryCountingAuthorityV1 struct {
	authorityport.Authority
	signs int
}

func (authority *runtimeActiveHistoryCountingAuthorityV1) Sign(ctx context.Context, body []byte) ([]byte, error) {
	authority.signs++
	return authority.Authority.Sign(ctx, body)
}

type runtimeActiveHistoryFixtureV1 struct {
	dataDir, durableRoot string
	authority            *runtimeActiveHistoryCountingAuthorityV1
	caseStore            *casestore.Store
	registry             *casethreadapp.Registry
	durable              *server.DurableEventSessionStore
	primary              *finaladapter.AcceptedFinalCASReader
	projector            *threadapp.TrustedPublicProjector
	identities           func(context.Context, string, []string) error
	source, target       map[string]any
	record               domainsecurity.CaseThreadAuthorityRecord
}

func newRuntimeActiveHistoryFixtureV1(t *testing.T, derivation string, cutoff bool) *runtimeActiveHistoryFixtureV1 {
	t.Helper()
	ctx := context.Background()
	root := t.TempDir()
	fixture := &runtimeActiveHistoryFixtureV1{dataDir: filepath.Join(root, "data"), durableRoot: filepath.Join(root, "sessions")}
	privateRoot := filepath.Join(fixture.dataDir, "private")
	if err := os.MkdirAll(privateRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	key, err := finaladapter.OpenOrCreateFileAuthority(filepath.Join(privateRoot, "authority", "final-answer-ed25519-v1.json"), false)
	if err != nil {
		t.Fatal(err)
	}
	fixture.authority = &runtimeActiveHistoryCountingAuthorityV1{Authority: key}
	access, err := privatecastest.NewAccessAuthority(privateRoot)
	if err != nil {
		t.Fatal(err)
	}
	fixture.caseStore, err = casestore.NewStore(filepath.Join(privateRoot, "case-thread-authority"), access)
	if err != nil {
		t.Fatal(err)
	}
	pending, err := pendingstore.NewStore(filepath.Join(privateRoot, "pending-work"), access)
	if err != nil {
		t.Fatal(err)
	}
	finals, err := finaladapter.NewPrivateStore(filepath.Join(privateRoot, "accepted-finals"), access)
	if err != nil {
		t.Fatal(err)
	}
	fixture.identities = runtimeActiveHistoryIdentityValidatorV1(fixture.dataDir, pendingapp.NewService(fixture.authority, pending, nil), finals, fixture.authority)
	fixture.registry, err = casethreadapp.NewRegistry(ctx, fixture.authority, fixture.caseStore)
	if err != nil {
		t.Fatal(err)
	}
	fixture.registry.SetActiveInheritedHistoryIdentityValidatorV1(fixture.identities)
	fixture.durable, err = server.NewProductionDurableEventSessionStore(fixture.durableRoot)
	if err != nil {
		t.Fatal(err)
	}
	fixture.durable.SetCaseThreadAuthority(fixture.registry)
	fixture.primary, err = finaladapter.NewAcceptedFinalCASReader(fixture.durableRoot)
	if err != nil {
		t.Fatal(err)
	}
	if err := fixture.durable.BindPrimaryThreadReaderV1(fixture.primary); err != nil {
		t.Fatal(err)
	}
	fixture.projector = threadapp.NewTrustedPublicProjectorWithPrimaryCAS(nil, fixture.registry, nil, fixture.primary)
	fixture.durable.SetActiveHistorySourceAdmissionV1(func(source map[string]any) error {
		_, err := fixture.projector.ProjectThread(source)
		return err
	})
	workspace := t.TempDir()
	created, err := fixture.durable.CreateThread(map[string]any{"title": "active history source"}, workspace)
	if err != nil {
		t.Fatal(err)
	}
	sourceID := created["id"].(string)
	for index := 0; index < 2; index++ {
		at := time.Date(2026, 9, 9, 1, 0, index, 0, time.UTC)
		turnID := fmt.Sprintf("turn_active_source_%d", index)
		frozen, err := securitytest.CaseExecutionContextV2(domainsecurity.TurnSecurityContextInput{
			ThreadID: sourceID, TurnID: turnID, WorkspaceRealPath: workspace, ContextEpoch: uint64(index + 1), IssuedAt: at,
		})
		if err != nil {
			t.Fatal(err)
		}
		state, err := contextepochapp.BootstrapState(sourceID, frozen.ContextEpoch, []domaincontextepoch.SourceEntry{contextepochapp.SecurityBindingEntry(frozen)}, at)
		if err != nil {
			t.Fatal(err)
		}
		if err := casethreadapp.RegisterRequired(ctx, fixture.registry, frozen); err != nil {
			t.Fatal(err)
		}
		stamp := at.Format(time.RFC3339Nano)
		turn := map[string]any{
			"id": turnID, "threadId": sourceID, "status": "running", "createdAt": stamp, "startedAt": stamp,
			"securityContext": frozen, "contextEpochSnapshot": contextepochapp.PublicSnapshot(state.AcceptedSnapshot),
			"items": []any{map[string]any{
				"id": fmt.Sprintf("user_active_%d", index), "threadId": sourceID, "turnId": turnID,
				"kind": "user_message", "role": "user", "status": "completed", "createdAt": stamp, "finishedAt": stamp,
				"text": fmt.Sprintf("PRIVATE_INHERITED_CANARY_%d", index),
			}},
		}
		if err := fixture.durable.AppendTurnToThread(sourceID, turn, "synthetic-provider", map[string]any{
			"securityState": frozen, "contextEpochState": contextepochapp.PublicState(state),
		}); err != nil {
			t.Fatal(err)
		}
		if err := casethreadapp.CommitRequired(ctx, fixture.registry, frozen, state, at); err != nil {
			t.Fatal(err)
		}
	}
	source, err := fixture.primary.ReadPrimaryThreadSnapshotV1(ctx, sourceID)
	if err != nil {
		t.Fatal(err)
	}
	fixture.source = source.Thread
	var targetID string
	if derivation == "resume" {
		result, err := fixture.durable.ResumeSession(sourceID, map[string]any{})
		if err != nil {
			t.Fatalf("real resume producer: %v", err)
		}
		targetID = result["thread_id"].(string)
	} else {
		request := map[string]any{}
		if cutoff {
			request["turnId"] = "turn_active_source_0"
		}
		result, err := fixture.durable.ForkThread(sourceID, request)
		if err != nil {
			t.Fatalf("real fork producer: %v", err)
		}
		targetID = result["id"].(string)
	}
	target, err := fixture.primary.ReadPrimaryThreadSnapshotV1(ctx, targetID)
	if err != nil {
		t.Fatal(err)
	}
	fixture.target = target.Thread
	var found bool
	fixture.record, found, err = fixture.registry.ActiveInheritedHistoryRecordV1(ctx, targetID)
	if err != nil || !found || fixture.record.ActiveInheritedHistory.SourcePrimarySHA256 != source.ThreadFileSHA256 {
		t.Fatalf("producer did not bind exact source primary: found=%v err=%v", found, err)
	}
	return fixture
}

func (fixture *runtimeActiveHistoryFixtureV1) targetPath() string {
	return filepath.Join(fixture.durableRoot, "threads", fixture.target["id"].(string), "thread.json")
}

func (fixture *runtimeActiveHistoryFixtureV1) recordPath() string {
	id := fixture.record.RecordDigest
	return filepath.Join(fixture.dataDir, "private", "case-thread-authority", id[:2], id+".json")
}

func runtimeActiveHistoryJSONV1(t *testing.T, value any) []byte {
	t.Helper()
	body, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func runtimeActiveHistoryCloneV1(t *testing.T, value map[string]any) map[string]any {
	t.Helper()
	var out map[string]any
	decoder := json.NewDecoder(bytes.NewReader(runtimeActiveHistoryJSONV1(t, value)))
	decoder.UseNumber()
	if err := decoder.Decode(&out); err != nil {
		t.Fatal(err)
	}
	return out
}

func runtimeActiveHistoryCompactionFixtureV1(t *testing.T) (*runtimeActiveHistoryFixtureV1, threadapp.PreparedCompaction) {
	t.Helper()
	fixture := newRuntimeActiveHistoryFixtureV1(t, "resume", false)
	ctx := context.Background()
	threadID := fixture.target["id"].(string)
	at := time.Now().UTC()
	var current domainsecurity.TurnSecurityContext
	for index := 0; index < 2; index++ {
		issued := at.Add(time.Duration(index) * time.Second)
		var err error
		current, err = securitytest.CaseExecutionContextV2(domainsecurity.TurnSecurityContextInput{
			ThreadID: threadID, TurnID: fmt.Sprintf("turn_active_target_%d", index),
			WorkspaceRealPath: fixture.target["workspace"].(string), ContextEpoch: uint64(index + 1), IssuedAt: issued,
		})
		if err != nil {
			t.Fatal(err)
		}
		state, err := contextepochapp.BootstrapState(threadID, current.ContextEpoch,
			[]domaincontextepoch.SourceEntry{contextepochapp.SecurityBindingEntry(current)}, issued)
		if err != nil {
			t.Fatal(err)
		}
		if err := casethreadapp.RegisterRequired(ctx, fixture.registry, current); err != nil {
			t.Fatal(err)
		}
		stamp := issued.Format(time.RFC3339Nano)
		turn := map[string]any{
			"id": current.TurnID, "threadId": threadID, "status": "completed", "createdAt": stamp, "finishedAt": stamp,
			"securityContext": current, "contextEpochSnapshot": contextepochapp.PublicSnapshot(state.AcceptedSnapshot),
			"items": []any{map[string]any{
				"id": fmt.Sprintf("user_active_target_%d", index), "threadId": threadID, "turnId": current.TurnID,
				"kind": "user_message", "role": "user", "status": "completed", "createdAt": stamp, "finishedAt": stamp,
				"text": "Independent target request for real compaction preparation.",
			}},
		}
		if err := fixture.durable.AppendTurnToThread(threadID, turn, "synthetic-provider", map[string]any{
			"securityState": current, "contextEpochState": contextepochapp.PublicState(state),
		}); err != nil {
			t.Fatal(err)
		}
		if err := casethreadapp.CommitRequired(ctx, fixture.registry, current, state, issued); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := fixture.durable.PatchThread(threadID, map[string]any{"status": "idle"}); err != nil {
		t.Fatal(err)
	}
	snapshot, err := fixture.primary.ReadPrimaryThreadSnapshotV1(ctx, threadID)
	if err != nil {
		t.Fatal(err)
	}
	fixture.target = snapshot.Thread
	inherited, err := threadapp.ActiveInheritedCompactionRecordV1(ctx, fixture.target, fixture.registry, fixture.primary)
	if err != nil || inherited == nil || inherited.RecordDigest != fixture.record.RecordDigest {
		t.Fatalf("compaction lacks the exact real inherited record: %v", err)
	}
	continuation, err := turnapp.BuildTaskContinuationSnapshotV1(fixture.target)
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := threadapp.PrepareCaseCompaction(fixture.target, threadID, "manual", at.Add(time.Minute), false,
		threadapp.CaseCompactionAuthorization{
			Continuation: continuation, SourceContextDigest: current.ContextDigest,
			AuthorityTurnIDs: casethreadapp.CommittedTurnIDs(fixture.registry, threadID), ActiveInheritedHistory: inherited,
		})
	if err != nil || prepared.Result.ReplacedTokens <= 0 {
		t.Fatalf("real active compaction was not prepared: %v", err)
	}
	return fixture, prepared
}

func runtimeActiveHistoryAssertCompactionV1(t *testing.T, fixture *runtimeActiveHistoryFixtureV1, prepared threadapp.PreparedCompaction, actual map[string]any) {
	t.Helper()
	if !bytes.Equal(runtimeActiveHistoryJSONV1(t, actual["activeInheritedHistoryReceipt"]), runtimeActiveHistoryJSONV1(t, fixture.target["activeInheritedHistoryReceipt"])) {
		t.Fatal("compaction changed the original inherited receipt")
	}
	turns := actual["turns"].([]any)
	for index, inherited := range fixture.record.ActiveInheritedHistory.Turns {
		turn := turns[index].(map[string]any)
		if domainsecurity.SHA256Hex(runtimeActiveHistoryJSONV1(t, turn)) != inherited.ContentSHA256 ||
			!bytes.Equal(runtimeActiveHistoryJSONV1(t, turn), runtimeActiveHistoryJSONV1(t, fixture.target["turns"].([]any)[index])) {
			t.Fatal("compaction changed private inherited prefix bytes")
		}
		if turn["securityContext"] != nil || turn["caseHistoryProjection"] != nil {
			t.Fatal("compaction promoted inherited history into execution authority")
		}
	}
	marker := turns[len(turns)-1].(map[string]any)
	if marker["id"] != prepared.Result.TurnID || marker["threadId"] != fixture.target["id"] || marker["caseHistoryProjection"] != "compaction_authority_v1" {
		t.Fatal("compaction marker is not independent target authority")
	}
	want, err := threadapp.CompactionBaselineDigest(prepared.Thread)
	if err != nil {
		t.Fatal(err)
	}
	got, err := threadapp.CompactionBaselineDigest(actual)
	if err != nil || got != want {
		t.Fatalf("compaction differs from exact prepared target: %v", err)
	}
}

func TestRuntimeActiveHistoryCompactionDurableCommitAndCASRecovery(t *testing.T) {
	for _, mode := range []string{"commit", "cas_first_recovery", "recovery_missing_cas", "recovery_missing_receipt"} {
		t.Run(mode, func(t *testing.T) {
			fixture, prepared := runtimeActiveHistoryCompactionFixtureV1(t)
			ctx := context.Background()
			threadID := fixture.target["id"].(string)
			if mode == "commit" {
				result, err := fixture.durable.CommitCompaction(prepared.CommitRequest())
				if err != nil || !result.Committed || !result.AuthorityCommitted {
					t.Fatalf("real durable compaction did not commit: %v", err)
				}
			} else {
				if err := casethreadapp.RegisterRequired(ctx, fixture.registry, prepared.SecurityContext); err != nil {
					t.Fatal(err)
				}
				if err := casethreadapp.CommitRequired(ctx, fixture.registry, prepared.SecurityContext, prepared.EpochState, time.Now().UTC()); err != nil {
					t.Fatal(err)
				}
				if mode == "recovery_missing_cas" {
					if err := os.Remove(fixture.recordPath()); err != nil {
						t.Fatal(err)
					}
					// Do not leave an empty shard as a second independent fault.
					shard := filepath.Dir(fixture.recordPath())
					entries, err := os.ReadDir(shard)
					if err != nil {
						t.Fatal(err)
					}
					if len(entries) == 0 {
						if err := os.Remove(shard); err != nil {
							t.Fatal(err)
						}
					}
				}
				if mode == "recovery_missing_receipt" {
					altered := runtimeActiveHistoryCloneV1(t, fixture.target)
					delete(altered, "activeInheritedHistoryReceipt")
					if err := os.WriteFile(fixture.targetPath(), runtimeActiveHistoryJSONV1(t, altered), 0o600); err != nil {
						t.Fatal(err)
					}
				}
				before, err := os.ReadFile(fixture.targetPath())
				if err != nil {
					t.Fatal(err)
				}
				if mode == "cas_first_recovery" && !bytes.Equal(before, runtimeActiveHistoryJSONV1(t, fixture.target)) {
					t.Fatal("CAS-first crash cut wrote the primary target")
				}
				signsBeforeRestart := fixture.authority.signs
				registry, err := casethreadapp.NewRegistry(ctx, fixture.authority, fixture.caseStore)
				if err != nil {
					if mode == "recovery_missing_cas" && errors.Is(err, os.ErrNotExist) {
						// The physical CAS inventory rejects this cut before
						// semantic recovery; it must not reconstruct the lineage.
						after, readErr := os.ReadFile(fixture.targetPath())
						_, recordErr := os.Lstat(fixture.recordPath())
						if readErr != nil || !bytes.Equal(before, after) || !os.IsNotExist(recordErr) || fixture.authority.signs != signsBeforeRestart {
							t.Fatal("missing CAS startup rejection reconstructed authority or changed primary")
						}
						return
					}
					t.Fatal(err)
				}
				registry.SetActiveInheritedHistoryIdentityValidatorV1(fixture.identities)
				durable, err := server.NewProductionDurableEventSessionStore(fixture.durableRoot)
				if err != nil {
					t.Fatal(err)
				}
				if err := durable.BindPrimaryThreadReaderV1(fixture.primary); err != nil {
					t.Fatal(err)
				}
				durable.SetCaseThreadAuthority(registry)
				inventory, err := fixture.caseStore.List(ctx)
				if err != nil {
					t.Fatal(err)
				}
				signs := fixture.authority.signs
				if err := threadapp.RecoverCommittedCaseCompactions(ctx, registry, durable); err != nil {
					t.Fatal(err)
				}
				afterInventory, err := fixture.caseStore.List(ctx)
				if err != nil || fixture.authority.signs != signs || !bytes.Equal(runtimeActiveHistoryJSONV1(t, inventory), runtimeActiveHistoryJSONV1(t, afterInventory)) {
					t.Fatal("compaction recovery signed or reconstructed private authority")
				}
				if strings.HasPrefix(mode, "recovery_missing_") {
					after, err := os.ReadFile(fixture.targetPath())
					if err != nil || !bytes.Equal(before, after) || registry.CanExecute(threadID) {
						t.Fatal("missing inherited proof recovery changed primary or admitted target")
					}
					return
				}
				if !registry.CanExecute(threadID) {
					t.Fatal("exact CAS-first compaction replay quarantined the target")
				}
			}
			actual, err := fixture.primary.ReadPrimaryThreadSnapshotV1(ctx, threadID)
			if err != nil {
				t.Fatal(err)
			}
			runtimeActiveHistoryAssertCompactionV1(t, fixture, prepared, actual.Thread)
			turns := actual.Thread["turns"].([]any)
			marker := turns[len(turns)-1].(map[string]any)
			validator := threadapp.NewCaseThreadAuthorityReader(fixture.durable, fixture.registry).(interface {
				ValidateCaseCompactionAuthorityTurnV1(string, map[string]any, map[string]any) error
			})
			if err := validator.ValidateCaseCompactionAuthorityTurnV1(threadID, actual.Thread, marker); err != nil {
				t.Fatalf("real final-authority compaction reader rejected authenticated prefix: %v", err)
			}
			if !threadapp.TrustedCaseCompactionTurnIDsV1(actual.Thread, fixture.registry, fixture.primary)[prepared.Result.TurnID] {
				t.Fatal("provider-history consumer lost authenticated compaction cut")
			}
			if len(threadapp.TrustedCaseCompactionTurnIDsV1(actual.Thread, fixture.registry)) != 0 {
				t.Fatal("provider-history consumer trusted active compaction without strict primary")
			}
			if turnapp.ValidateCaseCompactionAuthorityTurnV1(threadID, actual.Thread, marker, prepared.EpochState) == nil {
				t.Fatal("bare compaction inventory admitted contextless history without provenance")
			}
			altered := runtimeActiveHistoryCloneV1(t, actual.Thread)
			altered["turns"].([]any)[0].(map[string]any)["createdAt"] = "2000-01-01T00:00:00Z"
			if validator.ValidateCaseCompactionAuthorityTurnV1(threadID, altered, marker) == nil || len(threadapp.TrustedCaseCompactionTurnIDsV1(altered, fixture.registry, fixture.primary)) != 0 {
				t.Fatal("compaction consumer admitted changed inherited prefix")
			}
		})
	}
}

func TestRuntimeActiveHistoryCompactionRejectsMissingOrForeignProofBeforeSigning(t *testing.T) {
	for _, mode := range []string{"omitted", "foreign"} {
		t.Run(mode, func(t *testing.T) {
			fixture, prepared := runtimeActiveHistoryCompactionFixtureV1(t)
			request := prepared.CommitRequest()
			request.ActiveInheritedHistory = nil
			if mode == "foreign" {
				foreign := newRuntimeActiveHistoryFixtureV1(t, "fork", true)
				request.ActiveInheritedHistory = &foreign.record
			}
			before, err := os.ReadFile(fixture.targetPath())
			if err != nil {
				t.Fatal(err)
			}
			inventory, err := fixture.caseStore.List(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			signs := fixture.authority.signs
			result, err := fixture.durable.CommitCompaction(request)
			if !errors.Is(err, threadapp.ErrCompactionBaselineConflict) || result.Committed || result.AuthorityCommitted {
				t.Fatalf("missing or foreign compaction proof did not fail at exact provenance comparison: %v", err)
			}
			after, err := os.ReadFile(fixture.targetPath())
			if err != nil || !bytes.Equal(before, after) {
				t.Fatal("rejected compaction changed primary bytes")
			}
			afterInventory, err := fixture.caseStore.List(context.Background())
			if err != nil || fixture.authority.signs != signs || !bytes.Equal(runtimeActiveHistoryJSONV1(t, inventory), runtimeActiveHistoryJSONV1(t, afterInventory)) {
				t.Fatal("rejected compaction signed or changed private CAS")
			}
		})
	}
}

func TestRuntimeActiveHistoryExactPrefixAndFreshRegistry(t *testing.T) {
	for _, mode := range []string{"resume", "fork", "fork_cutoff"} {
		t.Run(mode, func(t *testing.T) {
			derivation := mode
			if mode == "fork_cutoff" {
				derivation = "fork"
			}
			fixture := newRuntimeActiveHistoryFixtureV1(t, derivation, mode == "fork_cutoff")
			want := 2
			if mode == "fork_cutoff" {
				want = 1
			}
			for _, fresh := range []bool{false, true} {
				registry := fixture.registry
				if fresh {
					var err error
					registry, err = casethreadapp.NewRegistry(context.Background(), fixture.authority, fixture.caseStore)
					if err != nil {
						t.Fatal(err)
					}
					registry.SetActiveInheritedHistoryIdentityValidatorV1(fixture.identities)
				}
				ids, err := threadapp.ValidateActiveInheritedHistoryV1(context.Background(), fixture.target, registry, fixture.primary)
				if err != nil || len(ids) != want {
					t.Fatalf("exact prefix fresh=%v ids=%d err=%v", fresh, len(ids), err)
				}
				projector := threadapp.NewTrustedPublicProjectorWithPrimaryCAS(nil, registry, nil, fixture.primary)
				public, err := projector.ProjectThread(fixture.target)
				if err != nil {
					t.Fatal(err)
				}
				body := runtimeActiveHistoryJSONV1(t, public)
				if bytes.Contains(body, []byte("PRIVATE_INHERITED_CANARY")) || bytes.Contains(body, []byte("activeInheritedHistoryReceipt")) {
					t.Fatal("inert inherited history exposed private content or receipt")
				}
			}
		})
	}
}

func TestRuntimeActiveHistoryRejectsPrimaryAndViewTampering(t *testing.T) {
	mutations := map[string]func(map[string]any){
		"user_content": func(target map[string]any) {
			target["turns"].([]any)[0].(map[string]any)["items"].([]any)[0].(map[string]any)["text"] = "changed"
		},
		"turn_metadata": func(target map[string]any) {
			target["turns"].([]any)[0].(map[string]any)["finishedAt"] = "2026-09-09T02:00:00Z"
		},
		"delete": func(target map[string]any) { target["turns"] = target["turns"].([]any)[1:] },
		"insert": func(target map[string]any) {
			target["turns"] = append([]any{map[string]any{"id": "turn_inserted", "threadId": target["id"], "status": "completed", "items": []any{}}}, target["turns"].([]any)...)
		},
		"duplicate": func(target map[string]any) {
			turns := target["turns"].([]any)
			target["turns"] = append(turns, turns[0])
		},
		"reorder":          func(target map[string]any) { turns := target["turns"].([]any); turns[0], turns[1] = turns[1], turns[0] },
		"foreign_receipt":  func(target map[string]any) { target["activeInheritedHistoryReceipt"] = strings.Repeat("a", 64) },
		"missing_receipt":  func(target map[string]any) { delete(target, "activeInheritedHistoryReceipt") },
		"foreign_source":   func(target map[string]any) { target["forkedFromThreadId"] = "thread_foreign" },
		"foreign_target":   func(target map[string]any) { target["id"] = "thread_foreign" },
		"foreign_relation": func(target map[string]any) { target["relation"] = "side" },
		"created_at":       func(target map[string]any) { target["createdAt"] = "2026-09-09T02:00:00Z" },
		"forked_at":        func(target map[string]any) { target["forkedAt"] = "2026-09-09T02:00:00Z" },
		"execution_authority": func(target map[string]any) {
			target["turns"].([]any)[0].(map[string]any)["securityContext"] = map[string]any{"version": 2}
		},
		"accepted_final": func(target map[string]any) {
			target["turns"].([]any)[0].(map[string]any)["acceptedFinal"] = map[string]any{"recordDigest": strings.Repeat("b", 64)}
		},
		"accepted_final_view": func(target map[string]any) {
			target["turns"].([]any)[0].(map[string]any)["acceptedFinalView"] = map[string]any{}
		},
		"compaction_authority": func(target map[string]any) {
			target["turns"].([]any)[0].(map[string]any)["caseHistoryProjection"] = "compaction_authority_v1"
		},
	}
	for name, mutate := range mutations {
		for _, physical := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/primary=%v", name, physical), func(t *testing.T) {
				fixture := newRuntimeActiveHistoryFixtureV1(t, "resume", false)
				view := runtimeActiveHistoryCloneV1(t, fixture.target)
				mutate(view)
				if physical {
					if err := os.WriteFile(fixture.targetPath(), runtimeActiveHistoryJSONV1(t, view), 0o600); err != nil {
						t.Fatal(err)
					}
					// A stale in-memory rendering must not hide changed primary bytes.
					view = fixture.target
				}
				if ids, err := threadapp.ValidateActiveInheritedHistoryV1(context.Background(), view, fixture.registry, fixture.primary); err == nil || ids != nil {
					t.Fatal("tampered inherited history received identity admission")
				}
				if public, err := fixture.projector.ProjectThread(view); err == nil || public != nil {
					t.Fatal("tampered inherited history received public GET projection")
				}
			})
		}
	}
}

func TestRuntimeActiveHistoryRejectsDurableProofTamperingAndPartialCommit(t *testing.T) {
	for _, mode := range []string{"missing_cas", "corrupt_cas", "cas_only", "corrupt_primary", "source_digest", "source_id", "target_id", "relation", "cutoff", "turn_digest"} {
		t.Run(mode, func(t *testing.T) {
			fixture := newRuntimeActiveHistoryFixtureV1(t, "resume", false)
			switch mode {
			case "missing_cas":
				if err := os.Remove(fixture.recordPath()); err != nil {
					t.Fatal(err)
				}
			case "corrupt_cas":
				if err := os.WriteFile(fixture.recordPath(), []byte("{"), 0o600); err != nil {
					t.Fatal(err)
				}
			case "cas_only":
				if err := os.Remove(fixture.targetPath()); err != nil {
					t.Fatal(err)
				}
			case "corrupt_primary":
				if err := os.WriteFile(fixture.targetPath(), []byte("{"), 0o600); err != nil {
					t.Fatal(err)
				}
			default:
				var record map[string]any
				if err := json.Unmarshal(runtimeActiveHistoryJSONV1(t, fixture.record), &record); err != nil {
					t.Fatal(err)
				}
				binding := record["activeInheritedHistory"].(map[string]any)
				switch mode {
				case "source_digest":
					binding["sourcePrimarySha256"] = strings.Repeat("c", 64)
				case "source_id":
					binding["sourceThreadId"] = "thread_foreign"
				case "target_id":
					binding["targetThreadId"] = "thread_foreign"
				case "relation":
					binding["targetRelation"] = "side"
				case "cutoff":
					binding["cutoffTurnId"] = "turn_active_source_0"
				case "turn_digest":
					binding["turns"].([]any)[0].(map[string]any)["contentSha256"] = strings.Repeat("d", 64)
				}
				if err := os.WriteFile(fixture.recordPath(), runtimeActiveHistoryJSONV1(t, record), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if public, err := fixture.projector.ProjectThread(fixture.target); err == nil || public != nil {
				t.Fatal("partial or corrupt durable authority received live projection")
			}
			registry, err := casethreadapp.NewRegistry(context.Background(), fixture.authority, fixture.caseStore)
			if err != nil {
				return
			} // Strict startup rejection is the expected fail-closed result.
			registry.SetActiveInheritedHistoryIdentityValidatorV1(fixture.identities)
			projector := threadapp.NewTrustedPublicProjectorWithPrimaryCAS(nil, registry, nil, fixture.primary)
			if public, err := projector.ProjectThread(fixture.target); err == nil || public != nil {
				t.Fatal("fresh registry accepted partial or corrupt durable authority")
			}
		})
	}
}

func TestRuntimeActiveHistoryCancelledDerivationDoesNotSignOrCreate(t *testing.T) {
	fixture := newRuntimeActiveHistoryFixtureV1(t, "resume", false)
	binding := *domainsecurity.CloneActiveInheritedHistoryBindingV1(fixture.record.ActiveInheritedHistory)
	binding.TargetThreadID = "thread_cancelled_derivation"
	before, err := fixture.caseStore.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	signs := fixture.authority.signs
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := fixture.registry.DeriveWithInheritedHistoryV1(ctx, binding); err == nil {
		t.Fatal("cancelled derivation succeeded")
	}
	after, err := fixture.caseStore.List(context.Background())
	if err != nil || fixture.authority.signs != signs || !bytes.Equal(runtimeActiveHistoryJSONV1(t, before), runtimeActiveHistoryJSONV1(t, after)) {
		t.Fatalf("cancelled derivation signed or changed the durable inventory: %v", err)
	}
	if thread, _ := fixture.durable.GetThread(binding.TargetThreadID); thread != nil {
		t.Fatal("cancelled derivation left a GET-visible thread")
	}
}

func TestRuntimeActiveHistoryRejectsUnsignedAppendBeforeFirstFinal(t *testing.T) {
	for _, physical := range []bool{false, true} {
		t.Run(fmt.Sprintf("primary=%v", physical), func(t *testing.T) {
			fixture := newRuntimeActiveHistoryFixtureV1(t, "resume", false)
			view := runtimeActiveHistoryCloneV1(t, fixture.target)
			threadID := view["id"].(string)
			const turnID = "turn_unsigned_after_inherited_prefix"
			const stamp = "2026-09-09T03:00:00Z"
			view["turns"] = append(view["turns"].([]any), map[string]any{
				"id": turnID, "threadId": threadID, "status": "completed", "createdAt": stamp, "finishedAt": stamp,
				"items": []any{map[string]any{
					"id": "user_unsigned_after_prefix", "threadId": threadID, "turnId": turnID,
					"kind": "user_message", "role": "user", "status": "completed", "createdAt": stamp, "finishedAt": stamp,
					"text": "UNSIGNED_APPEND_MUST_NOT_GAIN_PUBLIC_ADMISSION",
				}},
			})
			if physical {
				if err := os.WriteFile(fixture.targetPath(), runtimeActiveHistoryJSONV1(t, view), 0o600); err != nil {
					t.Fatal(err)
				}
				snapshot, err := fixture.primary.ReadPrimaryThreadSnapshotV1(context.Background(), threadID)
				if err != nil {
					t.Fatal(err)
				}
				view = snapshot.Thread
			}
			// Prefix provenance does not admit independent later turns. The public
			// owner must reject this unsigned append even before any target final.
			if public, err := fixture.projector.ProjectThread(view); err == nil || public != nil {
				t.Fatal("bound target accepted an unsigned appended turn before its first final")
			}
		})
	}
}

func TestRuntimeActiveHistoryStartupRejectsMissingPrimaryBeforeRepair(t *testing.T) {
	root := t.TempDir()
	fixture := newRuntimeOptionalPluginLifecycleFixtureV1(t, "missing", Config{
		RuntimeToken: DefaultRuntimeToken, UserDataDir: filepath.Join(root, "user-data"),
		ProductionDurableRoot: filepath.Join(root, "durable"),
		ProviderID:            "sidecar-recovery", Model: "sidecar-model", APIKey: "synthetic-only",
		BaseURL: "http://127.0.0.1:1/v1", EndpointFormat: "chat_completions",
	})
	config := fixture.config
	seedProviderRegistryExecutionAuthorityV1(t, config.DataDir, config.ProviderID, config.BaseURL, []string{config.Model}, config.Model, "synthetic-only")
	handler, err := fixture.start()
	if err != nil {
		t.Fatal(err)
	}
	request := func(method, path string, body map[string]any, want int) map[string]any {
		t.Helper()
		req := httptest.NewRequest(method, path, bytes.NewReader(runtimeActiveHistoryJSONV1(t, body)))
		req.Header.Set("Authorization", "Bearer "+DefaultRuntimeToken)
		req.Header.Set("Content-Type", "application/json")
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, req)
		if recorder.Code != want {
			t.Fatalf("sidecar recovery %s %s status=%d want=%d", method, path, recorder.Code, want)
		}
		var result map[string]any
		if err := json.Unmarshal(recorder.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		return result
	}
	created := request(http.MethodPost, "/v1/threads", map[string]any{"title": "ordinary sidecar source", "workspace": t.TempDir(), "providerId": config.ProviderID, "model": config.Model}, http.StatusCreated)
	threadID, _ := created["id"].(string)
	if threadID == "" {
		shutdownOwnedRuntimeHandler(t, handler)
		t.Fatal("ordinary source creation returned no identity")
	}
	shutdownOwnedRuntimeHandler(t, handler)
	fixture.assertFault(1)
	// The real ordinary writer supplies both sidecars while no composition owns
	// the fixture. Only its primary is removed to model existing sidecar history.
	durable, err := server.NewProductionDurableEventSessionStore(config.ProductionDurableRoot)
	if err != nil {
		t.Fatal(err)
	}
	const turnID = "turn_ordinary_sidecar_recovery"
	const text = "R131 ordinary sidecar user history survives startup"
	const stamp = "2026-09-09T04:00:00Z"
	if err := durable.AppendTurnToThread(threadID, map[string]any{
		"id": turnID, "threadId": threadID, "status": "completed", "createdAt": stamp, "startedAt": stamp, "finishedAt": stamp,
		"items": []any{map[string]any{
			"id": "user_sidecar_recovery", "threadId": threadID, "turnId": turnID,
			"kind": "user_message", "role": "user", "status": "completed", "createdAt": stamp, "finishedAt": stamp, "text": text,
		}},
	}, config.ProviderID, nil); err != nil {
		t.Fatal(err)
	}
	threadDir := filepath.Join(config.ProductionDurableRoot, "threads", threadID)
	sidecars := map[string][]byte{}
	for _, name := range []string{"metadata.jsonl", "messages.jsonl"} {
		body, err := os.ReadFile(filepath.Join(threadDir, name))
		if err != nil || len(body) == 0 {
			t.Fatalf("real writer did not create %s: %v", name, err)
		}
		sidecars[name] = body
	}
	primaryPath := filepath.Join(threadDir, "thread.json")
	if err := os.Remove(primaryPath); err != nil {
		t.Fatal(err)
	}
	// The existing Core child-identity inventory requires every primary
	// before any active-history or context repair. Sidecars cannot supply it.
	handler, err = fixture.start()
	if handler != nil {
		shutdownOwnedRuntimeHandler(t, handler)
	}
	if !errors.Is(err, pendingapp.ErrChildProducerInventoryIncomplete) {
		t.Fatalf("missing primary did not fail at its existing Core owner: %v", err)
	}
	if _, err := os.Lstat(primaryPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("startup backfilled missing primary authority")
	}
	for name, before := range sidecars {
		after, err := os.ReadFile(filepath.Join(threadDir, name))
		if err != nil || !bytes.Equal(before, after) {
			t.Fatal("failed startup changed preserved sidecar history")
		}
	}
}
