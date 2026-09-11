package cachetelemetry

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"
)

func TestProviderCallObservationV1StrictRoundTrip(t *testing.T) {
	original := validObservation()
	body, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeProviderCallObservationV1(body)
	if err != nil {
		t.Fatal(err)
	}
	if decoded != original {
		t.Fatalf("round trip changed observation:\n got: %#v\nwant: %#v", decoded, original)
	}
}

func TestProviderCallObservationV1RejectsUnknownMissingAndDuplicateProperties(t *testing.T) {
	body := observationJSONMap(t, validObservation())

	t.Run("unknown top-level property", func(t *testing.T) {
		candidate := cloneJSONMap(t, body)
		candidate["prompt"] = "case text"
		assertObservationDecodeFails(t, candidate)
	})

	t.Run("unknown nested property", func(t *testing.T) {
		candidate := cloneJSONMap(t, body)
		usage := candidate["usage"].(map[string]any)
		input := usage["inputTokens"].(map[string]any)
		input["raw"] = "provider payload"
		assertObservationDecodeFails(t, candidate)
	})

	t.Run("missing required property", func(t *testing.T) {
		candidate := cloneJSONMap(t, body)
		shape := candidate["shape"].(map[string]any)
		delete(shape, "modelHmac")
		assertObservationDecodeFails(t, candidate)
	})

	t.Run("missing nested required property", func(t *testing.T) {
		candidate := cloneJSONMap(t, body)
		usage := candidate["usage"].(map[string]any)
		count := usage["cacheHitTokens"].(map[string]any)
		delete(count, "known")
		assertObservationDecodeFails(t, candidate)
	})

	t.Run("null cannot impersonate unknown usage", func(t *testing.T) {
		candidate := cloneJSONMap(t, body)
		usage := candidate["usage"].(map[string]any)
		count := usage["cacheHitTokens"].(map[string]any)
		count["known"] = nil
		count["value"] = nil
		assertObservationDecodeFails(t, candidate)
	})

	t.Run("duplicate property", func(t *testing.T) {
		raw, err := json.Marshal(validObservation())
		if err != nil {
			t.Fatal(err)
		}
		duplicated := strings.Replace(
			string(raw),
			`"status":"succeeded"`,
			`"status":"succeeded","status":"failed"`,
			1,
		)
		if _, err := DecodeProviderCallObservationV1([]byte(duplicated)); err == nil || !strings.Contains(err.Error(), "duplicate property") {
			t.Fatalf("expected duplicate property rejection, got %v", err)
		}
	})

	t.Run("trailing JSON", func(t *testing.T) {
		raw, err := json.Marshal(validObservation())
		if err != nil {
			t.Fatal(err)
		}
		if _, err := DecodeProviderCallObservationV1(append(raw, []byte(` {}`)...)); err == nil {
			t.Fatal("expected trailing JSON rejection")
		}
	})
}

func TestTokenCountV1DistinguishesUnknownFromExplicitZero(t *testing.T) {
	unknown := validObservation()
	unknown.Usage.CacheHitTokens = TokenCountV1{Known: false, Value: 0}
	unknown.Usage.CacheMissTokens = TokenCountV1{Known: false, Value: 0}
	explicitZero := validObservation()
	explicitZero.Usage.CacheHitTokens = TokenCountV1{Known: true, Value: 0}
	explicitZero.Usage.CacheMissTokens = TokenCountV1{Known: true, Value: 100}

	unknownBody, err := json.Marshal(unknown)
	if err != nil {
		t.Fatal(err)
	}
	zeroBody, err := json.Marshal(explicitZero)
	if err != nil {
		t.Fatal(err)
	}
	decodedUnknown, err := DecodeProviderCallObservationV1(unknownBody)
	if err != nil {
		t.Fatal(err)
	}
	decodedZero, err := DecodeProviderCallObservationV1(zeroBody)
	if err != nil {
		t.Fatal(err)
	}
	if decodedUnknown.Usage.CacheHitTokens.Known {
		t.Fatal("unknown counter was upgraded to known")
	}
	if !decodedZero.Usage.CacheHitTokens.Known || decodedZero.Usage.CacheHitTokens.Value != 0 {
		t.Fatal("explicit zero counter was not preserved")
	}

	invalid := unknown
	invalid.Usage.CacheHitTokens.Value = 1
	if err := invalid.Validate(); err == nil {
		t.Fatal("expected unknown nonzero counter rejection")
	}
}

func TestCacheVisibleShapeV1RejectsRawOrNoncanonicalWireIdentity(t *testing.T) {
	tests := map[string]func(*CacheVisibleShapeV1){
		"raw endpoint URL": func(shape *CacheVisibleShapeV1) {
			shape.Endpoint = EndpointFormatV1("https://api.example.test/v1/chat?key=secret")
		},
		"uppercase HMAC": func(shape *CacheVisibleShapeV1) {
			shape.WireBodyHMAC = strings.Repeat("A", 64)
		},
		"missing digest epoch": func(shape *CacheVisibleShapeV1) {
			shape.DigestEpoch = 0
		},
		"noncanonical time": func(shape *CacheVisibleShapeV1) {
			shape.StartedAt = "2026-07-13T00:00:00+00:00"
		},
		"unsafe model text": func(shape *CacheVisibleShapeV1) {
			shape.ModelHMAC = "model with case text"
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			shape := validShape("logical-call-1", 1, "2026-07-13T00:00:00Z")
			mutate(&shape)
			if err := shape.Validate(); err == nil {
				t.Fatal("expected invalid shape rejection")
			}
		})
	}
}

