//go:build darwin || linux

package runtimeapp

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	nativecomponenthost "analytix.local/runtime-go/internal/adapters/outbound/nativecomponenthost"
	datasetsnapshotapp "analytix.local/runtime-go/internal/app/datasetsnapshot"
	evidenceauthorityapp "analytix.local/runtime-go/internal/app/evidenceauthority"
)

func TestFundsCSVAdmissionTraceUsesOnlyFixedAssemblyCategories(t *testing.T) {
	t.Setenv("ANALYTIX_STARTUP_TRACE", "1")
	native := &nativecomponenthost.Owner{}
	evidence := &evidenceauthorityapp.Authority{}
	snapshot := &datasetsnapshotapp.SealedServiceV2{}
	for _, test := range []struct {
		name       string
		simulation bool
		native     *nativecomponenthost.Owner
		configured bool
		shared     runtimeSharedEvidenceDatasetSnapshotV2
		hostLocal  bool
		want       fundsCSVAdmissionTraceCodeV1
	}{
		{"simulation", true, nil, true, runtimeSharedEvidenceDatasetSnapshotV2{evidence: evidence, snapshot: snapshot}, false, fundsCSVNotEvaluatedV1},
		{"native", false, nil, true, runtimeSharedEvidenceDatasetSnapshotV2{evidence: evidence, snapshot: snapshot}, false, fundsCSVNativeOwnerUnavailableV1},
		{"enrollment", false, native, false, runtimeSharedEvidenceDatasetSnapshotV2{}, false, fundsCSVEnrollmentAbsentV1},
		{"credentials", false, native, true, runtimeSharedEvidenceDatasetSnapshotV2{credentialsUnavailable: true}, false, fundsCSVCredentialUnavailableV1},
		{"snapshot", false, native, true, runtimeSharedEvidenceDatasetSnapshotV2{evidence: evidence}, false, fundsCSVDatasetSnapshotUnavailableV1},
		{"other", false, native, true, runtimeSharedEvidenceDatasetSnapshotV2{snapshot: snapshot}, false, fundsCSVOtherUnavailableV1},
		{"ready witnessed", false, native, true, runtimeSharedEvidenceDatasetSnapshotV2{evidence: evidence, snapshot: snapshot}, false, fundsCSVReadyV1},
		{"ready host local", false, native, false, runtimeSharedEvidenceDatasetSnapshotV2{}, true, fundsCSVReadyV1},
	} {
		t.Run(test.name, func(t *testing.T) {
			var output bytes.Buffer
			ctx := context.WithValue(context.Background(), fundsCSVAdmissionTraceWriterContextKeyV1{}, &output)
			traceFundsCSVAdmissionAssemblyV1(ctx, test.simulation, test.native, test.configured, test.shared, test.hostLocal)
			phase := "activation"
			if test.simulation {
				phase = "semantic_preparation"
			}
			if got, want := output.String(), fundsCSVAdmissionTracePrefixV2+phase+" "+string(test.want)+"\n"; got != want {
				t.Fatalf("fixed assembly category = %q, want %q", got, want)
			}
		})
	}
}

func TestFundsCSVAdmissionTraceIsSilentUnlessExplicitlyEnabled(t *testing.T) {
	for _, value := range []string{"", "0", "true", "2"} {
		t.Run("value="+value, func(t *testing.T) {
			t.Setenv("ANALYTIX_STARTUP_TRACE", value)
			var output bytes.Buffer
			ctx := context.WithValue(context.Background(), fundsCSVAdmissionTraceWriterContextKeyV1{}, &output)
			traceFundsCSVAdmissionAssemblyV1(ctx, false, nil, false, runtimeSharedEvidenceDatasetSnapshotV2{}, false)
			if output.Len() != 0 {
				t.Fatalf("trace was emitted while disabled: %q", output.String())
			}
		})
	}
}

func TestFundsCSVAdmissionTraceRunsAtRealHandlerAssemblyWithoutUnsafeError(t *testing.T) {
	const unsafeDetail = "unsafe=/private/secret-case key=do-not-print"
	for _, trace := range []struct {
		env  string
		want string
	}{
		{"1", fundsCSVAdmissionTracePrefixV2 + "activation " + string(fundsCSVNativeOwnerUnavailableV1) + "\n"},
		{"0", ""},
	} {
		t.Run("trace="+trace.env, func(t *testing.T) {
			t.Setenv("ANALYTIX_STARTUP_TRACE", trace.env)
			_, config := runtimeWitnessedRegistryConfigV2(t)
			config.UserDataDir = t.TempDir()
			dependencies := defaultBundledFundsHostValidationDependenciesV1()
			dependencies.openNativeOwnerForTest = func(string) (*nativecomponenthost.Owner, error) {
				return nil, fmt.Errorf("%s: %w", unsafeDetail, nativecomponenthost.ErrUnavailable)
			}
			var output bytes.Buffer
			ctx := context.WithValue(context.Background(), bundledFundsHostValidationContextKeyV1{}, dependencies)
			ctx = context.WithValue(ctx, fundsCSVAdmissionTraceWriterContextKeyV1{}, &output)
			lease, err := AcquireRuntimePersistenceLease(config)
			if err != nil {
				t.Fatal(err)
			}
			handler, err := newRuntimeServerHandlerWithPersistenceLeaseContextE(ctx, config, lease)
			if err != nil {
				_ = lease.Close()
				t.Fatal(err)
			}
			owned := &ownedPersistenceLeaseHandler{Handler: handler, lease: lease}
			defer shutdownOwnedRuntimeHandler(t, owned)
			response := httptest.NewRecorder()
			owned.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/health", nil))
			if response.Code != http.StatusOK {
				t.Fatalf("ordinary health changed: %d", response.Code)
			}
			if got := output.String(); got != trace.want || strings.Contains(got, unsafeDetail) {
				t.Fatalf("real assembly trace was unsafe or incorrect: %q", got)
			}
		})
	}
}
