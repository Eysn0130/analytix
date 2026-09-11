use anyhow::{anyhow, Result};
use std::mem::{self, MaybeUninit};
use std::sync::mpsc;
use std::thread;
use std::time::Duration;

const OWNER_LIVENESS_FD: libc::c_int = 4;
const CANONICAL_OUTPUT_FD: libc::c_int = 5;
const WATCHDOG_ARM_TIMEOUT: Duration = Duration::from_secs(2);
const WATCHDOG_EOF_EXIT: libc::c_int = 190;
const WATCHDOG_PROTOCOL_EXIT: libc::c_int = 191;

pub(crate) fn arm() -> Result<()> {
    establish_nproc_limit().map_err(|_| anyhow!("data_engine_owner_liveness_invalid"))?;
    validate_nproc_limit().map_err(|_| anyhow!("data_engine_owner_liveness_invalid"))?;
    validate_descriptor_inventory().map_err(|_| anyhow!("data_engine_owner_liveness_invalid"))?;

    let (armed_sender, armed_receiver) = mpsc::sync_channel::<()>(0);
    let watchdog = thread::Builder::new()
        .name("analytix-owner-liveness".to_string())
        .spawn(move || owner_liveness_watchdog(armed_sender))
        .map_err(|_| anyhow!("data_engine_owner_liveness_invalid"))?;
    if armed_receiver.recv_timeout(WATCHDOG_ARM_TIMEOUT).is_err() {
        return Err(anyhow!("data_engine_owner_liveness_invalid"));
    }
    // Dropping the join handle detaches the watchdog without leaking the
    // handle allocation. The watchdog owns FD 4 until process termination and
    // deliberately never unwinds after an owner-channel event.
    drop(watchdog);
    Ok(())
}

fn establish_nproc_limit() -> Result<()> {
    let limit = libc::rlimit {
        rlim_cur: 0,
        rlim_max: 0,
    };
    // SAFETY: setrlimit reads the complete immutable input structure.
    if unsafe { libc::setrlimit(libc::RLIMIT_NPROC, &limit) } != 0 {
        return Err(anyhow!("data_engine_owner_liveness_invalid"));
    }
    Ok(())
}

fn validate_nproc_limit() -> Result<()> {
    let mut limit = MaybeUninit::<libc::rlimit>::zeroed();
    // SAFETY: getrlimit initializes the complete rlimit on success.
    if unsafe { libc::getrlimit(libc::RLIMIT_NPROC, limit.as_mut_ptr()) } != 0 {
        return Err(anyhow!("data_engine_owner_liveness_invalid"));
    }
    // SAFETY: getrlimit returned success.
    let limit = unsafe { limit.assume_init() };
    if limit.rlim_cur != 0 || limit.rlim_max != 0 {
        return Err(anyhow!("data_engine_owner_liveness_invalid"));
    }
    Ok(())
}

fn validate_descriptor_inventory() -> Result<()> {
    let descriptors = current_descriptor_inventory().map_err(|_| anyhow!("descriptor-list"))?;
    let has_snapshot = descriptors.binary_search(&3).is_ok();
    let has_output = descriptors.binary_search(&CANONICAL_OUTPUT_FD).is_ok();
    let expected = if has_snapshot && has_output {
        validate_optional_snapshot_descriptor(3).map_err(|_| anyhow!("snapshot"))?;
        validate_optional_canonical_output_descriptor(CANONICAL_OUTPUT_FD)
            .map_err(|_| anyhow!("canonical-output"))?;
        vec![0, 1, 2, 3, OWNER_LIVENESS_FD, CANONICAL_OUTPUT_FD]
    } else if has_snapshot {
        validate_optional_snapshot_descriptor(3).map_err(|_| anyhow!("snapshot"))?;
        vec![0, 1, 2, 3, OWNER_LIVENESS_FD]
    } else {
        vec![0, 1, 2, OWNER_LIVENESS_FD]
    };
    if descriptors != expected {
        return Err(anyhow!("descriptor-set"));
    }
    validate_anonymous_pipe_descriptor(0, libc::O_RDONLY).map_err(|_| anyhow!("stdin"))?;
    validate_anonymous_pipe_descriptor(1, libc::O_WRONLY).map_err(|_| anyhow!("stdout"))?;
    validate_anonymous_pipe_descriptor(2, libc::O_WRONLY).map_err(|_| anyhow!("stderr"))?;
    validate_anonymous_pipe_descriptor(OWNER_LIVENESS_FD, libc::O_RDONLY)
        .map_err(|_| anyhow!("owner"))
}

