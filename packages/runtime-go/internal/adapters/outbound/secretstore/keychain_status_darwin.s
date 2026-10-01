#include "textflag.h"

TEXT sec_interaction_trampoline<>(SB),NOSPLIT,$0-0
	JMP analytix_sec_interaction(SB)
GLOBL ·secInteractionAddress(SB), RODATA, $8
DATA ·secInteractionAddress(SB)/8, $sec_interaction_trampoline<>(SB)

TEXT sec_open_trampoline<>(SB),NOSPLIT,$0-0
	JMP analytix_sec_open(SB)
GLOBL ·secOpenAddress(SB), RODATA, $8
DATA ·secOpenAddress(SB)/8, $sec_open_trampoline<>(SB)

TEXT sec_status_trampoline<>(SB),NOSPLIT,$0-0
	JMP analytix_sec_status(SB)
GLOBL ·secStatusAddress(SB), RODATA, $8
DATA ·secStatusAddress(SB)/8, $sec_status_trampoline<>(SB)

TEXT cf_release_trampoline<>(SB),NOSPLIT,$0-0
	JMP analytix_cf_release(SB)
GLOBL ·cfReleaseAddress(SB), RODATA, $8
DATA ·cfReleaseAddress(SB)/8, $cf_release_trampoline<>(SB)
