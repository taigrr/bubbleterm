package bubbleterm

import (
	"io"
	"os/exec"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/taigrr/bubbleterm/emulator"
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

func (k testKeyMsg) String() string { return string(k) }
func (k testKeyMsg) Key() tea.Key   { return tea.Key{} }

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

func TestModelFocusAndBlur(t *testing.T) {
	model, err := New(80, 24)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer model.Close()

	if !model.Focused() {
		t.Fatal("expected new model to start focused")
	}

	model.Blur()
	if model.Focused() {
		t.Fatal("expected model to be blurred")
	}

	model.Focus()
	if !model.Focused() {
		t.Fatal("expected model to be focused after Focus()")
	}
}

func TestModelInitReturnsTerminalOutputMsg(t *testing.T) {
	model, err := New(80, 24)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer model.Close()

	msg := model.Init()()
	outputMsg, ok := msg.(terminalOutputMsg)
	if !ok {
		t.Fatalf("expected terminalOutputMsg, got %T", msg)
	}
	if outputMsg.EmulatorID != model.GetEmulator().ID() {
		t.Fatalf("expected emulator ID %q, got %q", model.GetEmulator().ID(), outputMsg.EmulatorID)
	}
	if len(outputMsg.Frame.Rows) != 24 {
		t.Fatalf("expected 24 rows, got %d", len(outputMsg.Frame.Rows))
	}
}

func TestModelUpdateIgnoresKeyboardWhenBlurred(t *testing.T) {
	pr, _ := io.Pipe()
	ir, iw := io.Pipe()

	model, err := NewWithPipes(80, 24, pr, iw)
	if err != nil {
		t.Fatalf("NewWithPipes failed: %v", err)
	}
	defer model.Close()
	model.Blur()

	updated, cmd := model.Update(testKeyMsg("a"))
	if updated != model {
		t.Fatal("expected Update to return same model pointer")
	}
	if cmd != nil {
		t.Fatal("expected no command when model is blurred")
	}

	select {
	case <-time.After(50 * time.Millisecond):
	case <-func() chan struct{} {
		done := make(chan struct{}, 1)
		go func() {
			buf := make([]byte, 1)
			_, _ = ir.Read(buf)
			done <- struct{}{}
		}()
		return done
	}():
		t.Fatal("unexpected input sent while blurred")
	}
}

func TestModelUpdateProcessesTerminalOutputAndView(t *testing.T) {
	model, err := New(80, 24)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer model.Close()

	frame := emulator.EmittedFrame{
		Rows:   []string{"hello", "world"},
		Damage: []emulator.LineDamage{{Row: 0, X1: 0, X2: 5, Reason: emulator.CRText}},
	}

	updated, cmd := model.Update(terminalOutputMsg{Frame: frame, EmulatorID: model.GetEmulator().ID()})
	if updated != model {
		t.Fatal("expected Update to return same model pointer")
	}
	if cmd == nil {
		t.Fatal("expected autopoll command after terminal output")
	}
	if got := model.View().Content; !strings.Contains(got, "hello\nworld") {
		t.Fatalf("expected cached view to contain updated rows, got %q", got)
	}
}

func TestModelUpdateIgnoresMessagesFromOtherEmulator(t *testing.T) {
	model, err := New(80, 24)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer model.Close()

	initial := model.View().Content
	updated, cmd := model.Update(terminalOutputMsg{
		EmulatorID: "someone-else",
		Frame: emulator.EmittedFrame{
			Rows:   []string{"changed"},
			Damage: []emulator.LineDamage{{Row: 0, X1: 0, X2: 7, Reason: emulator.CRText}},
		},
	})
	if updated != model {
		t.Fatal("expected Update to return same model pointer")
	}
	if cmd != nil {
		t.Fatal("expected no command for other emulator messages")
	}
	if got := model.View().Content; got != initial {
		t.Fatalf("expected view to stay unchanged, got %q", got)
	}
}

func TestModelResizeUpdatesDimensions(t *testing.T) {
	model, err := New(80, 24)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer model.Close()

	cmd := model.Resize(50, 12)
	if model.width != 50 || model.height != 12 {
		t.Fatalf("expected model dimensions 50x12, got %dx%d", model.width, model.height)
	}
	if cmd == nil {
		t.Fatal("expected resize command")
	}

	msg := cmd()
	if _, ok := msg.(terminalErrorMsg); ok {
		t.Fatalf("unexpected resize error: %+v", msg)
	}

	frame := model.GetEmulator().GetScreen()
	if len(frame.Rows) != 12 {
		t.Fatalf("expected 12 rows after resize, got %d", len(frame.Rows))
	}
}

func TestStartCommandCmdReturnsStartCommandMsg(t *testing.T) {
	model, err := New(80, 24)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer model.Close()

	cmd := exec.Command("true")
	msg := model.StartCommand(cmd)()
	startMsg, ok := msg.(startCommandMsg)
	if !ok {
		t.Fatalf("expected startCommandMsg, got %T", msg)
	}
	if startMsg.Cmd != cmd {
		t.Fatal("expected start command message to hold original command")
	}
	if startMsg.EmulatorID != model.GetEmulator().ID() {
		t.Fatalf("expected emulator ID %q, got %q", model.GetEmulator().ID(), startMsg.EmulatorID)
	}
}

func TestCloseNilEmulator(t *testing.T) {
	model := &Model{}
	if err := model.Close(); err != nil {
		t.Fatalf("expected nil error closing model without emulator, got %v", err)
	}
}
