package eventlog

import "errors"

func errorsJoin(values ...error) error {
	return errors.Join(values...)
}
