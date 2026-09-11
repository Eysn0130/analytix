package security

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"

	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
)

const (
	RiskAuthorityBindingSchemaVersion = 1
	RiskAuthorityBindingPurpose       = "analytix.risk-authority-binding/v1"
)

const (
	RiskAuthorityBindingStateWitnessed       = "witnessed"
	RiskAuthorityBindingStateHostPolicy      = "host_policy"
	RiskAuthorityBindingStateHostGeneralOnly = "host_general_only"
	RiskAuthorityBindingStateQuarantined     = "quarantined"
)

const hostThreadRiskPolicyPurposeV1 = "analytix.host-thread-risk-policy-authority/v1"

// RiskAuthorityBindingV1 records which exact risk-authority inventory and
// fresh witness observation were used when a V2 turn context was frozen.
//
// These compact references are not authority merely because their digests are
// non-empty. Before an execution boundary trusts a witnessed binding, the app
// must still prove live registry membership for the exact index and validate a
// fresh observe request/response against the enrolled witness. A host-general-
// only binding is a separate non-elevating variant tied to the compiled policy
// and exact missing case-binding observation; it is never witness authority. A
// quarantined binding deliberately carries no authority references and can
// only support a host-rendered case boundary.
type RiskAuthorityBindingV1 struct {
	SchemaVersion            int    `json:"-"`
	Purpose                  string `json:"-"`
	State                    string `json:"-"`
	IndexDigest              string `json:"-"`
	Generation               uint64 `json:"-"`
	CheckpointDigest         string `json:"-"`
	ObservationDigest        string `json:"-"`
	ThreadPolicyDigest       string `json:"-"`
	GeneralPolicyDigest      string `json:"-"`
	HostPolicyDigest         string `json:"-"`
	BindingObservationDigest string `json:"-"`
}

type witnessedRiskAuthorityBindingV1Wire struct {
	SchemaVersion     int    `json:"schemaVersion"`
	Purpose           string `json:"purpose"`
	State             string `json:"state"`
	IndexDigest       string `json:"indexDigest"`
	Generation        uint64 `json:"generation"`
	CheckpointDigest  string `json:"checkpointDigest"`
	ObservationDigest string `json:"observationDigest"`
}

type quarantinedRiskAuthorityBindingV1Wire struct {
	SchemaVersion int    `json:"schemaVersion"`
	Purpose       string `json:"purpose"`
	State         string `json:"state"`
}

type hostGeneralOnlyRiskAuthorityBindingV1Wire struct {
	SchemaVersion            int    `json:"schemaVersion"`
	Purpose                  string `json:"purpose"`
	State                    string `json:"state"`
	GeneralPolicyDigest      string `json:"generalPolicyDigest"`
	HostPolicyDigest         string `json:"hostPolicyDigest"`
	BindingObservationDigest string `json:"bindingObservationDigest"`
}

type hostPolicyRiskAuthorityBindingV1Wire struct {
	SchemaVersion            int    `json:"schemaVersion"`
	Purpose                  string `json:"purpose"`
	State                    string `json:"state"`
	ThreadPolicyDigest       string `json:"threadPolicyDigest"`
	HostPolicyDigest         string `json:"hostPolicyDigest"`
	BindingObservationDigest string `json:"bindingObservationDigest"`
}

// HostThreadRiskPolicyDigestV1 identifies the fixed host rules for a
// callback-current, installation-signed ThreadRiskPolicyV1. It deliberately
// makes no monotonic-head or rollback claim: current protected effects must
// re-observe the exact case binding and revalidate the exact signed policy.
func HostThreadRiskPolicyDigestV1() string {
	body, _ := json.Marshal(struct {
		SchemaVersion           int    `json:"schemaVersion"`
		Purpose                 string `json:"purpose"`
		InstallationSignature   bool   `json:"installationSignature"`
		ExactPolicyReadback     bool   `json:"exactPolicyReadback"`
		FreshBindingObservation bool   `json:"freshBindingObservation"`
		IndependentWitnessClaim bool   `json:"independentWitnessClaim"`
	}{
		SchemaVersion: 1, Purpose: hostThreadRiskPolicyPurposeV1,
		InstallationSignature: true, ExactPolicyReadback: true,
		FreshBindingObservation: true,
	})
	return SHA256Hex(body)
}

