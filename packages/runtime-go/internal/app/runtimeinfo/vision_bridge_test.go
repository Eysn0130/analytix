package runtimeinfo

import (
	"encoding/json"
	"strings"
	"testing"

	domainmodel "analytix.local/runtime-go/internal/domain/model"
)

func domainTurnConfigForVisionBridgeTest() domainmodel.TurnConfig {
	return domainmodel.TurnConfig{
		ProviderID:      "default",
		BaseURL:         "https://default.example.test/v1",
		APIKey:          "default-key",
		Model:           "default-model",
		EndpointFormat:  "chat_completions",
		InputModalities: []string{"text"},
		MessageParts:    []string{"text"},
	}
}

func TestExtractToolResultImagesFindsNestedDataURLAndRespectsLimits(t *testing.T) {
	png := "iVBORw0KGgo="
	jpeg := "/9j/"
	images := ExtractToolResultImages(map[string]any{
		"nested": []any{
			map[string]any{"url": "data:image/png;base64," + png},
			map[string]any{"mimeType": "image/jpeg", "dataBase64": jpeg},
		},
	}, 1, 10, DefaultVisionBridgeConfig())
	if len(images) != 1 || images[0].MediaType != "image/png" || images[0].DataBase64 != png || images[0].BlobSHA256 == "" {
		t.Fatalf("image extraction mismatch: %#v", images)
	}
	if oversized := ExtractToolResultImages(map[string]any{"dataBase64": png}, 1, 1, DefaultVisionBridgeConfig()); len(oversized) != 0 {
		t.Fatalf("oversized image should be omitted: %#v", oversized)
	}
	for name, value := range map[string]any{
		"mime mismatch": map[string]any{"mimeType": "image/jpeg", "dataBase64": png},
		"non image":     map[string]any{"mimeType": "image/png", "dataBase64": "aGk="},
		"svg":           map[string]any{"mimeType": "image/svg+xml", "dataBase64": "PHN2Zz48L3N2Zz4="},
	} {
		t.Run(name, func(t *testing.T) {
			if got := ExtractToolResultImages(value, 1, 100, DefaultVisionBridgeConfig()); len(got) != 0 {
				t.Fatalf("unsafe tool image was accepted: %#v", got)
			}
		})
	}
}

func TestExtractToolResultImagesIsDeterministicAndDeduplicatesByFullHash(t *testing.T) {
	first := "iVBORw0KGgox"
	second := "iVBORw0KGgoy"
	input := map[string]any{
		"z": map[string]any{"mimeType": "image/png", "dataBase64": second},
		"a": []any{
			map[string]any{"mimeType": "image/png", "dataBase64": first},
			map[string]any{"mimeType": "image/png", "dataBase64": first},
		},
	}
	left := ExtractToolResultImages(input, 4, 100, DefaultVisionBridgeConfig())
	right := ExtractToolResultImages(input, 4, 100, DefaultVisionBridgeConfig())
	if len(left) != 2 || len(right) != 2 || left[0].DataBase64 != first || left[1].DataBase64 != second ||
		left[0].BlobSHA256 != right[0].BlobSHA256 || left[1].BlobSHA256 != right[1].BlobSHA256 {
		t.Fatalf("tool image extraction is nondeterministic or duplicated: left=%#v right=%#v", left, right)
	}
}

func TestToolResultWithVisionBridgeStatusClonesMaps(t *testing.T) {
	original := map[string]any{"ok": true}
	enriched, _ := ToolResultWithVisionBridgeStatus(original, map[string]any{"status": "supported"}).(map[string]any)
	if enriched["vision_bridge"] == nil || original["vision_bridge"] != nil {
		t.Fatalf("vision bridge status should be added to a cloned map: original=%#v enriched=%#v", original, enriched)
	}
	wrapped, _ := ToolResultWithVisionBridgeStatus("plain", map[string]any{"status": "unavailable"}).(map[string]any)
	if wrapped["result"] != "plain" || wrapped["vision_bridge"] == nil {
		t.Fatalf("non-map output should be wrapped: %#v", wrapped)
	}
}

func TestRedactToolResultImageDataForPersistenceRemovesRawImagePayloads(t *testing.T) {
	rawBase64 := "iVBORw0KGgo="
	redacted := RedactToolResultImageDataForPersistence(map[string]any{
		"images": []any{
			map[string]any{"type": "image", "mime_type": "image/png", "data_base64": rawBase64},
			map[string]any{"mimeType": "image/jpeg", "data": rawBase64},
			map[string]any{"url": "data:image/webp;base64," + rawBase64},
		},
		"plainDataURL": "data:image/png;base64," + rawBase64,
		"nested": map[string]any{"source": map[string]any{
			"type": "base64", "media_type": "image/png", "data": rawBase64,
		}},
		"bytes": []byte(rawBase64),
		"raw":   json.RawMessage(`{"private":"binary"}`),
		"text":  "state ready",
	})
	data, _ := json.Marshal(redacted)
	text := string(data)
	for _, forbidden := range []string{rawBase64, "data_base64", "data:image/", `"data":`, `"url":`, `"private"`} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("redacted output still contains %q: %s", forbidden, text)
		}
	}
	if !strings.Contains(text, "imageDataRedacted") || !strings.Contains(text, "state ready") {
		t.Fatalf("redacted output should preserve metadata and text: %s", text)
	}
}

