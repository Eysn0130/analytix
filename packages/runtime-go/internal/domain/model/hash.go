package model

import (
	"crypto/sha256"
	"encoding/hex"
)

func BytesHash(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func StringHash(value string) string {
	return BytesHash([]byte(value))
}

func SameStringSlice(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for index := range a {
		if a[index] != b[index] {
			return false
		}
	}
	return true
}
