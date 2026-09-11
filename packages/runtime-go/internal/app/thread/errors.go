package thread

import "errors"

var ErrTurnNotFound = errors.New("turn not found")
var ErrThreadNotFound = errors.New("thread not found")
var ErrThreadRunning = errors.New("cannot rewind while a turn is running")
var ErrAcceptedFinalRewind = errors.New("accepted final cannot be removed by rewind")
var ErrCurrentSecurityContextRewind = errors.New("current security context cannot be removed by rewind")
