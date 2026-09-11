package childenv

import (
	"strings"
	"testing"
)

func TestSanitizedRemovesHostSecretsCaseInsensitively(t *testing.T) {
	env := Sanitized([]string{
		"PATH=/bin", "ANALYTIX_RUNTIME_TOKEN=runtime-secret", "analytix_api_key=model-secret",
		"ANALYTIX_RUNTIME_DEEPSEEK_API_KEY=provider-secret",
		"ANALYTIX_AUTHORITY_ANCHOR_V1=protected-anchor",
		"ANALYTIX_AUTHORITY_MANIFEST_ROOT=/protected/manifest",
		"ANALYTIX_AUTHORITY_CREDENTIAL_PROFILE_ROOT=/protected/profile",
		"ANALYTIX_AUTHORITY_CREDENTIAL_BUNDLE_ROOT=/protected/bundle",
		"ANALYTIX_CONFIG=/protected/config.json",
		"ANALYTIX_CONTROLLED_ARTIFACT_HOST_V2_URL=https://127.0.0.1:45678",
		"ANALYTIX_CONTROLLED_ARTIFACT_HOST_V2_TLS_ROOT_CERT_DER=protected-cert",
		"ANALYTIX_DATA_DIR=/protected/runtime-data",
		"ANALYTIX_DATASET_SNAPSHOT_MATERIAL_ROOT_V2=/protected/snapshot",
		"ANALYTIX_DATASET_SNAPSHOT_SELECTION_V2=protected-selection",
		"ANALYTIX_SQLITE_PATH=/protected/runtime.sqlite",
		"ANALYTIX_USER_DATA_DIR=/protected/electron-data",
		"ANALYTIX_PROTECTED_READ_DIRS=/protected/private",
		"ANALYTIX_MCP_CONFIG_PATH=/protected/mcp.json",
		"ANALYTIX_MCP_PROXY_URL=https://secret@example.test",
		"SAFE_VALUE=ok",
	})
	joined := strings.Join(env, "\n")
	if strings.Contains(joined, "secret") || strings.Contains(joined, "protected") ||
		!strings.Contains(joined, "PATH=/bin") || !strings.Contains(joined, "SAFE_VALUE=ok") {
		t.Fatalf("sanitized environment mismatch: %#v", env)
	}
}

func TestMergeRejectsReservedSecretOverridesAndReplacesOrdinaryKeys(t *testing.T) {
	if _, err := Merge([]string{"PATH=/bin"}, map[string]string{"analytix_runtime_token": "forged"}); err == nil {
		t.Fatal("reserved host bearer token override should fail closed")
	}
	if _, err := Merge([]string{"PATH=/bin"}, map[string]string{"analytix_authority_anchor_v1": "forged"}); err == nil {
		t.Fatal("protected authority anchor override should fail closed")
	}
	env, err := Merge([]string{"A=old", "PATH=/bin"}, map[string]string{"A": "new", "B": "two"})
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(env, "\n")
	if strings.Contains(joined, "A=old") || !strings.Contains(joined, "A=new") || !strings.Contains(joined, "B=two") {
		t.Fatalf("merged environment mismatch: %#v", env)
	}
}