// NewWitnessedRiskAuthorityBindingV1 derives compact references only after
// validating the exact signed index, the caller's fresh signed challenge, and
// the witness observation that answers that challenge with the matching
// checkpoint. Challenge unpredictability and current-run issuance remain app
// responsibilities and are rechecked at live execution boundaries.
func NewWitnessedRiskAuthorityBindingV1(
	index ThreadRiskAuthorityIndexV1,
	freshRequest MonotonicHeadObserveRequestV1,
	observation MonotonicHeadObservationV1,
) (RiskAuthorityBindingV1, error) {
	binding := RiskAuthorityBindingV1{
		SchemaVersion: RiskAuthorityBindingSchemaVersion,
		Purpose:       RiskAuthorityBindingPurpose,
		State:         RiskAuthorityBindingStateWitnessed,
	}
	if err := validateRiskAuthorityWitnessContractsV1(index, freshRequest, observation); err != nil {
		return RiskAuthorityBindingV1{}, err
	}
	binding.IndexDigest = index.IndexDigest
	binding.Generation = index.Generation
	binding.CheckpointDigest = observation.Checkpoint.CheckpointDigest
	binding.ObservationDigest = observation.ObservationDigest
	if err := ValidateWitnessedRiskAuthorityBindingV1(binding, index, freshRequest, observation); err != nil {
		return RiskAuthorityBindingV1{}, err
	}
	return binding, nil
}

func NewQuarantinedRiskAuthorityBindingV1() RiskAuthorityBindingV1 {
	return RiskAuthorityBindingV1{
		SchemaVersion: RiskAuthorityBindingSchemaVersion,
		Purpose:       RiskAuthorityBindingPurpose,
		State:         RiskAuthorityBindingStateQuarantined,
	}
}

func NewHostGeneralOnlyRiskAuthorityBindingV1(policy GeneralOnlyRiskPolicyV1) (RiskAuthorityBindingV1, error) {
	if err := ValidateGeneralOnlyRiskPolicyV1(policy); err != nil {
		return RiskAuthorityBindingV1{}, err
	}
	binding := RiskAuthorityBindingV1{
		SchemaVersion: RiskAuthorityBindingSchemaVersion, Purpose: RiskAuthorityBindingPurpose,
		State: RiskAuthorityBindingStateHostGeneralOnly, GeneralPolicyDigest: policy.PolicyDigest,
		HostPolicyDigest: policy.HostPolicyDigest, BindingObservationDigest: policy.BindingObservationDigest,
	}
	return binding, ValidateHostGeneralOnlyRiskAuthorityBindingV1(binding, policy)
}

// NewHostPolicyRiskAuthorityBindingV1 binds the existing signed thread policy
// to the exact host-observed workspace/case state. This is the accepted
// first-stage variant used when an independent monotonic witness is not
// composed; it never impersonates a witnessed index or checkpoint.
func NewHostPolicyRiskAuthorityBindingV1(
	policy ThreadRiskPolicyV1,
	observation CaseBindingObservationV1,
) (RiskAuthorityBindingV1, error) {
	if ValidateThreadRiskPolicyV1(policy) != nil || policy.RiskClass != RiskClassCase ||
		ValidateCaseBindingObservationV1(observation) != nil ||
		policy.WorkspaceRealPath != observation.WorkspaceRealPath {
		return RiskAuthorityBindingV1{}, errors.New("host thread risk policy authority input is invalid")
	}
	binding := RiskAuthorityBindingV1{
		SchemaVersion:            RiskAuthorityBindingSchemaVersion,
		Purpose:                  RiskAuthorityBindingPurpose,
		State:                    RiskAuthorityBindingStateHostPolicy,
		ThreadPolicyDigest:       policy.PolicyDigest,
		HostPolicyDigest:         HostThreadRiskPolicyDigestV1(),
		BindingObservationDigest: observation.ObservationDigest,
	}
	return binding, ValidateHostPolicyRiskAuthorityBindingV1(binding, policy, observation)
}

