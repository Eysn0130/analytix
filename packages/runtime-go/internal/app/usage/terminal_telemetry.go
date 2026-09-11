package usage

import (
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainterminaltelemetry "analytix.local/runtime-go/internal/domain/terminaltelemetry"
)

const (
	TerminalCacheDiagnosticsSchemaV1 = domainterminaltelemetry.TerminalCacheDiagnosticsSchemaV1
	terminalCacheDiagnosticsRejected = "rejected"
)

type TerminalTelemetryV1 = domainterminaltelemetry.TerminalTelemetryV1

func NewTerminalTelemetryV1(usage domainmodel.Usage, diagnostics map[string]any) TerminalTelemetryV1 {
	return domainterminaltelemetry.NewTerminalTelemetryV1(usage, diagnostics)
}

func ValidateTerminalTelemetryPublicMapsV1(usageMap, diagnosticsMap map[string]any) bool {
	return domainterminaltelemetry.ValidateTerminalTelemetryPublicMapsV1(usageMap, diagnosticsMap)
}
