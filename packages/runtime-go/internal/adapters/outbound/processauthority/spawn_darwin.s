#include "textflag.h"

TEXT libc_posix_spawn_trampoline<>(SB),NOSPLIT,$0-0
	JMP	libc_posix_spawn(SB)
GLOBL	·libc_posix_spawn_trampoline_addr(SB), RODATA, $8
DATA	·libc_posix_spawn_trampoline_addr(SB)/8, $libc_posix_spawn_trampoline<>(SB)

TEXT libc_posix_spawnattr_init_trampoline<>(SB),NOSPLIT,$0-0
	JMP	libc_posix_spawnattr_init(SB)
GLOBL	·libc_posix_spawnattr_init_trampoline_addr(SB), RODATA, $8
DATA	·libc_posix_spawnattr_init_trampoline_addr(SB)/8, $libc_posix_spawnattr_init_trampoline<>(SB)

TEXT libc_posix_spawnattr_destroy_trampoline<>(SB),NOSPLIT,$0-0
	JMP	libc_posix_spawnattr_destroy(SB)
GLOBL	·libc_posix_spawnattr_destroy_trampoline_addr(SB), RODATA, $8
DATA	·libc_posix_spawnattr_destroy_trampoline_addr(SB)/8, $libc_posix_spawnattr_destroy_trampoline<>(SB)

TEXT libc_posix_spawnattr_setflags_trampoline<>(SB),NOSPLIT,$0-0
	JMP	libc_posix_spawnattr_setflags(SB)
GLOBL	·libc_posix_spawnattr_setflags_trampoline_addr(SB), RODATA, $8
DATA	·libc_posix_spawnattr_setflags_trampoline_addr(SB)/8, $libc_posix_spawnattr_setflags_trampoline<>(SB)

TEXT libc_posix_spawn_file_actions_init_trampoline<>(SB),NOSPLIT,$0-0
	JMP	libc_posix_spawn_file_actions_init(SB)
GLOBL	·libc_posix_spawn_file_actions_init_trampoline_addr(SB), RODATA, $8
DATA	·libc_posix_spawn_file_actions_init_trampoline_addr(SB)/8, $libc_posix_spawn_file_actions_init_trampoline<>(SB)

TEXT libc_posix_spawn_file_actions_destroy_trampoline<>(SB),NOSPLIT,$0-0
	JMP	libc_posix_spawn_file_actions_destroy(SB)
GLOBL	·libc_posix_spawn_file_actions_destroy_trampoline_addr(SB), RODATA, $8
DATA	·libc_posix_spawn_file_actions_destroy_trampoline_addr(SB)/8, $libc_posix_spawn_file_actions_destroy_trampoline<>(SB)

TEXT libc_posix_spawn_file_actions_adddup2_trampoline<>(SB),NOSPLIT,$0-0
	JMP	libc_posix_spawn_file_actions_adddup2(SB)
GLOBL	·libc_posix_spawn_file_actions_adddup2_trampoline_addr(SB), RODATA, $8
DATA	·libc_posix_spawn_file_actions_adddup2_trampoline_addr(SB)/8, $libc_posix_spawn_file_actions_adddup2_trampoline<>(SB)

TEXT libc_posix_spawn_file_actions_addfchdir_np_trampoline<>(SB),NOSPLIT,$0-0
	JMP	libc_posix_spawn_file_actions_addfchdir_np(SB)
GLOBL	·libc_posix_spawn_file_actions_addfchdir_np_trampoline_addr(SB), RODATA, $8
DATA	·libc_posix_spawn_file_actions_addfchdir_np_trampoline_addr(SB)/8, $libc_posix_spawn_file_actions_addfchdir_np_trampoline<>(SB)

TEXT libc_waitid_trampoline<>(SB),NOSPLIT,$0-0
	JMP	libc_waitid(SB)
GLOBL	·libc_waitid_trampoline_addr(SB), RODATA, $8
DATA	·libc_waitid_trampoline_addr(SB)/8, $libc_waitid_trampoline<>(SB)

TEXT libc_proc_pidpath_trampoline<>(SB),NOSPLIT,$0-0
	JMP	libc_proc_pidpath(SB)
GLOBL	·libc_proc_pidpath_trampoline_addr(SB), RODATA, $8
DATA	·libc_proc_pidpath_trampoline_addr(SB)/8, $libc_proc_pidpath_trampoline<>(SB)

TEXT libc___proc_info_trampoline<>(SB),NOSPLIT,$0-0
	JMP	libc___proc_info(SB)
GLOBL	·libc___proc_info_trampoline_addr(SB), RODATA, $8
DATA	·libc___proc_info_trampoline_addr(SB)/8, $libc___proc_info_trampoline<>(SB)
