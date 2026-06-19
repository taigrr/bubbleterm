package bubbleterm

import (
	"io"
	"os/exec"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
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
		msg      tea.KeyPressMsg
		expected string
	}{
		{"enter", tea.KeyPressMsg{Code: tea.KeyEnter}, "\r"},
		{"tab", tea.KeyPressMsg{Code: tea.KeyTab}, "\t"},
		{"backspace", tea.KeyPressMsg{Code: tea.KeyBackspace}, "\x7f"},
		{"delete", tea.KeyPressMsg{Code: tea.KeyDelete}, "\x1b[3~"},
		{"escape", tea.KeyPressMsg{Code: tea.KeyEscape}, "\x1b"},
		{"space", tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}, " "},
		{"up", tea.KeyPressMsg{Code: tea.KeyUp}, "\x1b[A"},
		{"down", tea.KeyPressMsg{Code: tea.KeyDown}, "\x1b[B"},
		{"right", tea.KeyPressMsg{Code: tea.KeyRight}, "\x1b[C"},
		{"left", tea.KeyPressMsg{Code: tea.KeyLeft}, "\x1b[D"},
		{"home", tea.KeyPressMsg{Code: tea.KeyHome}, "\x1b[H"},
		{"end", tea.KeyPressMsg{Code: tea.KeyEnd}, "\x1b[F"},
		{"pageup", tea.KeyPressMsg{Code: tea.KeyPgUp}, "\x1b[5~"},
		{"pagedown", tea.KeyPressMsg{Code: tea.KeyPgDown}, "\x1b[6~"},
		{"insert", tea.KeyPressMsg{Code: tea.KeyInsert}, "\x1b[2~"},
		{"ctrl+a", tea.KeyPressMsg{Code: 'a', Mod: tea.ModCtrl}, "\x01"},
		{"ctrl+c", tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}, "\x03"},
		{"ctrl+d", tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl}, "\x04"},
		{"ctrl+e", tea.KeyPressMsg{Code: 'e', Mod: tea.ModCtrl}, "\x05"},
		{"ctrl+k", tea.KeyPressMsg{Code: 'k', Mod: tea.ModCtrl}, "\x0b"},
		{"ctrl+l", tea.KeyPressMsg{Code: 'l', Mod: tea.ModCtrl}, "\x0c"},
		{"ctrl+r", tea.KeyPressMsg{Code: 'r', Mod: tea.ModCtrl}, "\x12"},
		{"ctrl+u", tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl}, "\x15"},
		{"ctrl+w", tea.KeyPressMsg{Code: 'w', Mod: tea.ModCtrl}, "\x17"},
		{"ctrl+z", tea.KeyPressMsg{Code: 'z', Mod: tea.ModCtrl}, "\x1a"},
		{"f1", tea.KeyPressMsg{Code: tea.KeyF1}, "\x1bOP"},
		{"f12", tea.KeyPressMsg{Code: tea.KeyF12}, "\x1b[24~"},
		{"letter a", tea.KeyPressMsg{Code: 'a', Text: "a"}, "a"},
		{"letter z", tea.KeyPressMsg{Code: 'z', Text: "z"}, "z"},
		{"digit 5", tea.KeyPressMsg{Code: '5', Text: "5"}, "5"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := keyToTerminalInput(tt.msg)
			if got != tt.expected {
				t.Errorf("keyToTerminalInput(%q) = %q, want %q", tt.msg.String(), got, tt.expected)
			}
		})
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

