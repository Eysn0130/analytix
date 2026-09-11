package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"
)

func TestRuntimeStartupPrivateFrameAcceptsOneClosedCanonicalAuthorityDocument(t *testing.T) {
	envelope := runtimeMainOwnedAuthorityEnvelopeFixtureV1()
	body, err := json.Marshal(runtimeStartupPrivateFrameV1{
		SchemaVersion: 1, Purpose: runtimeStartupPrivateFramePurposeV1, ProtectedAuthorityV1: &envelope,
	})
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := readRuntimeStartupPrivateFrameV1(bytes.NewReader(runtimeStartupPrivateFrameBytesV1(body)))
	if err != nil || parsed.ProtectedAuthorityV1 == nil {
		t.Fatalf("read private authority frame: parsed=%#v err=%v", parsed, err)
	}
	config, err := parsed.ProtectedAuthorityV1.config()
	if err != nil {
		t.Fatal(err)
	}
	if config.ManifestRoot != envelope.AuthorityManifestRoot ||
		config.CredentialProfileRoot != envelope.AuthorityCredentialProfileRoot ||
		config.CredentialBundleRoot != envelope.AuthorityCredentialBundleRoot ||
		!strings.Contains(config.AnchorJSON, envelope.AuthorityAnchorV1.AuthorityPublicKey) {
		t.Fatalf("private authority projection changed: %#v", config)
	}
}

func TestRuntimeStartupPrivateFrameCarriesExactHostScheduleMCPBinding(t *testing.T) {
	binding := runtimeHostScheduleMCPBindingFixtureV1()
	body, err := json.Marshal(runtimeStartupPrivateFrameV1{
		SchemaVersion:            1,
		Purpose:                  runtimeStartupPrivateFramePurposeV1,
		HostScheduleMCPBindingV1: &binding,
	})
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := readRuntimeStartupPrivateFrameV1(
		bytes.NewReader(runtimeStartupPrivateFrameBytesV1(body)),
	)
	if err != nil || parsed.HostScheduleMCPBindingV1 == nil ||
		parsed.ProtectedAuthorityV1 != nil {
		t.Fatalf("read host schedule private binding: present=%t err=%v", parsed.HostScheduleMCPBindingV1 != nil, err)
	}
	spec, err := parsed.HostScheduleMCPBindingV1.spec()
	if err != nil || spec.ID != "gui_schedule" || spec.Args[3] != "http://127.0.0.1:9787" {
		t.Fatalf("host schedule projection changed: id=%q args=%d err=%v", spec.ID, len(spec.Args), err)
	}

	envelope := runtimeMainOwnedAuthorityEnvelopeFixtureV1()
	combined, err := json.Marshal(runtimeStartupPrivateFrameV1{
		SchemaVersion:            1,
		Purpose:                  runtimeStartupPrivateFramePurposeV1,
		ProtectedAuthorityV1:     &envelope,
		HostScheduleMCPBindingV1: &binding,
	})
	if err != nil {
		t.Fatal(err)
	}
	combinedParsed, err := readRuntimeStartupPrivateFrameV1(
		bytes.NewReader(runtimeStartupPrivateFrameBytesV1(combined)),
	)
	if err != nil || combinedParsed.ProtectedAuthorityV1 == nil ||
		combinedParsed.HostScheduleMCPBindingV1 == nil {
		t.Fatalf("read combined private startup frame: authority=%t schedule=%t err=%v",
			combinedParsed.ProtectedAuthorityV1 != nil,
			combinedParsed.HostScheduleMCPBindingV1 != nil,
			err,
		)
	}

	for name, mutate := range map[string]func(*runtimeHostScheduleMCPBindingV1){
		"relative command": func(value *runtimeHostScheduleMCPBindingV1) { value.Command = "relative" },
		"wrong host":       func(value *runtimeHostScheduleMCPBindingV1) { value.Args[3] = "http://localhost:9787" },
		"wrong path":       func(value *runtimeHostScheduleMCPBindingV1) { value.Args[3] = "http://127.0.0.1:9787/other" },
		"extra env":        func(value *runtimeHostScheduleMCPBindingV1) { value.Env["EXTRA"] = "1" },
	} {
		t.Run(name, func(t *testing.T) {
			invalid := runtimeHostScheduleMCPBindingFixtureV1()
			mutate(&invalid)
			invalidBody, marshalErr := json.Marshal(runtimeStartupPrivateFrameV1{
				SchemaVersion:            1,
				Purpose:                  runtimeStartupPrivateFramePurposeV1,
				HostScheduleMCPBindingV1: &invalid,
			})
			if marshalErr != nil {
				t.Fatal(marshalErr)
			}
			if _, err := readRuntimeStartupPrivateFrameV1(bytes.NewReader(
				runtimeStartupPrivateFrameBytesV1(invalidBody),
			)); err == nil {
				t.Fatal("invalid host schedule private binding was accepted")
			}
		})
	}
}

