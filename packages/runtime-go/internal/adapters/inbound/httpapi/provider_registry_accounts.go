package httpapi

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strconv"

	providerregistryapp "analytix.local/runtime-go/internal/app/providerregistry"
	domainregistry "analytix.local/runtime-go/internal/domain/providerregistry"
)

type providerRegistryPrivateAccountScopeRequest struct {
	Owner     string `json:"owner"`
	Provider  string `json:"provider"`
	AccountID string `json:"accountId"`
	ChannelID string `json:"channelId,omitempty"`
	Purpose   string `json:"purpose"`
}

func (scope providerRegistryPrivateAccountScopeRequest) domain() (domainregistry.PrivateAccountScope, bool) {
	result := domainregistry.PrivateAccountScope{
		SchemaVersion: 1, Owner: scope.Owner, Provider: scope.Provider,
		AccountID: scope.AccountID, ChannelID: scope.ChannelID, Purpose: scope.Purpose,
	}
	return result, result.Validate() == nil
}

type providerRegistryPrivateAccountStatusRequest struct {
	SchemaVersion int                                        `json:"schemaVersion"`
	Scope         providerRegistryPrivateAccountScopeRequest `json:"scope"`
}

type providerRegistryPrivateAccountListRequest struct {
	SchemaVersion int    `json:"schemaVersion"`
	Owner         string `json:"owner"`
	Purpose       string `json:"purpose"`
}

type providerRegistryPrivateAccountListResponse struct {
	SchemaVersion int                                           `json:"schemaVersion"`
	Accounts      []providerRegistryPrivateAccountStateResponse `json:"accounts"`
}

type providerRegistryPrivateAccountPutRequest struct {
	SchemaVersion int                                        `json:"schemaVersion"`
	Scope         providerRegistryPrivateAccountScopeRequest `json:"scope"`
	Expected      providerRegistryExpectedRequest            `json:"expected"`
	ValueBase64   json.RawMessage                            `json:"valueBase64"`
}

type providerRegistryPrivateAccountMutationRequest struct {
	SchemaVersion int                                        `json:"schemaVersion"`
	Scope         providerRegistryPrivateAccountScopeRequest `json:"scope"`
	Expected      providerRegistryExpectedRequest            `json:"expected"`
	Disposition   string                                     `json:"disposition"`
}

type providerRegistryPrivateAccountStateResponse struct {
	SchemaVersion       int                                        `json:"schemaVersion"`
	Scope               providerRegistryPrivateAccountScopeRequest `json:"scope"`
	Status              string                                     `json:"status"`
	RegistryRevision    string                                     `json:"registryRevision"`
	RegistryIncarnation string                                     `json:"registryIncarnation"`
	ProviderRevision    string                                     `json:"providerRevision"`
	ProviderGeneration  string                                     `json:"providerGeneration"`
	ProviderIncarnation string                                     `json:"providerIncarnation"`
	CredentialPurpose   string                                     `json:"credentialPurpose,omitempty"`
}

type providerRegistryPrivateAccountResolveResponse struct {
	providerRegistryPrivateAccountStateResponse
	ValueBase64 string `json:"valueBase64"`
}

func (handlers ProviderRegistryHandlers) handlePrivateAccountStatus(w http.ResponseWriter, r *http.Request) {
	var request providerRegistryPrivateAccountStatusRequest
	if !requireProviderRegistryUniqueRequest(w, r, &request) {
		return
	}
	scope, ok := request.Scope.domain()
	if request.SchemaVersion != 1 || !ok {
		writeProviderRegistryFailure(w, http.StatusBadRequest, "invalid_request")
		return
	}
	state, err := handlers.Service.AccountCredentialState(r.Context(), scope)
	if err != nil {
		writeProviderRegistryServiceError(w, err)
		return
	}
	writeProviderRegistryJSON(w, http.StatusOK, projectProviderRegistryPrivateAccountState(state))
}

func (handlers ProviderRegistryHandlers) handlePrivateAccountList(w http.ResponseWriter, r *http.Request) {
	var request providerRegistryPrivateAccountListRequest
	if !requireProviderRegistryUniqueRequest(w, r, &request) {
		return
	}
	filter := providerregistryapp.AccountCredentialListFilter{Owner: request.Owner, Purpose: request.Purpose}
	if request.SchemaVersion != 1 ||
		!((filter.Owner == "provider" && filter.Purpose == "provider-oauth-authorization-state") ||
			(filter.Owner == "provider" && filter.Purpose == "provider-oauth-token-bundle") ||
			(filter.Owner == "mcp" && filter.Purpose == "mcp-oauth-authorization-state") ||
			(filter.Owner == "mcp" && filter.Purpose == "mcp-oauth-access-token") ||
			(filter.Owner == "extension" && (filter.Purpose == "extension-provider-account-token" ||
				filter.Purpose == "extension-oauth-authorization-state"))) {
		writeProviderRegistryFailure(w, http.StatusBadRequest, "invalid_request")
		return
	}
	states, err := handlers.Service.ListAccountCredentialStates(r.Context(), filter)
	if err != nil {
		writeProviderRegistryServiceError(w, err)
		return
	}
	accounts := make([]providerRegistryPrivateAccountStateResponse, 0, len(states))
	for _, state := range states {
		accounts = append(accounts, projectProviderRegistryPrivateAccountState(state))
	}
	writeProviderRegistryJSON(w, http.StatusOK, providerRegistryPrivateAccountListResponse{
		SchemaVersion: 1,
		Accounts:      accounts,
	})
}