fn current_descriptor_inventory() -> Result<Vec<libc::c_int>> {
    let entry_size = mem::size_of::<libc::proc_fdinfo>();
    if entry_size == 0 {
        return Err(anyhow!("data_engine_owner_liveness_invalid"));
    }
    // The admitted inventory has at most six descriptors. A larger fixed
    // buffer lets proc_pidinfo prove that the result was not truncated before
    // we apply that exact bound.
    let mut entries = vec![unsafe { MaybeUninit::<libc::proc_fdinfo>::zeroed().assume_init() }; 8];
    let read = unsafe {
        libc::proc_pidinfo(
            libc::getpid(),
            libc::PROC_PIDLISTFDS,
            0,
            entries.as_mut_ptr().cast(),
            (entries.len() * entry_size) as libc::c_int,
        )
    };
    if read <= 0 || read as usize % entry_size != 0 || read as usize == entries.len() * entry_size {
        return Err(anyhow!("data_engine_owner_liveness_invalid"));
    }
    let count = read as usize / entry_size;
    if !(4..=6).contains(&count) {
        return Err(anyhow!("data_engine_owner_liveness_invalid"));
    }
    entries.truncate(count);
    let mut descriptors = entries
        .into_iter()
        .map(|entry| entry.proc_fd)
        .collect::<Vec<_>>();
    descriptors.sort_unstable();
    descriptors.dedup();
    if descriptors.len() != count {
        return Err(anyhow!("data_engine_owner_liveness_invalid"));
    }
    Ok(descriptors)
}

fn validate_anonymous_pipe_descriptor(fd: libc::c_int, access_mode: libc::c_int) -> Result<()> {
    // SAFETY: fcntl/fstat/F_GETPATH only inspect the fixed inherited fd.
    let status_flags = unsafe { libc::fcntl(fd, libc::F_GETFL) };
    let descriptor_flags = unsafe { libc::fcntl(fd, libc::F_GETFD) };
    let mut stat = MaybeUninit::<libc::stat>::zeroed();
    if status_flags < 0
        || descriptor_flags < 0
        || status_flags & libc::O_ACCMODE != access_mode
        || status_flags & libc::O_NONBLOCK != 0
        || descriptor_flags & libc::FD_CLOEXEC != 0
        || unsafe { libc::fstat(fd, stat.as_mut_ptr()) } != 0
    {
        return Err(anyhow!("data_engine_owner_liveness_invalid"));
    }
    // SAFETY: fstat returned success.
    let stat = unsafe { stat.assume_init() };
    if stat.st_mode & libc::S_IFMT != libc::S_IFIFO
        || stat.st_nlink != 0
        || stat.st_uid != unsafe { libc::geteuid() }
    {
        return Err(anyhow!("data_engine_owner_liveness_invalid"));
    }
    let mut path = [0 as libc::c_char; libc::PATH_MAX as usize];
    if unsafe { libc::fcntl(fd, libc::F_GETPATH, path.as_mut_ptr()) } != -1
        || std::io::Error::last_os_error().raw_os_error() != Some(libc::EBADF)
    {
        return Err(anyhow!("data_engine_owner_liveness_invalid"));
    }
    Ok(())
}

