//go:build darwin || linux

package officeengineassets

import (
	"golang.org/x/sys/unix"
	"os"
)

func privateUnixStamp(s unix.Stat_t) privateStamp {
	return privateStamp{device: uint64(s.Dev), inode: uint64(s.Ino), links: uint64(s.Nlink), mode: uint32(s.Mode), uid: s.Uid, gid: s.Gid,
		size: s.Size, modifiedSeconds: s.Mtim.Sec, modifiedNanos: s.Mtim.Nsec, changedSeconds: s.Ctim.Sec, changedNanos: s.Ctim.Nsec}
}
func privateLstat(path string) (privateStamp, error) {
	var s unix.Stat_t
	err := unix.Lstat(path, &s)
	return privateUnixStamp(s), err
}
func privateFstat(f *os.File) (privateStamp, error) {
	var s unix.Stat_t
	err := unix.Fstat(int(f.Fd()), &s)
	return privateUnixStamp(s), err
}
func privateOpen(path string) (*os.File, error) {
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(fd), "office-private-local"), nil
}