func ValidateRiskAuthorityBindingV1(binding RiskAuthorityBindingV1) error {
	if binding.SchemaVersion != RiskAuthorityBindingSchemaVersion || binding.Purpose != RiskAuthorityBindingPurpose {
		return errors.New("risk authority binding contract is invalid")
	}
	switch binding.State {
	case RiskAuthorityBindingStateWitnessed:
		if !isCanonicalSHA256Hex(binding.IndexDigest) || binding.Generation == 0 ||
			!isCanonicalSHA256Hex(binding.CheckpointDigest) || !isCanonicalSHA256Hex(binding.ObservationDigest) ||
			binding.ThreadPolicyDigest != "" || binding.GeneralPolicyDigest != "" || binding.HostPolicyDigest != "" || binding.BindingObservationDigest != "" {
			return errors.New("witnessed risk authority binding is incomplete")
		}
	case RiskAuthorityBindingStateHostPolicy:
		if binding.IndexDigest != "" || binding.Generation != 0 || binding.CheckpointDigest != "" || binding.ObservationDigest != "" ||
			!isCanonicalSHA256Hex(binding.ThreadPolicyDigest) || binding.GeneralPolicyDigest != "" ||
			binding.HostPolicyDigest != HostThreadRiskPolicyDigestV1() ||
			!isCanonicalSHA256Hex(binding.BindingObservationDigest) {
			return errors.New("host thread risk policy authority binding is invalid")
		}
	case RiskAuthorityBindingStateHostGeneralOnly:
		if binding.IndexDigest != "" || binding.Generation != 0 || binding.CheckpointDigest != "" || binding.ObservationDigest != "" ||
			binding.ThreadPolicyDigest != "" || !isCanonicalSHA256Hex(binding.GeneralPolicyDigest) || binding.HostPolicyDigest != GeneralOnlyRiskHostPolicyDigestV1() ||
			!isCanonicalSHA256Hex(binding.BindingObservationDigest) {
			return errors.New("host general-only risk authority binding is invalid")
		}
	case RiskAuthorityBindingStateQuarantined:
		if binding.IndexDigest != "" || binding.Generation != 0 || binding.CheckpointDigest != "" || binding.ObservationDigest != "" ||
			binding.ThreadPolicyDigest != "" || binding.GeneralPolicyDigest != "" || binding.HostPolicyDigest != "" || binding.BindingObservationDigest != "" {
			return errors.New("quarantined risk authority binding carries authority references")
		}
	default:
		return errors.New("risk authority binding state is invalid")
	}
	return nil
}

func ValidateHostPolicyRiskAuthorityBindingV1(
	binding RiskAuthorityBindingV1,
	policy ThreadRiskPolicyV1,
	observation CaseBindingObservationV1,
) error {
	if ValidateRiskAuthorityBindingV1(binding) != nil ||
		binding.State != RiskAuthorityBindingStateHostPolicy ||
		ValidateThreadRiskPolicyV1(policy) != nil || policy.RiskClass != RiskClassCase ||
		ValidateCaseBindingObservationV1(observation) != nil ||
		binding.ThreadPolicyDigest != policy.PolicyDigest ||
		binding.HostPolicyDigest != HostThreadRiskPolicyDigestV1() ||
		binding.BindingObservationDigest != observation.ObservationDigest ||
		policy.WorkspaceRealPath != observation.WorkspaceRealPath {
		return errors.New("risk authority binding does not match exact host thread policy")
	}
	return nil
}

