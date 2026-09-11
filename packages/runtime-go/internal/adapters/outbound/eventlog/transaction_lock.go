package eventlog

import (
	"crypto/sha256"
	"path/filepath"
	"strings"
	"sync"

	contracts "analytix.local/runtime-go/internal/contracts"
)

const eventlogTransactionStripeCount = 256

var eventlogTransactionStripes [eventlogTransactionStripeCount]sync.Mutex

func canonicalEventlogTransactionRoot(root string) string {
	canonical := filepath.Clean(strings.TrimSpace(root))
	if absolute, err := filepath.Abs(canonical); err == nil {
		canonical = absolute
	}
	if resolved, err := filepath.EvalSymlinks(canonical); err == nil {
		canonical = resolved
	}
	return canonical
}

func eventlogTransactionMutex(root string, threadID string) *sync.Mutex {
	key := root + "\x00" + contracts.SafeRecordID(strings.TrimSpace(threadID))
	digest := sha256.Sum256([]byte(key))
	return &eventlogTransactionStripes[int(digest[0])]
}

func (s *Store) lockThreadTransaction(threadID string) func() {
	mutex := eventlogTransactionMutex(s.transactionRoot, threadID)
	mutex.Lock()
	return mutex.Unlock
}