func TestProviderCallObservationV1RejectsNonterminalOrBackwardsTime(t *testing.T) {
	invalidStatus := validObservation()
	invalidStatus.Status = "pending"
	if err := invalidStatus.Validate(); err == nil {
		t.Fatal("expected nonterminal status rejection")
	}

	backwards := validObservation()
	backwards.SettledAt = "2026-07-12T23:59:59Z"
	if err := backwards.Validate(); err == nil {
		t.Fatal("expected backwards settlement rejection")
	}
}

func TestProviderCallObservationV1RejectsContradictoryCacheUsage(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*ProviderCallObservationV1)
	}{
		{
			name: "hit plus miss exceeds input",
			mutate: func(value *ProviderCallObservationV1) {
				value.Usage.CacheMissTokens.Value = 11
			},
		},
		{
			name: "DeepSeek coverage is incomplete",
			mutate: func(value *ProviderCallObservationV1) {
				value.Usage.CacheMissTokens.Value = 9
			},
		},
		{
			name: "one cache counter is unknown",
			mutate: func(value *ProviderCallObservationV1) {
				value.Usage.CacheMissTokens = TokenCountV1{}
			},
		},
		{
			name: "input counter is unknown",
			mutate: func(value *ProviderCallObservationV1) {
				value.Usage.InputTokens = TokenCountV1{}
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			observation := validObservation()
			test.mutate(&observation)
			if err := observation.Validate(); err == nil {
				t.Fatal("contradictory cache usage was accepted")
			}
		})
	}
}

func TestProviderCallObservationV1HasNoRawDataFields(t *testing.T) {
	body, err := json.Marshal(validObservation())
	if err != nil {
		t.Fatal(err)
	}
	serialized := strings.ToLower(string(body))
	for _, forbidden := range []string{
		"prompt", "reasoningcontent", "reasoning_content", `"credential":`, `"credentials":`, "apikey", "api_key",
		"authorization", "requestbody", "responsebody", "endpointurl", "https://",
		`"provider":`, `"model":`, "logicalcallid", "digestkeyid", "deepseek-chat",
	} {
		if strings.Contains(serialized, forbidden) {
			t.Fatalf("serialized observation contains forbidden raw-data field/value %q: %s", forbidden, body)
		}
	}
}

func validObservation() ProviderCallObservationV1 {
	return ProviderCallObservationV1{
		SchemaVersion: ProviderCallObservationV1SchemaVersion,
		Shape:         validShape("logical-call-1", 1, "2026-07-13T00:00:00Z"),
		Status:        ProviderCallStatusSucceeded,
		Usage: ProviderUsageV1{
			InputTokens:     TokenCountV1{Known: true, Value: 100},
			OutputTokens:    TokenCountV1{Known: true, Value: 20},
			CacheHitTokens:  TokenCountV1{Known: true, Value: 90},
			CacheMissTokens: TokenCountV1{Known: true, Value: 10},
			ReasoningTokens: TokenCountV1{Known: false, Value: 0},
		},
		SettledAt: "2026-07-13T00:00:01Z",
	}
}

func validShape(logicalCallID string, attempt uint32, startedAt string) CacheVisibleShapeV1 {
	return CacheVisibleShapeV1{
		SchemaVersion:       CacheVisibleShapeV1SchemaVersion,
		LogicalCallHMAC:     testHMAC(logicalCallID),
		Attempt:             attempt,
		ProviderFamily:      ProviderFamilyDeepSeek,
		ModelHMAC:           strings.Repeat("c", 64),
		Endpoint:            EndpointFormatChatCompletions,
		EndpointHMAC:        strings.Repeat("a", 64),
		WireBodyHMAC:        strings.Repeat("b", 64),
		CredentialScopeHMAC: strings.Repeat("d", 64),
		ProviderConfigHMAC:  strings.Repeat("e", 64),
		DigestEpoch:         1,
		StartedAt:           startedAt,
	}
}

func testHMAC(value string) string {
	digest := sha256.Sum256([]byte("cache-telemetry-test-hmac:" + value))
	return hex.EncodeToString(digest[:])
}

func observationJSONMap(t *testing.T, observation ProviderCallObservationV1) map[string]any {
	t.Helper()
	body, err := json.Marshal(observation)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatal(err)
	}
	return decoded
}

func cloneJSONMap(t *testing.T, value map[string]any) map[string]any {
	t.Helper()
	body, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var clone map[string]any
	if err := json.Unmarshal(body, &clone); err != nil {
		t.Fatal(err)
	}
	return clone
}

func assertObservationDecodeFails(t *testing.T, value map[string]any) {
	t.Helper()
	body, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeProviderCallObservationV1(body); err == nil {
		t.Fatalf("expected strict decode failure for %s", body)
	}
}
