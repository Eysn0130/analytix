package cache

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	mcpprotocol "analytix.local/runtime-go/internal/adapters/outbound/mcp/protocol"
	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
	domainmcp "analytix.local/runtime-go/internal/domain/mcp"
	domainmcpname "analytix.local/runtime-go/internal/domain/mcpname"
)

const schemaVersion = 5
const maxCacheBytes = 4 * 1024 * 1024

type Schema struct {
	Version       int                             `json:"version"`
	SpecHash      string                          `json:"specHash"`
	Tools         []domainmcp.ToolSpec            `json:"tools"`
	LastValidated time.Time                       `json:"lastValidated"`
	Quarantined   []mcpprotocol.ToolContractIssue `json:"-"`
}

func SpecFingerprint(spec domainmcp.ServerSpec) string {
	type fingerprint struct {
		ServerID                string               `json:"serverId"`
		Transport               string               `json:"transport"`
		Command                 string               `json:"command,omitempty"`
		Args                    []string             `json:"args,omitempty"`
		Env                     map[string]string    `json:"env,omitempty"`
		URL                     string               `json:"url,omitempty"`
		Headers                 map[string]string    `json:"headers,omitempty"`
		CWD                     string               `json:"cwd,omitempty"`
		ExpectedName            string               `json:"expectedServerName,omitempty"`
		ExpectedVersion         string               `json:"expectedServerVersion,omitempty"`
		IdentitySource          string               `json:"identitySource,omitempty"`
		ManifestSHA256          string               `json:"manifestSha256,omitempty"`
		EntrypointPath          string               `json:"entrypointPath,omitempty"`
		EntrypointSHA256        string               `json:"entrypointSha256,omitempty"`
		PluginRootPath          string               `json:"pluginRootPath,omitempty"`
		SourceTreeSHA256        string               `json:"sourceTreeSha256,omitempty"`
		HostInstallMarkerSHA256 string               `json:"hostInstallMarkerSha256,omitempty"`
		TrustScope              string               `json:"trustScope,omitempty"`
		TrustedRoots            []string             `json:"trustedWorkspaceRoots,omitempty"`
		ReadOnlyToolNames       []string             `json:"readOnlyToolNames,omitempty"`
		LowPriority             bool                 `json:"lowPriority"`
		BackgroundStart         bool                 `json:"backgroundStart"`
		TimeoutMS               int64                `json:"timeoutMs,omitempty"`
		Tools                   []domainmcp.ToolSpec `json:"tools,omitempty"`
	}
	readOnly := make([]string, 0, len(spec.ReadOnlyToolNames))
	for name, enabled := range spec.ReadOnlyToolNames {
		if enabled {
			readOnly = append(readOnly, name)
		}
	}
	sort.Strings(readOnly)
	body, _ := json.Marshal(fingerprint{
		ServerID:                spec.ID,
		Transport:               strings.ToLower(strings.TrimSpace(firstNonEmpty(spec.Transport, "stdio"))),
		Command:                 strings.TrimSpace(spec.Command),
		Args:                    append([]string(nil), spec.Args...),
		Env:                     cloneStringMap(spec.Env),
		URL:                     strings.TrimSpace(spec.URL),
		Headers:                 cloneStringMap(spec.Headers),
		CWD:                     filepath.ToSlash(strings.TrimSpace(spec.CWD)),
		ExpectedName:            strings.TrimSpace(spec.ExpectedServerName),
		ExpectedVersion:         strings.TrimSpace(spec.ExpectedServerVersion),
		IdentitySource:          strings.TrimSpace(spec.IdentitySource),
		ManifestSHA256:          strings.TrimSpace(spec.ManifestSHA256),
		EntrypointPath:          filepath.ToSlash(strings.TrimSpace(spec.EntrypointPath)),
		EntrypointSHA256:        strings.TrimSpace(spec.EntrypointSHA256),
		PluginRootPath:          filepath.ToSlash(strings.TrimSpace(spec.PluginRootPath)),
		SourceTreeSHA256:        strings.TrimSpace(spec.SourceTreeSHA256),
		HostInstallMarkerSHA256: strings.TrimSpace(spec.HostInstallMarkerSHA256),
		TrustScope:              strings.TrimSpace(spec.TrustScope),
		TrustedRoots:            append([]string(nil), spec.TrustedWorkspaceRoots...),
		ReadOnlyToolNames:       readOnly,
		LowPriority:             spec.LowPriority,
		BackgroundStart:         spec.BackgroundStart,
		TimeoutMS:               spec.TimeoutMS,
		Tools:                   append([]domainmcp.ToolSpec(nil), spec.Tools...),
	})
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

func Load(cacheDir string, serverID string, expectedHash string) (Schema, bool) {
	if _, err := domainmcpname.Namespace(serverID); err != nil {
		return Schema{}, false
	}
	path := Path(cacheDir, serverID)
	if path == "" {
		return Schema{}, false
	}
	file, err := os.Open(path)
	if err != nil {
		return Schema{}, false
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxCacheBytes+1))
	if err != nil || len(data) > maxCacheBytes {
		return Schema{}, false
	}
	root, err := domainjsonstrict.DecodeRawObject(data, domainjsonstrict.Options{
		MaxBytes: maxCacheBytes, MaxTokens: 200_000, MaxStringBytes: 1024 * 1024,
	})
	if err != nil || !rawKeysEqual(root, "version", "specHash", "tools", "lastValidated") {
		return Schema{}, false
	}
	if !rawJSONInteger(root["version"]) || !rawJSONString(root["specHash"]) || !rawJSONArray(root["tools"]) || !rawJSONString(root["lastValidated"]) {
		return Schema{}, false
	}
	var rawTools []json.RawMessage
	if json.Unmarshal(root["tools"], &rawTools) != nil {
		return Schema{}, false
	}
	for _, rawTool := range rawTools {
		tool, err := domainjsonstrict.DecodeRawObject(rawTool, domainjsonstrict.Options{
			MaxBytes: maxCacheBytes, MaxTokens: 200_000, MaxStringBytes: 1024 * 1024,
		})
		if err != nil || !rawKeysAllowed(tool, "name", "description", "inputSchema", "outputSchema", "readOnlyHint", "resultText", "taskSupport") {
			return Schema{}, false
		}
		if !rawJSONString(tool["name"]) || !rawJSONString(tool["description"]) ||
			!rawJSONBool(tool["readOnlyHint"]) || !rawJSONString(tool["resultText"]) || !rawJSONString(tool["taskSupport"]) {
			return Schema{}, false
		}
	}
	var schema Schema
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&schema); err != nil {
		return Schema{}, false
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return Schema{}, false
	}
	if schema.Version != schemaVersion || schema.SpecHash != expectedHash || !isSHA256Hex(schema.SpecHash) || schema.LastValidated.IsZero() {
		return Schema{}, false
	}
	seen := map[string]bool{}
	for index := range schema.Tools {
		name := schema.Tools[index].Name
		canonical, canonicalErr := domainmcpname.Canonical(serverID, name)
		taskSupport, taskSupportOK := domainmcp.NormalizeToolTaskSupport(schema.Tools[index].TaskSupport)
		if canonicalErr != nil || !taskSupportOK || name == "" || seen[canonical] {
			return Schema{}, false
		}
		seen[canonical] = true
		schema.Tools[index].Name = name
		schema.Tools[index].TaskSupport = taskSupport
	}
	validTools := make([]domainmcp.ToolSpec, 0, len(schema.Tools))
	quarantined := make([]mcpprotocol.ToolContractIssue, 0)
	for index := range schema.Tools {
		input, inputErr := mcpprotocol.NormalizeToolInputSchemaBytes(schema.Tools[index].InputSchema)
		output, outputErr := mcpprotocol.NormalizeToolOutputSchemaBytes(schema.Tools[index].OutputSchema)
		if inputErr != nil {
			code := mcpprotocol.ToolContractInvalidInputSchema
			if missingCachedToolSchema(schema.Tools[index].InputSchema) {
				code = mcpprotocol.ToolContractMissingInputSchema
			}
			quarantined = append(quarantined, mcpprotocol.ToolContractIssue{Name: schema.Tools[index].Name, Code: code})
			continue
		}
		if outputErr != nil {
			code := mcpprotocol.ToolContractInvalidOutputSchema
			if missingCachedToolSchema(schema.Tools[index].OutputSchema) {
				code = mcpprotocol.ToolContractMissingOutputSchema
			}
			quarantined = append(quarantined, mcpprotocol.ToolContractIssue{Name: schema.Tools[index].Name, Code: code})
			continue
		}
		schema.Tools[index].InputSchema = input
		schema.Tools[index].OutputSchema = output
		validTools = append(validTools, schema.Tools[index])
	}
	if len(validTools) == 0 && len(quarantined) > 0 {
		return Schema{}, false
	}
	sort.SliceStable(quarantined, func(i, j int) bool {
		if quarantined[i].Name == quarantined[j].Name {
			return quarantined[i].Code < quarantined[j].Code
		}
		return quarantined[i].Name < quarantined[j].Name
	})
	schema.Tools = validTools
	schema.Quarantined = quarantined
	return schema, true
}

