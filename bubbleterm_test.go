package bubbleterm

import (
	"io"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
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

func TestKeyToTerminalInput(t *testing.T) {
	tests := []struct {
		name     string
		key      string
		expected string
	}{
		{"enter", "enter", "\r"},
		{"tab", "tab", "\t"},
		{"backspace", "backspace", "\x7f"},
		{"delete", "delete", "\x1b[3~"},
		{"escape", "esc", "\x1b"},
		{"space", " ", " "},
		{"up", "up", "\x1b[A"},
		{"down", "down", "\x1b[B"},
		{"right", "right", "\x1b[C"},
		{"left", "left", "\x1b[D"},
		{"home", "home", "\x1b[H"},
		{"end", "end", "\x1b[F"},
		{"pageup", "pageup", "\x1b[5~"},
		{"pagedown", "pagedown", "\x1b[6~"},
		{"insert", "insert", "\x1b[2~"},
		{"ctrl+a", "ctrl+a", "\x01"},
		{"ctrl+c", "ctrl+c", "\x03"},
		{"ctrl+d", "ctrl+d", "\x04"},
		{"ctrl+e", "ctrl+e", "\x05"},
		{"ctrl+k", "ctrl+k", "\x0b"},
		{"ctrl+l", "ctrl+l", "\x0c"},
		{"ctrl+r", "ctrl+r", "\x12"},
		{"ctrl+u", "ctrl+u", "\x15"},
		{"ctrl+w", "ctrl+w", "\x17"},
		{"ctrl+z", "ctrl+z", "\x1a"},
		{"f1", "f1", "\x1bOP"},
		{"f12", "f12", "\x1b[24~"},
		{"letter a", "a", "a"},
		{"letter z", "z", "z"},
		{"digit 5", "5", "5"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a minimal KeyMsg using bubbletea's parser
			// We test the string-based matching directly
			msg := testKeyMsg(tt.key)
			got := keyToTerminalInput(msg)
			if got != tt.expected {
				t.Errorf("keyToTerminalInput(%q) = %q, want %q", tt.key, got, tt.expected)
			}
		})
	}
}

// testKeyMsg creates a tea.KeyMsg that returns the given string from String()
type testKeyMsg string

func (k testKeyMsg) String() string    { return string(k) }
func (k testKeyMsg) Key() tea.Key      { return tea.Key{} }

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
