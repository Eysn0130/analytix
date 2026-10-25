package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"sync"
	"testing"
	"time"

	finalauthorityadapter "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	appmodel "analytix.local/runtime-go/internal/app/model"
	threadapp "analytix.local/runtime-go/internal/app/thread"
	turnsecurityapp "analytix.local/runtime-go/internal/app/turnsecurity"
	domainevent "analytix.local/runtime-go/internal/domain/event"
	domainthread "analytix.local/runtime-go/internal/domain/thread"
	"analytix.local/runtime-go/internal/testsupport/workspacetest"
)

func messageFeedbackStoreFixtureV1(t *testing.T) (*DurableEventSessionStore, string, string, func(map[string]any) (map[string]any, error)) {
	t.Helper()
	store, err := newForkBoundaryStoreV1(t, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	workspace := workspacetest.New(t)
	thread, err := store.CreateThread(map[string]any{"title": "Message feedback fixture", "workspace": workspace}, workspace)
	if err != nil {
		t.Fatal(err)
	}
	threadID := stringField(thread, "id")
	parent := appendForkLoggedParentV1(t, store, threadID, "feedback-turn", workspace)
	finishForkFixtureV1(t, store, parent.Context)
	primary, err := finalauthorityadapter.NewAcceptedFinalCASReader(store.root)
	if err != nil {
		t.Fatal(err)
	}
	project := threadapp.NewTrustedPublicProjectorWithPrimaryCAS(nil, nil, nil, primary).ProjectThread
	thread, err = store.GetThread(threadID)
	if err != nil {
		t.Fatal(err)
	}
	public, err := project(thread)
	if err != nil {
		t.Fatal(err)
	}
	turns, _ := public["turns"].([]any)
	for _, rawTurn := range turns {
		turn, _ := rawTurn.(map[string]any)
		items, _ := turn["items"].([]any)
		for _, rawItem := range items {
			item, _ := rawItem.(map[string]any)
			if stringField(item, "kind") == "assistant_text" && stringField(item, "status") == "completed" && stringField(item, "text") != "" {
				return store, threadID, stringField(item, "id"), project
			}
		}
	}
	t.Fatal("normal committed terminal fixture has no public assistant item")
	return nil, "", "", nil
}

func TestMessageFeedbackDurableCASColdReadAndIsolationV1(t *testing.T) {
	store, threadID, messageID, project := messageFeedbackStoreFixtureV1(t)
	before, err := os.ReadFile(store.threadPath(threadID))
	if err != nil {
		t.Fatal(err)
	}
	thread, err := store.GetThread(threadID)
	if err != nil {
		t.Fatal(err)
	}
	history := appmodel.ProviderHistoryMessagesFromThread(thread)
	if len(history) == 0 {
		t.Fatal("fixture has no semantic provider history")
	}
	live, unsubscribe := store.SubscribeEvents(threadID)
	defer unsubscribe()
	note := "Optional synthetic rating detail"
	request := domainthread.MessageFeedbackPutRequestV1{Rating: "positive", Note: &note}
	first, err := store.PutMessageFeedbackV1(threadID, messageID, request, project)
	if err != nil {
		t.Fatal(err)
	}
	var conflict *domainthread.MessageFeedbackVersionConflictV1
	if _, err := store.PutMessageFeedbackV1(threadID, messageID, request, project); !errors.As(err, &conflict) || conflict.Current == nil || conflict.Current.Version != first.Version {
		t.Fatal("stale repeated create did not report current", err)
	}
	request.IfVersion = &first.Version
	count, err := store.eventLog.LoadSince(threadID, 0)
	if err != nil {
		t.Fatal(err)
	}
	noop, err := store.PutMessageFeedbackV1(threadID, messageID, request, project)
	if err != nil || !reflect.DeepEqual(noop, first) {
		t.Fatal("same material did not preserve version", err)
	}
	afterNoop, err := store.eventLog.LoadSince(threadID, 0)
	if err != nil || len(afterNoop.Events) != len(count.Events) {
		t.Fatal("same material appended a second event", err)
	}
	request.Rating = "negative"
	second, err := store.PutMessageFeedbackV1(threadID, messageID, request, project)
	if err != nil || second.Version == first.Version || second.CreatedAt != first.CreatedAt || second.UpdatedAt <= first.UpdatedAt {
		t.Fatal("material change did not rotate monotonic version", err)
	}
	if err := store.DeleteMessageFeedbackV1(threadID, messageID, first.Version, project); !errors.As(err, &conflict) || conflict.Current.Version != second.Version {
		t.Fatal("stale delete did not conflict", err)
	}
	reopened, err := newForkBoundaryStoreV1(t, store.root)
	if err != nil {
		t.Fatal(err)
	}
	items, err := reopened.ListMessageFeedbackV1(threadID, project)
	if err != nil || len(items) != 1 || !reflect.DeepEqual(items[0], second) {
		t.Fatal("cold read lost feedback", err)
	}
	after, err := os.ReadFile(store.threadPath(threadID))
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("feedback changed semantic primary history", err)
	}
	thread, err = store.GetThread(threadID)
	if err != nil || !reflect.DeepEqual(history, appmodel.ProviderHistoryMessagesFromThread(thread)) {
		t.Fatal("feedback changed provider context", err)
	}
	select {
	case event := <-live:
		t.Fatalf("private feedback was published live: %s", stringField(event, "kind"))
	default:
	}
	loaded, err := store.eventLog.LoadSince(threadID, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range loaded.Events {
		if !domainthread.IsMessageFeedbackEventV1(event) {
			continue
		}
		visible, keep, err := threadapp.ProjectPublicThreadEvent(threadID, thread, event)
		if err != nil || keep || visible != nil {
			t.Fatal("feedback entered public replay", err)
		}
		body, _ := json.Marshal(event)
		if bytes.Contains(body, []byte("bounded background work")) {
			t.Fatal("feedback copied transcript text")
		}
	}
	if err := reopened.DeleteMessageFeedbackV1(threadID, messageID, second.Version, project); err != nil {
		t.Fatal(err)
	}
	if err := reopened.DeleteMessageFeedbackV1(threadID, messageID, second.Version, project); err != nil {
		t.Fatal("absent delete not idempotent", err)
	}
	third, err := reopened.PutMessageFeedbackV1(threadID, messageID, domainthread.MessageFeedbackPutRequestV1{Rating: "positive"}, project)
	if err != nil || third.Version == first.Version || third.Version == second.Version {
		t.Fatal("recreate reused a version", err)
	}
	if err := reopened.DeleteMessageFeedbackV1(threadID, messageID, second.Version, project); !errors.As(err, &conflict) {
		t.Fatal("delete/recreate ABA was not rejected", err)
	}
}

func TestMessageFeedbackConcurrentCreateAndMissingPrimaryV1(t *testing.T) {
	store, threadID, messageID, project := messageFeedbackStoreFixtureV1(t)
	var wait sync.WaitGroup
	outcomes := make(chan error, 2)
	for index := 0; index < 2; index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			_, err := store.PutMessageFeedbackV1(threadID, messageID, domainthread.MessageFeedbackPutRequestV1{Rating: "positive"}, project)
			outcomes <- err
		}()
	}
	wait.Wait()
	close(outcomes)
	successes, conflicts := 0, 0
	for err := range outcomes {
		var conflict *domainthread.MessageFeedbackVersionConflictV1
		if err == nil {
			successes++
		} else if errors.As(err, &conflict) {
			conflicts++
		} else {
			t.Fatal(err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatal("concurrent creation was not serialized")
	}
	if _, err := store.PutMessageFeedbackV1(threadID, "frontend-invented-block", domainthread.MessageFeedbackPutRequestV1{Rating: "positive"}, project); !errors.Is(err, domainthread.ErrMessageFeedbackTargetNotFoundV1) {
		t.Fatal("arbitrary block was accepted", err)
	}
	if err := os.Remove(store.threadPath(threadID)); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ListMessageFeedbackV1(threadID, project); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("missing primary silently became a sidecar-backed empty list", err)
	}
}