func TestRuntimeStartupPrivateFrameAcceptsGoEscapedTypeScriptScheduleBinding(t *testing.T) {
	body := []byte(`{"schemaVersion":1,"purpose":"analytix.runtime-startup-private-frame/v1","hostScheduleMcpBindingV1":{"schemaVersion":1,"purpose":"analytix.runtime-host-schedule-mcp-binding/v1","serverId":"gui_schedule","command":"/Applications/R\u0026D/Analytix Helper","args":["/Applications/R\u0026D/claw-schedule-mcp-node-entry.js","--gui-schedule-mcp-server","--base-url","http://127.0.0.1:9787","--secret","secret\u003c\u003e\u0026\u2028middle\u2029value"],"env":{"ELECTRON_RUN_AS_NODE":"1"},"trustScope":"user","timeoutMs":5000}}`)
	parsed, err := readRuntimeStartupPrivateFrameV1(
		bytes.NewReader(runtimeStartupPrivateFrameBytesV1(body)),
	)
	if err != nil || parsed.HostScheduleMCPBindingV1 == nil {
		t.Fatalf("read Go-escaped TypeScript schedule binding: present=%t err=%v", parsed.HostScheduleMCPBindingV1 != nil, err)
	}
	if parsed.HostScheduleMCPBindingV1.Command != "/Applications/R&D/Analytix Helper" ||
		parsed.HostScheduleMCPBindingV1.Args[5] != "secret<>&\u2028middle\u2029value" {
		t.Fatal("Go-escaped TypeScript schedule binding changed semantic strings")
	}
}

func TestRuntimeStartupPrivateFrameRejectsMissingTruncatedMultipleUnknownNoncanonicalAndExpandedFields(t *testing.T) {
	envelope := runtimeMainOwnedAuthorityEnvelopeFixtureV1()
	validBody, err := json.Marshal(runtimeStartupPrivateFrameV1{
		SchemaVersion: 1, Purpose: runtimeStartupPrivateFramePurposeV1, ProtectedAuthorityV1: &envelope,
	})
	if err != nil {
		t.Fatal(err)
	}
	valid := runtimeStartupPrivateFrameBytesV1(validBody)
	unknown := append(append([]byte(nil), validBody[:len(validBody)-1]...), []byte(`,"unknown":true}`)...)
	funds := append(append([]byte(nil), validBody[:len(validBody)-1]...), []byte(`,"bundledFundsMaterializationReadyV1":{}}`)...)
	dataset := append(append([]byte(nil), validBody[:len(validBody)-1]...), []byte(`,"datasetSnapshotSelectionV2":{}}`)...)
	noncanonical := append([]byte(" "), validBody...)
	var oversize [8]byte
	binary.BigEndian.PutUint64(oversize[:], uint64(runtimeStartupPrivateFrameMaxBytesV1+1))
	cases := map[string][]byte{
		"missing": nil, "short header": valid[:7], "short payload": valid[:len(valid)-1],
		"trailing byte": append(append([]byte(nil), valid...), 0),
		"second frame":  append(append([]byte(nil), valid...), valid...),
		"unknown field": runtimeStartupPrivateFrameBytesV1(unknown),
		"funds field":   runtimeStartupPrivateFrameBytesV1(funds),
		"dataset field": runtimeStartupPrivateFrameBytesV1(dataset),
		"noncanonical":  runtimeStartupPrivateFrameBytesV1(noncanonical),
		"oversize":      oversize[:],
	}
	for name, input := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := readRuntimeStartupPrivateFrameV1(bytes.NewReader(input)); err == nil {
				t.Fatal("invalid private startup frame was accepted")
			}
		})
	}
}

