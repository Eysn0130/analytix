package security

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"sort"
	"strings"

	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
)

const (
	ThreadRiskAuthorityIndexSchemaVersion = 1
	ThreadRiskAuthorityIndexPurpose       = "analytix.thread-risk-authority-index/v1"
	ThreadRiskAuthorityNamespaceV1        = "analytix.thread-risk-authority/v1"
	ThreadRiskAuthorityAlgorithm          = "Ed25519"
)

var (
	threadRiskAuthorityIndexSignatureDomain = []byte("analytix.thread-risk-authority-index/v1\x00")
	threadRiskAuthorityStateDigestDomain    = []byte("analytix.thread-risk-authority-state/v1\x00")
)

// ThreadRiskAuthorityEntryV1 is the installation-authoritative head of one
// thread's risk-policy chain. PreviousPolicyDigest makes a changed entry an
// explicit single-policy step; it is not an assertion that the policy bytes
// are present or valid. Call ValidateThreadRiskAuthorityEntryForPolicyV1 at
// the policy-store boundary as well.
type ThreadRiskAuthorityEntryV1 struct {
	ThreadID             string `json:"threadId"`
	WorkspaceRealPath    string `json:"workspaceRealPath"`
	RiskClass            string `json:"riskClass"`
	CurrentPolicyDigest  string `json:"currentPolicyDigest"`
	PreviousPolicyDigest string `json:"previousPolicyDigest"`
}

// ThreadRiskAuthorityIndexV1 is a closed, installation-signed inventory. Its
// signature proves authenticity only. Freshness requires a separately
// enrolled MonotonicHeadCheckpointV1 whose currentStateDigest equals
// IndexDigest; callers must not treat ValidateThreadRiskAuthorityIndexV1 as a
// latest-head proof.
type ThreadRiskAuthorityIndexV1 struct {
	SchemaVersion       int                          `json:"schemaVersion"`
	Purpose             string                       `json:"purpose"`
	InstallationID      string                       `json:"installationId"`
	EnrollmentID        string                       `json:"enrollmentId"`
	Namespace           string                       `json:"namespace"`
	Generation          uint64                       `json:"generation"`
	Entries             []ThreadRiskAuthorityEntryV1 `json:"entries"`
	PreviousIndexDigest string                       `json:"previousIndexDigest"`
	StateDigest         string                       `json:"stateDigest"`
	MutationID          string                       `json:"mutationId"`
	AuthorityAlgorithm  string                       `json:"authorityAlgorithm"`
	AuthorityKeyID      string                       `json:"authorityKeyId"`
	AuthorityPublicKey  string                       `json:"authorityPublicKey"`
	AuthoritySignature  string                       `json:"authoritySignature"`
	IndexDigest         string                       `json:"indexDigest"`
}

type ThreadRiskAuthorityIndexInputV1 struct {
	InstallationID      string
	EnrollmentID        string
	Namespace           string
	Generation          uint64
	Entries             []ThreadRiskAuthorityEntryV1
	PreviousIndexDigest string
	MutationID          string
	AuthorityKeyID      string
	AuthorityPublicKey  []byte
}

type ThreadRiskAuthoritySignFunc func([]byte) ([]byte, error)

func ThreadRiskAuthorityEntryFromPolicyV1(policy ThreadRiskPolicyV1) (ThreadRiskAuthorityEntryV1, error) {
	if err := ValidateThreadRiskPolicyV1(policy); err != nil {
		return ThreadRiskAuthorityEntryV1{}, err
	}
	return ThreadRiskAuthorityEntryV1{
		ThreadID:             policy.ThreadID,
		WorkspaceRealPath:    policy.WorkspaceRealPath,
		RiskClass:            policy.RiskClass,
		CurrentPolicyDigest:  policy.PolicyDigest,
		PreviousPolicyDigest: policy.PredecessorPolicyDigest,
	}, nil
}

func ValidateThreadRiskAuthorityEntryForPolicyV1(entry ThreadRiskAuthorityEntryV1, policy ThreadRiskPolicyV1) error {
	if err := validateThreadRiskAuthorityEntryV1(entry); err != nil {
		return err
	}
	derived, err := ThreadRiskAuthorityEntryFromPolicyV1(policy)
	if err != nil {
		return err
	}
	if entry != derived {
		return errors.New("thread risk authority entry does not match policy")
	}
	return nil
}

