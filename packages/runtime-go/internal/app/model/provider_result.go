package model

import domainmodel "analytix.local/runtime-go/internal/domain/model"

// NormalizeProviderResult binds every persisted/result identity to the host
// request and recomputes cache-prefix authority. Adapter-returned URLs, body
// fields, timing, model identity, and prefix metadata are diagnostic input,
// never publication or persistence authority.
func NormalizeProviderResult(request domainmodel.Request, result domainmodel.Result) domainmodel.Result {
	result.HostTiming = domainmodel.ProviderTiming{}
	result.ProviderID = request.ProviderID
	result.Family = request.Family
	result.EndpointFormat = request.EndpointFormat
	result.RequestURL = ""
	result.RequestBodyFields = nil
	result.Usage = domainmodel.NormalizeUsage(result.Usage)
	result.PrefixShape = CapturePrefixShape(request)
	result.FirstTokenLatencyMs = 0
	result.HasFirstTokenLatency = false
	result.FirstReasoningLatencyMs = 0
	result.HasFirstReasoningLatency = false
	result.FirstRawTextLatencyMs = 0
	result.HasFirstRawTextLatency = false
	result.DurationMs = 0
	result.HasDuration = false
	return result
}
