package emulator

import (
	"strings"
	"testing"
	"time"
)

// TestPTYQueryDoesNotDeadlock feeds a Primary Device Attributes query (ESC[c) —
// which the underlying vt emulator answers — through the PTY exactly the way a
// child program would, followed by visible text.
//
// Before the response-drain fix, the vt wrote its query response to a
// synchronous io.Pipe that nothing drained, so the write blocked inside
// ptyReadLoop while it held the emulator mutex. That wedged the read loop: the
// trailing text was never processed and GetScreen (which takes the same mutex)
// blocked too. With the fix, responseLoop drains the response back into the
// PTY and the read loop keeps running, so the text renders.
func TestPTYQueryDoesNotDeadlock(t *testing.T) {
	e, err := New(20, 5)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer e.Close()

	// e.tty is the child side: writing here simulates the child's stdout, which
	// the read loop consumes from the master. The DA1 query triggers a vt
	// response; the text after it only renders if the loop didn't deadlock.
	if _, err := e.tty.Write([]byte("\x1b[cHELLO")); err != nil {
		t.Fatalf("write to tty: %v", err)
	}

	done := make(chan struct{})
	go func() {
		for {
			if strings.Contains(strings.Join(e.GetScreen().Rows, ""), "HELLO") {
				close(done)
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
	}()

	select {
	case <-done:
		// Rendered: the read loop survived the query response.
	case <-time.After(3 * time.Second):
		t.Fatal("timed out after a DA query — PTY read loop deadlocked on the undrained vt response")
	}
}
