package sidecarlaunch

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"net"
	"net/url"
	"strconv"
	"strings"
)

const (
	PurposeV2        = "analytix.runtime-sidecar-launch-binding/v2"
	secretBytesV2    = 32
	maxSafeIntegerV2 = uint64(1<<53 - 1)
)

type BindingV2 struct {
	RuntimeURL                 string
	RuntimePID                 uint64
	ControlledArtifactHostURL  string
	BackendGeneration          uint64
	AllocationRecordDigest     string
	TLSRootCertificateSHA256   string
	TLSLeafSPKISHA256          string
	ControlledArtifactReady    bool
	RuntimeTokenConfigured     bool
	PersistenceRootsConfigured bool
	ProductionRuntime          bool
}

func ProofV2(secretText string, binding BindingV2) (string, error) {
	secret, err := base64.RawURLEncoding.Strict().DecodeString(secretText)
	if err != nil || len(secret) != secretBytesV2 ||
		base64.RawURLEncoding.EncodeToString(secret) != secretText || validateBindingV2(binding) != nil {
		clearBytes(secret)
		return "", errors.New("runtime sidecar launch binding input is invalid")
	}
	defer clearBytes(secret)

	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte(PurposeV2))
	_, _ = mac.Write([]byte{0})
	for _, field := range []string{
		binding.RuntimeURL,
		strconv.FormatUint(binding.RuntimePID, 10),
		binding.ControlledArtifactHostURL,
		strconv.FormatUint(binding.BackendGeneration, 10),
		binding.AllocationRecordDigest,
		binding.TLSRootCertificateSHA256,
		binding.TLSLeafSPKISHA256,
		strconv.FormatBool(binding.ControlledArtifactReady),
		strconv.FormatBool(binding.RuntimeTokenConfigured),
		strconv.FormatBool(binding.PersistenceRootsConfigured),
		strconv.FormatBool(binding.ProductionRuntime),
	} {
		var length [4]byte
		binary.BigEndian.PutUint32(length[:], uint32(len(field)))
		_, _ = mac.Write(length[:])
		_, _ = mac.Write([]byte(field))
	}
	return hex.EncodeToString(mac.Sum(nil)), nil
}

func validateBindingV2(binding BindingV2) error {
	if !validLoopbackOrigin(binding.RuntimeURL, "http") ||
		binding.RuntimePID == 0 || binding.RuntimePID > maxSafeIntegerV2 ||
		!validLoopbackOrigin(binding.ControlledArtifactHostURL, "https") ||
		binding.BackendGeneration == 0 || binding.BackendGeneration > maxSafeIntegerV2 ||
		!validSHA256Hex(binding.AllocationRecordDigest) ||
		!validSHA256Hex(binding.TLSRootCertificateSHA256) ||
		!validSHA256Hex(binding.TLSLeafSPKISHA256) ||
		!binding.RuntimeTokenConfigured || !binding.PersistenceRootsConfigured || !binding.ProductionRuntime {
		return errors.New("runtime sidecar launch binding is invalid")
	}
	return nil
}

func validSHA256Hex(value string) bool {
	if len(value) != sha256.Size*2 || value != strings.ToLower(value) {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func validLoopbackOrigin(value string, scheme string) bool {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme != scheme || parsed.User != nil || parsed.Path != "" && parsed.Path != "/" ||
		parsed.RawQuery != "" || parsed.Fragment != "" {
		return false
	}
	host, port, err := net.SplitHostPort(parsed.Host)
	if err != nil || port == "" {
		return false
	}
	portNumber, err := strconv.ParseUint(port, 10, 16)
	ip := net.ParseIP(strings.Trim(host, "[]"))
	return err == nil && portNumber > 0 && ip != nil && ip.IsLoopback()
}

func clearBytes(value []byte) {
	for index := range value {
		value[index] = 0
	}
}