func NewThreadRiskAuthorityIndexV1(input ThreadRiskAuthorityIndexInputV1, sign ThreadRiskAuthoritySignFunc) (ThreadRiskAuthorityIndexV1, error) {
	publicKey := append([]byte(nil), input.AuthorityPublicKey...)
	entries := make([]ThreadRiskAuthorityEntryV1, len(input.Entries))
	copy(entries, input.Entries)
	sort.Slice(entries, func(i, j int) bool { return entries[i].ThreadID < entries[j].ThreadID })
	index := ThreadRiskAuthorityIndexV1{
		SchemaVersion:       ThreadRiskAuthorityIndexSchemaVersion,
		Purpose:             ThreadRiskAuthorityIndexPurpose,
		InstallationID:      strings.TrimSpace(input.InstallationID),
		EnrollmentID:        strings.TrimSpace(input.EnrollmentID),
		Namespace:           strings.TrimSpace(input.Namespace),
		Generation:          input.Generation,
		Entries:             entries,
		PreviousIndexDigest: strings.TrimSpace(input.PreviousIndexDigest),
		MutationID:          strings.TrimSpace(input.MutationID),
		AuthorityAlgorithm:  ThreadRiskAuthorityAlgorithm,
		AuthorityKeyID:      strings.TrimSpace(input.AuthorityKeyID),
		AuthorityPublicKey:  base64.RawURLEncoding.EncodeToString(publicKey),
	}
	index.StateDigest = threadRiskAuthorityStateDigestV1(index.Entries)
	if sign == nil || len(publicKey) != ed25519.PublicKeySize || index.AuthorityKeyID != SHA256Hex(publicKey) {
		return ThreadRiskAuthorityIndexV1{}, errors.New("thread risk authority signing authority is invalid")
	}
	if err := validateThreadRiskAuthorityIndexUnsignedV1(index); err != nil {
		return ThreadRiskAuthorityIndexV1{}, err
	}
	signature, err := sign(ThreadRiskAuthorityIndexSigningBytesV1(index))
	if err != nil || len(signature) != ed25519.SignatureSize {
		return ThreadRiskAuthorityIndexV1{}, errors.New("thread risk authority signing failed")
	}
	index.AuthoritySignature = base64.RawURLEncoding.EncodeToString(signature)
	index.IndexDigest = threadRiskAuthorityIndexDigestV1(index)
	if err := ValidateThreadRiskAuthorityIndexV1(index); err != nil {
		return ThreadRiskAuthorityIndexV1{}, err
	}
	return index, nil
}

func ValidateThreadRiskAuthorityIndexV1(index ThreadRiskAuthorityIndexV1) error {
	if err := validateThreadRiskAuthorityIndexUnsignedV1(index); err != nil {
		return err
	}
	if !isCanonicalSHA256Hex(index.IndexDigest) || index.IndexDigest != threadRiskAuthorityIndexDigestV1(index) {
		return errors.New("thread risk authority index digest is invalid")
	}
	publicKey, publicErr := base64.RawURLEncoding.DecodeString(index.AuthorityPublicKey)
	signature, signatureErr := base64.RawURLEncoding.DecodeString(index.AuthoritySignature)
	if publicErr != nil || signatureErr != nil || len(publicKey) != ed25519.PublicKeySize || len(signature) != ed25519.SignatureSize ||
		base64.RawURLEncoding.EncodeToString(publicKey) != index.AuthorityPublicKey ||
		base64.RawURLEncoding.EncodeToString(signature) != index.AuthoritySignature ||
		index.AuthorityKeyID != SHA256Hex(publicKey) ||
		!ed25519.Verify(ed25519.PublicKey(publicKey), ThreadRiskAuthorityIndexSigningBytesV1(index), signature) {
		return errors.New("thread risk authority index signature is invalid")
	}
	return nil
}

func ValidateThreadRiskAuthorityIndexForInstallationV1(index ThreadRiskAuthorityIndexV1, installationID, authorityKeyID string, authorityPublicKey []byte) error {
	if err := ValidateThreadRiskAuthorityIndexV1(index); err != nil {
		return err
	}
	installationID = strings.TrimSpace(installationID)
	authorityKeyID = strings.TrimSpace(authorityKeyID)
	authorityPublicKey = append([]byte(nil), authorityPublicKey...)
	if index.InstallationID != installationID || len(authorityPublicKey) != ed25519.PublicKeySize ||
		authorityKeyID != SHA256Hex(authorityPublicKey) || index.AuthorityKeyID != authorityKeyID ||
		index.AuthorityPublicKey != base64.RawURLEncoding.EncodeToString(authorityPublicKey) {
		return errors.New("thread risk authority installation mismatch")
	}
	return nil
}

