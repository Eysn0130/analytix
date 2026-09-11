package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"path/filepath"
	"strings"

	authorityanchorenv "analytix.local/runtime-go/internal/adapters/outbound/authorityanchorenv"
	domainmcp "analytix.local/runtime-go/internal/domain/mcp"
	mcp "analytix.local/runtime-go/internal/mcp"
)

const runtimeStartupPrivateFrameMaxBytesV1 = 512 << 10
const runtimeStartupPrivateFramePurposeV1 = "analytix.runtime-startup-private-frame/v1"
const runtimeMainOwnedAuthorityPurposeV1 = "analytix.runtime-main-owned-authority/v1"
const runtimeHostScheduleMCPBindingPurposeV1 = "analytix.runtime-host-schedule-mcp-binding/v1"
const runtimeDarwinSecretStoreKeychainBindingPurposeV1 = "analytix.runtime-darwin-secret-store-keychain-binding/v1"
const runtimeExternalStateModeIsolatedLocalV1 = "isolated-local-v1"

type runtimeStartupPrivateFrameV1 struct {
	SchemaVersion                      int                                        `json:"schemaVersion"`
	Purpose                            string                                     `json:"purpose"`
	ProtectedAuthorityV1               *runtimeMainOwnedAuthorityEnvelopeV1       `json:"protectedAuthorityV1,omitempty"`
	DarwinSecretStoreKeychainBindingV1 *runtimeDarwinSecretStoreKeychainBindingV1 `json:"darwinSecretStoreKeychainBindingV1,omitempty"`
	HostScheduleMCPBindingV1           *runtimeHostScheduleMCPBindingV1           `json:"hostScheduleMcpBindingV1,omitempty"`
}

type runtimeDarwinSecretStoreKeychainBindingV1 struct {
	SchemaVersion          int    `json:"schemaVersion"`
	Purpose                string `json:"purpose"`
	ExternalStateMode      string `json:"externalStateMode"`
	IsolationRoot          string `json:"isolationRoot"`
	UserDataDir            string `json:"userDataDir"`
	DataDir                string `json:"dataDir"`
	KeychainDBPath         string `json:"keychainDBPath"`
	KeychainSecurityDigest string `json:"keychainSecurityDigest"`
	BindingDigest          string `json:"bindingDigest"`
}

type runtimeDarwinSecretStoreKeychainConfigV1 struct {
	DBPath         string
	BindingDigest  string
	SecurityDigest string
}

type runtimeHostScheduleMCPBindingV1 struct {
	SchemaVersion int               `json:"schemaVersion"`
	Purpose       string            `json:"purpose"`
	ServerID      string            `json:"serverId"`
	Command       string            `json:"command"`
	Args          []string          `json:"args"`
	Env           map[string]string `json:"env"`
	TrustScope    string            `json:"trustScope"`
	TimeoutMS     int64             `json:"timeoutMs"`
}

type runtimeMainOwnedAuthorityEnvelopeV1 struct {
	SchemaVersion                  int                       `json:"schemaVersion"`
	Purpose                        string                    `json:"purpose"`
	AuthorityAnchorV1              authorityAnchorEnvelopeV1 `json:"authorityAnchorV1"`
	AuthorityManifestRoot          string                    `json:"authorityManifestRoot"`
	AuthorityCredentialProfileRoot string                    `json:"authorityCredentialProfileRoot"`
	AuthorityCredentialBundleRoot  string                    `json:"authorityCredentialBundleRoot"`
}

type authorityAnchorEnvelopeV1 struct {
	SchemaVersion         int    `json:"schemaVersion"`
	InstallationID        string `json:"installationId"`
	AuthorityKeyID        string `json:"authorityKeyId"`
	AuthorityPublicKey    string `json:"authorityPublicKey"`
	CurrentManifestDigest string `json:"currentManifestDigest"`
}

type runtimeMainOwnedAuthorityConfigV1 struct {
	AnchorJSON            string
	ManifestRoot          string
	CredentialProfileRoot string
	CredentialBundleRoot  string
}

