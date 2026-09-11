package runtimeapp

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"analytix.local/runtime-go/internal/contracts"
	domainevent "analytix.local/runtime-go/internal/domain/event"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
)

const packagedSourceUnavailableHydrationPromptV1 = "Continue in this exact Agent and thread. " +
	"Attempt a protected funds fact request for the current case without asking for, inventing, or displaying any complete account, card, identity, or phone value. " +
	"Request inflow, outflow, net amount, transaction count, and evidence rows for the current protected account and current authorized time range. " +
	"If an exact current case and DatasetSnapshot authority is absent, fail closed only for this protected fact effect through the typed host boundary. " +
	"Do not use ordinary read, file, shell, Todo, Skill, subagent, or non-funds MCP tools for this request."

type packagedSourceUnavailableHydrationSnapshotV1 struct {
	accepted  domainevidence.AcceptedFinalPublicViewV3
	delivery  domainevent.AcceptedFinalDeliveryBatchV2
	latestSeq int
}

func TestRuntimeHTTPRawPackagedProtectedPromptPublishesSourceUnavailableAndHydratesAcrossRestart(t *testing.T) {
	var providerCalls atomic.Int64
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		providerCalls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"message": "test provider must not be reached"}})
	}))
	defer provider.Close()

	root := t.TempDir()
	workspace := filepath.Join(root, "ordinary-workspace")
	if err := os.MkdirAll(workspace, 0o700); err != nil {
		t.Fatal(err)
	}
	config := Config{
		RuntimeToken: DefaultRuntimeToken, ProductionDurableRoot: filepath.Join(root, "durable"),
		DataDir: filepath.Join(root, "runtime-data"), UserDataDir: filepath.Join(root, "user-data"),
		ProviderID: "accepted-final-hydration-provider", BaseURL: provider.URL + "/v1", APIKey: "test-only",
		Model: "accepted-final-hydration-model", EndpointFormat: "chat_completions",
	}
	handler, err := NewRuntimeServerHandlerE(config)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer func() {
		if server != nil {
			server.Close()
			shutdownOwnedRuntimeHandler(t, handler)
		}
	}()
	client := &http.Client{Timeout: 5 * time.Second}

	status, created := packagedSourceUnavailableHydrationHTTPJSONV1(t, client, server.URL, http.MethodPost, "/v1/threads", map[string]any{
		"title": "accepted final hydration", "workspace": workspace,
		"providerId": config.ProviderID, "model": config.Model,
	})
	threadID := contracts.StringField(created, "id")
	if status != http.StatusCreated || threadID == "" {
		t.Fatalf("create thread status=%d body=%#v", status, created)
	}
	if got := providerCalls.Load(); got != 0 {
		t.Fatalf("provider was reached before the protected turn: calls=%d", got)
	}
	status, started := packagedSourceUnavailableHydrationHTTPJSONV1(
		t, client, server.URL, http.MethodPost, "/v1/threads/"+threadID+"/turns",
		map[string]any{
			"prompt": packagedSourceUnavailableHydrationPromptV1,
			"mode":   "agent",
		},
	)
	turnID := contracts.StringField(started, "turnId")
	if status != http.StatusAccepted || turnID == "" {
		t.Fatalf("protected turn status=%d body=%#v", status, started)
	}
	first := packagedSourceUnavailableHydrationReadV1(t, client, server.URL, threadID, turnID)
	if got := providerCalls.Load(); got != 0 {
		t.Fatalf("protected source-unavailable turn reached provider: calls=%d", got)
	}

	server.Close()
	shutdownOwnedRuntimeHandler(t, handler)
	server = nil

	restarted, err := NewRuntimeServerHandlerE(config)
	if err != nil {
		t.Fatal(err)
	}
	restartedServer := httptest.NewServer(restarted)
	defer func() {
		restartedServer.Close()
		shutdownOwnedRuntimeHandler(t, restarted)
	}()
	second := packagedSourceUnavailableHydrationReadV1(t, client, restartedServer.URL, threadID, turnID)
	if first.accepted.AcceptedFinalDigest != second.accepted.AcceptedFinalDigest ||
		first.latestSeq != second.latestSeq || !reflect.DeepEqual(first.delivery, second.delivery) {
		t.Fatalf("accepted-final hydration changed across restart: first=%#v second=%#v", first, second)
	}
	if got := providerCalls.Load(); got != 0 {
		t.Fatalf("accepted-final readback reached provider: calls=%d", got)
	}
}