func ValidateHostGeneralOnlyRiskAuthorityBindingV1(binding RiskAuthorityBindingV1, policy GeneralOnlyRiskPolicyV1) error {
	if ValidateRiskAuthorityBindingV1(binding) != nil || binding.State != RiskAuthorityBindingStateHostGeneralOnly ||
		ValidateGeneralOnlyRiskPolicyV1(policy) != nil || binding.GeneralPolicyDigest != policy.PolicyDigest ||
		binding.HostPolicyDigest != policy.HostPolicyDigest || binding.BindingObservationDigest != policy.BindingObservationDigest {
		return errors.New("risk authority binding does not match exact host general-only policy")
	}
	return nil
}

// ValidateWitnessedRiskAuthorityBindingV1 matches a compact binding back to
// the exact authority contracts. It does not replace the app's installation
// trust anchor, current registry membership, or current-run witness observe.
func ValidateWitnessedRiskAuthorityBindingV1(
	binding RiskAuthorityBindingV1,
	index ThreadRiskAuthorityIndexV1,
	freshRequest MonotonicHeadObserveRequestV1,
	observation MonotonicHeadObservationV1,
) error {
	if err := ValidateRiskAuthorityBindingV1(binding); err != nil || binding.State != RiskAuthorityBindingStateWitnessed {
		return errors.New("risk authority binding is not witnessed")
	}
	if err := validateRiskAuthorityWitnessContractsV1(index, freshRequest, observation); err != nil {
		return err
	}
	if binding.IndexDigest != index.IndexDigest || binding.Generation != index.Generation ||
		binding.CheckpointDigest != observation.Checkpoint.CheckpointDigest ||
		binding.ObservationDigest != observation.ObservationDigest {
		return errors.New("risk authority binding does not match exact witnessed authority")
	}
	return nil
}

func validateRiskAuthorityWitnessContractsV1(
	index ThreadRiskAuthorityIndexV1,
	freshRequest MonotonicHeadObserveRequestV1,
	observation MonotonicHeadObservationV1,
) error {
	if ValidateThreadRiskAuthorityIndexV1(index) != nil || ValidateMonotonicHeadObserveRequestV1(freshRequest) != nil ||
		ValidateMonotonicHeadObservationV1(observation) != nil {
		return errors.New("risk authority witness contract is invalid")
	}
	if index.InstallationID != freshRequest.InstallationID || index.EnrollmentID != freshRequest.EnrollmentID ||
		index.Namespace != freshRequest.Namespace || index.AuthorityAlgorithm != freshRequest.AuthorityAlgorithm ||
		index.AuthorityKeyID != freshRequest.AuthorityKeyID || index.AuthorityPublicKey != freshRequest.AuthorityPublicKey {
		return errors.New("risk authority observe request does not match exact index authority")
	}
	if observation.RequestDigest != freshRequest.RequestDigest || observation.ChallengeNonce != freshRequest.ChallengeNonce {
		return errors.New("risk authority observation does not answer fresh challenge")
	}
	if err := ValidateThreadRiskAuthorityIndexCheckpointV1(index, observation.Checkpoint); err != nil {
		return err
	}
	return nil
}

