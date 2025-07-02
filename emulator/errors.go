package emulator

import "errors"

var (
	ErrPTYNotInitialized = errors.New("PTY not initialized")
	ErrInvalidSize       = errors.New("invalid terminal size")
)