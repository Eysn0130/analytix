//go:build linux

package finalauthority

import (
	"encoding/binary"

	"golang.org/x/sys/unix"
)

func privateCASUnixStatTimes(stat unix.Stat_t) [32]byte {
	var encoded [32]byte
	binary.BigEndian.PutUint64(encoded[0:8], uint64(stat.Mtim.Sec))
	binary.BigEndian.PutUint64(encoded[8:16], uint64(stat.Mtim.Nsec))
	binary.BigEndian.PutUint64(encoded[16:24], uint64(stat.Ctim.Sec))
	binary.BigEndian.PutUint64(encoded[24:32], uint64(stat.Ctim.Nsec))
	return encoded
}
