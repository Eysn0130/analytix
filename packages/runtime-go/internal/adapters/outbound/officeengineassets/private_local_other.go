//go:build !darwin && !linux

package officeengineassets

import "os"

// No private-local qualification is supported without the held-fd/single-link
// and ctime witness. The only admitted package target remains darwin-arm64.
func privateLstat(string) (privateStamp, error)   { return privateStamp{}, ErrUnavailable }
func privateFstat(*os.File) (privateStamp, error) { return privateStamp{}, ErrUnavailable }
func privateOpen(string) (*os.File, error)        { return nil, ErrUnavailable }
