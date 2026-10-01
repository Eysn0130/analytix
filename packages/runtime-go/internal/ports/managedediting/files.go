// Package managedediting defines filesystem observations used by capture leases.
package managedediting

// Identity is an immutable filesystem observation. Its concrete identity remains
// private to the adapter; the registry compares observations through Files.
type Identity interface {
	Regular() bool
	SingleLink() bool
}

// Files validates canonical absolute paths without following symbolic links.
// Inspect may return a nil identity only when allowMissing is true and every
// existing prefix was verified. Ancestors are ordered from root to parent.
// Contains includes equality. SameFile compares filesystem identity, not spelling.
type Files interface {
	Inspect(path string, allowMissing bool) (Identity, []Identity, error)
	Contains(parent, path string) bool
	SameFile(left, right Identity) bool
}
