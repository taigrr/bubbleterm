package main

import (
	"testing"
)

func TestCreateNewTerminalWindow_CommandFailure(t *testing.T) {
	// Regression test: createNewTerminalWindow used to call
	// terminal.SetAutoPoll(false) before checking the error from
	// NewWithCommand. When NewWithCommand returned (nil, err),
	// this caused a nil pointer dereference.

	m := NewMultiWindowOS()

	// Temporarily swap PATH so that "bash" cannot be found,
	// causing NewWithCommand to fail when it tries to start the process.
	t.Setenv("PATH", "/nonexistent")

	// This must not panic.
	cmd := m.createNewTerminalWindow(5, 5)

	// When the command fails, no window should be added.
	if len(m.Windows) != 0 {
		t.Fatalf("expected 0 windows after failed command, got %d", len(m.Windows))
	}

	// The returned command should be nil since creation failed.
	if cmd != nil {
		t.Fatal("expected nil command when terminal creation fails")
	}
}
