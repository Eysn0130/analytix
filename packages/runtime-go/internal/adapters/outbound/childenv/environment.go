package childenv

import (
	"errors"
	"sort"
	"strings"
)

var reservedHostEnvironmentKeys = map[string]struct{}{
	"ANALYTIX_API_KEY":                                              {},
	"ANALYTIX_AUTHORITY_ANCHOR_V1":                                  {},
	"ANALYTIX_AUTHORITY_MANIFEST_ROOT":                              {},
	"ANALYTIX_AUTHORITY_CREDENTIAL_PROFILE_ROOT":                    {},
	"ANALYTIX_AUTHORITY_CREDENTIAL_BUNDLE_ROOT":                     {},
	"ANALYTIX_CONFIG":                                               {},
	"ANALYTIX_CONTROLLED_ARTIFACT_HOST_URL":                         {},
	"ANALYTIX_CONTROLLED_ARTIFACT_HOST_V2_URL":                      {},
	"ANALYTIX_CONTROLLED_ARTIFACT_HOST_V2_BACKEND_GENERATION":       {},
	"ANALYTIX_CONTROLLED_ARTIFACT_HOST_V2_ALLOCATION_RECORD_DIGEST": {},
	"ANALYTIX_CONTROLLED_ARTIFACT_HOST_V2_TLS_ROOT_CERT_DER":        {},
	"ANALYTIX_CONTROLLED_ARTIFACT_HOST_V2_TLS_LEAF_SPKI_SHA256":     {},
	"ANALYTIX_DATA_DIR":                                             {},
	"ANALYTIX_DATASET_SNAPSHOT_MATERIAL_ROOT_V2":                    {},
	"ANALYTIX_DATASET_SNAPSHOT_SELECTION_V2":                        {},
	"ANALYTIX_MCP_CONFIG_JSON":                                      {},
	"ANALYTIX_MCP_CONFIG_PATH":                                      {},
	"ANALYTIX_MCP_PROXY_URL":                                        {},
	"ANALYTIX_MODEL_PROVIDERS":                                      {},
	"ANALYTIX_MODEL_PROXY_URL":                                      {},
	"ANALYTIX_PROTECTED_READ_DIRS":                                  {},
	"ANALYTIX_RUNTIME_TOKEN":                                        {},
	"ANALYTIX_SQLITE_PATH":                                          {},
	"ANALYTIX_USER_DATA_DIR":                                        {},
	"ANTHROPIC_API_KEY":                                             {},
	"DEEPSEEK_API_KEY":                                              {},
	"OPENAI_API_KEY":                                                {},
}

func IsReservedHostSecret(key string) bool {
	key = strings.ToUpper(strings.TrimSpace(key))
	if _, reserved := reservedHostEnvironmentKeys[key]; reserved {
		return true
	}
	if !strings.HasPrefix(key, "ANALYTIX_") {
		return false
	}
	for _, suffix := range []string{"_API_KEY", "_PASSWORD", "_PRIVATE_KEY", "_SECRET", "_TOKEN"} {
		if strings.HasSuffix(key, suffix) {
			return true
		}
	}
	return false
}

func Sanitized(base []string) []string {
	out := make([]string, 0, len(base))
	for _, entry := range base {
		key, ok := environmentEntryKey(entry)
		if !ok || IsReservedHostSecret(key) {
			continue
		}
		out = append(out, entry)
	}
	return out
}

func Merge(base []string, overrides map[string]string) ([]string, error) {
	overrideKeys := make([]string, 0, len(overrides))
	normalizedOverrides := map[string]string{}
	for key := range overrides {
		key = strings.TrimSpace(key)
		if key == "" || strings.Contains(key, "=") {
			return nil, errors.New("child environment override key is invalid")
		}
		if IsReservedHostSecret(key) {
			return nil, errors.New("child environment override uses a reserved host secret")
		}
		normalized := strings.ToUpper(key)
		if _, duplicate := normalizedOverrides[normalized]; duplicate {
			return nil, errors.New("child environment overrides contain a duplicate key")
		}
		normalizedOverrides[normalized] = key
		overrideKeys = append(overrideKeys, key)
	}
	sort.Slice(overrideKeys, func(i, j int) bool {
		return strings.ToUpper(overrideKeys[i]) < strings.ToUpper(overrideKeys[j])
	})
	out := make([]string, 0, len(base)+len(overrides))
	for _, entry := range Sanitized(base) {
		key, ok := environmentEntryKey(entry)
		if !ok {
			continue
		}
		if _, replaced := normalizedOverrides[strings.ToUpper(key)]; replaced {
			continue
		}
		out = append(out, entry)
	}
	for _, key := range overrideKeys {
		out = append(out, key+"="+overrides[key])
	}
	return out, nil
}

func environmentEntryKey(entry string) (string, bool) {
	if entry == "" {
		return "", false
	}
	start := 0
	if entry[0] == '=' {
		start = 1
	}
	index := strings.IndexByte(entry[start:], '=')
	if index < 0 {
		return "", false
	}
	key := entry[:start+index]
	return key, strings.TrimSpace(key) != ""
}
