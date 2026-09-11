package control

import domaincontinuation "analytix.local/runtime-go/internal/domain/continuation"

func SecureGateID(kind string, threadID string, turnID string, contextDigest string, grantID string, callID string) string {
	return domaincontinuation.GateID(kind, threadID, turnID, contextDigest, grantID, callID)
}