func TestNewWithCommandStartsProcess(t *testing.T) {
	model, err := NewWithCommand(80, 24, exec.Command("sh", "-c", "printf 'started via command'"))
	if err != nil {
		t.Fatalf("NewWithCommand failed: %v", err)
	}
	defer model.Close()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		frame := model.GetEmulator().GetScreen()
		if strings.Contains(strings.Join(frame.Rows, "\n"), "started via command") {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}

	frame := model.GetEmulator().GetScreen()
	t.Fatalf("expected started command output, got %q", strings.Join(frame.Rows, "\n"))
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

	updated, cmd := model.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
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

func TestModelUpdateSkipsUndamagedFrameWithoutMutatingView(t *testing.T) {
	model, err := New(80, 24)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer model.Close()

	initialView := model.View().Content
	updated, cmd := model.Update(terminalOutputMsg{
		EmulatorID: model.GetEmulator().ID(),
		Frame:      emulator.EmittedFrame{Rows: []string{"ignored"}},
	})
	if updated != model {
		t.Fatal("expected Update to return same model pointer")
	}
	if cmd == nil {
		t.Fatal("expected follow-up poll command for undamaged frame when auto-poll is enabled")
	}
	if got := model.View().Content; got != initialView {
		t.Fatalf("expected view to stay unchanged, got %q", got)
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

func TestResizeTerminalPassesFullWidth(t *testing.T) {
	model, err := New(80, 24)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer model.Close()

	// Send a WindowSizeMsg through Update, which calls resizeTerminal.
	// Before the fix, resizeTerminal subtracted 2 from width, so a
	// 40-wide message would resize the emulator to 38.
	_, cmd := model.Update(tea.WindowSizeMsg{Width: 40, Height: 10})
	if cmd == nil {
		t.Fatal("expected resize command from WindowSizeMsg")
	}

	// Execute the command to apply the resize
	msg := cmd()
	if errMsg, ok := msg.(terminalErrorMsg); ok {
		t.Fatalf("resize error: %v", errMsg.Err)
	}

	// The emulator should have the full width (40), not width-2 (38).
	// GetScreen row count reflects the emulator's height, and each row
	// is padded to the emulator's width using ansi.StringWidth.
	frame := model.GetEmulator().GetScreen()
	if len(frame.Rows) != 10 {
		t.Fatalf("expected 10 rows, got %d", len(frame.Rows))
	}
	// Use ansi.StringWidth to measure visible width, ignoring ANSI codes
	rowWidth := ansi.StringWidth(frame.Rows[0])
	if rowWidth != 40 {
		t.Fatalf("expected visible row width 40, got %d", rowWidth)
	}
}

func TestModelUpdateWindowSizeNoopWhenUnchanged(t *testing.T) {
	model, err := New(80, 24)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer model.Close()

	updated, cmd := model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	if updated != model {
		t.Fatal("expected Update to return same model pointer")
	}
	if cmd != nil {
		t.Fatal("expected no resize command when size is unchanged")
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

func TestModelUpdateKeyMsgSendsTranslatedInput(t *testing.T) {
	pr, _ := io.Pipe()
	ir, iw := io.Pipe()

	model, err := NewWithPipes(80, 24, pr, iw)
	if err != nil {
		t.Fatalf("NewWithPipes failed: %v", err)
	}
	defer model.Close()

	_, cmd := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected command for translated key input")
	}

	done := make(chan string, 1)
	go func() {
		buf := make([]byte, 8)
		n, _ := ir.Read(buf)
		done <- string(buf[:n])
	}()

	msg := cmd()
	if msg != nil {
		t.Fatalf("expected nil message for successful sendInput, got %T", msg)
	}

	select {
	case got := <-done:
		if got != "\r" {
			t.Fatalf("expected carriage return, got %q", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for translated input")
	}
}

func TestModelSendInputCmdWritesToPipe(t *testing.T) {
	pr, _ := io.Pipe()
	ir, iw := io.Pipe()

	model, err := NewWithPipes(80, 24, pr, iw)
	if err != nil {
		t.Fatalf("NewWithPipes failed: %v", err)
	}
	defer model.Close()

	done := make(chan string, 1)
	go func() {
		buf := make([]byte, 16)
		n, _ := ir.Read(buf)
		done <- string(buf[:n])
	}()

	msg := model.SendInput("hello")()
	if msg != nil {
		t.Fatalf("expected nil message for successful sendInput, got %T", msg)
	}

	select {
	case got := <-done:
		if got != "hello" {
			t.Fatalf("expected %q, got %q", "hello", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for model SendInput output")
	}
}

func TestModelUpdateMouseMsgReturnsSendCommand(t *testing.T) {
	pr, _ := io.Pipe()
	_, iw := io.Pipe()

	model, err := NewWithPipes(80, 24, pr, iw)
	if err != nil {
		t.Fatalf("NewWithPipes failed: %v", err)
	}
	defer model.Close()

	updated, cmd := model.Update(translatedMouseMsg{
		EmulatorID:  model.GetEmulator().ID(),
		OriginalMsg: tea.MouseMotionMsg{},
		X:           4,
		Y:           7,
	})
	if updated != model {
		t.Fatal("expected Update to return same model pointer")
	}
	if cmd == nil {
		t.Fatal("expected sendMouseEvent command")
	}
	if msg := cmd(); msg != nil {
		t.Fatalf("expected nil message for successful mouse send, got %T", msg)
	}
}

func TestModelUpdateHandlesMouseWheelMsg(t *testing.T) {
	pr, _ := io.Pipe()
	_, iw := io.Pipe()

	model, err := NewWithPipes(80, 24, pr, iw)
	if err != nil {
		t.Fatalf("NewWithPipes failed: %v", err)
	}
	defer model.Close()

	// MouseWheelMsg should produce a command (not be silently dropped)
	_, cmd := model.Update(tea.MouseWheelMsg{X: 5, Y: 5, Button: tea.MouseWheelUp})
	if cmd == nil {
		t.Fatal("expected a command from MouseWheelMsg, got nil")
	}

	_, cmd = model.Update(tea.MouseWheelMsg{X: 5, Y: 5, Button: tea.MouseWheelDown})
	if cmd == nil {
		t.Fatal("expected a command from MouseWheelMsg (down), got nil")
	}
}

func TestModelUpdateIgnoresMouseWheelWhenBlurred(t *testing.T) {
	pr, _ := io.Pipe()
	_, iw := io.Pipe()

	model, err := NewWithPipes(80, 24, pr, iw)
	if err != nil {
		t.Fatalf("NewWithPipes failed: %v", err)
	}
	defer model.Close()
	model.Blur()

	_, cmd := model.Update(tea.MouseWheelMsg{X: 5, Y: 5, Button: tea.MouseWheelUp})
	if cmd != nil {
		t.Fatal("expected no command when model is blurred")
	}
}

func TestModelUpdateTranslatedMouseIgnoresWrongEmulatorAndUnknownMessage(t *testing.T) {
	pr, _ := io.Pipe()
	_, iw := io.Pipe()

	model, err := NewWithPipes(80, 24, pr, iw)
	if err != nil {
		t.Fatalf("NewWithPipes failed: %v", err)
	}
	defer model.Close()

	cases := []translatedMouseMsg{
		{EmulatorID: "someone-else", OriginalMsg: tea.MouseMotionMsg{}, X: 1, Y: 2},
		{EmulatorID: model.GetEmulator().ID(), OriginalMsg: tea.WindowSizeMsg{}, X: 1, Y: 2},
	}

	for _, msg := range cases {
		updated, cmd := model.Update(msg)
		if updated != model {
			t.Fatal("expected Update to return same model pointer")
		}
		if cmd != nil {
			t.Fatalf("expected no command for translated mouse message %+v", msg)
		}
	}
}

func TestModelUpdateHandlesTranslatedMouseWheelMsg(t *testing.T) {
	pr, _ := io.Pipe()
	_, iw := io.Pipe()

	model, err := NewWithPipes(80, 24, pr, iw)
	if err != nil {
		t.Fatalf("NewWithPipes failed: %v", err)
	}
	defer model.Close()

	msg := translatedMouseMsg{
		EmulatorID:  model.emulator.ID(),
		X:           10,
		Y:           10,
		OriginalMsg: tea.MouseWheelMsg{X: 15, Y: 15, Button: tea.MouseWheelDown},
	}
	_, cmd := model.Update(msg)
	if cmd == nil {
		t.Fatal("expected a command from translated MouseWheelMsg, got nil")
	}
}

func TestModelUpdateTerminalReturnsPollCommand(t *testing.T) {
	model, err := New(80, 24)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer model.Close()

	cmd := model.UpdateTerminal()
	if cmd == nil {
		t.Fatal("expected poll command from UpdateTerminal")
	}

	msg := cmd()
	outputMsg, ok := msg.(terminalOutputMsg)
	if !ok {
		t.Fatalf("expected terminalOutputMsg, got %T", msg)
	}
	if outputMsg.EmulatorID != model.GetEmulator().ID() {
		t.Fatalf("expected emulator ID %q, got %q", model.GetEmulator().ID(), outputMsg.EmulatorID)
	}
}

func TestModelUpdateTerminalOutputHonorsAutoPoll(t *testing.T) {
	model, err := New(80, 24)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer model.Close()
	model.SetAutoPoll(false)

	frame := emulator.EmittedFrame{
		Rows:   []string{"updated"},
		Damage: []emulator.LineDamage{{Row: 0, X1: 0, X2: 7, Reason: emulator.CRText}},
	}

	updated, cmd := model.Update(terminalOutputMsg{Frame: frame, EmulatorID: model.GetEmulator().ID()})
	if updated != model {
		t.Fatal("expected Update to return same model pointer")
	}
	if cmd != nil {
		t.Fatal("expected no auto-poll command when auto-poll is disabled")
	}
	if got := model.View().Content; !strings.Contains(got, "updated") {
		t.Fatalf("expected cached view to include updated frame, got %q", got)
	}
}

func TestModelViewReturnsTerminalError(t *testing.T) {
	model, err := New(80, 24)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer model.Close()

	expectedErr := exec.ErrNotFound
	updated, cmd := model.Update(terminalErrorMsg{Err: expectedErr, EmulatorID: model.GetEmulator().ID()})
	if updated != model {
		t.Fatal("expected Update to return same model pointer")
	}
	if cmd != nil {
		t.Fatal("expected no command for terminal error")
	}
	if got := model.View().Content; got != "Terminal error: executable file not found in $PATH" {
		t.Fatalf("unexpected error view: %q", got)
	}
}

func TestPollTerminalBlocksUntilDamage(t *testing.T) {
	pr, pw := io.Pipe()
	_, iw := io.Pipe()

	model, err := NewWithPipes(10, 5, pr, iw)
	if err != nil {
		t.Fatalf("NewWithPipes failed: %v", err)
	}
	defer model.Close()

	// Consume initial damage so the emulator is in a clean state.
	initMsg := model.Init()()
	if _, ok := initMsg.(terminalOutputMsg); !ok {
		t.Fatalf("expected terminalOutputMsg from Init, got %T", initMsg)
	}

	// Start polling — should block because no new data has arrived.
	cmd := pollTerminal(model.emulator)
	done := make(chan tea.Msg, 1)
	go func() { done <- cmd() }()

	select {
	case msg := <-done:
		t.Fatalf("pollTerminal returned without new data: %T", msg)
	case <-time.After(50 * time.Millisecond):
		// Expected: still blocking.
	}

	// Write data through the pipe to trigger damage.
	if _, err := pw.Write([]byte("hello")); err != nil {
		t.Fatalf("pipe write failed: %v", err)
	}

	select {
	case msg := <-done:
		outputMsg, ok := msg.(terminalOutputMsg)
		if !ok {
			t.Fatalf("expected terminalOutputMsg, got %T", msg)
		}
		if len(outputMsg.Frame.Damage) == 0 {
			t.Fatal("expected damage in returned frame")
		}
		combined := strings.Join(outputMsg.Frame.Rows, "")
		if !strings.Contains(combined, "hello") {
			t.Errorf("expected 'hello' in frame rows, got: %q", combined)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("pollTerminal did not return after data was written")
	}
}

func TestCloseNilEmulator(t *testing.T) {
	model := &Model{}
	if err := model.Close(); err != nil {
		t.Fatalf("expected nil error closing model without emulator, got %v", err)
	}
}
