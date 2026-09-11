//go:build !analytix_prod

package readiness

const runtimeReadinessSemanticsID = "runtime-readiness-semantics"

type RuntimeReadinessSemantics struct {
	SchemaVersion                         int      `json:"schemaVersion"`
	ChangeID                              string   `json:"changeId"`
	ReasonixCapabilityMatrixGreen         bool     `json:"reasonixCapabilityMatrixGreen"`
	ReasonixAbsorptionMatrixGreen         bool     `json:"reasonixAbsorptionMatrixGreen"`
	KunAnalytixBaselineGuardGreen         bool     `json:"kunAnalytixBaselineGuardGreen"`
	CapabilityMatrixGreen                 bool     `json:"capabilityMatrixGreen"`
	AbsorptionMatrixGreen                 bool     `json:"absorptionMatrixGreen"`
	BaselineGuardGreen                    bool     `json:"baselineGuardGreen"`
	DefaultBackendReady                   bool     `json:"defaultBackendReady"`
	DefaultBackendGateChangeID            string   `json:"defaultBackendGateChangeId"`
	RequiredDefaultBackendGates           []string `json:"requiredDefaultBackendGates"`
	SkippedCountsAsPassed                 bool     `json:"skippedCountsAsPassed"`
	FixtureMatrixCountsAsCredentialedPass bool     `json:"fixtureMatrixCountsAsCredentialedPass"`
	MatrixGreenEnablesDefaultBackend      bool     `json:"matrixGreenEnablesDefaultBackend"`
	TypeScriptRuntimeDefault              bool     `json:"typeScriptRuntimeDefault"`
	GoRuntimeCandidateInternalOnly        bool     `json:"goRuntimeCandidateInternalOnly"`
	Notes                                 []string `json:"notes"`
}

type RuntimeReadinessSemanticsInput struct {
	CapabilityMatrixGreen   bool
	AbsorptionMatrixGreen   bool
	BaselineGuardGreen      bool
	DefaultBackendReadiness RuntimeReadinessStatus
}

func BuildRuntimeReadinessSemantics(input RuntimeReadinessSemanticsInput) RuntimeReadinessSemantics {
	return RuntimeReadinessSemantics{
		SchemaVersion:                 1,
		ChangeID:                      runtimeReadinessSemanticsID,
		ReasonixCapabilityMatrixGreen: input.CapabilityMatrixGreen,
		ReasonixAbsorptionMatrixGreen: input.AbsorptionMatrixGreen,
		KunAnalytixBaselineGuardGreen: input.BaselineGuardGreen,
		CapabilityMatrixGreen:         input.CapabilityMatrixGreen,
		AbsorptionMatrixGreen:         input.AbsorptionMatrixGreen,
		BaselineGuardGreen:            input.BaselineGuardGreen,
		DefaultBackendReady:           input.CapabilityMatrixGreen && input.AbsorptionMatrixGreen && input.BaselineGuardGreen,
		DefaultBackendGateChangeID:    "runtime-readiness",
		RequiredDefaultBackendGates: []string{
			"reasonixCapabilityMatrixGreen",
			"reasonixAbsorptionMatrixGreen",
			"kunAnalytixBaselineGuardGreen",
			"goRuntimeServerCanary",
			"adapterGoDefaultTest",
		},
		SkippedCountsAsPassed:                 false,
		FixtureMatrixCountsAsCredentialedPass: false,
		MatrixGreenEnablesDefaultBackend:      false,
		TypeScriptRuntimeDefault:              false,
		GoRuntimeCandidateInternalOnly:        false,
		Notes: []string{
			"Engine absorption matrix green means Reasonix absorption and Kun/Analytix baseline guard are clear of red/deferred rows.",
			"Go runtime server is the default core after deterministic local conformance and adapter canary pass.",
			"Credentialed provider, credentialed MCP, packaged QA, and operator evidence remain post-cutover live validation and do not block deterministic default selection.",
			"Skipped credentialed checks and fixture/local matrix passes still do not count as live provider/MCP evidence.",
		},
	}
}
