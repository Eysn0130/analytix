package security

import (
	"testing"
	"time"
)

func TestTurnSecurityContextPublicationAdmissionMatrix(t *testing.T) {
	caseContext := mustCaseTurnSecurityContextV2(
		t,
		"thread-publication-case",
		"turn-publication-case",
		"/workspace/publication-case",
		"case-publication",
		7,
		time.Date(2026, 7, 12, 8, 0, 0, 0, time.UTC),
	)
	boundaryContext := mustWitnessedBoundaryTurnSecurityContextV2(t)
	generalContext := mustGeneralTurnSecurityContextV2(
		t,
		"thread-publication-general",
		"turn-publication-general",
		"/workspace/publication-general",
	)
	auditContext := NewTurnSecurityContext(TurnSecurityContextInput{
		ThreadID:           "thread-publication-audit",
		TurnID:             "turn-publication-audit",
		WorkspaceRealPath:  "/workspace/publication-audit",
		CaseID:             "case-publication-audit",
		CaseBindingHash:    SHA256Hex([]byte("binding:case-publication-audit")),
		DatasetSnapshotID:  DatasetSnapshotIDPrefixV1 + SHA256Hex([]byte("snapshot:case-publication-audit")),
		SourceManifestHash: SHA256Hex([]byte("manifest:case-publication-audit")),
		ContextEpoch:       4,
		IssuedAt:           time.Date(2026, 7, 12, 8, 2, 0, 0, time.UTC),
	})
	invalidContext := caseContext
	invalidContext.ContextDigest = SHA256Hex([]byte("invalid-context-digest"))
	quarantinedCaseContext := caseContext
	quarantinedCaseContext.RiskAuthorityBinding = NewQuarantinedRiskAuthorityBindingV1()
	quarantinedCaseContext.ContextDigest = ""
	quarantinedCaseContext.ContextDigest = hashContract(quarantinedCaseContext)

	tests := []struct {
		name            string
		context         TurnSecurityContext
		wantPublication bool
		wantCaseFacts   bool
	}{
		{
			name:            "witnessed case evidence V2",
			context:         caseContext,
			wantPublication: true,
			wantCaseFacts:   true,
		},
		{
			name:            "witnessed boundary-only V2",
			context:         boundaryContext,
			wantPublication: true,
			wantCaseFacts:   false,
		},
		{
			name:            "witnessed general V2",
			context:         generalContext,
			wantPublication: false,
			wantCaseFacts:   false,
		},
		{
			name:            "audit-only V1",
			context:         auditContext,
			wantPublication: false,
			wantCaseFacts:   false,
		},
		{
			name:            "invalid integrity",
			context:         invalidContext,
			wantPublication: false,
			wantCaseFacts:   false,
		},
		{
			name:            "quarantined case evidence V2",
			context:         quarantinedCaseContext,
			wantPublication: false,
			wantCaseFacts:   false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			publicationErr := ValidateTurnSecurityContextForCasePublication(test.context)
			if got := publicationErr == nil; got != test.wantPublication {
				t.Fatalf("case publication admission = %t, want %t (err=%v)", got, test.wantPublication, publicationErr)
			}
			factErr := ValidateTurnSecurityContextForCaseFactPublication(test.context)
			if got := factErr == nil; got != test.wantCaseFacts {
				t.Fatalf("case fact publication admission = %t, want %t (err=%v)", got, test.wantCaseFacts, factErr)
			}
		})
	}
}

func mustWitnessedBoundaryTurnSecurityContextV2(t *testing.T) TurnSecurityContext {
	t.Helper()
	const (
		threadID  = "thread-publication-boundary"
		turnID    = "turn-publication-boundary"
		workspace = "/workspace/publication-boundary"
	)
	policy := mustTurnPublicationPolicyV1(t, TurnPublicationPolicyInputV1{
		ThreadRiskPolicyDigest:   SHA256Hex([]byte("case-policy:" + threadID)),
		RiskClass:                RiskClassCase,
		Disposition:              PublicationDispositionCaseBoundaryOnly,
		CaseBindingState:         CaseBindingStateMissing,
		BindingObservationDigest: SHA256Hex([]byte("missing-binding:" + workspace)),
		BlockerCode:              PublicationBlockerCaseBindingMissing,
	})
	context, err := NewTurnSecurityContextV2(TurnSecurityContextInput{
		ThreadID:           threadID,
		TurnID:             turnID,
		WorkspaceRealPath:  workspace,
		TenantID:           LocalTenantID,
		UserID:             LocalUserID,
		CaseID:             UnboundCaseID,
		CaseBindingHash:    UnboundCaseBindingHash(workspace),
		DatasetSnapshotID:  NoDatasetSnapshotID,
		SourceManifestHash: EmptySourceManifestHash,
		ContextEpoch:       8,
		IssuedAt:           time.Date(2026, 7, 12, 8, 1, 0, 0, time.UTC),
		PublicationPolicy:  policy,
		RiskAuthorityBinding: mustTestWitnessedRiskAuthorityBindingV1(
			t,
			threadID,
			workspace,
			RiskClassCase,
			policy.ThreadRiskPolicyDigest,
		),
	})
	if err != nil {
		t.Fatal(err)
	}
	return context
}
