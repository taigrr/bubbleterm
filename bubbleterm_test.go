package bubbleterm

import (
	"io"
	"strings"
	"testing"
	"time"
)

func TestNewWithPipes(t *testing.T) {
	// Create a pipe pair: we write ANSI output to pw, the emulator reads from pr
	pr, pw := io.Pipe()
	// Create a pipe for input: emulator writes to iw, we could read from ir
	ir, iw := io.Pipe()
	_ = ir

	model, err := NewWithPipes(80, 24, pr, iw)
	if err != nil {
		t.Fatalf("NewWithPipes failed: %v", err)
	}
	defer model.Close()

	// Write some text to the emulator's output
	go func() {
		pw.Write([]byte("Hello, pipes!\r\n"))
	}()

	// Give the read loop time to process
	time.Sleep(100 * time.Millisecond)

	// Read directly from the emulator's screen (the View() cache
	// is only updated via bubbletea message passing)
	frame := model.GetEmulator().GetScreen()
	combined := strings.Join(frame.Rows, "\n")
	if !strings.Contains(combined, "Hello, pipes!") {
		t.Errorf("expected screen to contain 'Hello, pipes!', got: %q", combined)
	}
}

func TestNewWithPipes_SendInput(t *testing.T) {
	pr, _ := io.Pipe()
	ir, iw := io.Pipe()

	model, err := NewWithPipes(80, 24, pr, iw)
	if err != nil {
		t.Fatalf("NewWithPipes failed: %v", err)
	}
	defer model.Close()

	// Send input through the model
	done := make(chan string, 1)
	go func() {
		buf := make([]byte, 256)
		n, _ := ir.Read(buf)
		done <- string(buf[:n])
	}()

	// Use the emulator's Write directly
	model.GetEmulator().Write([]byte("test input"))

	select {
	case got := <-done:
		if got != "test input" {
			t.Errorf("expected 'test input', got %q", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for input")
	}
}

func TestNew(t *testing.T) {
	model, err := New(80, 24)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer model.Close()

	view := model.View()
	if view.Content == "" {
		t.Error("expected non-empty initial view")
	}
}
