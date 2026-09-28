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
		native     *nativecomponenthost.Owner
		configured bool
		shared     runtimeSharedEvidenceDatasetSnapshotV2
		want       fundsCSVAdmissionTraceCodeV1
	}{
		{"native", nil, true, runtimeSharedEvidenceDatasetSnapshotV2{evidence: evidence, snapshot: snapshot}, fundsCSVNativeOwnerUnavailableV1},
		{"enrollment", native, false, runtimeSharedEvidenceDatasetSnapshotV2{}, fundsCSVEnrollmentAbsentV1},
		{"credentials", native, true, runtimeSharedEvidenceDatasetSnapshotV2{credentialsUnavailable: true}, fundsCSVCredentialUnavailableV1},
		{"snapshot", native, true, runtimeSharedEvidenceDatasetSnapshotV2{evidence: evidence}, fundsCSVDatasetSnapshotUnavailableV1},
		{"other", native, true, runtimeSharedEvidenceDatasetSnapshotV2{snapshot: snapshot}, fundsCSVOtherUnavailableV1},
		{"ready", native, true, runtimeSharedEvidenceDatasetSnapshotV2{evidence: evidence, snapshot: snapshot}, fundsCSVReadyV1},
	} {
		t.Run(test.name, func(t *testing.T) {
			var output bytes.Buffer
			ctx := context.WithValue(context.Background(), fundsCSVAdmissionTraceWriterContextKeyV1{}, &output)
			traceFundsCSVAdmissionAssemblyV1(ctx, test.native, test.configured, test.shared)
			if got, want := output.String(), fundsCSVAdmissionTracePrefixV1+string(test.want)+"\n"; got != want {
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
			traceFundsCSVAdmissionAssemblyV1(ctx, nil, false, runtimeSharedEvidenceDatasetSnapshotV2{})
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
		{"1", fundsCSVAdmissionTracePrefixV1 + string(fundsCSVNativeOwnerUnavailableV1) + "\n"},
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
