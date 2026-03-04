package emulator

import (
	"testing"
)

func TestEmulatorCreation(t *testing.T) {
	e, err := New(80, 24)
	if err != nil {
		t.Fatalf("failed to create emulator: %v", err)
	}
	defer e.Close()

	if e.ID() == "" {
		t.Fatal("expected non-empty ID")
	}
}

func TestEmulatorGetScreen(t *testing.T) {
	e, err := New(10, 5)
	if err != nil {
		t.Fatalf("failed to create emulator: %v", err)
	}
	defer e.Close()

	// Initial screen should have damage (full redraw)
	frame := e.GetScreen()
	if len(frame.Rows) != 5 {
		t.Fatalf("expected 5 rows, got %d", len(frame.Rows))
	}
	if len(frame.Damage) == 0 {
		t.Fatal("expected initial damage")
	}

	// After consuming, no more damage
	frame = e.GetScreen()
	if len(frame.Damage) != 0 {
		t.Fatalf("expected no damage after consumption, got %d", len(frame.Damage))
	}
}

func TestEmulatorResize(t *testing.T) {
	e, err := New(80, 24)
	if err != nil {
		t.Fatalf("failed to create emulator: %v", err)
	}
	defer e.Close()

	// Resize to new dimensions
	err = e.Resize(40, 12)
	if err != nil {
		t.Fatalf("failed to resize: %v", err)
	}

	// Get screen and verify new dimensions
	frame := e.GetScreen()
	if len(frame.Rows) != 12 {
		t.Fatalf("expected 12 rows after resize, got %d", len(frame.Rows))
	}
}

func TestEmulatorCursor(t *testing.T) {
	e, err := New(80, 24)
	if err != nil {
		t.Fatalf("failed to create emulator: %v", err)
	}
	defer e.Close()

	pos, visible := e.Cursor()
	// Cursor should be at origin initially
	if pos.X != 0 || pos.Y != 0 {
		t.Fatalf("expected cursor at (0,0), got (%d,%d)", pos.X, pos.Y)
	}
	if !visible {
		t.Fatal("expected cursor to be visible")
	}
}

func TestEmulatorPipeCreation(t *testing.T) {
	// Create a simple pipe pair
	r, w, err := createTestPipe()
	if err != nil {
		t.Skipf("could not create test pipe: %v", err)
	}

	e, err := NewFromPipes(80, 24, r, w)
	if err != nil {
		t.Fatalf("failed to create pipe-based emulator: %v", err)
	}
	defer e.Close()

	if e.ID() == "" {
		t.Fatal("expected non-empty ID")
	}
}

// createTestPipe creates a simple reader/writer pair for testing
func createTestPipe() (*testReader, *testWriter, error) {
	return &testReader{}, &testWriter{}, nil
}

type testReader struct{}

func (r *testReader) Read(p []byte) (n int, err error) {
	// Block forever - will be stopped by close
	select {}
}

type testWriter struct {
	closed bool
}

func (w *testWriter) Write(p []byte) (n int, err error) {
	return len(p), nil
}

func (w *testWriter) Close() error {
	w.closed = true
	return nil
}
