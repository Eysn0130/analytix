package event

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"math"
	"reflect"
	"testing"

	"analytix.local/runtime-go/internal/contracts"
	domainterminal "analytix.local/runtime-go/internal/domain/terminal"
)

func TestAcceptedFinalDeliverySealV1CrossLanguageGolden(t *testing.T) {
	seed := make([]byte, ed25519.SeedSize)
	for index := range seed {
		seed[index] = byte(index)
	}
	privateKey := ed25519.NewKeyFromSeed(seed)
	publicKey := privateKey.Public().(ed25519.PublicKey)
	seal, err := NewAcceptedFinalDeliverySealV1(AcceptedFinalDeliverySealInputV1{
		ThreadID:                       "golden-thread",
		TurnID:                         "golden-turn",
		PublicationCommitID:            digestAcceptedFinalBatchTest("golden-commit"),
		AcceptedFinalDispositionDigest: digestAcceptedFinalBatchTest("golden-accepted-disposition"),
		TerminalDispositionID:          digestAcceptedFinalBatchTest("golden-terminal-disposition"),
		EventManifestDigest:            digestAcceptedFinalBatchTest("golden-manifest"),
		SequencedEventsDigest:          digestAcceptedFinalBatchTest("golden-sequenced-events"),
		BatchID:                        digestAcceptedFinalBatchTest("golden-batch"),
		FirstSeq:                       41,
		LastSeq:                        43,
		Timestamp:                      "2026-07-18T01:02:03.456789Z",
		AuthorityKeyID:                 digestAcceptedFinalBatchBytesTest(publicKey),
		AuthorityPublicKey:             publicKey,
	}, func(message []byte) ([]byte, error) {
		return ed25519.Sign(privateKey, message), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	golden := map[string]string{
		"keyId":        "56475aa75463474c0285df5dbf2bcab73da651358839e9b77481b2eab107708c",
		"publicKey":    "A6EHv_POEL4dcN0Y50vAmWfk1jCbpQ1fHdyGZBJVMbg",
		"sealId":       "0e791187f9378e42889c94b07aaf1f10f68c51ba7f226973dc8b73e8950518f9",
		"signature":    "HYSwxE8d_aMP_8BV1_JTYtnZ9PFw1mI9vnSfPLSuHlJQImI0WNOVD5OuZNVzJHFk0Ngz1JEe_UxId9c9-lE9DQ",
		"signingBytes": "616e616c797469782f61636365707465642d66696e616c2d64656c69766572792d7365616c2d7369676e61747572652f763100835b3d14e8672b6c061d0913ded79e14839a83cdb5f19de3c5d6db1d51a3517f",
	}
	actual := map[string]string{
		"keyId":        seal.AuthorityKeyID,
		"publicKey":    base64.RawURLEncoding.EncodeToString(publicKey),
		"sealId":       seal.SealID,
		"signature":    seal.AuthoritySignature,
		"signingBytes": hex.EncodeToString(AcceptedFinalDeliverySealSigningBytesV1(seal)),
	}
	for field, expected := range golden {
		if actual[field] != expected {
			t.Fatalf("cross-language accepted-final seal %s drifted:\n got %s\nwant %s", field, actual[field], expected)
		}
	}
}

func TestAcceptedFinalDeliveryBatchV2RejectsPartialAndMutatedBundles(t *testing.T) {
	commitID := digestAcceptedFinalBatchTest("commit")
	events := make([]map[string]any, 0, 3)
	for index, candidate := range []struct{ slot, kind string }{
		{"assistant-final", "item_completed"}, {"usage", "usage"}, {"terminal", "turn_completed"},
	} {
		event := map[string]any{
			"kind": candidate.kind, "threadId": "thread-1", "turnId": "turn-1", "seq": float64(index + 10),
			"timestamp": "2026-07-18T00:00:00Z", "acceptedFinalDigest": commitID, "publicationCommitId": commitID,
			"publicationEventId": acceptedFinalDeliveryEventID(commitID, candidate.slot), "publicationSlot": candidate.slot,
		}
		switch candidate.slot {
		case "assistant-final":
			itemID := "item-turn-1-assistant"
			event["itemId"] = itemID
			event["item"] = map[string]any{
				"id": itemID, "threadId": "thread-1", "turnId": "turn-1", "role": "assistant",
				"kind": "assistant_text", "text": "verified boundary", "status": "completed",
				"acceptedFinalView": acceptedFinalPublicViewV3ForBatchTest(commitID, "success", "2026-07-18T00:00:00Z"),
			}
		case "usage":
			event["usageFinalStatus"] = "completed"
		case "terminal":
			event["status"] = "completed"
			event["terminalReason"] = "success"
		}
		event["publicationPayloadDigest"] = acceptedFinalDeliveryPayloadDigest(event)
		events = append(events, event)
	}
	privateKey := ed25519.NewKeyFromSeed(make([]byte, ed25519.SeedSize))
	publicKey := privateKey.Public().(ed25519.PublicKey)
	seal, err := NewAcceptedFinalDeliverySealForEventsV2(
		events, digestAcceptedFinalBatchTest("accepted-disposition"), digestAcceptedFinalBatchTest("terminal-disposition"),
		digestAcceptedFinalBatchBytesTest(publicKey), publicKey,
		func(message []byte) ([]byte, error) { return ed25519.Sign(privateKey, message), nil },
	)
	if err != nil {
		t.Fatal(err)
	}
	batch, err := NewAcceptedFinalDeliveryBatchV2(events, seal)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseAcceptedFinalDeliveryBatchV2(AcceptedFinalDeliveryBatchV2Map(batch)); err != nil {
		t.Fatal(err)
	}
	for _, hostile := range []struct {
		name  string
		key   string
		value any
	}{
		{name: "private public view digest", key: "publicViewDigest", value: digestAcceptedFinalBatchTest("private-view")},
		{name: "private envelope digest", key: "envelopeDigest", value: digestAcceptedFinalBatchTest("private-envelope")},
		{name: "private context digest", key: "contextDigest", value: digestAcceptedFinalBatchTest("private-context")},
		{name: "private context epoch", key: "contextEpoch", value: 7},
		{name: "private dataset identity", key: "datasetSnapshotId", value: "private-dataset"},
		{name: "private envelope time", key: "envelopeIssuedAt", value: "2026-07-17T00:00:00Z"},
		{name: "private thread identity", key: "threadId", value: "thread-private"},
		{name: "private turn identity", key: "turnId", value: "turn-private"},
		{name: "private rendered text digest", key: "renderedTextSha256", value: digestAcceptedFinalBatchTest("private-text")},
		{name: "private witness admission", key: "factFinalWitnessAdmission", value: map[string]any{"private": true}},
		{name: "private publication proof", key: "publicationSnapshotProof", value: map[string]any{"private": true}},
		{name: "private registry head", key: "registryHead", value: map[string]any{"private": true}},
		{name: "private publication intent", key: "publicationIntent", value: map[string]any{"private": true}},
		{name: "private store digest", key: "storeDigest", value: digestAcceptedFinalBatchTest("private-store")},
		{name: "raw source sentinel", key: "sourceExactValue", value: "PRIVATE_SOURCE_VALUE"},
	} {
		t.Run(hostile.name, func(t *testing.T) {
			body, _ := json.Marshal(events)
			var candidate []map[string]any
			_ = json.Unmarshal(body, &candidate)
			item := candidate[0]["item"].(map[string]any)
			view := item["acceptedFinalView"].(map[string]any)
			view[hostile.key] = hostile.value
			rehashAcceptedFinalSemanticEventsForTest(candidate)
			if err := ValidateAcceptedFinalDeliveryEventsV2(candidate); err == nil {
				t.Fatalf("closed V3 accepted forbidden %s=%#v", hostile.key, hostile.value)
			}
		})
	}
	for _, mutate := range []func(map[string]any){
		func(view map[string]any) { delete(view, "publicationState") },
		func(view map[string]any) { view["publicationState"] = "pending" },
	} {
		body, _ := json.Marshal(events)
		var candidate []map[string]any
		_ = json.Unmarshal(body, &candidate)
		item := candidate[0]["item"].(map[string]any)
		view := item["acceptedFinalView"].(map[string]any)
		mutate(view)
		rehashAcceptedFinalSemanticEventsForTest(candidate)
		if err := ValidateAcceptedFinalDeliveryEventsV2(candidate); err == nil {
			t.Fatal("closed V3 accepted missing or non-accepted publicationState")
		}
	}
	partial := batch
	partial.Events = partial.Events[:2]
	if ValidateAcceptedFinalDeliveryBatchV2(partial) == nil {
		t.Fatal("partial accepted-final delivery batch was accepted")
	}
	mutated := batch
	body, _ := json.Marshal(batch.Events)
	_ = json.Unmarshal(body, &mutated.Events)
	mutated.Events[0]["publicationPayloadDigest"] = digestAcceptedFinalBatchTest("forged")
	if ValidateAcceptedFinalDeliveryBatchV2(mutated) == nil {
		t.Fatal("mutated accepted-final delivery batch was accepted")
	}
	resequenced := batch
	body, _ = json.Marshal(batch.Events)
	_ = json.Unmarshal(body, &resequenced.Events)
	for _, event := range resequenced.Events {
		event["seq"] = event["seq"].(float64) + 100
	}
	resequenced.FirstSeq += 100
	resequenced.LastSeq += 100
	resequenced.Seq += 100
	resequenced.BatchID = acceptedFinalDeliveryBatchIDV2(resequenced)
	if ValidateAcceptedFinalDeliveryBatchV2(resequenced) == nil {
		t.Fatal("re-sequenced accepted-final delivery reused an authority seal")
	}
}

func TestAcceptedFinalDeliveryBatchVersionsAreClosedAndNonInterchangeable(t *testing.T) {
	privateEvents := acceptedFinalSemanticEventsForTest(t, "success", "completed")
	commitID := contracts.StringField(privateEvents[0], "publicationCommitId")
	itemID := "item-turn-semantic-assistant"
	publicViewCore := map[string]any{
		"schemaVersion": 2, "publicationState": "accepted",
		"envelopeDigest": digestAcceptedFinalBatchTest("v1-envelope"),
		"contextDigest":  digestAcceptedFinalBatchTest("v1-context"), "contextEpoch": 1,
		"datasetSnapshotId": "dsv2_" + digestAcceptedFinalBatchTest("v1-dataset"),
		"variant":           "GeneralGuidanceAnswer", "terminalReason": "success", "blockerCode": "",
		"coverageStatus": "guidance_only", "checkedScopeDigest": "", "missingScopeCount": 0,
		"claimCount": 0, "claimTypes": []any{},
		"receiptMetadata": map[string]any{
			"projection": "masked_metadata_only", "count": 0,
			"setDigest": digestAcceptedFinalBatchTest("v1-empty-receipts"), "citations": []any{},
		},
		"noHitWording": "", "envelopeIssuedAt": "2026-07-18T00:00:00Z",
		"acceptedAt": "2026-07-18T00:00:00Z",
	}
	publicViewDigest := historicalAcceptedFinalPublicViewCoreV2Digest(publicViewCore)
	publicView := contracts.CloneMap(publicViewCore)
	publicView["acceptedFinalDigest"] = commitID
	publicView["publicViewDigest"] = publicViewDigest
	privateEvents[0]["itemId"] = itemID
	privateEvents[0]["item"] = map[string]any{
		"id": itemID, "threadId": "thread-semantic", "turnId": "turn-semantic",
		"role": "assistant", "status": "completed", "kind": "assistant_text", "text": "verified boundary",
		"createdAt": "2026-07-18T00:00:00Z", "finishedAt": "2026-07-18T00:00:00Z",
		"acceptedFinal": map[string]any{
			"schemaVersion": 5, "authorityPurpose": "analytix.case-final/v1", "authorityAlgorithm": "Ed25519",
			"authorityKeyId": digestAcceptedFinalBatchTest("v1-authority"), "authorityPublicKey": "fixture-public-key",
			"threadId": "thread-semantic", "turnId": "turn-semantic",
			"envelopeDigest": digestAcceptedFinalBatchTest("v1-envelope"),
			"contextDigest":  digestAcceptedFinalBatchTest("v1-context"), "contextEpoch": 1,
			"datasetSnapshotId": "dsv2_" + digestAcceptedFinalBatchTest("v1-dataset"),
			"variant":           "GeneralGuidanceAnswer", "terminalReason": "success",
			"renderedTextSha256": digestAcceptedFinalBatchTest("verified boundary"),
			"registrySequence":   0, "registryStateDigest": digestAcceptedFinalBatchTest("v1-registry"),
			"rendererVersion": "analytix.host-final-renderer/v2", "finalGateVersion": "analytix.final-evidence-gate/v4",
			"verifierVersion": "analytix.claim-verifier-policy/v1",
			"publicView":      publicViewCore, "publicViewDigest": publicViewDigest,
			"privateRecordDigest": digestAcceptedFinalBatchTest("v1-private"),
			"acceptedAt":          "2026-07-18T00:00:00Z", "authoritySignature": "fixture-signature",
			"recordDigest": commitID,
		},
		"acceptedFinalView": publicView,
	}
	rehashAcceptedFinalSemanticEventsForTest(privateEvents)
	privateKey := ed25519.NewKeyFromSeed(make([]byte, ed25519.SeedSize))
	publicKey := privateKey.Public().(ed25519.PublicKey)
	sealV1, err := NewAcceptedFinalDeliverySealForEventsV1(
		privateEvents, digestAcceptedFinalBatchTest("v1-disposition"), digestAcceptedFinalBatchTest("v1-terminal"),
		digestAcceptedFinalBatchBytesTest(publicKey), publicKey,
		func(message []byte) ([]byte, error) { return ed25519.Sign(privateKey, message), nil },
	)
	if err != nil {
		t.Fatal(err)
	}
	batchV1, err := NewAcceptedFinalDeliveryBatchV1(privateEvents, sealV1)
	if err != nil {
		t.Fatal(err)
	}
	wiredV1 := AcceptedFinalDeliveryBatchV1Map(batchV1)
	if _, err := ParseAcceptedFinalDeliveryBatchV1(wiredV1); err != nil {
		t.Fatalf("frozen historical V1 was rejected: %v", err)
	}
	if _, err := ParseAcceptedFinalDeliveryBatchV2(wiredV1); err == nil {
		t.Fatal("historical private V1 was accepted as current public V2")
	}
	for name, mutate := range map[string]func([]map[string]any){
		"missing private record": func(events []map[string]any) {
			delete(events[0]["item"].(map[string]any), "acceptedFinal")
		},
		"partial private record": func(events []map[string]any) {
			delete(events[0]["item"].(map[string]any)["acceptedFinal"].(map[string]any), "authoritySignature")
		},
		"private terminal mismatch": func(events []map[string]any) {
			events[0]["item"].(map[string]any)["acceptedFinal"].(map[string]any)["terminalReason"] = "provider_failure"
		},
		"private acceptedAt mismatch": func(events []map[string]any) {
			events[0]["item"].(map[string]any)["acceptedFinal"].(map[string]any)["acceptedAt"] = "2026-07-18T00:00:01Z"
		},
		"missing expanded view": func(events []map[string]any) {
			delete(events[0]["item"].(map[string]any), "acceptedFinalView")
		},
		"detached expanded digest": func(events []map[string]any) {
			events[0]["item"].(map[string]any)["acceptedFinalView"].(map[string]any)["acceptedFinalDigest"] =
				digestAcceptedFinalBatchTest("detached-v1-final")
		},
		"expanded unknown field": func(events []map[string]any) {
			events[0]["item"].(map[string]any)["acceptedFinalView"].(map[string]any)["privateExtra"] = true
		},
		"expanded V3 view": func(events []map[string]any) {
			events[0]["item"].(map[string]any)["acceptedFinalView"] = acceptedFinalPublicViewV3ForBatchTest(
				contracts.StringField(events[0], "publicationCommitId"), "success", "2026-07-18T00:00:00Z",
			)
		},
		"assistant role": func(events []map[string]any) {
			events[0]["item"].(map[string]any)["role"] = "system"
		},
		"assistant kind": func(events []map[string]any) {
			events[0]["item"].(map[string]any)["kind"] = "assistant_reasoning"
		},
		"assistant status": func(events []map[string]any) {
			events[0]["item"].(map[string]any)["status"] = "failed"
		},
		"assistant finishedAt": func(events []map[string]any) {
			events[0]["item"].(map[string]any)["finishedAt"] = "2026-07-18T00:00:01Z"
		},
		"event unknown field": func(events []map[string]any) {
			events[0]["privateExtra"] = true
		},
		"item unknown field": func(events []map[string]any) {
			events[0]["item"].(map[string]any)["privateExtra"] = true
		},
		"record unknown field": func(events []map[string]any) {
			events[0]["item"].(map[string]any)["acceptedFinal"].(map[string]any)["privateExtra"] = true
		},
	} {
		t.Run(name, func(t *testing.T) {
			hostile := cloneAcceptedFinalDeliveryEvents(privateEvents)
			mutate(hostile)
			rehashAcceptedFinalSemanticEventsForTest(hostile)
			if ValidateAcceptedFinalDeliveryEventsV1(hostile) == nil {
				t.Fatal("hostile historical V1 assistant tuple was accepted")
			}
		})
	}

	publicEvents := acceptedFinalSemanticEventsForTest(t, "success", "completed")
	sealV2, err := NewAcceptedFinalDeliverySealForEventsV2(
		publicEvents, digestAcceptedFinalBatchTest("v2-disposition"), digestAcceptedFinalBatchTest("v2-terminal"),
		digestAcceptedFinalBatchBytesTest(publicKey), publicKey,
		func(message []byte) ([]byte, error) { return ed25519.Sign(privateKey, message), nil },
	)
	if err != nil {
		t.Fatal(err)
	}
	batchV2, err := NewAcceptedFinalDeliveryBatchV2(publicEvents, sealV2)
	if err != nil {
		t.Fatal(err)
	}
	wiredV2 := AcceptedFinalDeliveryBatchV2Map(batchV2)
	if _, err := ParseAcceptedFinalDeliveryBatchV2(wiredV2); err != nil {
		t.Fatalf("current public V2 was rejected: %v", err)
	}
	if _, err := ParseAcceptedFinalDeliveryBatchV1(wiredV2); err == nil {
		t.Fatal("current public V2 was accepted as historical private V1")
	}
	v1WrapperWithV3Event := contracts.CloneMap(wiredV2)
	v1WrapperWithV3Event["schemaVersion"] = 1
	v1WrapperWithV3Event["purpose"] = AcceptedFinalDeliveryBatchV1Purpose
	v1WrapperWithV3EventBefore, _ := json.Marshal(v1WrapperWithV3Event)
	if _, err := ParseAcceptedFinalDeliveryBatchV1(v1WrapperWithV3Event); err == nil {
		t.Fatal("V1 wrapper carrying a V3 assistant event was accepted as historical private V1")
	}
	if _, err := ParseAcceptedFinalDeliveryBatchV2(v1WrapperWithV3Event); err == nil {
		t.Fatal("V1 wrapper carrying a V3 assistant event was accepted as current public V2")
	}
	v1WrapperWithV3EventAfter, _ := json.Marshal(v1WrapperWithV3Event)
	if !reflect.DeepEqual(v1WrapperWithV3EventBefore, v1WrapperWithV3EventAfter) {
		t.Fatal("rejected V1 wrapper carrying a V3 assistant event was rewritten")
	}
	mixed := []map[string]any{contracts.CloneMap(wiredV1), contracts.CloneMap(wiredV2)}
	before, _ := json.Marshal(mixed)
	parsedV1, v1Err := ParseAcceptedFinalDeliveryBatchV1(mixed[0])
	parsedV2, v2Err := ParseAcceptedFinalDeliveryBatchV2(mixed[1])
	after, _ := json.Marshal(mixed)
	if v1Err != nil || v2Err != nil ||
		parsedV1.SchemaVersion != AcceptedFinalDeliveryBatchV1Version ||
		parsedV1.Purpose != AcceptedFinalDeliveryBatchV1Purpose ||
		parsedV2.SchemaVersion != AcceptedFinalDeliveryBatchV2Version ||
		parsedV2.Purpose != AcceptedFinalDeliveryBatchV2Purpose ||
		!reflect.DeepEqual(before, after) {
		t.Fatalf("mixed persisted delivery families were rewritten: v1=%#v err=%v v2=%#v err=%v before=%s after=%s", parsedV1, v1Err, parsedV2, v2Err, before, after)
	}
}

func TestAcceptedFinalDeliveryBatchV2ClosesEveryTerminalDisposition(t *testing.T) {
	for _, disposition := range domainterminal.AllDispositionsV1() {
		t.Run(disposition.Reason, func(t *testing.T) {
			events := acceptedFinalSemanticEventsForTest(t, disposition.Reason, disposition.Status)
			if err := ValidateAcceptedFinalDeliveryEventsV2(events); err != nil {
				t.Fatalf("canonical %s delivery was rejected: %v\n%#v", disposition.Reason, err, events)
			}
			requiresError, ok := domainterminal.AcceptedFinalDeliveryRequiresErrorItemV1(disposition.Reason)
			if !ok || len(events) != 3+boolIntAcceptedFinalBatchTest(requiresError) {
				t.Fatalf("canonical %s slot profile drifted: %#v", disposition.Reason, events)
			}
		})
	}
}

func TestAcceptedFinalPublicViewV3WireClosesEveryVariantAndHostileCombination(t *testing.T) {
	for _, variant := range []string{
		"EvidenceBackedAnswer",
		"PartialEvidenceAnswer",
		"VerifiedNoHitAnswer",
		"SourceUnavailableAnswer",
		"NeedsEvidenceAnswer",
		"GeneralGuidanceAnswer",
	} {
		t.Run("canonical "+variant, func(t *testing.T) {
			view := acceptedFinalPublicViewV3VariantForBatchTest(variant)
			if !validAcceptedFinalPublicViewV3Wire(view) {
				t.Fatalf("canonical %s public view was rejected: %#v", variant, view)
			}
		})
	}

	canonical := acceptedFinalPublicViewV3VariantForBatchTest("GeneralGuidanceAnswer")
	for key := range acceptedFinalPublicViewV3WireKeys {
		t.Run("missing "+key, func(t *testing.T) {
			view := cloneAcceptedFinalPublicViewV3ForBatchTest(canonical)
			delete(view, key)
			if validAcceptedFinalPublicViewV3Wire(view) {
				t.Fatalf("public view missing required key %q was accepted", key)
			}
		})
	}
	t.Run("extra key", func(t *testing.T) {
		view := cloneAcceptedFinalPublicViewV3ForBatchTest(canonical)
		view["privateExtra"] = true
		if validAcceptedFinalPublicViewV3Wire(view) {
			t.Fatal("public view carrying an extra key was accepted")
		}
	})

	wrongTypes := map[string]any{
		"schemaVersion":       "3",
		"acceptedFinalDigest": false,
		"publicationState":    3,
		"variant":             false,
		"terminalReason":      false,
		"blockerCode":         false,
		"coverageStatus":      false,
		"checkedScopeDigest":  false,
		"missingScopeCount":   "0",
		"claimCount":          "0",
		"claimTypes":          map[string]any{},
		"receiptMetadata":     []any{},
		"noHitWording":        false,
		"acceptedAt":          float64(1),
	}
	for key, value := range wrongTypes {
		t.Run("wrong type "+key, func(t *testing.T) {
			view := cloneAcceptedFinalPublicViewV3ForBatchTest(canonical)
			view[key] = value
			if validAcceptedFinalPublicViewV3Wire(view) {
				t.Fatalf("public view accepted wrong type for %q: %#v", key, value)
			}
		})
	}

	for name, value := range map[string]any{
		"negative zero": math.Copysign(0, -1),
		"negative":      float64(-1),
		"above safe":    float64(1 << 53),
	} {
		for _, field := range []string{"missingScopeCount", "claimCount", "receipt count"} {
			t.Run(name+" "+field, func(t *testing.T) {
				view := cloneAcceptedFinalPublicViewV3ForBatchTest(canonical)
				if field == "receipt count" {
					view["receiptMetadata"].(map[string]any)["count"] = value
				} else {
					view[field] = value
				}
				if validAcceptedFinalPublicViewV3Wire(view) {
					t.Fatalf("public view accepted %s for %s", name, field)
				}
			})
		}
	}

	claimView := acceptedFinalPublicViewV3VariantForBatchTest("EvidenceBackedAnswer")
	for name, claimTypes := range map[string][]any{
		"unsorted":  {"amount", "account"},
		"duplicate": {"amount", "amount"},
		"unknown":   {"private_claim"},
	} {
		t.Run("claim types "+name, func(t *testing.T) {
			view := cloneAcceptedFinalPublicViewV3ForBatchTest(claimView)
			view["claimCount"] = 2
			view["claimTypes"] = claimTypes
			if validAcceptedFinalPublicViewV3Wire(view) {
				t.Fatalf("public view accepted %s claim types: %#v", name, claimTypes)
			}
		})
	}

	receiptView := acceptedFinalPublicViewV3VariantForBatchTest("VerifiedNoHitAnswer")
	for name, mutate := range map[string]func(map[string]any){
		"projection": func(metadata map[string]any) { metadata["projection"] = "raw" },
		"set digest": func(metadata map[string]any) { metadata["setDigest"] = "not-a-digest" },
		"count":      func(metadata map[string]any) { metadata["count"] = 2 },
		"label": func(metadata map[string]any) {
			metadata["citations"].([]any)[0].(map[string]any)["label"] = "evidence-2"
		},
		"handle": func(metadata map[string]any) {
			metadata["citations"].([]any)[0].(map[string]any)["handle"] = "receipt-private"
		},
		"citation extra": func(metadata map[string]any) {
			metadata["citations"].([]any)[0].(map[string]any)["privateExtra"] = true
		},
	} {
		t.Run("receipt "+name, func(t *testing.T) {
			view := cloneAcceptedFinalPublicViewV3ForBatchTest(receiptView)
			mutate(view["receiptMetadata"].(map[string]any))
			if validAcceptedFinalPublicViewV3Wire(view) {
				t.Fatalf("public view accepted hostile receipt %s: %#v", name, view)
			}
		})
	}
	t.Run("receipt duplicate handle", func(t *testing.T) {
		view := cloneAcceptedFinalPublicViewV3ForBatchTest(receiptView)
		metadata := view["receiptMetadata"].(map[string]any)
		first := metadata["citations"].([]any)[0].(map[string]any)
		metadata["count"] = 2
		metadata["citations"] = []any{
			first,
			map[string]any{"handle": first["handle"], "label": "evidence-2"},
		}
		if validAcceptedFinalPublicViewV3Wire(view) {
			t.Fatal("public view accepted duplicate receipt handles")
		}
	})

	for name, acceptedAt := range map[string]string{
		"offset":        "2026-07-18T08:00:00+08:00",
		"trailing zero": "2026-07-18T00:00:00.000Z",
	} {
		t.Run("acceptedAt "+name, func(t *testing.T) {
			view := cloneAcceptedFinalPublicViewV3ForBatchTest(canonical)
			view["acceptedAt"] = acceptedAt
			if validAcceptedFinalPublicViewV3Wire(view) {
				t.Fatalf("public view accepted non-canonical acceptedAt %q", acceptedAt)
			}
		})
	}

	for name, candidate := range map[string]map[string]any{
		"coverage mismatch": func() map[string]any {
			view := acceptedFinalPublicViewV3VariantForBatchTest("EvidenceBackedAnswer")
			view["coverageStatus"] = "partial"
			return view
		}(),
		"blocker mismatch": func() map[string]any {
			view := acceptedFinalPublicViewV3VariantForBatchTest("SourceUnavailableAnswer")
			view["blockerCode"] = ""
			return view
		}(),
		"scope mismatch": func() map[string]any {
			view := acceptedFinalPublicViewV3VariantForBatchTest("GeneralGuidanceAnswer")
			view["checkedScopeDigest"] = digestAcceptedFinalBatchTest("forged-scope")
			return view
		}(),
		"count mismatch": func() map[string]any {
			view := acceptedFinalPublicViewV3VariantForBatchTest("PartialEvidenceAnswer")
			view["missingScopeCount"] = 0
			return view
		}(),
		"no-hit mismatch": func() map[string]any {
			view := acceptedFinalPublicViewV3VariantForBatchTest("VerifiedNoHitAnswer")
			view["noHitWording"] = ""
			return view
		}(),
	} {
		t.Run(name, func(t *testing.T) {
			if validAcceptedFinalPublicViewV3Wire(candidate) {
				t.Fatalf("public view accepted hostile cross-combination %s: %#v", name, candidate)
			}
		})
	}
}

func TestAcceptedFinalDeliveryUsageUsesExactClosedReasoningEffort(t *testing.T) {
	for _, effort := range []string{"auto", "off", "low", "medium", "high", "max"} {
		events := acceptedFinalSemanticEventsForTest(t, "success", "completed")
		events[len(events)-2]["effort"] = effort
		rehashAcceptedFinalSemanticEventsForTest(events)
		if err := ValidateAcceptedFinalDeliveryEventsV2(events); err != nil {
			t.Fatalf("valid accepted-final usage effort %q was rejected: %v", effort, err)
		}
	}
	const sentinel = "SOL_PRIVATE_REASONING_SENTINEL_7F3C"
	for _, effort := range []any{"", " high ", "HIGH", sentinel, float64(1)} {
		events := acceptedFinalSemanticEventsForTest(t, "success", "completed")
		events[len(events)-2]["effort"] = effort
		rehashAcceptedFinalSemanticEventsForTest(events)
		if err := ValidateAcceptedFinalDeliveryEventsV2(events); err == nil {
			t.Fatalf("invalid accepted-final usage effort was accepted: %#v", effort)
		}
	}
}

func TestAcceptedFinalDeliveryBatchV2RejectsRehashedSemanticMutations(t *testing.T) {
	tests := []struct {
		name   string
		reason string
		status string
		mutate func([]map[string]any)
	}{
		{name: "usage status", reason: "provider_failure", status: "failed", mutate: func(events []map[string]any) {
			events[2]["usageFinalStatus"] = "completed"
		}},
		{name: "terminal kind", reason: "provider_failure", status: "failed", mutate: func(events []map[string]any) {
			events[3]["kind"] = "turn_completed"
		}},
		{name: "four slot completed reason", reason: "provider_failure", status: "failed", mutate: func(events []map[string]any) {
			events[3]["terminalReason"] = "success"
			events[3]["kind"] = "turn_completed"
			events[3]["status"] = "completed"
			events[2]["usageFinalStatus"] = "completed"
		}},
		{name: "error item event identity", reason: "provider_failure", status: "failed", mutate: func(events []map[string]any) {
			events[1]["itemId"] = "item-detached"
		}},
		{name: "error item digest", reason: "provider_failure", status: "failed", mutate: func(events []map[string]any) {
			events[1]["item"].(map[string]any)["acceptedFinalDigest"] = digestAcceptedFinalBatchTest("detached")
		}},
		{name: "error item status", reason: "provider_failure", status: "failed", mutate: func(events []map[string]any) {
			events[1]["item"].(map[string]any)["status"] = "completed"
		}},
		{name: "terminal item identity", reason: "provider_failure", status: "failed", mutate: func(events []map[string]any) {
			events[3]["itemId"] = "item-detached"
		}},
		{name: "terminal code", reason: "provider_failure", status: "failed", mutate: func(events []map[string]any) {
			events[3]["code"] = "detached_code"
		}},
		{name: "terminal text", reason: "provider_failure", status: "failed", mutate: func(events []map[string]any) {
			events[3]["message"] = "detached message"
		}},
		{name: "event timestamp", reason: "provider_failure", status: "failed", mutate: func(events []map[string]any) {
			events[2]["timestamp"] = "2026-07-18T00:00:01Z"
		}},
		{name: "accepted final digest binding", reason: "success", status: "completed", mutate: func(events []map[string]any) {
			events[0]["item"].(map[string]any)["acceptedFinalView"].(map[string]any)["acceptedFinalDigest"] =
				digestAcceptedFinalBatchTest("detached-final")
		}},
		{name: "accepted at binding", reason: "success", status: "completed", mutate: func(events []map[string]any) {
			events[0]["item"].(map[string]any)["acceptedFinalView"].(map[string]any)["acceptedAt"] =
				"2026-07-18T00:00:01Z"
		}},
		{name: "terminal reason binding", reason: "provider_failure", status: "failed", mutate: func(events []map[string]any) {
			events[0]["item"].(map[string]any)["acceptedFinalView"].(map[string]any)["terminalReason"] = "success"
		}},
		{name: "missing abort disposition", reason: "cancel", status: "aborted", mutate: func(events []map[string]any) {
			delete(events[3], "cancelledPendingGates")
		}},
		{name: "inconsistent cancel disposition", reason: "cancel", status: "aborted", mutate: func(events []map[string]any) {
			events[3]["cancelled"] = false
			events[3]["cancelledPendingGates"] = 1
		}},
		{name: "negative zero cancelled gate count", reason: "cancel", status: "aborted", mutate: func(events []map[string]any) {
			events[3]["cancelledPendingGates"] = math.Copysign(0, -1)
		}},
		{name: "completed abort disposition", reason: "success", status: "completed", mutate: func(events []map[string]any) {
			events[2]["discard"] = true
		}},
		{name: "three slot error identity", reason: "success", status: "completed", mutate: func(events []map[string]any) {
			events[2]["itemId"] = "item-invented"
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			events := acceptedFinalSemanticEventsForTest(t, test.reason, test.status)
			test.mutate(events)
			rehashAcceptedFinalSemanticEventsForTest(events)
			if err := ValidateAcceptedFinalDeliveryEventsV2(events); err == nil {
				t.Fatalf("self-consistent semantic mutation was accepted: %#v", events)
			}
		})
	}
}

func acceptedFinalSemanticEventsForTest(t *testing.T, reason, status string) []map[string]any {
	t.Helper()
	commitID := digestAcceptedFinalBatchTest("semantic-" + reason)
	timestamp := "2026-07-18T00:00:00Z"
	requiresError, ok := domainterminal.AcceptedFinalDeliveryRequiresErrorItemV1(reason)
	if !ok {
		t.Fatalf("unknown accepted-final reason %q", reason)
	}
	projection, ok := domainterminal.FailureProjectionV1(reason)
	if !ok || projection.Status != status {
		t.Fatalf("terminal projection drifted for %q/%q: %#v", reason, status, projection)
	}
	assistantItemID := "item-turn-semantic-assistant"
	events := []map[string]any{{
		"kind": "item_completed", "threadId": "thread-semantic", "turnId": "turn-semantic", "timestamp": timestamp,
		"itemId": assistantItemID,
		"item": map[string]any{
			"id": assistantItemID, "threadId": "thread-semantic", "turnId": "turn-semantic",
			"role": "assistant", "kind": "assistant_text", "text": "verified boundary", "status": "completed",
			"acceptedFinalView": acceptedFinalPublicViewV3ForBatchTest(commitID, reason, timestamp),
		},
	}}
	if requiresError {
		itemID := "item_turn-semantic_case_terminal"
		events = append(events, map[string]any{
			"kind": "item_completed", "threadId": "thread-semantic", "turnId": "turn-semantic", "timestamp": timestamp,
			"itemId": itemID,
			"item": map[string]any{
				"id": itemID, "threadId": "thread-semantic", "turnId": "turn-semantic", "role": "system",
				"kind": "error", "status": status, "createdAt": timestamp, "finishedAt": timestamp,
				"code": projection.Code, "message": projection.Message, "severity": projection.Severity,
				"acceptedFinalDigest": commitID,
			},
		})
	}
	events = append(events, map[string]any{
		"kind": "usage", "threadId": "thread-semantic", "turnId": "turn-semantic", "timestamp": timestamp,
		"usageFinalStatus": status,
	})
	terminalEvent := map[string]any{
		"kind": "turn_" + status, "threadId": "thread-semantic", "turnId": "turn-semantic", "timestamp": timestamp,
		"status": status, "terminalReason": reason, "code": projection.Code,
	}
	if requiresError {
		terminalEvent["itemId"] = "item_turn-semantic_case_terminal"
	}
	if status == "failed" {
		terminalEvent["error"] = projection.Message
		terminalEvent["message"] = projection.Message
	}
	if status == "aborted" {
		terminalEvent["discard"] = true
		terminalEvent["cancelled"] = true
		terminalEvent["cancelledPendingGates"] = 2
	}
	events = append(events, terminalEvent)
	for index, event := range events {
		event["seq"] = index + 1
		event["acceptedFinalDigest"] = commitID
		event["publicationCommitId"] = commitID
	}
	rehashAcceptedFinalSemanticEventsForTest(events)
	return events
}

func acceptedFinalPublicViewV3ForBatchTest(commitID, terminalReason, acceptedAt string) map[string]any {
	return map[string]any{
		"schemaVersion": 3, "acceptedFinalDigest": commitID, "publicationState": "accepted",
		"variant": "GeneralGuidanceAnswer", "terminalReason": terminalReason, "blockerCode": "",
		"coverageStatus": "guidance_only", "checkedScopeDigest": "", "missingScopeCount": 0,
		"claimCount": 0, "claimTypes": []any{},
		"receiptMetadata": map[string]any{
			"projection": "masked_metadata_only", "count": 0,
			"setDigest": digestAcceptedFinalBatchTest("empty-receipts"), "citations": []any{},
		},
		"noHitWording": "", "acceptedAt": acceptedAt,
	}
}

func acceptedFinalPublicViewV3VariantForBatchTest(variant string) map[string]any {
	view := acceptedFinalPublicViewV3ForBatchTest(
		digestAcceptedFinalBatchTest("view-"+variant),
		"success",
		"2026-07-18T00:00:00Z",
	)
	metadata := view["receiptMetadata"].(map[string]any)
	switch variant {
	case "EvidenceBackedAnswer", "PartialEvidenceAnswer":
		view["variant"] = variant
		view["coverageStatus"] = "complete"
		if variant == "PartialEvidenceAnswer" {
			view["coverageStatus"] = "partial"
			view["missingScopeCount"] = 1
		}
		view["checkedScopeDigest"] = digestAcceptedFinalBatchTest("scope-" + variant)
		view["claimCount"] = 1
		view["claimTypes"] = []any{"amount"}
		metadata["count"] = 1
		metadata["setDigest"] = digestAcceptedFinalBatchTest("receipts-" + variant)
		metadata["citations"] = []any{map[string]any{
			"handle": "cite_" + digestAcceptedFinalBatchTest("receipt-"+variant),
			"label":  "evidence-1",
		}}
	case "VerifiedNoHitAnswer":
		view["variant"] = variant
		view["coverageStatus"] = "complete"
		view["checkedScopeDigest"] = digestAcceptedFinalBatchTest("scope-" + variant)
		view["noHitWording"] = "not_found_in_checked_scope"
		metadata["count"] = 1
		metadata["setDigest"] = digestAcceptedFinalBatchTest("receipts-" + variant)
		metadata["citations"] = []any{map[string]any{
			"handle": "cite_" + digestAcceptedFinalBatchTest("receipt-"+variant),
			"label":  "evidence-1",
		}}
	case "SourceUnavailableAnswer":
		view["variant"] = variant
		view["coverageStatus"] = "unavailable"
		view["blockerCode"] = "source_unavailable"
	case "NeedsEvidenceAnswer":
		view["variant"] = variant
		view["coverageStatus"] = "unverified"
		view["missingScopeCount"] = 1
	case "GeneralGuidanceAnswer":
	default:
		view["variant"] = variant
	}
	return view
}

func cloneAcceptedFinalPublicViewV3ForBatchTest(view map[string]any) map[string]any {
	body, _ := json.Marshal(view)
	var cloned map[string]any
	_ = json.Unmarshal(body, &cloned)
	return cloned
}

func rehashAcceptedFinalSemanticEventsForTest(events []map[string]any) {
	slots := acceptedFinalDeliverySlots
	if len(events) == 4 {
		slots = acceptedFinalFailureDeliverySlots
	}
	for index, event := range events {
		event["publicationSlot"] = slots[index]
		event["publicationEventId"] = acceptedFinalDeliveryEventID(contractsStringAcceptedFinalBatchTest(event, "publicationCommitId"), slots[index])
		delete(event, "publicationPayloadDigest")
		event["publicationPayloadDigest"] = acceptedFinalDeliveryPayloadDigest(event)
	}
}

func contractsStringAcceptedFinalBatchTest(record map[string]any, key string) string {
	value, _ := record[key].(string)
	return value
}

func boolIntAcceptedFinalBatchTest(value bool) int {
	if value {
		return 1
	}
	return 0
}

func digestAcceptedFinalBatchTest(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}

func digestAcceptedFinalBatchBytesTest(value []byte) string {
	digest := sha256.Sum256(value)
	return hex.EncodeToString(digest[:])
}
