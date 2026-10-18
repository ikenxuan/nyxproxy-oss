package app

import "errors"

var ErrApplyInProgress = errors.New("a configuration apply is already in progress")
var ErrProxyAlreadyRunning = errors.New("proxy is already running")

type ApplyError struct {
	Code       string
	Message    string
	Fields     map[string]string
	RolledBack bool
}

func (e *ApplyError) Error() string {
	return e.Message
}
