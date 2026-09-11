package usage

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	domainevent "analytix.local/runtime-go/internal/domain/event"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
)

func TestTerminalTelemetryV1AcceptsOnlyClosedHostDiagnostics(t *testing.T) {
	diagnostics := map[string]any{
		"prefixHash":                   strings.Repeat("a", 64),
		"provider":                     "deepseek",
		"prefixChanged":                false,
		"toolSourceIds":                []string{"builtin", "mcp:funds"},
		"cacheTelemetrySupported":      true,
		"cacheTelemetryPresent":        true,
		"providerNativeCacheTelemetry": true,
		"cacheHitTokens":               80,
		"cacheMissTokens":              20,
		"cacheHitRate":                 0.8,
		"cacheTelemetrySource":         "provider_usage",
		"cacheBaselineSchema":          PrefixBaselineSchemaV1,
		"cacheContinuityDigest":        strings.Repeat("b", 64),
		"cacheProviderNamespaceDigest": strings.Repeat("c", 64),
		"cacheBaselineObserved":        true,
		"contextEpochStateValid":       true,
		"contextEpoch":                 uint64(7),
		"contextEpochDigest":           strings.Repeat("d", 64),
		"contextEpochRegistryDigest":   strings.Repeat("e", 64),
		"contextEpochChangeReasons":    []string{"source-digest-changed"},
		"contextEpochImpact": map[string]any{
			"stablePrefix": false, "dynamicContext": true, "turnTail": false, "diagnosticsOnly": false,
		},
	}
	projection := NewTerminalTelemetryV1(domainmodel.Usage{
		PromptTokens: 100, CompletionTokens: 25, TotalTokens: 125,
		CacheHitTokens: 80, CacheMissTokens: 20, HasCacheHit: true, HasCacheMiss: true,
	}, diagnostics)

	want := map[string]any{
		"prefixHash": strings.Repeat("a", 64), "prefixChanged": false,
		"cacheTelemetrySupported": true, "cacheTelemetryPresent": true, "providerNativeCacheTelemetry": true,
		"cacheHitTokens": uint64(80), "cacheMissTokens": uint64(20), "cacheHitRate": 0.8,
		"cacheTelemetrySource": "provider_usage", "cacheBaselineSchema": PrefixBaselineSchemaV1,
		"cacheContinuityDigest": strings.Repeat("b", 64), "cacheProviderNamespaceDigest": strings.Repeat("c", 64),
		"cacheBaselineObserved": true, "contextEpochStateValid": true, "contextEpoch": uint64(7),
		"contextEpochDigest": strings.Repeat("d", 64), "contextEpochRegistryDigest": strings.Repeat("e", 64),
		"contextEpochChangeReasons": []string{"source-digest-changed"},
		"contextEpochImpact": map[string]any{
			"stablePrefix": false, "dynamicContext": true, "turnTail": false, "diagnosticsOnly": false,
		},
	}
	got := projection.PublicCacheDiagnosticsMap()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("closed terminal diagnostics changed: got=%#v want=%#v", got, want)
	}
	got["provider"] = "mutated"
	got["contextEpochImpact"].(map[string]any)["dynamicContext"] = false
	second := projection.PublicCacheDiagnosticsMap()
	if _, exposed := second["provider"]; exposed || second["contextEpochImpact"].(map[string]any)["dynamicContext"] != true {
		t.Fatalf("terminal diagnostics were not defensively cloned: %#v", second)
	}
	usage := projection.PublicUsageMap()
	if usage["promptTokens"] != 100 || usage["cacheHitTokens"] != 80 || usage["cacheMissTokens"] != 20 {
		t.Fatalf("terminal usage projection mismatch: %#v", usage)
	}
	if !ValidateTerminalTelemetryPublicMapsV1(usage, second) {
		t.Fatalf("accepted closed terminal telemetry cannot pass its replay guard: usage=%#v diagnostics=%#v", usage, second)
	}
	body, err := json.Marshal(map[string]any{"usage": usage, "diagnostics": second})
	if err != nil {
		t.Fatal(err)
	}
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	decoder.UseNumber()
	var replay map[string]map[string]any
	if err := decoder.Decode(&replay); err != nil ||
		!ValidateTerminalTelemetryPublicMapsV1(replay["usage"], replay["diagnostics"]) {
		t.Fatalf("accepted persisted terminal telemetry was rejected: replay=%#v err=%v", replay, err)
	}
}