// readRuntimeStartupPrivateFrameV1 consumes one length-prefixed canonical
// startup document and requires EOF. Authority, the exact host schedule
// projection, and the validated Darwin task Keychain binding are the only
// accepted capabilities; every unknown field is closed.
func readRuntimeStartupPrivateFrameV1(reader io.Reader) (runtimeStartupPrivateFrameV1, error) {
	if reader == nil {
		return runtimeStartupPrivateFrameV1{}, errors.New("runtime private startup frame is unavailable")
	}
	var header [8]byte
	if _, err := io.ReadFull(reader, header[:]); err != nil {
		return runtimeStartupPrivateFrameV1{}, errors.New("runtime private startup frame is truncated")
	}
	length := binary.BigEndian.Uint64(header[:])
	clear(header[:])
	if length == 0 || length > runtimeStartupPrivateFrameMaxBytesV1 {
		return runtimeStartupPrivateFrameV1{}, errors.New("runtime private startup frame length is invalid")
	}
	body := make([]byte, int(length))
	defer clear(body)
	if _, err := io.ReadFull(reader, body); err != nil {
		return runtimeStartupPrivateFrameV1{}, errors.New("runtime private startup frame is truncated")
	}
	trailing, err := io.ReadAll(io.LimitReader(reader, 2))
	defer clear(trailing)
	if err != nil || len(trailing) != 0 {
		return runtimeStartupPrivateFrameV1{}, errors.New("runtime private startup frame contains multiple payloads")
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var frame runtimeStartupPrivateFrameV1
	if err := decoder.Decode(&frame); err != nil {
		return runtimeStartupPrivateFrameV1{}, errors.New("runtime private startup frame payload is invalid")
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return runtimeStartupPrivateFrameV1{}, errors.New("runtime private startup frame payload is invalid")
	}
	if frame.SchemaVersion != 1 || frame.Purpose != runtimeStartupPrivateFramePurposeV1 ||
		(frame.ProtectedAuthorityV1 == nil && frame.HostScheduleMCPBindingV1 == nil &&
			frame.DarwinSecretStoreKeychainBindingV1 == nil) {
		return runtimeStartupPrivateFrameV1{}, errors.New("runtime private startup frame payload is invalid")
	}
	if frame.ProtectedAuthorityV1 != nil {
		if _, err := frame.ProtectedAuthorityV1.config(); err != nil {
			return runtimeStartupPrivateFrameV1{}, errors.New("runtime private startup frame payload is invalid")
		}
	}
	if frame.HostScheduleMCPBindingV1 != nil {
		if _, err := frame.HostScheduleMCPBindingV1.spec(); err != nil {
			return runtimeStartupPrivateFrameV1{}, errors.New("runtime private startup frame payload is invalid")
		}
	}
	if frame.DarwinSecretStoreKeychainBindingV1 != nil {
		if err := frame.DarwinSecretStoreKeychainBindingV1.validateDocument(); err != nil {
			return runtimeStartupPrivateFrameV1{}, errors.New("runtime private startup frame payload is invalid")
		}
	}
	canonical, err := json.Marshal(frame)
	if err != nil || !bytes.Equal(canonical, body) {
		clear(canonical)
		return runtimeStartupPrivateFrameV1{}, errors.New("runtime private startup frame payload is invalid")
	}
	clear(canonical)
	return frame, nil
}

func (binding runtimeDarwinSecretStoreKeychainBindingV1) validateDocument() error {
	if binding.SchemaVersion != 1 || binding.Purpose != runtimeDarwinSecretStoreKeychainBindingPurposeV1 ||
		binding.ExternalStateMode != runtimeExternalStateModeIsolatedLocalV1 ||
		!runtimeSafeTaskKeychainPathV1(binding.IsolationRoot) || !runtimeSafeTaskKeychainPathV1(binding.UserDataDir) ||
		!runtimeSafeTaskKeychainPathV1(binding.DataDir) || !runtimeSafeTaskKeychainPathV1(binding.KeychainDBPath) ||
		binding.UserDataDir == binding.DataDir ||
		!runtimeStrictDescendantV1(binding.IsolationRoot, binding.UserDataDir) ||
		!runtimeStrictDescendantV1(binding.IsolationRoot, binding.DataDir) ||
		binding.KeychainDBPath != filepath.Join(binding.IsolationRoot, "darwin-secret-store-keychain", "analytix-task.keychain-db") ||
		!runtimeSHA256HexV1(binding.KeychainSecurityDigest) || !runtimeSHA256HexV1(binding.BindingDigest) {
		return errors.New("runtime Darwin Secret Store task Keychain binding is invalid")
	}
	document := struct {
		SchemaVersion          int    `json:"schemaVersion"`
		Purpose                string `json:"purpose"`
		ExternalStateMode      string `json:"externalStateMode"`
		IsolationRoot          string `json:"isolationRoot"`
		UserDataDir            string `json:"userDataDir"`
		DataDir                string `json:"dataDir"`
		KeychainDBPath         string `json:"keychainDBPath"`
		KeychainSecurityDigest string `json:"keychainSecurityDigest"`
	}{1, binding.Purpose, binding.ExternalStateMode, binding.IsolationRoot, binding.UserDataDir,
		binding.DataDir, binding.KeychainDBPath, binding.KeychainSecurityDigest}
	encoded, _ := json.Marshal(document)
	digest := runtimeSHA256HexBytesV1(encoded)
	clear(encoded)
	if digest != binding.BindingDigest {
		return errors.New("runtime Darwin Secret Store task Keychain binding is invalid")
	}
	return nil
}

func (binding runtimeDarwinSecretStoreKeychainBindingV1) config(userDataDir, dataDir string) (runtimeDarwinSecretStoreKeychainConfigV1, error) {
	if err := binding.validateDocument(); err != nil || binding.UserDataDir != userDataDir || binding.DataDir != dataDir {
		return runtimeDarwinSecretStoreKeychainConfigV1{}, errors.New("runtime Darwin Secret Store task Keychain binding is invalid")
	}
	if err := validateRuntimeDarwinKeychainBindingFilesystemV1(binding); err != nil {
		return runtimeDarwinSecretStoreKeychainConfigV1{}, errors.New("runtime Darwin Secret Store task Keychain binding is invalid")
	}
	return runtimeDarwinSecretStoreKeychainConfigV1{
		DBPath: binding.KeychainDBPath, BindingDigest: binding.BindingDigest,
		SecurityDigest: binding.KeychainSecurityDigest,
	}, nil
}

func runtimeSafeTaskKeychainPathV1(value string) bool {
	return runtimeExactAbsolutePathV1(value) && !strings.Contains(strings.ToLower(value), "login.keychain")
}

func runtimeExactAbsolutePathV1(value string) bool {
	if value == "" || len(value) > 1024 || value != strings.TrimSpace(value) ||
		!filepath.IsAbs(value) || filepath.Clean(value) != value {
		return false
	}
	for index := 0; index < len(value); index++ {
		character := value[index]
		if (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') || character == '/' || character == '.' ||
			character == '_' || character == '-' {
			continue
		}
		return false
	}
	return true
}

func runtimeStrictDescendantV1(parent, child string) bool {
	relative, err := filepath.Rel(parent, child)
	return err == nil && relative != "." && relative != ".." &&
		!strings.HasPrefix(relative, ".."+string(filepath.Separator)) && !filepath.IsAbs(relative)
}

func runtimeSHA256HexV1(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	decoded, err := hex.DecodeString(value)
	valid := err == nil && len(decoded) == sha256.Size
	clear(decoded)
	return valid
}

func runtimeSHA256HexBytesV1(value []byte) string {
	digest := sha256.Sum256(value)
	return hex.EncodeToString(digest[:])
}

func (binding runtimeHostScheduleMCPBindingV1) spec() (domainmcp.ServerSpec, error) {
	if binding.SchemaVersion != 1 || binding.Purpose != runtimeHostScheduleMCPBindingPurposeV1 {
		return domainmcp.ServerSpec{}, errors.New("runtime host schedule MCP binding is invalid")
	}
	spec := domainmcp.ServerSpec{
		ID: binding.ServerID, Transport: "stdio", Command: binding.Command,
		Args: append([]string(nil), binding.Args...), Env: cloneRuntimeStringMapV1(binding.Env),
		TrustScope: binding.TrustScope, TimeoutMS: binding.TimeoutMS,
	}
	if _, err := mcp.ValidateHostScheduleMCPBindingV1(spec); err != nil {
		return domainmcp.ServerSpec{}, errors.New("runtime host schedule MCP binding is invalid")
	}
	return spec, nil
}

func cloneRuntimeStringMapV1(input map[string]string) map[string]string {
	out := make(map[string]string, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}

func (envelope runtimeMainOwnedAuthorityEnvelopeV1) config() (runtimeMainOwnedAuthorityConfigV1, error) {
	if envelope.SchemaVersion != 1 || envelope.Purpose != runtimeMainOwnedAuthorityPurposeV1 {
		return runtimeMainOwnedAuthorityConfigV1{}, errors.New("runtime main-owned authority envelope is invalid")
	}
	anchorJSON, err := json.Marshal(envelope.AuthorityAnchorV1)
	if err != nil {
		return runtimeMainOwnedAuthorityConfigV1{}, errors.New("runtime main-owned authority envelope is invalid")
	}
	defer clear(anchorJSON)
	source := authorityanchorenv.Source{Lookup: func(name string) (string, bool) {
		if name != authorityanchorenv.AnchorEnvelopeV1Variable {
			return "", false
		}
		return string(anchorJSON), true
	}}
	if _, err := source.Load(context.Background()); err != nil {
		return runtimeMainOwnedAuthorityConfigV1{}, errors.New("runtime main-owned authority envelope is invalid")
	}
	roots := []string{
		envelope.AuthorityManifestRoot,
		envelope.AuthorityCredentialProfileRoot,
		envelope.AuthorityCredentialBundleRoot,
	}
	seen := make(map[string]struct{}, len(roots))
	for _, root := range roots {
		if root == "" || root != strings.TrimSpace(root) || !filepath.IsAbs(root) || filepath.Clean(root) != root {
			return runtimeMainOwnedAuthorityConfigV1{}, errors.New("runtime main-owned authority envelope is invalid")
		}
		if _, duplicate := seen[root]; duplicate {
			return runtimeMainOwnedAuthorityConfigV1{}, errors.New("runtime main-owned authority envelope is invalid")
		}
		seen[root] = struct{}{}
	}
	return runtimeMainOwnedAuthorityConfigV1{
		AnchorJSON: string(anchorJSON), ManifestRoot: roots[0],
		CredentialProfileRoot: roots[1], CredentialBundleRoot: roots[2],
	}, nil
}