func missingCachedToolSchema(raw json.RawMessage) bool {
	trimmed := bytes.TrimSpace(raw)
	return len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null"))
}

func rawKeysEqual(object map[string]json.RawMessage, allowed ...string) bool {
	if len(object) != len(allowed) {
		return false
	}
	for _, key := range allowed {
		if _, exists := object[key]; !exists {
			return false
		}
	}
	return true
}

func rawKeysAllowed(object map[string]json.RawMessage, allowed ...string) bool {
	allowedSet := make(map[string]bool, len(allowed))
	for _, key := range allowed {
		allowedSet[key] = true
	}
	for key := range object {
		if !allowedSet[key] {
			return false
		}
	}
	return true
}

func rawJSONString(raw json.RawMessage) bool {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) < 2 || trimmed[0] != '"' {
		return false
	}
	var value string
	return json.Unmarshal(trimmed, &value) == nil
}

func rawJSONBool(raw json.RawMessage) bool {
	trimmed := bytes.TrimSpace(raw)
	return bytes.Equal(trimmed, []byte("true")) || bytes.Equal(trimmed, []byte("false"))
}

func rawJSONInteger(raw json.RawMessage) bool {
	var value json.Number
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if decoder.Decode(&value) != nil {
		return false
	}
	_, err := value.Int64()
	return err == nil
}