func TestTerminalTelemetryV1KnownStringFieldsCannotLaunderContent(t *testing.T) {
	sentinel := "PRIVATE_REASONING_SENTINEL_ACCOUNT_6222020000000000000"
	for _, test := range []struct {
		name        string
		diagnostics map[string]any
		rejected    bool
	}{
		{name: "provider identity dropped", diagnostics: map[string]any{"provider": sentinel}},
		{name: "model identity dropped", diagnostics: map[string]any{"model": sentinel}},
		{name: "route requires closed enum", diagnostics: map[string]any{"route": sentinel}, rejected: true},
		{name: "tool source ids dropped", diagnostics: map[string]any{"toolSourceIds": []string{sentinel}}},
		{name: "hash requires digest", diagnostics: map[string]any{"prefixHash": sentinel}, rejected: true},
		{name: "reason requires enum", diagnostics: map[string]any{"prefixChangeReasons": []string{sentinel}}, rejected: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			projection := NewTerminalTelemetryV1(domainmodel.Usage{}, test.diagnostics)
			got := projection.PublicCacheDiagnosticsMap()
			if err := domainevent.ValidatePublicRecord(got); err != nil {
				t.Fatalf("closed terminal diagnostics are not publicly valid: diagnostics=%#v err=%v", got, err)
			}
			if strings.Contains(firstTerminalDiagnosticString(got), sentinel) {
				t.Fatalf("known cache key laundered private content: %#v", got)
			}
			_, markedRejected := got["terminalCacheDiagnosticsDisposition"]
			if markedRejected != test.rejected {
				t.Fatalf("known cache key rejection mismatch: got=%#v rejected=%v", got, test.rejected)
			}
		})
	}
}

func TestTerminalTelemetryV1RetainsOnlyClosedPromptRoutes(t *testing.T) {
	for _, route := range []string{"direct_answer", "light_agent", "tool_agent", "subagent_agent"} {
		projection := NewTerminalTelemetryV1(domainmodel.Usage{}, map[string]any{"route": route})
		if got := projection.PublicCacheDiagnosticsMap(); got["route"] != route {
			t.Fatalf("closed prompt route %q was not retained: %#v", route, got)
		}
	}
	for _, route := range []any{"agent", "DIRECT_ANSWER", "direct_answer ", 1} {
		projection := NewTerminalTelemetryV1(domainmodel.Usage{}, map[string]any{"route": route})
		got := projection.PublicCacheDiagnosticsMap()
		if got["terminalCacheDiagnosticsDisposition"] != terminalCacheDiagnosticsRejected || got["route"] != nil {
			t.Fatalf("open prompt route was not rejected without echo: route=%#v diagnostics=%#v", route, got)
		}
	}
}

func TestTerminalTelemetryV1RejectsLegacyShortPrefixFingerprint(t *testing.T) {
	projection := NewTerminalTelemetryV1(domainmodel.Usage{}, map[string]any{
		"prefixHash": strings.Repeat("a", 16),
	})
	got := projection.PublicCacheDiagnosticsMap()
	if got["terminalCacheDiagnosticsDisposition"] != terminalCacheDiagnosticsRejected || got["prefixHash"] != nil {
		t.Fatalf("legacy short prefix fingerprint did not fail closed: %#v", got)
	}
}

func TestTerminalTelemetryV1HasNoOpenOrProviderPayloadField(t *testing.T) {
	forbidden := map[reflect.Type]bool{
		reflect.TypeOf(domainmodel.Result{}):   true,
		reflect.TypeOf(domainmodel.Chunk{}):    true,
		reflect.TypeOf(domainmodel.ToolCall{}): true,
	}
	var inspect func(reflect.Type, string)
	inspect = func(value reflect.Type, path string) {
		if forbidden[value] {
			t.Fatalf("terminal telemetry contains provider payload type at %s: %s", path, value)
		}
		switch value.Kind() {
		case reflect.Map, reflect.Interface:
			t.Fatalf("terminal telemetry contains open type at %s: %s", path, value)
		case reflect.Pointer:
			inspect(value.Elem(), path+".*")
		case reflect.Array, reflect.Slice:
			if value.Elem().Kind() == reflect.Uint8 {
				t.Fatalf("terminal telemetry contains raw bytes at %s: %s", path, value)
			}
			inspect(value.Elem(), path+"[]")
		case reflect.Struct:
			for index := 0; index < value.NumField(); index++ {
				field := value.Field(index)
				inspect(field.Type, path+"."+field.Name)
			}
		}
	}
	inspect(reflect.TypeOf(TerminalTelemetryV1{}), "TerminalTelemetryV1")
}

