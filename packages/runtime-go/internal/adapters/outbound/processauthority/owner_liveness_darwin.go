//go:build darwin

package processauthority

import (
	"os"

	"golang.org/x/sys/unix"
)

const darwinOwnerLivenessTargetFD = 4

// openDarwinOwnerLivenessPipe returns one CLOEXEC anonymous pipe. The child
// receives only the read endpoint at the process-authority-owned descriptor;
// the parent session retains the sole writer as its lifetime signal.
func openDarwinOwnerLivenessPipe() (*os.File, *os.File, error) {
	reader, writer, err := os.Pipe()
	if err != nil {
		return nil, nil, ErrUnavailable
	}
	if !validDarwinOwnerLivenessEndpoint(reader, unix.O_RDONLY) ||
		!validDarwinOwnerLivenessEndpoint(writer, unix.O_WRONLY) {
		_ = reader.Close()
		_ = writer.Close()
		return nil, nil, ErrUnavailable
	}
	return reader, writer, nil
}

func validDarwinOwnerLivenessEndpoint(file *os.File, accessMode int) bool {
	if file == nil {
		return false
	}
	fd := int(file.Fd())
	statusFlags, statusErr := unix.FcntlInt(uintptr(fd), unix.F_GETFL, 0)
	descriptorFlags, descriptorErr := unix.FcntlInt(uintptr(fd), unix.F_GETFD, 0)
	var stat unix.Stat_t
	return statusErr == nil && descriptorErr == nil && unix.Fstat(fd, &stat) == nil &&
		statusFlags&unix.O_ACCMODE == accessMode && descriptorFlags&unix.FD_CLOEXEC != 0 &&
		stat.Mode&unix.S_IFMT == unix.S_IFIFO && stat.Nlink == 0 && stat.Uid == uint32(os.Geteuid())
}
