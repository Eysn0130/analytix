package model

import (
	"errors"
	"strings"
	"time"

	domainmodel "analytix.local/runtime-go/internal/domain/model"
)

type Service struct{}

func NewService() *Service {
	return &Service{}
}

func (s *Service) Ref(providerID, id, variant string) domainmodel.ModelRef {
	return domainmodel.ModelRef{ProviderID: providerID, ID: id, Variant: variant}
}

func ProviderFamily(providerID string, baseURL string, model string, endpointFormat string) string {
	return domainmodel.ProviderFamily(providerID, baseURL, model, endpointFormat)
}

func OptionalEndpointFormat(value string) string {
	return domainmodel.OptionalEndpointFormat(value)
}

func ExecutionRef(providerID, model, variant, endpointFormat, baseURL, source string) (map[string]any, error) {
	source = ExecutionSourceName(source)
	providerID = strings.TrimSpace(providerID)
	model = strings.TrimSpace(model)
	if providerID == "" {
		return nil, errors.New("provider_not_found: model execution ref requires a providerId")
	}
	if model == "" {
		return nil, errors.New("model_not_found: model execution ref requires a modelId")
	}
	ref := map[string]any{
		"providerId": providerID,
		"modelId":    model,
		"source":     source,
		"resolvedAt": time.Now().UTC().Format(time.RFC3339Nano),
	}
	if strings.TrimSpace(variant) != "" {
		ref["variant"] = strings.TrimSpace(variant)
	}
	if format := OptionalEndpointFormat(endpointFormat); format != "" {
		ref["endpointFormat"] = format
	}
	if trimmedBase := strings.TrimRight(strings.TrimSpace(baseURL), "/"); trimmedBase != "" {
		fingerprint := domainmodel.StringHash(trimmedBase)
		if OptionalEndpointFormat(endpointFormat) == "custom_endpoint" {
			ref["customFullEndpointFingerprint"] = fingerprint
		} else {
			ref["baseUrlFingerprint"] = fingerprint
		}
	}
	ref["capabilityFingerprint"] = domainmodel.StringHash(strings.Join([]string{
		providerID,
		model,
		strings.TrimSpace(variant),
		OptionalEndpointFormat(endpointFormat),
	}, "\x00"))
	return ref, nil
}

func ExecutionSourceName(source string) string {
	switch strings.TrimSpace(source) {
	case "explicit-input", "subagent-profile", "session", "thread", "runtime-default":
		return strings.TrimSpace(source)
	default:
		return "runtime-default"
	}
}