func TestTerminalTelemetryV1RejectsUnknownProviderPayloadWithoutEcho(t *testing.T) {
	sentinel := "PRIVATE_REASONING_AND_CASE_CLAIM_MUST_NOT_PERSIST"
	projection := NewTerminalTelemetryV1(
		domainmodel.Usage{PromptTokens: -1, TotalTokens: -1, FinishReason: sentinel},
		map[string]any{
			"reasoningContent": sentinel,
			"providerClaim":    "account 6222020000000000000 has amount 2645472",
		},
	)
	diagnostics := projection.PublicCacheDiagnosticsMap()
	want := map[string]any{
		"terminalCacheDiagnosticsSchema":      TerminalCacheDiagnosticsSchemaV1,
		"terminalCacheDiagnosticsValid":       false,
		"terminalCacheDiagnosticsDisposition": terminalCacheDiagnosticsRejected,
	}
	if !reflect.DeepEqual(diagnostics, want) || strings.Contains(strings.TrimSpace(firstTerminalDiagnosticString(diagnostics)), sentinel) {
		t.Fatalf("unknown cache payload was retained or echoed: %#v", diagnostics)
	}
	usage := projection.PublicUsageMap()
	if usage["promptTokens"] != 0 || usage["totalTokens"] != 0 {
		t.Fatalf("invalid provider usage was not normalized: %#v", usage)
	}
}

func TestValidateTerminalTelemetryPublicMapsV1RejectsOpenReplayPayloads(t *testing.T) {
	projection := NewTerminalTelemetryV1(
		domainmodel.Usage{PromptTokens: 10, CompletionTokens: 2, TotalTokens: 12},
		map[string]any{},
	)
	usage := projection.PublicUsageMap()
	diagnostics := projection.PublicCacheDiagnosticsMap()
	if !ValidateTerminalTelemetryPublicMapsV1(usage, diagnostics) {
		t.Fatal("canonical terminal telemetry was rejected")
	}
	body, err := json.Marshal(map[string]any{"usage": usage, "diagnostics": diagnostics})
	if err != nil {
		t.Fatal(err)
	}
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	decoder.UseNumber()
	var replay map[string]map[string]any
	if err := decoder.Decode(&replay); err != nil ||
		!ValidateTerminalTelemetryPublicMapsV1(replay["usage"], replay["diagnostics"]) {
		t.Fatalf("canonical persisted telemetry was rejected: replay=%#v err=%v", replay, err)
	}
	for name, mutate := range map[string]func(map[string]any, map[string]any){
		"unknown diagnostic": func(_ map[string]any, diagnostics map[string]any) {
			diagnostics["outputPreview"] = "RAW_PROVIDER_SENTINEL"
		},
		"unknown usage": func(usage map[string]any, _ map[string]any) {
			usage["providerPayload"] = "RAW_PROVIDER_SENTINEL"
		},
		"inconsistent total": func(usage map[string]any, _ map[string]any) {
			usage["totalTokens"] = 1
		},
		"cache details without presence authority": func(_ map[string]any, diagnostics map[string]any) {
			diagnostics["cacheHitTokens"] = 1
			diagnostics["cacheMissTokens"] = 0
			diagnostics["cacheHitRate"] = 1.0
			diagnostics["cacheTelemetrySource"] = "provider_usage"
		},
	} {
		t.Run(name, func(t *testing.T) {
			candidateUsage := cloneTerminalTestMap(usage)
			candidateDiagnostics := cloneTerminalTestMap(diagnostics)
			mutate(candidateUsage, candidateDiagnostics)
			if ValidateTerminalTelemetryPublicMapsV1(candidateUsage, candidateDiagnostics) {
				t.Fatalf("open telemetry payload was accepted: usage=%#v diagnostics=%#v", candidateUsage, candidateDiagnostics)
			}
		})
	}
}

func cloneTerminalTestMap(value map[string]any) map[string]any {
	body, _ := json.Marshal(value)
	out := map[string]any{}
	_ = json.Unmarshal(body, &out)
	return out
}

func firstTerminalDiagnosticString(value any) string {
	switch typed := value.(type) {
	case map[string]any:
		for _, child := range typed {
			if text := firstTerminalDiagnosticString(child); text != "" {
				return text
			}
		}
	case []string:
		return strings.Join(typed, " ")
	case string:
		return typed
	}
	return ""
}
