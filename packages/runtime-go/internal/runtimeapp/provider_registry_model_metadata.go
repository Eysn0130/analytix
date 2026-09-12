package runtimeapp

import (
	"encoding/json"
	"errors"
	"strings"

	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainregistry "analytix.local/runtime-go/internal/domain/providerregistry"
	"analytix.local/runtime-go/internal/provider"
)

// These key-free preferences describe only models the Registry already
// admits. Profile keys, aliases and reasoning cannot supply another Provider,
// endpoint, credential, executable model, or additional tool/media capability.
type providerRegistryModelMetadataV1 struct {
	ID       string `json:"id"`
	Endpoint string `json:"baseUrl"`
	Protocol string `json:"endpointFormat"`
	Profiles map[string]struct {
		Aliases   []string                            `json:"aliases"`
		Reasoning *domainmodel.ModelProviderReasoning `json:"reasoning"`
	} `json:"modelProfiles"`
}

func (resolver *providerRegistryExecutionResolverV1) boundedModelProfilesV1(selected domainregistry.Provider) (map[string]domainmodel.ModelProviderProfile, error) {
	protocol, err := providerRegistryEndpointFormatV1(selected.Kind)
	if err != nil {
		return nil, err
	}
	var exact *providerRegistryModelMetadataV1
	matches, hasProfiles := 0, false
	for index := range resolver.modelMetadata {
		candidate := &resolver.modelMetadata[index]
		format, _, parseErr := domainmodel.ParseEndpointFormat(candidate.Protocol)
		if parseErr != nil {
			continue
		}
		if format == "" {
			format = "chat_completions"
		}
		if strings.TrimSpace(candidate.ID) != selected.ID ||
			strings.TrimRight(strings.TrimSpace(candidate.Endpoint), "/") != strings.TrimRight(strings.TrimSpace(selected.Endpoint), "/") || format != protocol {
			continue
		}
		exact, matches = candidate, matches+1
		hasProfiles = hasProfiles || len(candidate.Profiles) != 0
	}
	if exact == nil || !hasProfiles {
		return nil, nil
	}
	if matches != 1 {
		return nil, errors.New("provider model metadata route is ambiguous")
	}
	profiles := make(map[string]domainmodel.ModelProviderProfile)
	for _, model := range selected.Models {
		if profile, exists := exact.Profiles[model]; exists {
			profiles[model] = domainmodel.ModelProviderProfile{Aliases: profile.Aliases, Reasoning: profile.Reasoning}
		}
	}
	if len(profiles) == 0 {
		return nil, nil
	}
	if err := validateRegistryModelAliasesV1(selected, protocol, profiles); err != nil {
		return nil, err
	}
	return profiles, nil
}

// Ask the existing model parser which canonical model owns each spelling.
// Its normalizer and alias semantics remain the only resolver. Counting its
// matches rejects aliases that shadow another admitted model or alias instead
// of relying on the parser's historical map-key ordering to choose a winner.
func validateRegistryModelAliasesV1(selected domainregistry.Provider, protocol string, profiles map[string]domainmodel.ModelProviderProfile) error {
	spellings := append([]string(nil), selected.Models...)
	resolvers := make([]provider.RuntimeProviderConfigSet, 0, len(selected.Models))
	for _, model := range selected.Models {
		profile := profiles[model]
		for _, alias := range profile.Aliases {
			if strings.TrimSpace(alias) == "" {
				return errors.New("provider model alias is empty")
			}
			spellings = append(spellings, alias)
		}
		metadata := domainmodel.ModelProviderConfig{ID: selected.ID, BaseURL: selected.Endpoint, EndpointFormat: protocol,
			Models: []string{model}, ModelProfiles: map[string]domainmodel.ModelProviderProfile{model: profile}}
		encoded, err := json.Marshal(domainmodel.ModelProvidersConfig{DefaultProviderID: selected.ID, Providers: []domainmodel.ModelProviderConfig{metadata}})
		if err != nil {
			return errors.New("provider model metadata is invalid")
		}
		config := provider.NewRuntimeProviderConfigSet(provider.RuntimeProviderConfigInput{ModelProvidersJSON: string(encoded)})
		if err := config.ConfigurationError(); err != nil {
			return err
		}
		resolvers = append(resolvers, config)
	}
	for _, spelling := range spellings {
		owners := 0
		for _, config := range resolvers {
			if config.ValidateExecutionModel(selected.ID, spelling) == nil {
				owners++
			}
		}
		if owners != 1 {
			return errors.New("provider model alias is ambiguous")
		}
	}
	return nil
}