func packagedSourceUnavailableHydrationReadV1(
	t *testing.T,
	client *http.Client,
	serverURL string,
	threadID string,
	turnID string,
) packagedSourceUnavailableHydrationSnapshotV1 {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		status, detail := packagedSourceUnavailableHydrationHTTPJSONV1(
			t, client, serverURL, http.MethodGet, "/v1/threads/"+threadID, nil,
		)
		if status == http.StatusOK {
			turn := packagedSourceUnavailableHydrationTurnV1(detail, turnID)
			if turn != nil && contracts.StringField(turn, "status") == "completed" && turn["acceptedFinalView"] != nil {
				return packagedSourceUnavailableHydrationValidateV1(t, detail, turn)
			}
		} else if status != http.StatusServiceUnavailable {
			t.Fatalf("thread hydration status=%d body=%#v", status, detail)
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for accepted-final hydration: status=%d body=%#v", status, detail)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func packagedSourceUnavailableHydrationValidateV1(
	t *testing.T,
	detail map[string]any,
	turn map[string]any,
) packagedSourceUnavailableHydrationSnapshotV1 {
	t.Helper()
	view, err := domainevidence.ParseAcceptedFinalPublicViewV3Value(turn["acceptedFinalView"])
	if err != nil {
		t.Fatal(err)
	}
	if turn["acceptedFinal"] != nil || view.SchemaVersion != domainevidence.AcceptedFinalPublicViewV3Version ||
		view.Variant != domainevidence.SourceUnavailableAnswer || view.TerminalReason != "source_unavailable" ||
		view.CoverageStatus != domainevidence.AcceptedFinalCoverageUnavailable || view.ClaimCount != 0 ||
		view.ReceiptMetadata.Count != 0 {
		t.Fatalf("source-unavailable public view is not the current typed boundary: turn=%#v view=%#v", turn, view)
	}
	items, _ := turn["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("source-unavailable public turn has unexpected items: %#v", turn)
	}
	item, _ := items[0].(map[string]any)
	if item == nil || item["acceptedFinal"] != nil ||
		!packagedSourceUnavailableHydrationSameJSONV1(item["acceptedFinalView"], turn["acceptedFinalView"]) ||
		contracts.StringField(item, "threadId") != contracts.StringField(turn, "threadId") ||
		contracts.StringField(item, "turnId") != contracts.StringField(turn, "id") ||
		contracts.StringField(item, "finishedAt") != view.AcceptedAt {
		t.Fatalf("source-unavailable public item is detached: turn=%#v item=%#v", turn, item)
	}
	rawDelivery, ok := detail["acceptedFinalDelivery"].(map[string]any)
	if !ok {
		t.Fatalf("thread detail has no accepted-final delivery: %#v", detail)
	}
	rawDeliveries, ok := detail["acceptedFinalDeliveries"].([]any)
	if !ok || len(rawDeliveries) != 1 || !packagedSourceUnavailableHydrationSameJSONV1(rawDeliveries[0], rawDelivery) {
		t.Fatalf("thread detail does not bind its visible final to one exact delivery: %#v", detail)
	}
	delivery, err := domainevent.ParseAcceptedFinalDeliveryBatchV2(rawDelivery)
	if err != nil {
		t.Fatal(err)
	}
	latestSeq, ok := contracts.NumericSeq(detail["latestSeq"])
	if !ok || delivery.ThreadID != contracts.StringField(turn, "threadId") || delivery.TurnID != contracts.StringField(turn, "id") ||
		delivery.PublicationCommitID != view.AcceptedFinalDigest || delivery.FirstSeq <= 0 ||
		delivery.LastSeq > latestSeq || len(delivery.Events) != 3 ||
		contracts.StringField(delivery.Events[0], "publicationSlot") != "assistant-final" ||
		contracts.StringField(delivery.Events[1], "publicationSlot") != "usage" ||
		contracts.StringField(delivery.Events[2], "publicationSlot") != "terminal" {
		t.Fatalf("accepted-final delivery is detached from the thread snapshot: latestSeq=%d delivery=%#v", latestSeq, delivery)
	}
	firstEventItem, _ := delivery.Events[0]["item"].(map[string]any)
	if firstEventItem == nil || firstEventItem["acceptedFinal"] != nil ||
		!packagedSourceUnavailableHydrationSameJSONV1(firstEventItem["acceptedFinalView"], turn["acceptedFinalView"]) {
		t.Fatalf("accepted-final delivery leaked or detached private authority: %#v", delivery.Events[0])
	}
	return packagedSourceUnavailableHydrationSnapshotV1{accepted: view, delivery: delivery, latestSeq: latestSeq}
}

func packagedSourceUnavailableHydrationSameJSONV1(left, right any) bool {
	leftBody, leftErr := json.Marshal(left)
	rightBody, rightErr := json.Marshal(right)
	return leftErr == nil && rightErr == nil && bytes.Equal(leftBody, rightBody)
}

func packagedSourceUnavailableHydrationTurnV1(detail map[string]any, turnID string) map[string]any {
	turns, _ := detail["turns"].([]any)
	for _, value := range turns {
		turn, _ := value.(map[string]any)
		if contracts.StringField(turn, "id") == turnID {
			return turn
		}
	}
	return nil
}

func packagedSourceUnavailableHydrationHTTPJSONV1(
	t *testing.T,
	client *http.Client,
	serverURL string,
	method string,
	path string,
	body map[string]any,
) (int, map[string]any) {
	t.Helper()
	var encoded []byte
	var err error
	if body != nil {
		encoded, err = json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
	}
	request, err := http.NewRequest(method, serverURL+path, bytes.NewReader(encoded))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+DefaultRuntimeToken)
	request.Header.Set("Content-Type", "application/json")
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	decoded := map[string]any{}
	if err := json.NewDecoder(response.Body).Decode(&decoded); err != nil {
		t.Fatal(err)
	}
	return response.StatusCode, decoded
}
