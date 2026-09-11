//go:build windows

package proc

import (
	"errors"
	"os/exec"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var errWindowsProcessContainmentUnavailable = errors.New("windows process containment unavailable")

// SetProcessGroupKill is intentionally a no-op on Windows. StartTracked owns
// the stronger boundary: it starts the child suspended and assigns it to a
// no-breakaway, kill-on-close Job Object before any child instruction runs.
func SetProcessGroupKill(*exec.Cmd) {}

// StartTracked starts cmd suspended and fails closed unless the child is
// assigned to a kill-on-close Job Object and then resumed. Returning a zero
// handle after a successful start would make descendant cleanup advisory, so
// this implementation terminates and waits for the child on every admission
// failure instead.
func StartTracked(cmd *exec.Cmd) (uintptr, error) {
	if cmd == nil {
		return 0, errWindowsProcessContainmentUnavailable
	}
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.CreationFlags |= windows.CREATE_SUSPENDED
	if err := cmd.Start(); err != nil {
		return 0, err
	}

	job, err := createKillOnCloseJob(cmd)
	if err != nil {
		terminateUnadmittedWindowsProcess(cmd, 0)
		return 0, errWindowsProcessContainmentUnavailable
	}
	if err := resumeSuspendedProcess(uint32(cmd.Process.Pid)); err != nil {
		terminateUnadmittedWindowsProcess(cmd, job)
		return 0, errWindowsProcessContainmentUnavailable
	}
	return uintptr(job), nil
}

func createKillOnCloseJob(cmd *exec.Cmd) (windows.Handle, error) {
	if cmd == nil || cmd.Process == nil || cmd.Process.Pid <= 0 {
		return 0, errWindowsProcessContainmentUnavailable
	}
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return 0, err
	}
	keepJob := false
	defer func() {
		if !keepJob {
			_ = windows.CloseHandle(job)
		}
	}()

	limits := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{
		BasicLimitInformation: windows.JOBOBJECT_BASIC_LIMIT_INFORMATION{
			LimitFlags: windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE |
				windows.JOB_OBJECT_LIMIT_DIE_ON_UNHANDLED_EXCEPTION,
		},
	}
	if _, err := windows.SetInformationJobObject(
		job,
		windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&limits)),
		uint32(unsafe.Sizeof(limits)),
	); err != nil {
		return 0, err
	}

	process, err := windows.OpenProcess(
		windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE,
		false,
		uint32(cmd.Process.Pid),
	)
	if err != nil {
		return 0, err
	}
	defer func() { _ = windows.CloseHandle(process) }()
	if err := windows.AssignProcessToJobObject(job, process); err != nil {
		return 0, err
	}
	keepJob = true
	return job, nil
}

// resumeSuspendedProcess resumes every thread that belongs to the newly
// created process and requires at least one successful resume. A
// CREATE_SUSPENDED child has one primary thread at this point; absence or an
// inaccessible thread is an admission failure, not permission to run without
// containment.
func resumeSuspendedProcess(pid uint32) error {
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPTHREAD, 0)
	if err != nil {
		return err
	}
	defer func() { _ = windows.CloseHandle(snapshot) }()

	var entry windows.ThreadEntry32
	entry.Size = uint32(unsafe.Sizeof(entry))
	resumed := false
	for err := windows.Thread32First(snapshot, &entry); err == nil; err = windows.Thread32Next(snapshot, &entry) {
		if entry.OwnerProcessID != pid {
			continue
		}
		thread, err := windows.OpenThread(windows.THREAD_SUSPEND_RESUME, false, entry.ThreadID)
		if err != nil {
			return err
		}
		_, resumeErr := windows.ResumeThread(thread)
		_ = windows.CloseHandle(thread)
		if resumeErr != nil {
			return resumeErr
		}
		resumed = true
	}
	if !resumed {
		return errWindowsProcessContainmentUnavailable
	}
	return nil
}

func terminateUnadmittedWindowsProcess(cmd *exec.Cmd, job windows.Handle) {
	if job != 0 {
		_ = windows.TerminateJobObject(job, 1)
		_ = windows.CloseHandle(job)
	}
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}
}

// KillTracked requests termination but deliberately keeps the Job handle open.
// Shell cancellation may call this more than once; closing here would leave a
// stale uintptr that could later alias an unrelated reused Windows handle.
func KillTracked(cmd *exec.Cmd, job uintptr) {
	if job != 0 {
		_ = windows.TerminateJobObject(windows.Handle(job), 1)
	}
	KillTree(cmd)
}

// ReapTracked is the single owner that closes the Job handle after terminating
// any descendant that outlived the direct child.
func ReapTracked(cmd *exec.Cmd, job uintptr) {
	if job != 0 {
		handle := windows.Handle(job)
		_ = windows.TerminateJobObject(handle, 1)
		_ = windows.CloseHandle(handle)
	}
	KillTree(cmd)
}

// KillTree is only a direct-child fallback. Production starts must pass
// StartTracked and receive a non-zero Job handle, so analytix never executes a
// PATH-resolved taskkill helper as a substitute for kernel-owned containment.
func KillTree(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	_ = cmd.Process.Kill()
}