func TestBuildVisionBridgeStatus(t *testing.T) {
	observation := map[string]any{"summary": "ok"}
	status := BuildVisionBridgeStatus(VisionBridgeStatusInput{
		Status:         " supported ",
		Reason:         " done ",
		ProviderID:     " xiaomi ",
		Model:          " mimo ",
		ImageCount:     2,
		SourceToolName: " screenshot ",
		Observation:    observation,
	})
	if status["kind"] != "vision_bridge_observation" ||
		status["status"] != "supported" ||
		status["reason"] != "done" ||
		status["providerId"] != "xiaomi" ||
		status["model"] != "mimo" ||
		status["imageCount"] != float64(2) ||
		status["sourceToolName"] != "screenshot" {
		t.Fatalf("status mismatch: %#v", status)
	}
	payload, _ := status["observation"].(map[string]any)
	payload["summary"] = "changed"
	if observation["summary"] != "ok" {
		t.Fatalf("observation was not cloned: %#v", observation)
	}
}

func TestBuildVisionBridgeToolStatusIncludesProviderForObservedStates(t *testing.T) {
	config := VisionBridgeConfig{ProviderID: " xiaomi ", Model: " mimo "}
	status := BuildVisionBridgeToolStatus(config, " screenshot ", 1, "failed", "nope", nil)
	if status["providerId"] != "xiaomi" || status["model"] != "mimo" || status["reason"] != "nope" {
		t.Fatalf("failed status should include configured provider identity: %#v", status)
	}
	unavailable := BuildVisionBridgeToolStatus(config, " screenshot ", 1, "unavailable", "", nil)
	if unavailable["providerId"] != nil || unavailable["model"] != nil {
		t.Fatalf("unavailable status should not claim provider execution: %#v", unavailable)
	}
}

func TestVisionBridgeTurnConfigOverridesModelTransportAndImageSupport(t *testing.T) {
	config := VisionBridgeTurnConfig(domainTurnConfigForVisionBridgeTest(), VisionBridgeConfig{
		ProviderID:     " xiaomi ",
		BaseURL:        " https://api.example.test/v1/ ",
		APIKey:         " key ",
		Model:          " mimo ",
		EndpointFormat: "messages",
	})
	if config.ProviderID != "xiaomi" ||
		config.BaseURL != "https://api.example.test/v1" ||
		config.APIKey != "key" ||
		config.Model != "mimo" ||
		config.EndpointFormat != "messages" ||
		config.Family != "anthropic-compatible" ||
		!config.SupportsImageInput {
		t.Fatalf("vision bridge turn config mismatch: %#v", config)
	}
	if len(config.InputModalities) != 2 || config.InputModalities[1] != "image" ||
		len(config.MessageParts) != 2 || config.MessageParts[1] != "image_url" {
		t.Fatalf("vision bridge image capabilities mismatch: %#v", config)
	}
}

func TestParseVisionBridgeObservationAcceptsJSONFence(t *testing.T) {
	observation, err := ParseVisionBridgeObservation("```json\n{\"summary\":\"ok\"}\n```")
	if err != nil {
		t.Fatalf("parse observation: %v", err)
	}
	if observation["summary"] != "ok" {
		t.Fatalf("observation mismatch: %#v", observation)
	}
	if _, err := ParseVisionBridgeObservation("{}"); err == nil {
		t.Fatal("empty observation should fail")
	}
}

func TestMessagePartsSupportImage(t *testing.T) {
	if !MessagePartsSupportImage([]string{"text", "input_image"}) {
		t.Fatal("input_image should be treated as image support")
	}
	if MessagePartsSupportImage([]string{"text"}) {
		t.Fatal("text-only parts should not support images")
	}
}

func TestShouldRunVisionBridge(t *testing.T) {
	config := VisionBridgeConfig{Enabled: true, Mode: "auto"}
	if !ShouldRunVisionBridge(config, false) {
		t.Fatal("auto vision bridge should run for text-only primary providers")
	}
	if ShouldRunVisionBridge(config, true) {
		t.Fatal("auto vision bridge should not run when the primary provider supports images")
	}
	config.Mode = "always"
	if !ShouldRunVisionBridge(config, true) {
		t.Fatal("always mode should run even when the primary provider supports images")
	}
	config.Mode = "off"
	if ShouldRunVisionBridge(config, false) {
		t.Fatal("off mode should not run")
	}
}
