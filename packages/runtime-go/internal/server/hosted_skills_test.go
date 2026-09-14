package server

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	pluginauthority "analytix.local/runtime-go/internal/adapters/outbound/pluginmaterializationauthority"
	pluginstore "analytix.local/runtime-go/internal/adapters/outbound/pluginmaterializationfs"
	materializationapp "analytix.local/runtime-go/internal/app/pluginmaterialization"
	hostapp "analytix.local/runtime-go/internal/app/pluginpackagehost"
	toolcatalogapp "analytix.local/runtime-go/internal/app/toolcatalog"
	domainplugin "analytix.local/runtime-go/internal/domain/pluginmaterialization"
	domainpackage "analytix.local/runtime-go/internal/domain/pluginpackage"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	codecport "analytix.local/runtime-go/internal/ports/documentgeneration"
	"analytix.local/runtime-go/internal/provider"
)

type unavailableDocumentsCodec struct{}

func (unavailableDocumentsCodec) Encode(context.Context, codecport.Input) ([]byte, error) {
	panic("catalog must not execute codec")
}

func documentsRuntimeFixture(t *testing.T) (*runtimeServerHandler, hostapp.PackageView) {
	return officeRuntimeFixture(t, "docx")
}
func (unavailableDocumentsCodec) Supports(kind string) bool {
	return kind == "docx" || kind == "xlsx" || kind == "pptx"
}
func officeRuntimeFixture(t *testing.T, kind string) (*runtimeServerHandler, hostapp.PackageView) {
	t.Helper()
	ctx := context.Background()
	packageID := toolcatalogapp.OfficeSkillForKind(kind)
	root, err := filepath.Abs(filepath.Join("../../../../plugins", packageID))
	if err != nil {
		t.Fatal(err)
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	observed, err := pluginstore.InspectDevelopmentSourceTreeV1(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	registration, err := domainpackage.ParseDevelopmentSourceRegistrationV1([]byte(observed.SourceRegistrationJSON))
	if err != nil {
		t.Fatal(err)
	}
	binding, err := materializationapp.NewDevelopmentSourceBindingV1(registration, root, domainplugin.TargetV1{Platform: "darwin", Arch: "arm64"})
	if err != nil {
		t.Fatal(err)
	}
	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	data := filepath.Join(home, "data")
	if err = os.Mkdir(data, 0700); err != nil {
		t.Fatal(err)
	}
	authority, err := pluginauthority.OpenOrCreateV1(data, false)
	if err != nil {
		t.Fatal(err)
	}
	store, err := pluginstore.NewPackageStoreV1(home, packageID, nil)
	if err != nil {
		t.Fatal(err)
	}
	service, err := materializationapp.NewDevelopmentSourceServiceV1(store, authority, binding, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	intent, err := binding.NewIntentV1(time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	installed, err := service.Materialize(ctx, intent)
	if err != nil {
		t.Fatal(err)
	}
	host, err := hostapp.New(testIdentityAuthority(), authority, []hostapp.Registration{{Identity: registration.Identity, SourceRegistrationSHA256: observed.SourceRegistrationSHA256, Materialization: service, State: store, SkillReader: pluginstore.OfficeSkillReader{Store: store, Authority: authority, SHA256: registration.OfficeSkillSHA256V1()}}}, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	h := &runtimeServerHandler{officePackageHost: host, documentCodec: unavailableDocumentsCodec{}}
	h.turnSecurity.Identity = testIdentityAuthority()
	view, err := host.SetDesiredState(ctx, hostapp.SetDesiredStateRequest{PackageID: packageID, GenerationID: installed.Receipt.GenerationID, DesiredState: domainplugin.DesiredEnabledV1})
	if err != nil {
		t.Fatal(err)
	}
	return h, view
}

func TestOfficeRuntimeDiscoveryInlineAndPreparedGenerationFollowActivation(t *testing.T) {
	for _, kind := range []string{"docx", "xlsx", "pptx"} {
		t.Run(kind, func(t *testing.T) {
			h, view := officeRuntimeFixture(t, kind)
			packageID := toolcatalogapp.OfficeSkillForKind(kind)
			ctx := context.Background()
			if !h.runtimeToolCatalog().DocumentGeneration {
				t.Fatal("enabled generation not advertised")
			}
			if _, ok := h.runtimeSkillByName("$" + packageID); !ok {
				t.Fatal("installed skill not discovered")
			}
			args := map[string]any{"name": packageID, "arguments": "Create a synthetic report."}
			if output, failed := h.executeRuntimeRunSkill(ctx, runtimePendingToolCall{}, args); failed || !strings.Contains(string(mustHostedJSON(t, output)), "generate_office_document") {
				t.Fatal("real inline body not loaded", output)
			}
			generationArgs := map[string]any{"kind": kind, "path": "report." + kind}
			switch kind {
			case "docx":
				generationArgs["markdown"] = "Synthetic"
			case "xlsx":
				generationArgs["workbook"] = map[string]any{"sheets": []any{map[string]any{"id": "s", "name": "数据", "cells": []any{map[string]any{"address": "A1", "type": "number", "value": 42}}}}}
			case "pptx":
				generationArgs["presentation"] = map[string]any{"slides": []any{map[string]any{"id": "s", "objects": []any{map[string]any{"id": "t", "kind": "text", "x": 0, "y": 0, "w": 1, "h": 1, "text": "Synthetic"}}}}}
			}
			advertised := h.runtimeToolCatalog().DocumentGenerationKinds
			if len(advertised) != 1 || advertised[0] != kind {
				t.Fatal("wrong advertised kinds", advertised)
			}
			pending := runtimePendingToolCall{Call: provider.ToolCall{Name: "generate_office_document", Arguments: json.RawMessage(mustHostedJSON(t, generationArgs))}, ExecutionGrant: domainsecurity.ExecutionGrant{ToolName: "generate_office_document"}, SecurityContext: domainsecurity.TurnSecurityContext{WorkspaceRealPath: t.TempDir()}}
			prepared, err := h.prepareRuntimeSideEffect(ctx, pending, time.Now().UTC())
			if err != nil || prepared.Rejection != nil || prepared.Bind == nil {
				t.Fatal("generation not prepared", err)
			}
			bound := prepared.Bind(ctx)
			if _, ok := bound.Value(preparedHostedSkillKey{}).(hostapp.HostedSkill); !ok {
				t.Fatal("generation lost frozen plugin binding")
			}
			disabled, err := h.officePackageHost.SetDesiredState(ctx, hostapp.SetDesiredStateRequest{PackageID: view.PackageID, GenerationID: view.GenerationID, ExpectedRevision: view.ActivationRevision, DesiredState: domainplugin.DesiredDisabledV1})
			if err != nil {
				t.Fatal(err)
			}
			if h.runtimeToolCatalog().DocumentGeneration {
				t.Fatal("disabled generation advertised")
			}
			if _, ok := h.runtimeSkillByName(packageID); ok {
				t.Fatal("disabled skill advertised")
			}
			if _, failed := h.executeRuntimeRunSkill(bound, runtimePendingToolCall{}, args); !failed {
				t.Fatal("disabled prepared body executed")
			}
			if _, err = h.officePackageHost.SetDesiredState(ctx, hostapp.SetDesiredStateRequest{PackageID: view.PackageID, GenerationID: view.GenerationID, ExpectedRevision: disabled.ActivationRevision, DesiredState: domainplugin.DesiredEnabledV1}); err != nil {
				t.Fatal(err)
			}
			if _, failed := h.executeRuntimeRunSkill(bound, runtimePendingToolCall{}, args); !failed {
				t.Fatal("reenable revived old prepared authority")
			}
			if _, failed := h.executeRuntimeRunSkill(ctx, runtimePendingToolCall{}, args); failed {
				t.Fatal("fresh skill unavailable after reenable")
			}
			fresh, err := h.prepareRuntimeSideEffect(ctx, pending, time.Now().UTC())
			if err != nil || fresh.SemanticIdentity.ArgsHash == prepared.SemanticIdentity.ArgsHash {
				t.Fatal("activation absent from semantic identity", err)
			}
		})
	}
}
func mustHostedJSON(t *testing.T, value any) []byte {
	t.Helper()
	body, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return body
}
