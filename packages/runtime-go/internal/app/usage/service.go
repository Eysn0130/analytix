package usage

import (
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainterminaltelemetry "analytix.local/runtime-go/internal/domain/terminaltelemetry"
)

type Service struct{}

func NewService() *Service {
	return &Service{}
}

func (s *Service) HasCacheTelemetry(usage domainmodel.Usage) bool {
	return usage.HasCacheTelemetry()
}

func ProviderUsageMap(usage domainmodel.Usage) map[string]any {
	return domainterminaltelemetry.ProviderUsageMap(usage)
}