func (binding RiskAuthorityBindingV1) MarshalJSON() ([]byte, error) {
	if err := ValidateRiskAuthorityBindingV1(binding); err != nil {
		return nil, err
	}
	if binding.State == RiskAuthorityBindingStateQuarantined {
		return json.Marshal(quarantinedRiskAuthorityBindingV1Wire{
			SchemaVersion: binding.SchemaVersion, Purpose: binding.Purpose, State: binding.State,
		})
	}
	if binding.State == RiskAuthorityBindingStateHostGeneralOnly {
		return json.Marshal(hostGeneralOnlyRiskAuthorityBindingV1Wire{
			SchemaVersion: binding.SchemaVersion, Purpose: binding.Purpose, State: binding.State,
			GeneralPolicyDigest: binding.GeneralPolicyDigest, HostPolicyDigest: binding.HostPolicyDigest,
			BindingObservationDigest: binding.BindingObservationDigest,
		})
	}
	if binding.State == RiskAuthorityBindingStateHostPolicy {
		return json.Marshal(hostPolicyRiskAuthorityBindingV1Wire{
			SchemaVersion: binding.SchemaVersion, Purpose: binding.Purpose, State: binding.State,
			ThreadPolicyDigest: binding.ThreadPolicyDigest, HostPolicyDigest: binding.HostPolicyDigest,
			BindingObservationDigest: binding.BindingObservationDigest,
		})
	}
	return json.Marshal(witnessedRiskAuthorityBindingV1Wire{
		SchemaVersion: binding.SchemaVersion, Purpose: binding.Purpose, State: binding.State,
		IndexDigest: binding.IndexDigest, Generation: binding.Generation,
		CheckpointDigest: binding.CheckpointDigest, ObservationDigest: binding.ObservationDigest,
	})
}

func (binding *RiskAuthorityBindingV1) UnmarshalJSON(body []byte) error {
	if binding == nil {
		return errors.New("risk authority binding target is nil")
	}
	if err := domainjsonstrict.Validate(body, domainjsonstrict.Options{
		RequireObject: true, MaxBytes: 16 * 1024, MaxDepth: 8, MaxTokens: 256, MaxStringBytes: 1024,
	}); err != nil {
		return err
	}
	var header struct {
		State string `json:"state"`
	}
	if err := json.Unmarshal(body, &header); err != nil {
		return err
	}
	switch header.State {
	case RiskAuthorityBindingStateWitnessed:
		var wire witnessedRiskAuthorityBindingV1Wire
		if err := decodeRiskAuthorityBindingWire(body, &wire); err != nil {
			return err
		}
		*binding = RiskAuthorityBindingV1{
			SchemaVersion: wire.SchemaVersion, Purpose: wire.Purpose, State: wire.State,
			IndexDigest: wire.IndexDigest, Generation: wire.Generation,
			CheckpointDigest: wire.CheckpointDigest, ObservationDigest: wire.ObservationDigest,
		}
	case RiskAuthorityBindingStateQuarantined:
		var wire quarantinedRiskAuthorityBindingV1Wire
		if err := decodeRiskAuthorityBindingWire(body, &wire); err != nil {
			return err
		}
		*binding = RiskAuthorityBindingV1{SchemaVersion: wire.SchemaVersion, Purpose: wire.Purpose, State: wire.State}
	case RiskAuthorityBindingStateHostGeneralOnly:
		var wire hostGeneralOnlyRiskAuthorityBindingV1Wire
		if err := decodeRiskAuthorityBindingWire(body, &wire); err != nil {
			return err
		}
		*binding = RiskAuthorityBindingV1{
			SchemaVersion: wire.SchemaVersion, Purpose: wire.Purpose, State: wire.State,
			GeneralPolicyDigest: wire.GeneralPolicyDigest, HostPolicyDigest: wire.HostPolicyDigest,
			BindingObservationDigest: wire.BindingObservationDigest,
		}
	case RiskAuthorityBindingStateHostPolicy:
		var wire hostPolicyRiskAuthorityBindingV1Wire
		if err := decodeRiskAuthorityBindingWire(body, &wire); err != nil {
			return err
		}
		*binding = RiskAuthorityBindingV1{
			SchemaVersion: wire.SchemaVersion, Purpose: wire.Purpose, State: wire.State,
			ThreadPolicyDigest: wire.ThreadPolicyDigest, HostPolicyDigest: wire.HostPolicyDigest,
			BindingObservationDigest: wire.BindingObservationDigest,
		}
	default:
		return errors.New("risk authority binding state is invalid")
	}
	return ValidateRiskAuthorityBindingV1(*binding)
}

func decodeRiskAuthorityBindingWire(body []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return errors.New("risk authority binding contains trailing JSON")
	}
	return nil
}