// ValidateThreadRiskAuthorityIndexTransitionV1 proves that next is exactly one
// index generation and exactly one thread-policy step after previous. It does
// not prove that next is the freshest signed index; that requires the witness
// checkpoint transition validated in monotonic_head.go.
func ValidateThreadRiskAuthorityIndexTransitionV1(previous, next ThreadRiskAuthorityIndexV1) error {
	if ValidateThreadRiskAuthorityIndexV1(previous) != nil || ValidateThreadRiskAuthorityIndexV1(next) != nil {
		return errors.New("thread risk authority index transition authenticity is invalid")
	}
	if previous.Generation == ^uint64(0) || next.Generation != previous.Generation+1 ||
		next.PreviousIndexDigest != previous.IndexDigest ||
		next.InstallationID != previous.InstallationID || next.EnrollmentID != previous.EnrollmentID ||
		next.Namespace != previous.Namespace || next.AuthorityKeyID != previous.AuthorityKeyID ||
		next.AuthorityPublicKey != previous.AuthorityPublicKey || next.MutationID == previous.MutationID {
		return errors.New("thread risk authority index lineage is invalid")
	}
	if len(next.Entries) != len(previous.Entries) && len(next.Entries) != len(previous.Entries)+1 {
		return errors.New("thread risk authority index cannot delete or batch-add entries")
	}
	previousByThread := make(map[string]ThreadRiskAuthorityEntryV1, len(previous.Entries))
	for _, entry := range previous.Entries {
		previousByThread[entry.ThreadID] = entry
	}
	changed := 0
	for _, entry := range next.Entries {
		prior, exists := previousByThread[entry.ThreadID]
		if !exists {
			if entry.PreviousPolicyDigest != "" {
				return errors.New("new thread risk authority entry has a predecessor")
			}
			changed++
			continue
		}
		delete(previousByThread, entry.ThreadID)
		if entry == prior {
			continue
		}
		if entry.WorkspaceRealPath != prior.WorkspaceRealPath &&
			(prior.RiskClass != RiskClassGeneral || entry.RiskClass != RiskClassGeneral) {
			return errors.New("thread risk authority workspace change is not authorized")
		}
		if prior.RiskClass == RiskClassCase && entry.RiskClass != RiskClassCase {
			return errors.New("thread risk authority risk class cannot downgrade")
		}
		if entry.CurrentPolicyDigest == prior.CurrentPolicyDigest || entry.PreviousPolicyDigest != prior.CurrentPolicyDigest {
			return errors.New("thread risk authority policy head does not advance one step")
		}
		changed++
	}
	if len(previousByThread) != 0 {
		return errors.New("thread risk authority index cannot delete entries")
	}
	if changed != 1 {
		return errors.New("thread risk authority index must advance exactly one thread")
	}
	return nil
}

func ParseThreadRiskAuthorityIndexV1(body []byte) (ThreadRiskAuthorityIndexV1, error) {
	var index ThreadRiskAuthorityIndexV1
	if err := decodeStrictThreadRiskAuthorityContract(body, &index); err != nil {
		return ThreadRiskAuthorityIndexV1{}, err
	}
	return index, ValidateThreadRiskAuthorityIndexV1(index)
}

func ThreadRiskAuthorityIndexV1Bytes(index ThreadRiskAuthorityIndexV1) ([]byte, error) {
	if err := ValidateThreadRiskAuthorityIndexV1(index); err != nil {
		return nil, err
	}
	return json.Marshal(index)
}

func ThreadRiskAuthorityIndexSigningBytesV1(index ThreadRiskAuthorityIndexV1) []byte {
	index.AuthoritySignature = ""
	index.IndexDigest = ""
	body, _ := json.Marshal(index)
	digest := sha256.Sum256(body)
	out := append([]byte(nil), threadRiskAuthorityIndexSignatureDomain...)
	return append(out, digest[:]...)
}

func validateThreadRiskAuthorityIndexUnsignedV1(index ThreadRiskAuthorityIndexV1) error {
	if index.SchemaVersion != ThreadRiskAuthorityIndexSchemaVersion || index.Purpose != ThreadRiskAuthorityIndexPurpose ||
		!isCanonicalSHA256Hex(index.InstallationID) || !isCanonicalSHA256Hex(index.EnrollmentID) ||
		index.Namespace != ThreadRiskAuthorityNamespaceV1 || index.Generation == 0 || index.Entries == nil || !isCanonicalSHA256Hex(index.MutationID) ||
		index.AuthorityAlgorithm != ThreadRiskAuthorityAlgorithm || !isCanonicalSHA256Hex(index.AuthorityKeyID) ||
		!isCanonicalSHA256Hex(index.StateDigest) || index.StateDigest != threadRiskAuthorityStateDigestV1(index.Entries) ||
		(index.Generation == 1 && index.PreviousIndexDigest != "") ||
		(index.Generation > 1 && !isCanonicalSHA256Hex(index.PreviousIndexDigest)) {
		return errors.New("thread risk authority index is incomplete")
	}
	seenPolicies := make(map[string]struct{}, len(index.Entries))
	for position, entry := range index.Entries {
		if err := validateThreadRiskAuthorityEntryV1(entry); err != nil {
			return err
		}
		if position > 0 && index.Entries[position-1].ThreadID >= entry.ThreadID {
			return errors.New("thread risk authority entries are not uniquely sorted")
		}
		if _, duplicate := seenPolicies[entry.CurrentPolicyDigest]; duplicate {
			return errors.New("thread risk authority policy head is duplicated")
		}
		seenPolicies[entry.CurrentPolicyDigest] = struct{}{}
	}
	return nil
}

