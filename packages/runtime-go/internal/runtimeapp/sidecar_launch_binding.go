package runtimeapp

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strconv"

	domainsidecarlaunch "analytix.local/runtime-go/internal/domain/sidecarlaunch"
)

type ControlledArtifactSidecarLaunchBindingV2 struct {
	Configured        bool
	Ready             bool
	BackendGeneration uint64
	Proof             string
}

func ControlledArtifactSidecarLaunchBindingProofV2(
	config Config,
	runtimeURL string,
	runtimePID uint64,
	runtimeTokenConfigured bool,
	controlledArtifactReady bool,
) (ControlledArtifactSidecarLaunchBindingV2, error) {
	if config.ControlledArtifactHostV2URL == "" {
		if controlledArtifactReady {
			return ControlledArtifactSidecarLaunchBindingV2{}, errors.New("desktop controlled artifact host V2 launch binding is invalid")
		}
		return ControlledArtifactSidecarLaunchBindingV2{}, nil
	}
	if err := validateControlledArtifactHostConfigV2(config); err != nil {
		return ControlledArtifactSidecarLaunchBindingV2{}, errors.New("desktop controlled artifact host V2 launch binding is invalid")
	}
	generation, err := strconv.ParseUint(config.ControlledArtifactHostV2BackendGeneration, 10, 64)
	if err != nil {
		return ControlledArtifactSidecarLaunchBindingV2{}, errors.New("desktop controlled artifact host V2 launch binding is invalid")
	}
	rootDER, err := base64.RawURLEncoding.Strict().DecodeString(config.ControlledArtifactHostV2TLSRootCertDER)
	if err != nil {
		return ControlledArtifactSidecarLaunchBindingV2{}, errors.New("desktop controlled artifact host V2 launch binding is invalid")
	}
	defer clearRuntimePrivateBytesV2(rootDER)
	rootDigest := sha256.Sum256(rootDER)
	proof, err := domainsidecarlaunch.ProofV2(config.ControlledArtifactHostV2Token, domainsidecarlaunch.BindingV2{
		RuntimeURL: runtimeURL, RuntimePID: runtimePID,
		ControlledArtifactHostURL: config.ControlledArtifactHostV2URL,
		BackendGeneration:         generation, AllocationRecordDigest: config.ControlledArtifactHostV2AllocationDigest,
		TLSRootCertificateSHA256:   hex.EncodeToString(rootDigest[:]),
		TLSLeafSPKISHA256:          config.ControlledArtifactHostV2TLSLeafSPKISHA256,
		ControlledArtifactReady:    controlledArtifactReady,
		RuntimeTokenConfigured:     runtimeTokenConfigured,
		PersistenceRootsConfigured: true, ProductionRuntime: true,
	})
	if err != nil {
		return ControlledArtifactSidecarLaunchBindingV2{}, errors.New("desktop controlled artifact host V2 launch binding is invalid")
	}
	return ControlledArtifactSidecarLaunchBindingV2{
		Configured: true, Ready: controlledArtifactReady,
		BackendGeneration: generation, Proof: proof,
	}, nil
}
