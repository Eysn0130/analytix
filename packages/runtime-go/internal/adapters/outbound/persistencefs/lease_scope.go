package persistencefs

// platformScopeLease supplies a kernel-backed lock on the managed directory
// scope itself. Path-addressed lockfiles remain useful for cold-path precision,
// but cannot be the sole authority because their containing directory may be
// renamed and recreated while an old inode stays locked.
type platformScopeLease interface {
	Validate() error
	Close() error
}