func validateThreadRiskAuthorityEntryV1(entry ThreadRiskAuthorityEntryV1) error {
	if strings.TrimSpace(entry.ThreadID) == "" || entry.ThreadID != strings.TrimSpace(entry.ThreadID) ||
		strings.TrimSpace(entry.WorkspaceRealPath) == "" || entry.WorkspaceRealPath != strings.TrimSpace(entry.WorkspaceRealPath) ||
		(entry.RiskClass != RiskClassGeneral && entry.RiskClass != RiskClassCase) ||
		!isCanonicalSHA256Hex(entry.CurrentPolicyDigest) ||
		(entry.PreviousPolicyDigest != "" && !isCanonicalSHA256Hex(entry.PreviousPolicyDigest)) ||
		entry.PreviousPolicyDigest == entry.CurrentPolicyDigest {
		return errors.New("thread risk authority entry is invalid")
	}
	return nil
}

func threadRiskAuthorityStateDigestV1(entries []ThreadRiskAuthorityEntryV1) string {
	body, _ := json.Marshal(entries)
	payload := append([]byte(nil), threadRiskAuthorityStateDigestDomain...)
	return SHA256Hex(append(payload, body...))
}

func threadRiskAuthorityIndexDigestV1(index ThreadRiskAuthorityIndexV1) string {
	index.IndexDigest = ""
	body, _ := json.Marshal(index)
	return SHA256Hex(body)
}

func decodeStrictThreadRiskAuthorityContract(body []byte, target any) error {
	if err := domainjsonstrict.Validate(body, domainjsonstrict.Options{
		RequireObject: true, MaxBytes: 4 << 20, MaxDepth: 16, MaxTokens: 200_000, MaxStringBytes: 32 * 1024,
	}); err != nil {
		return err
	}
	targetType := reflect.TypeOf(target)
	if targetType == nil || targetType.Kind() != reflect.Pointer || targetType.Elem().Kind() != reflect.Struct {
		return errors.New("thread risk authority contract target is invalid")
	}
	if err := validateExactThreadRiskAuthorityJSONShape(body, targetType.Elem()); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return errors.New("thread risk authority contract contains trailing JSON")
	}
	return nil
}

func validateExactThreadRiskAuthorityJSONShape(body []byte, targetType reflect.Type) error {
	for targetType.Kind() == reflect.Pointer {
		targetType = targetType.Elem()
	}
	switch targetType.Kind() {
	case reflect.Struct:
		var object map[string]json.RawMessage
		if err := json.Unmarshal(body, &object); err != nil || object == nil {
			return errors.New("thread risk authority object shape is invalid")
		}
		expected := make(map[string]reflect.Type, targetType.NumField())
		for index := 0; index < targetType.NumField(); index++ {
			field := targetType.Field(index)
			if !field.IsExported() {
				continue
			}
			name := strings.Split(field.Tag.Get("json"), ",")[0]
			if name == "" {
				name = field.Name
			}
			if name == "-" {
				continue
			}
			expected[name] = field.Type
		}
		if len(object) != len(expected) {
			return errors.New("thread risk authority object fields are incomplete")
		}
		for name, fieldType := range expected {
			raw, exists := object[name]
			if !exists {
				return errors.New("thread risk authority object field name is not canonical")
			}
			kind := fieldType.Kind()
			if kind == reflect.Struct || kind == reflect.Pointer || kind == reflect.Slice && fieldType.Elem().Kind() == reflect.Struct {
				if err := validateExactThreadRiskAuthorityJSONShape(raw, fieldType); err != nil {
					return err
				}
			}
		}
	case reflect.Slice:
		if bytes.Equal(bytes.TrimSpace(body), []byte("null")) {
			return errors.New("thread risk authority array cannot be null")
		}
		var elements []json.RawMessage
		if err := json.Unmarshal(body, &elements); err != nil {
			return errors.New("thread risk authority array shape is invalid")
		}
		for _, element := range elements {
			if err := validateExactThreadRiskAuthorityJSONShape(element, targetType.Elem()); err != nil {
				return err
			}
		}
	}
	return nil
}