func (handlers ProviderRegistryHandlers) handlePrivateAccountPut(w http.ResponseWriter, r *http.Request) {
	var request providerRegistryPrivateAccountPutRequest
	defer func() {
		clearProviderRegistryBytes(request.ValueBase64)
		if request.ValueBase64 != nil && handlers.observeEncodedCredentialBufferCleared != nil {
			handlers.observeEncodedCredentialBufferCleared(request.ValueBase64)
		}
		request.ValueBase64 = nil
	}()
	if !requireProviderRegistryUniqueRequest(w, r, &request) {
		return
	}
	credential, credentialOK := decodeProviderRegistryCanonicalBase64(request.ValueBase64)
	defer func() {
		clearProviderRegistryBytes(credential)
		if credential != nil && handlers.observeCredentialBufferCleared != nil {
			handlers.observeCredentialBufferCleared(credential)
		}
	}()
	scope, scopeOK := request.Scope.domain()
	expected, expectedOK := request.Expected.parseLegacyMigration()
	if request.SchemaVersion != 1 || !scopeOK || !expectedOK || !credentialOK ||
		providerRegistryCredentialIsSentinel(credential) {
		writeProviderRegistryFailure(w, http.StatusBadRequest, "invalid_request")
		return
	}
	state, err := handlers.Service.PutAccountCredential(r.Context(), providerregistryapp.AccountCredentialPutCommand{
		Scope: scope, Expected: expected, Credential: credential,
	})
	if err != nil {
		writeProviderRegistryServiceError(w, err)
		return
	}
	writeProviderRegistryJSON(w, http.StatusOK, projectProviderRegistryPrivateAccountState(state))
}

func (handlers ProviderRegistryHandlers) handlePrivateAccountResolve(w http.ResponseWriter, r *http.Request) {
	var request providerRegistryPrivateAccountStatusRequest
	if !requireProviderRegistryUniqueRequest(w, r, &request) {
		return
	}
	scope, ok := request.Scope.domain()
	if request.SchemaVersion != 1 || !ok {
		writeProviderRegistryFailure(w, http.StatusBadRequest, "invalid_request")
		return
	}
	resolution, err := handlers.Service.ResolveAccountCredential(r.Context(), scope)
	if err != nil {
		writeProviderRegistryServiceError(w, err)
		return
	}
	defer resolution.Clear()
	encoded := make([]byte, base64.StdEncoding.EncodedLen(len(resolution.Credential)))
	base64.StdEncoding.Encode(encoded, resolution.Credential)
	defer clear(encoded)
	response := providerRegistryPrivateAccountResolveResponse{
		providerRegistryPrivateAccountStateResponse: projectProviderRegistryPrivateAccountState(resolution.State),
		ValueBase64: string(encoded),
	}
	writeProviderRegistryJSON(w, http.StatusOK, response)
}

func (handlers ProviderRegistryHandlers) handlePrivateAccountMutate(w http.ResponseWriter, r *http.Request) {
	var request providerRegistryPrivateAccountMutationRequest
	if !requireProviderRegistryUniqueRequest(w, r, &request) {
		return
	}
	scope, scopeOK := request.Scope.domain()
	expected, expectedOK := request.Expected.parseLegacyMigration()
	if request.SchemaVersion != 1 || !scopeOK || !expectedOK ||
		(request.Disposition != providerregistryapp.AccountCredentialDispositionRevoke &&
			request.Disposition != providerregistryapp.AccountCredentialDispositionDisconnect &&
			request.Disposition != providerregistryapp.AccountCredentialDispositionDelete) {
		writeProviderRegistryFailure(w, http.StatusBadRequest, "invalid_request")
		return
	}
	state, err := handlers.Service.MutateAccountCredential(r.Context(), providerregistryapp.AccountCredentialMutationCommand{
		Scope: scope, Expected: expected, Disposition: request.Disposition,
	})
	if err != nil {
		writeProviderRegistryServiceError(w, err)
		return
	}
	writeProviderRegistryJSON(w, http.StatusOK, projectProviderRegistryPrivateAccountState(state))
}

func projectProviderRegistryPrivateAccountState(
	state providerregistryapp.AccountCredentialState,
) providerRegistryPrivateAccountStateResponse {
	return providerRegistryPrivateAccountStateResponse{
		SchemaVersion: 1,
		Scope: providerRegistryPrivateAccountScopeRequest{
			Owner: state.Scope.Owner, Provider: state.Scope.Provider,
			AccountID: state.Scope.AccountID, ChannelID: state.Scope.ChannelID, Purpose: state.Scope.Purpose,
		},
		Status: string(state.Status), RegistryRevision: strconv.FormatUint(state.RegistryRevision, 10),
		RegistryIncarnation: state.RegistryIncarnation,
		ProviderRevision:    strconv.FormatUint(state.ProviderRevision, 10),
		ProviderGeneration:  strconv.FormatUint(state.ProviderGeneration, 10),
		ProviderIncarnation: state.ProviderIncarnation,
		CredentialPurpose:   state.CredentialPurpose,
	}
}