func TestRuntimeStartupPrivateFrameCLIUsesOneBooleanWithoutArgvPayload(t *testing.T) {
	cli, err := parseRuntimeServerCLI([]string{"--private-startup-frame-v1"})
	if err != nil || !cli.PrivateStartupFrameV1 || cli.MainOwnedAuthorityV1 != nil ||
		cli.MainOwnedAuthorityIdentityV1 != nil {
		t.Fatalf("private startup carrier flag was not parsed exactly: cli=%#v err=%v", cli, err)
	}
	if _, err := parseRuntimeServerCLI([]string{
		"--private-startup-frame-v1", "--private-startup-frame-v1",
	}); err == nil {
		t.Fatal("duplicated private startup frame flag was accepted")
	}
	if _, err := parseRuntimeServerCLI([]string{
		"--private-startup-frame-v1", `{"purpose":"analytix.runtime-startup-private-frame/v1"}`,
	}); err == nil {
		t.Fatal("a private startup argv payload was accepted")
	}
}

func TestRuntimeStartupPrivateFrameRejectsInvalidAuthorityProjection(t *testing.T) {
	for name, mutate := range map[string]func(*runtimeMainOwnedAuthorityEnvelopeV1){
		"purpose": func(value *runtimeMainOwnedAuthorityEnvelopeV1) {
			value.Purpose = "analytix.runtime-main-owned-authority/unknown"
		},
		"anchor key": func(value *runtimeMainOwnedAuthorityEnvelopeV1) {
			value.AuthorityAnchorV1.AuthorityKeyID = strings.Repeat("f", 64)
		},
		"relative root": func(value *runtimeMainOwnedAuthorityEnvelopeV1) {
			value.AuthorityManifestRoot = "relative"
		},
		"duplicate root": func(value *runtimeMainOwnedAuthorityEnvelopeV1) {
			value.AuthorityCredentialProfileRoot = value.AuthorityManifestRoot
		},
	} {
		t.Run(name, func(t *testing.T) {
			envelope := runtimeMainOwnedAuthorityEnvelopeFixtureV1()
			mutate(&envelope)
			body, err := json.Marshal(runtimeStartupPrivateFrameV1{
				SchemaVersion: 1, Purpose: runtimeStartupPrivateFramePurposeV1, ProtectedAuthorityV1: &envelope,
			})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := readRuntimeStartupPrivateFrameV1(
				bytes.NewReader(runtimeStartupPrivateFrameBytesV1(body)),
			); err == nil {
				t.Fatal("invalid private authority was accepted")
			}
		})
	}
}

func runtimeStartupPrivateFrameBytesV1(body []byte) []byte {
	frame := make([]byte, 8+len(body))
	binary.BigEndian.PutUint64(frame[:8], uint64(len(body)))
	copy(frame[8:], body)
	return frame
}

func runtimeMainOwnedAuthorityEnvelopeFixtureV1() runtimeMainOwnedAuthorityEnvelopeV1 {
	publicKey := bytes.Repeat([]byte{0x42}, 32)
	keyDigest := sha256.Sum256(publicKey)
	return runtimeMainOwnedAuthorityEnvelopeV1{
		SchemaVersion: 1, Purpose: runtimeMainOwnedAuthorityPurposeV1,
		AuthorityAnchorV1: authorityAnchorEnvelopeV1{
			SchemaVersion: 1, InstallationID: strings.Repeat("1", 64),
			AuthorityKeyID:        hex.EncodeToString(keyDigest[:]),
			AuthorityPublicKey:    base64.RawURLEncoding.EncodeToString(publicKey),
			CurrentManifestDigest: strings.Repeat("2", 64),
		},
		AuthorityManifestRoot:          "/private/authority/manifest",
		AuthorityCredentialProfileRoot: "/private/authority/profile",
		AuthorityCredentialBundleRoot:  "/private/authority/bundle",
	}
}

func runtimeHostScheduleMCPBindingFixtureV1() runtimeHostScheduleMCPBindingV1 {
	return runtimeHostScheduleMCPBindingV1{
		SchemaVersion: 1,
		Purpose:       runtimeHostScheduleMCPBindingPurposeV1,
		ServerID:      "gui_schedule",
		Command:       "/Applications/Analytix.app/Contents/Frameworks/Analytix Helper.app/Contents/MacOS/Analytix Helper",
		Args: []string{
			"/Applications/Analytix.app/Contents/Resources/app.asar/out/main/claw-schedule-mcp-node-entry.js",
			"--gui-schedule-mcp-server",
			"--base-url",
			"http://127.0.0.1:9787",
		},
		Env:        map[string]string{"ELECTRON_RUN_AS_NODE": "1"},
		TrustScope: "user",
		TimeoutMS:  5000,
	}
}