func TestMessageFeedbackUsesExistingMapContentGuardBeforeAppendV1(t *testing.T) {
	for _, existing := range []bool{false, true} {
		t.Run(map[bool]string{false: "create", true: "update"}[existing], func(t *testing.T) {
			store, threadID, messageID, project := messageFeedbackStoreFixtureV1(t)
			request := domainthread.MessageFeedbackPutRequestV1{Rating: "negative"}
			if existing {
				current, err := store.PutMessageFeedbackV1(threadID, messageID, domainthread.MessageFeedbackPutRequestV1{Rating: "positive"}, project)
				if err != nil {
					t.Fatal(err)
				}
				request.IfVersion = &current.Version
			}
			beforeLog, err := os.ReadFile(store.eventsPath(threadID))
			if err != nil {
				t.Fatal(err)
			}
			beforePrimary, err := os.ReadFile(store.threadPath(threadID))
			if err != nil {
				t.Fatal(err)
			}
			beforeItems, err := store.ListMessageFeedbackV1(threadID, project)
			if err != nil {
				t.Fatal(err)
			}
			note := "<think>SYNTHETIC_PRIVATE_FEEDBACK_GUARD_SENTINEL</think>"
			request.Note = &note
			if _, err := store.PutMessageFeedbackV1(threadID, messageID, request, project); !errors.Is(err, domainevent.ErrPrivateReasoningPersistence) {
				t.Fatal("feedback bypassed existing recursive map content guard", err)
			}
			afterLog, err := os.ReadFile(store.eventsPath(threadID))
			if err != nil || !bytes.Equal(beforeLog, afterLog) {
				t.Fatal("rejected feedback changed event log", err)
			}
			afterPrimary, err := os.ReadFile(store.threadPath(threadID))
			if err != nil || !bytes.Equal(beforePrimary, afterPrimary) {
				t.Fatal("rejected feedback changed primary", err)
			}
			reopened, err := newForkBoundaryStoreV1(t, store.root)
			if err != nil {
				t.Fatal(err)
			}
			afterItems, err := reopened.ListMessageFeedbackV1(threadID, project)
			if err != nil || !reflect.DeepEqual(beforeItems, afterItems) {
				t.Fatal("rejected feedback changed persisted current rating", err)
			}
		})
	}
}

