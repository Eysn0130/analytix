//go:build !analytix_prod

package upstreamaudit

import readiness "analytix.local/runtime-go/internal/readiness"

func RuntimeUpstreamAbsorptionCapabilities() map[string]any {
	reasonixCapabilityMatrix := BuildReasonixCapabilityAuditMatrix()
	kunAnalytixBaselineGuard := BuildKunAnalytixBaselineGuard()
	reasonixAbsorptionMatrix := BuildReasonixAbsorptionMatrix()
	defaultBackendReadiness := readiness.RuntimeReadinessStatusFromEnv(nil)

	return map[string]any{
		"reasonixCapabilityMatrix":    reasonixCapabilityMatrix,
		"kunAnalytixBaselineGuard":    kunAnalytixBaselineGuard,
		"reasonixAbsorptionMatrix":    reasonixAbsorptionMatrix,
		"reasonixIntegrationTopology": BuildReasonixIntegrationTopology(defaultBackendReadiness),
		"reasonixSuperiorityMatrix":   BuildReasonixSuperiorityMatrix(defaultBackendReadiness),
		"defaultBackendReadiness":     defaultBackendReadiness,
		"readinessSemantics": readiness.BuildRuntimeReadinessSemantics(readiness.RuntimeReadinessSemanticsInput{
			CapabilityMatrixGreen:   reasonixCapabilityMatrix.CapabilityMatrixGreen,
			AbsorptionMatrixGreen:   reasonixAbsorptionMatrix.AbsorptionMatrixGreen,
			BaselineGuardGreen:      baselineGuardGreen(kunAnalytixBaselineGuard),
			DefaultBackendReadiness: defaultBackendReadiness,
		}),
	}
}

func baselineGuardGreen(baselineGuard KunAnalytixBaselineGuard) bool {
	return baselineGuard.FullFunctionBaseline &&
		baselineGuard.ForbiddenEntrypointExposed == false &&
		baselineGuard.DeprecatedBridgeAliasAllowed == false &&
		baselineGuard.LegacySettingsWriteAllowed == false &&
		baselineGuard.DeepSeekOnlyRuntimeAllowed == false &&
		baselineGuard.ReadyForReasonixAbsorption
}
