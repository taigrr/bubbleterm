package emulator

import (
	"errors"
	"io"
	"strings"
	"testing"
)

type finalDataReader struct {
	data  string
	err   error
	calls int
}

func (r *finalDataReader) Read(buf []byte) (int, error) {
	r.calls++
	if r.calls > 1 {
		return 0, io.EOF
	}
	return copy(buf, r.data), r.err
}

func TestReadLoopProcessesDataBeforeError(t *testing.T) {
	for _, readErr := range []error{io.EOF, io.ErrUnexpectedEOF, errors.New("connection lost")} {
		t.Run(readErr.Error(), func(t *testing.T) {
			reader := &finalDataReader{data: "final output", err: readErr}
			emu := newEmulator(20, 3)
			emu.isPipe = true
			emu.reader = reader
			defer emu.Close()
			emu.GetScreen() // Consume initial damage and notification state.
			emu.ptyReadLoop()
			frame := emu.GetScreen()
			if got := strings.Join(frame.Rows, "\n"); !strings.Contains(got, reader.data) {
				t.Fatalf("final read bytes missing from screen: %q", got)
			}
			if len(frame.Damage) == 0 {
				t.Fatal("final read did not damage the screen")
			}
			select {
			case <-emu.NotifyChanged():
			default:
				t.Fatal("final read did not notify pollers")
			}
			if reader.calls != 1 {
				t.Fatalf("read %d times; should stop after the terminal error", reader.calls)
			}
		})
	}
}
