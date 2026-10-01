package runtimeapp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"time"

	providerregistryapp "analytix.local/runtime-go/internal/app/providerregistry"
	domainregistry "analytix.local/runtime-go/internal/domain/providerregistry"
	"analytix.local/runtime-go/internal/ports"
	secretstoreport "analytix.local/runtime-go/internal/ports/secretstore"
	provider "analytix.local/runtime-go/internal/provider"
)

// VerifyDevelopmentProvider uses the normal Core Registry, protected credential
// resolution and Provider adapter. It never exports credential or response bytes.
// Bootstrap is one explicit stdin-only operation; ordinary verification consumes
// no credential input. The bounded synthetic request has no tools or local data.
func VerifyDevelopmentProvider(ctx context.Context, root string, bootstrap io.Reader, output io.Writer) error {
	client := provider.NewHTTPProviderClient(provider.NewDefaultHTTPClient(30 * time.Second))
	client.MaxStreamReconnects = -1
	return verifyDevelopmentProvider(ctx, root, bootstrap, output, client)
}

func verifyDevelopmentProvider(ctx context.Context, root string, bootstrap io.Reader, output io.Writer, client ports.ProviderClient) error {
	result := struct {
		ProviderID           string  `json:"providerId"`
		SelectedModel        string  `json:"selectedModel"`
		CredentialConfigured bool    `json:"credentialConfigured"`
		CredentialResolved   bool    `json:"credentialResolved"`
		ProviderReachable    bool    `json:"providerReachable"`
		ReturnedModel        *string `json:"returnedModel"`
		LatencyMillis        int64   `json:"latencyMillis"`
		PromptTokens         int     `json:"promptTokens"`
		CompletionTokens     int     `json:"completionTokens"`
		Status               string  `json:"status"`
	}{ProviderID: "deepseek", SelectedModel: "deepseek-flash", Status: "FAIL"}
	err := func() error {
		ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		authority, err := openDevelopmentProviderAuthority(ctx, Config{DevelopmentProviderAuthorityDir: root}, false)
		if err != nil {
			return err
		}
		defer authority.Close()
		snapshot, err := authority.Manager().Snapshot(ctx)
		if err != nil {
			return err
		}
		var current domainregistry.Provider
		for _, entry := range snapshot.Providers {
			if entry.ID == result.ProviderID {
				current = entry
			}
		}
		if bootstrap != nil {
			if current.ID != "" {
				return errors.New("development credential already configured; use Settings to replace")
			}
			secret, err := io.ReadAll(io.LimitReader(bootstrap, 4097))
			defer clear(secret)
			if err != nil || len(secret) == 0 || len(secret) > 4096 || bytes.ContainsAny(secret, "\r\n\x00") {
				return errors.New("invalid bootstrap input")
			}
			mutation, err := secretstoreport.SetCredential(secret)
			if err != nil {
				return err
			}
			current, err = authority.Manager().Connect(ctx, providerregistryapp.ConnectCommand{
				DeferSelection: true, Expected: domainregistry.ExpectedState{RegistryRevision: snapshot.Revision, RegistryIncarnation: snapshot.Incarnation},
				Provider:          domainregistry.ProviderInput{ID: result.ProviderID, Kind: "deepseek", Endpoint: "https://api.deepseek.com", Models: []string{result.SelectedModel}, SelectedModel: result.SelectedModel},
				CredentialPurpose: "provider-api-key", Credential: mutation,
			})
			if err != nil {
				return err
			}
			snapshot, err = authority.Manager().Snapshot(ctx)
			if err != nil {
				return err
			}
		}
		if current.ID != result.ProviderID || current.Tombstone || current.Endpoint != "https://api.deepseek.com" || current.Proxy != "" || current.SelectedModel != result.SelectedModel || current.Kind != "deepseek" {
			return errors.New("development Provider configuration mismatch")
		}
		result.CredentialConfigured = current.CredentialRef != ""
		command := providerregistryapp.ProviderOperationCommand{ProviderID: current.ID, Expected: domainregistry.ExpectedState{
			RegistryRevision: snapshot.Revision, RegistryIncarnation: snapshot.Incarnation, ProviderRevision: current.Revision, ProviderGeneration: current.Generation, ProviderIncarnation: current.Incarnation, ProviderCredentialPurpose: current.CredentialPurpose,
		}}
		resolution, err := authority.Service().resolve(ctx, command)
		if err != nil {
			return err
		}
		defer resolution.Clear()
		result.CredentialResolved = true
		execution, err := newProviderRegistryExecutionResolverV1(authority.Manager()).resolveTurnExecutionV1(provider.TurnExecutionInput{}, resolution.Provider, string(resolution.Credential))
		if err != nil {
			return err
		}
		defer func() { execution.Config.APIKey = "" }()
		config := execution.Config
		request := provider.Request{ProviderID: config.ProviderID, Family: config.Family, EndpointFormat: config.EndpointFormat, BaseURL: config.BaseURL, APIKey: config.APIKey, Model: config.Model, MaxOutputTokens: 32,
			PrivateProviderProxyAuthority:        true,
			Messages:                             []provider.Message{{Role: "user", Content: "Synthetic connection check. Reply with exactly OK."}},
			PrivateProviderCurrentnessBeforeSend: func(int) error { return authority.Manager().ValidateProviderOperationCurrent(ctx, command) },
		}
		defer func() { config.APIKey = ""; request.APIKey = "" }()
		started := time.Now()
		response, err := client.Stream(ctx, request)
		result.LatencyMillis = time.Since(started).Milliseconds()
		if err != nil {
			return errors.New("development Provider request failed")
		}
		if err = authority.Manager().ValidateProviderOperationCurrent(ctx, command); err != nil {
			return err
		}
		var text strings.Builder
		for _, chunk := range response.Chunks {
			if chunk.Kind == provider.ChunkText {
				text.WriteString(chunk.Text)
			}
		}
		if !response.StreamCompleted || strings.TrimSpace(text.String()) != "OK" {
			return errors.New("development Provider response did not match bounded check")
		}
		result.ProviderReachable = true
		result.PromptTokens = response.Usage.PromptTokens
		result.CompletionTokens = response.Usage.CompletionTokens
		// Adapter currently does not retain the upstream model echo. Report null
		// rather than inventing returnedModel from the selected model.
		if snapshot.SelectedProviderID != current.ID {
			if _, err = authority.Manager().Select(ctx, providerregistryapp.SelectCommand{ProviderID: current.ID, Expected: command.Expected}); err != nil {
				return err
			}
		}
		result.Status = "PASS"
		return nil
	}()
	if encodeErr := json.NewEncoder(output).Encode(result); encodeErr != nil {
		return errors.New("development verification output failed")
	}
	if err != nil {
		return errors.New("development Provider verification failed")
	}
	return nil
}
