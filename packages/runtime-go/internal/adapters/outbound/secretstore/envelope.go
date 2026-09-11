package secretstore

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"encoding/binary"
	"io"

	portsecretstore "analytix.local/runtime-go/internal/ports/secretstore"
)

const envelopeVersion = 1

type encryptedEnvelope struct {
	Version    int    `json:"version"`
	Nonce      string `json:"nonce"`
	Ciphertext string `json:"ciphertext"`
}

func encryptCredential(
	random io.Reader,
	masterKey []byte,
	ref portsecretstore.CredentialRef,
	purpose portsecretstore.Purpose,
	plaintext []byte,
) (encryptedEnvelope, error) {
	if len(masterKey) != masterKeySize || len(plaintext) == 0 {
		return encryptedEnvelope{}, portsecretstore.ErrCryptographicFailure
	}
	block, err := aes.NewCipher(masterKey)
	if err != nil {
		return encryptedEnvelope{}, portsecretstore.ErrCryptographicFailure
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return encryptedEnvelope{}, portsecretstore.ErrCryptographicFailure
	}
	nonce := make([]byte, aead.NonceSize())
	defer clearBytes(nonce)
	if _, err := io.ReadFull(random, nonce); err != nil {
		return encryptedEnvelope{}, portsecretstore.ErrCryptographicFailure
	}
	ciphertext := aead.Seal(nil, nonce, plaintext, envelopeAdditionalData(envelopeVersion, ref, purpose))
	return encryptedEnvelope{
		Version:    envelopeVersion,
		Nonce:      base64.RawStdEncoding.EncodeToString(nonce),
		Ciphertext: base64.RawStdEncoding.EncodeToString(ciphertext),
	}, nil
}

func decryptCredential(
	masterKey []byte,
	ref portsecretstore.CredentialRef,
	purpose portsecretstore.Purpose,
	envelope encryptedEnvelope,
) ([]byte, error) {
	if len(masterKey) != masterKeySize || envelope.Version != envelopeVersion {
		return nil, portsecretstore.ErrCryptographicFailure
	}
	nonce, err := base64.RawStdEncoding.Strict().DecodeString(envelope.Nonce)
	if err != nil || len(nonce) != 12 {
		clearBytes(nonce)
		return nil, portsecretstore.ErrCryptographicFailure
	}
	defer clearBytes(nonce)
	ciphertext, err := base64.RawStdEncoding.Strict().DecodeString(envelope.Ciphertext)
	if err != nil || len(ciphertext) < 16 {
		clearBytes(ciphertext)
		return nil, portsecretstore.ErrCryptographicFailure
	}
	defer clearBytes(ciphertext)
	block, err := aes.NewCipher(masterKey)
	if err != nil {
		return nil, portsecretstore.ErrCryptographicFailure
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, portsecretstore.ErrCryptographicFailure
	}
	plaintext, err := aead.Open(nil, nonce, ciphertext, envelopeAdditionalData(envelopeVersion, ref, purpose))
	if err != nil {
		clearBytes(plaintext)
		return nil, portsecretstore.ErrCryptographicFailure
	}
	return plaintext, nil
}

func envelopeAdditionalData(version int, ref portsecretstore.CredentialRef, purpose portsecretstore.Purpose) []byte {
	domain := []byte("analytix.secret-store.envelope")
	refBytes := []byte(ref)
	purposeBytes := []byte(purpose)
	additionalData := make([]byte, 0, len(domain)+6+len(refBytes)+len(purposeBytes))
	additionalData = append(additionalData, domain...)
	var field [2]byte
	binary.BigEndian.PutUint16(field[:], uint16(version))
	additionalData = append(additionalData, field[:]...)
	binary.BigEndian.PutUint16(field[:], uint16(len(refBytes)))
	additionalData = append(additionalData, field[:]...)
	additionalData = append(additionalData, refBytes...)
	binary.BigEndian.PutUint16(field[:], uint16(len(purposeBytes)))
	additionalData = append(additionalData, field[:]...)
	additionalData = append(additionalData, purposeBytes...)
	return additionalData
}
