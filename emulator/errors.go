package emulator

import "errors"

var (
	ErrPTYNotInitialized = errors.New("PTY not initialized")
	ErrInvalidSize       = errors.New("invalid terminal size")
	ErrInvalidFrameRate  = errors.New("invalid frame rate: fps must be greater than 0")
)