func TestMessageFeedbackDoesNotFollowForkV1(t *testing.T) {
	store, threadID, messageID, project := messageFeedbackStoreFixtureV1(t)
	if _, err := store.PutMessageFeedbackV1(threadID, messageID, domainthread.MessageFeedbackPutRequestV1{Rating: "positive"}, project); err != nil {
		t.Fatal(err)
	}
	fork, err := store.ForkThreadWithProjectionV1(threadID, map[string]any{}, threadapp.NewTrustedPublicProjector(nil))
	if err != nil {
		t.Fatal(err)
	}
	items, err := store.ListMessageFeedbackV1(stringField(fork, "id"), project)
	if err != nil || len(items) != 0 {
		t.Fatal("parent feedback was copied to fork", err)
	}
}

func TestMessageFeedbackRemovedTurnAndCorruptLogV1(t *testing.T) {
	store, threadID, messageID, project := messageFeedbackStoreFixtureV1(t)
	item, err := store.PutMessageFeedbackV1(threadID, messageID, domainthread.MessageFeedbackPutRequestV1{Rating: "positive"}, project)
	if err != nil {
		t.Fatal(err)
	}
	thread, digest, err := store.ReadThreadMutationBaseline(threadID)
	if err != nil {
		t.Fatal(err)
	}
	current, found, err := turnsecurityapp.LatestContext(thread)
	if err != nil || !found {
		t.Fatal("rewind context unavailable", err)
	}
	prepared, err := threadapp.PrepareRewindMutation(thread, threadID, "feedback-turn", current, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CommitRewindMutation(threadapp.RewindMutationCommitRequest{ThreadID: threadID, RewindTurnID: "feedback-turn", Stamp: prepared.Stamp, ExpectedBaselineDigest: digest, ExpectedContextDigest: prepared.SecurityContext.ContextDigest, CurrentContext: current}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.PutMessageFeedbackV1(threadID, messageID, domainthread.MessageFeedbackPutRequestV1{Rating: "negative", IfVersion: &item.Version}, project); !errors.Is(err, domainthread.ErrMessageFeedbackTargetNotFoundV1) {
		t.Fatal("removed assistant was accepted", err)
	}
	items, err := store.ListMessageFeedbackV1(threadID, project)
	if err != nil || len(items) != 1 {
		t.Fatal("rewind silently cascaded feedback deletion", err)
	}
	if err := store.DeleteMessageFeedbackV1(threadID, messageID, item.Version, project); err != nil {
		t.Fatal("old rating cannot be retracted", err)
	}
	file, err := os.OpenFile(store.eventsPath(threadID), os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	_, writeErr := file.WriteString(`{"kind":"message_feedback_put"`)
	closeErr := file.Close()
	if writeErr != nil || closeErr != nil {
		t.Fatal("failed corrupt fixture write")
	}
	if _, err := store.ListMessageFeedbackV1(threadID, project); err == nil {
		t.Fatal("corrupt feedback log became successful empty result")
	}
}
