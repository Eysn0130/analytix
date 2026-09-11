//go:build darwin && analytix_native_build_probe && !analytix_prod

package processauthority

import (
	"bytes"
	"context"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"unsafe"

	"golang.org/x/sys/unix"
)

const (
	darwinBuildProbePIDVnodePathInfo      = 9
	darwinBuildProbeVinfoStatBytes        = 136
	darwinBuildProbeVnodeInfoBytes        = 152
	darwinBuildProbeVnodeInfoPathBytes    = 1176
	darwinBuildProbeProcVnodePathInfoSize = 2352
	darwinBuildProbePathBytes             = 1024
)

type darwinBuildProbeVinfoStat struct {
	Dev         uint32
	Mode        uint16
	Nlink       uint16
	Ino         uint64
	UID         uint32
	GID         uint32
	ATime       int64
	ATimeNS     int64
	MTime       int64
	MTimeNS     int64
	CTime       int64
	CTimeNS     int64
	BirthTime   int64
	BirthTimeNS int64
	Size        int64
	Blocks      int64
	BlockSize   int32
	Flags       uint32
	Generation  uint32
	Rdev        uint32
	Reserved    [2]int64
}

type darwinBuildProbeVnodeInfo struct {
	Stat darwinBuildProbeVinfoStat
	Type int32
	Pad  int32
	FSID [2]int32
}

type darwinBuildProbeVnodeInfoPath struct {
	Info darwinBuildProbeVnodeInfo
	Path [darwinBuildProbePathBytes]byte
}

type darwinBuildProbeProcVnodePathInfo struct {
	Current darwinBuildProbeVnodeInfoPath
	Root    darwinBuildProbeVnodeInfoPath
}

var (
	_ [darwinBuildProbeVinfoStatBytes - int(unsafe.Sizeof(darwinBuildProbeVinfoStat{}))]byte
	_ [int(unsafe.Sizeof(darwinBuildProbeVinfoStat{})) - darwinBuildProbeVinfoStatBytes]byte
	_ [darwinBuildProbeVnodeInfoBytes - int(unsafe.Sizeof(darwinBuildProbeVnodeInfo{}))]byte
	_ [int(unsafe.Sizeof(darwinBuildProbeVnodeInfo{})) - darwinBuildProbeVnodeInfoBytes]byte
	_ [darwinBuildProbeVnodeInfoPathBytes - int(unsafe.Sizeof(darwinBuildProbeVnodeInfoPath{}))]byte
	_ [int(unsafe.Sizeof(darwinBuildProbeVnodeInfoPath{})) - darwinBuildProbeVnodeInfoPathBytes]byte
	_ [darwinBuildProbeProcVnodePathInfoSize - int(unsafe.Sizeof(darwinBuildProbeProcVnodePathInfo{}))]byte
	_ [int(unsafe.Sizeof(darwinBuildProbeProcVnodePathInfo{})) - darwinBuildProbeProcVnodePathInfoSize]byte
	_ [darwinBuildProbeVnodeInfoBytes - int(unsafe.Offsetof(darwinBuildProbeVnodeInfoPath{}.Path))]byte
	_ [int(unsafe.Offsetof(darwinBuildProbeVnodeInfoPath{}.Path)) - darwinBuildProbeVnodeInfoBytes]byte
)

func darwinBuildProbeFrozenCWDMatches(ctx context.Context, pid int, expectedPath string, authority *os.File) bool {
	if ctx == nil || pid <= 0 || pid > math.MaxInt32 || authority == nil {
		return false
	}
	before, err := readDarwinProcessState(ctx, pid, true)
	if err != nil || before.Status != darwinProcStatusStopped {
		return false
	}
	var info darwinBuildProbeProcVnodePathInfo
	for {
		if darwinContextFailure(ctx) != nil {
			return false
		}
		result, errno := realDarwinProcInfoCaller(
			darwinProcInfoCallPIDInfo,
			pid,
			darwinBuildProbePIDVnodePathInfo,
			0,
			unsafe.Pointer(&info),
			darwinBuildProbeProcVnodePathInfoSize,
		)
		runtime.KeepAlive(&info)
		if result == -1 && errno == syscall.EINTR {
			continue
		}
		if result != darwinBuildProbeProcVnodePathInfoSize || errno != 0 {
			return false
		}
		break
	}
	pathEnd := bytes.IndexByte(info.Current.Path[:], 0)
	if pathEnd <= 0 {
		return false
	}
	actualPath := string(info.Current.Path[:pathEnd])
	expectedPath = filepath.Clean(expectedPath)
	if !filepath.IsAbs(actualPath) || filepath.Clean(actualPath) != actualPath || actualPath != expectedPath {
		return false
	}
	var current unix.Stat_t
	provenance, provenanceOK := darwinBuildProbeProvenance(int(authority.Fd()))
	if unix.Fstat(int(authority.Fd()), &current) != nil || !provenanceOK ||
		!darwinBuildProbeCreatedObjectSafe(authority, unix.S_IFDIR, 0o700, uint32(os.Geteuid()), provenance) ||
		info.Current.Info.Stat.Dev != uint32(current.Dev) || info.Current.Info.Stat.Ino != current.Ino ||
		info.Current.Info.Stat.Mode != current.Mode || info.Current.Info.Stat.UID != current.Uid ||
		info.Current.Info.Stat.Flags != current.Flags ||
		!darwinDirectoryPathMatchesAuthority(expectedPath, authority) {
		return false
	}
	after, err := readDarwinProcessState(ctx, pid, true)
	return err == nil && after.Status == darwinProcStatusStopped && after.Identity == before.Identity
}