func rawJSONArray(raw json.RawMessage) bool {
	trimmed := bytes.TrimSpace(raw)
	return len(trimmed) > 0 && trimmed[0] == '['
}

func rawJSONObject(raw json.RawMessage) bool {
	trimmed := bytes.TrimSpace(raw)
	return len(trimmed) > 0 && trimmed[0] == '{'
}

func isSHA256Hex(value string) bool {
	if len(value) != sha256.Size*2 || strings.ToLower(value) != value {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func Save(cacheDir string, serverID string, schema Schema) error {
	if _, err := domainmcpname.Namespace(serverID); err != nil {
		return err
	}
	path := Path(cacheDir, serverID)
	if path == "" || !isSHA256Hex(schema.SpecHash) {
		return errors.New("invalid MCP cache spec hash")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	schema.Version = schemaVersion
	if schema.LastValidated.IsZero() {
		schema.LastValidated = time.Now().UTC()
	}
	seen := map[string]bool{}
	for index := range schema.Tools {
		name := schema.Tools[index].Name
		canonical, canonicalErr := domainmcpname.Canonical(serverID, name)
		input, inputErr := mcpprotocol.NormalizeToolInputSchemaBytes(schema.Tools[index].InputSchema)
		output, outputErr := mcpprotocol.NormalizeToolOutputSchemaBytes(schema.Tools[index].OutputSchema)
		taskSupport, taskSupportOK := domainmcp.NormalizeToolTaskSupport(schema.Tools[index].TaskSupport)
		if name == "" {
			return fmt.Errorf("invalid cached mcp tool schema: empty tool name")
		}
		if canonicalErr != nil {
			return fmt.Errorf("invalid cached mcp tool identity for %q: %w", name, canonicalErr)
		}
		if seen[canonical] {
			return fmt.Errorf("invalid cached mcp tool schema: duplicate tool %q", name)
		}
		if !taskSupportOK {
			return fmt.Errorf("invalid cached mcp task support for %q", name)
		}
		seen[canonical] = true
		if inputErr != nil {
			return fmt.Errorf("invalid cached mcp input schema for %q: %w", name, inputErr)
		}
		if outputErr != nil {
			return fmt.Errorf("invalid cached mcp output schema for %q: %w", name, outputErr)
		}
		schema.Tools[index].Name = name
		schema.Tools[index].InputSchema = input
		schema.Tools[index].OutputSchema = output
		schema.Tools[index].TaskSupport = taskSupport
	}
	sort.SliceStable(schema.Tools, func(i, j int) bool { return schema.Tools[i].Name < schema.Tools[j].Name })
	data, err := json.MarshalIndent(schema, "", "  ")
	if err != nil {
		return err
	}
	if len(data) > maxCacheBytes {
		return errors.New("MCP cache exceeds maximum size")
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".schema-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	keepTemp := true
	defer func() {
		if keepTemp {
			_ = os.Remove(tmpPath)
		}
	}()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return err
	}
	keepTemp = false
	return nil
}

func Path(cacheDir string, serverID string) string {
	if strings.TrimSpace(cacheDir) == "" || strings.TrimSpace(serverID) == "" {
		return ""
	}
	if _, err := domainmcpname.Namespace(serverID); err != nil {
		return ""
	}
	digest := sha256.Sum256([]byte(serverID))
	return filepath.Join(cacheDir, safeName(serverID)+"-"+hex.EncodeToString(digest[:])+".json")
}

func CacheableTools(tools []domainmcp.ToolSpec, spec domainmcp.ServerSpec) ([]domainmcp.ToolSpec, error) {
	if _, err := domainmcpname.Namespace(spec.ID); err != nil {
		return nil, err
	}
	out := make([]domainmcp.ToolSpec, 0, len(tools))
	seen := map[string]bool{}
	for _, tool := range tools {
		name := strings.TrimSpace(tool.Name)
		canonical, canonicalErr := domainmcpname.Canonical(spec.ID, name)
		if name == "" {
			return nil, errors.New("invalid mcp tool schema: empty tool name")
		}
		if canonicalErr != nil {
			return nil, fmt.Errorf("invalid mcp tool identity for %q: %w", name, canonicalErr)
		}
		if seen[canonical] {
			return nil, fmt.Errorf("invalid mcp tool schema: duplicate tool %q", name)
		}
		taskSupport, taskSupportOK := domainmcp.NormalizeToolTaskSupport(tool.TaskSupport)
		if !taskSupportOK {
			return nil, fmt.Errorf("invalid mcp task support for %q", name)
		}
		seen[canonical] = true
		tool.Name = name
		if spec.ReadOnlyToolNames[name] {
			tool.ReadOnlyHint = true
		}
		input, err := mcpprotocol.NormalizeToolInputSchemaBytes(tool.InputSchema)
		if err != nil {
			return nil, fmt.Errorf("invalid mcp input schema for %q: %w", name, err)
		}
		output, err := mcpprotocol.NormalizeToolOutputSchemaBytes(tool.OutputSchema)
		if err != nil {
			return nil, fmt.Errorf("invalid mcp output schema for %q: %w", name, err)
		}
		tool.InputSchema = input
		tool.OutputSchema = output
		tool.TaskSupport = taskSupport
		out = append(out, tool)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func safeName(value string) string {
	trimmed := strings.TrimSpace(value)
	var builder strings.Builder
	for _, r := range trimmed {
		switch {
		case r >= 'a' && r <= 'z':
			builder.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			builder.WriteRune(r)
		case r >= '0' && r <= '9':
			builder.WriteRune(r)
		case r == '-', r == '_', r == '.':
			builder.WriteRune(r)
		default:
			builder.WriteRune('_')
		}
	}
	name := strings.Trim(builder.String(), "._-")
	if name != "" {
		if len(name) > 64 {
			name = name[:64]
		}
		return name
	}
	sum := sha256.Sum256([]byte(trimmed))
	return hex.EncodeToString(sum[:8])
}

func cloneStringMap(input map[string]string) map[string]string {
	if len(input) == 0 {
		return nil
	}
	out := make(map[string]string, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