fn validate_optional_snapshot_descriptor(fd: libc::c_int) -> Result<()> {
    let status_flags = unsafe { libc::fcntl(fd, libc::F_GETFL) };
    let descriptor_flags = unsafe { libc::fcntl(fd, libc::F_GETFD) };
    let mut stat = MaybeUninit::<libc::stat>::zeroed();
    if status_flags < 0
        || descriptor_flags < 0
        || status_flags & libc::O_ACCMODE != libc::O_RDONLY
        || descriptor_flags & libc::FD_CLOEXEC != 0
        || unsafe { libc::fstat(fd, stat.as_mut_ptr()) } != 0
    {
        return Err(anyhow!("data_engine_owner_liveness_invalid"));
    }
    let stat = unsafe { stat.assume_init() };
    if stat.st_mode & libc::S_IFMT != libc::S_IFREG
        || stat.st_nlink != 0
        || stat.st_uid != unsafe { libc::geteuid() }
        || stat.st_mode & 0o7777 != 0o400
        || stat.st_size <= 0
        || unsafe { libc::lseek(fd, 0, libc::SEEK_CUR) } < 0
    {
        return Err(anyhow!("data_engine_owner_liveness_invalid"));
    }
    Ok(())
}

fn validate_optional_canonical_output_descriptor(fd: libc::c_int) -> Result<()> {
    let status_flags = unsafe { libc::fcntl(fd, libc::F_GETFL) };
    let descriptor_flags = unsafe { libc::fcntl(fd, libc::F_GETFD) };
    let mut stat = MaybeUninit::<libc::stat>::zeroed();
    if status_flags < 0
        || descriptor_flags < 0
        || status_flags & libc::O_ACCMODE != libc::O_RDWR
        || descriptor_flags & libc::FD_CLOEXEC != 0
        || unsafe { libc::fstat(fd, stat.as_mut_ptr()) } != 0
    {
        return Err(anyhow!("data_engine_owner_liveness_invalid"));
    }
    // SAFETY: fstat returned success.
    let stat = unsafe { stat.assume_init() };
    if stat.st_mode & libc::S_IFMT != libc::S_IFREG
        || stat.st_nlink != 0
        || stat.st_uid != unsafe { libc::geteuid() }
        || stat.st_gid != unsafe { libc::getegid() }
        || stat.st_mode & 0o7777 != 0o600
        || stat.st_size != 0
        || unsafe { libc::lseek(fd, 0, libc::SEEK_CUR) } != 0
    {
        return Err(anyhow!("data_engine_owner_liveness_invalid"));
    }
    Ok(())
}

fn owner_liveness_watchdog(armed: mpsc::SyncSender<()>) -> ! {
    let mut pollfd = libc::pollfd {
        fd: OWNER_LIVENESS_FD,
        events: libc::POLLIN | libc::POLLHUP | libc::POLLERR,
        revents: 0,
    };
    // SAFETY: poll only observes the fixed inherited descriptor.
    let initial = unsafe { libc::poll(&mut pollfd, 1, 0) };
    if initial < 0 {
        unsafe { libc::_exit(WATCHDOG_PROTOCOL_EXIT) }
    }
    if initial > 0 || pollfd.revents != 0 {
        owner_liveness_terminal_read();
    }
    if armed.send(()).is_err() {
        unsafe { libc::_exit(WATCHDOG_PROTOCOL_EXIT) }
    }
    owner_liveness_terminal_read()
}

fn owner_liveness_terminal_read() -> ! {
    let mut byte = 0u8;
    // Any return is terminal: the owner never writes payload bytes, and EOF is
    // the owner-death signal. _exit deliberately runs no Rust destructors. This
    // same read handles owner loss both before and after watchdog arming.
    let read = unsafe {
        libc::read(
            OWNER_LIVENESS_FD,
            (&mut byte as *mut u8).cast(),
            mem::size_of_val(&byte),
        )
    };
    let code = if read == 0 {
        WATCHDOG_EOF_EXIT
    } else {
        WATCHDOG_PROTOCOL_EXIT
    };
    unsafe { libc::_exit(code) }
}
