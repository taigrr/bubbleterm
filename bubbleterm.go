package bubbleterm

import (
	"io"
	"os/exec"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/taigrr/bubbleterm/emulator"
)

// translatedMouseMsg wraps mouse events with translated coordinates
type translatedMouseMsg struct {
	OriginalMsg tea.Msg
	EmulatorID  string // ID of the emulator this message is for
	X, Y        int
}

// Model represents the terminal bubble state
type Model struct {
	emulator   *emulator.Emulator
	width      int
	height     int
	focused    bool
	err        error
	frame      emulator.EmittedFrame
	cachedView string // Cache the rendered view string
	autoPoll   bool   // Whether to automatically poll for updates
}

// New creates a new terminal bubble with the specified dimensions
func New(width, height int) (*Model, error) {
	emu, err := emulator.New(width, height)
	if err != nil {
		return nil, err
	}

	return &Model{
		emulator:   emu,
		width:      width,
		height:     height,
		focused:    true,
		frame:      emulator.EmittedFrame{Rows: make([]string, height)},
		cachedView: strings.Repeat("\n", height-1), // Initialize with empty lines
		autoPoll:   true,
	}, nil
}

func (m *Model) SetAutoPoll(autoPoll bool) {
	m.autoPoll = autoPoll
}

// NewWithPipes creates a new terminal bubble that reads process output from r
// and writes user input to w. This allows embedding a terminal view for an
// already-running process where you have access to its stdin/stdout pipes
// (e.g., when the process was started by a third-party library).
//
// Example:
//
//	cmd := exec.Command("bash")
//	stdin, _ := cmd.StdinPipe()
//	stdout, _ := cmd.StdoutPipe()
//	cmd.Start()
//	model, _ := bubbleterm.NewWithPipes(80, 24, stdout, stdin)
func NewWithPipes(width, height int, r io.Reader, w io.WriteCloser) (*Model, error) {
	emu, err := emulator.NewFromPipes(width, height, r, w)
	if err != nil {
		return nil, err
	}

	return &Model{
		emulator:   emu,
		width:      width,
		height:     height,
		focused:    true,
		frame:      emulator.EmittedFrame{Rows: make([]string, height)},
		cachedView: strings.Repeat("\n", height-1),
		autoPoll:   true,
	}, nil
}

// NewWithCommand creates a new terminal bubble and starts the specified command
func NewWithCommand(width, height int, cmd *exec.Cmd) (*Model, error) {
	model, err := New(width, height)
	if err != nil {
		return nil, err
	}

	err = model.emulator.StartCommand(cmd)
	if err != nil {
		model.emulator.Close()
		return nil, err
	}

	return model, nil
}

// Init initializes the bubble (no automatic ticking)
func (m *Model) Init() tea.Cmd {
	// When auto-polling, start the self-rescheduling blocking poll loop.
	// Otherwise grab the initial frame once and let the external ticker
	// drive subsequent updates.
	if m.autoPoll {
		return pollTerminal(m.emulator)
	}
	return pollTerminalOnce(m.emulator)
}

// Update handles messages and updates the model state
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if !m.focused {
			return m, nil
		}

		// Convert bubbletea key events to terminal input
		input := keyToTerminalInput(msg)
		if input != "" {
			return m, sendInput(m.emulator, input)
		}

	case tea.MouseClickMsg:
		if !m.focused {
			return m, nil
		}
		// Send mouse click to terminal
		return m, sendMouseEvent(m.emulator, msg.Mouse().X, msg.Mouse().Y, int(msg.Mouse().Button), true)

	case tea.MouseReleaseMsg:
		if !m.focused {
			return m, nil
		}
		// Send mouse release to terminal
		return m, sendMouseEvent(m.emulator, msg.Mouse().X, msg.Mouse().Y, int(msg.Mouse().Button), false)

	case tea.MouseMotionMsg:
		if !m.focused {
			return m, nil
		}
		// Send mouse motion to terminal (button -1 indicates motion without button)
		return m, sendMouseEvent(m.emulator, msg.Mouse().X, msg.Mouse().Y, -1, false)

	case tea.MouseWheelMsg:
		if !m.focused {
			return m, nil
		}
		return m, sendMouseWheel(m.emulator, msg.Mouse().X, msg.Mouse().Y, int(msg.Mouse().Button))

	case translatedMouseMsg:
		if !m.focused {
			return m, nil
		}
		if msg.EmulatorID != m.emulator.ID() {
			return m, nil // Ignore messages from other emulators
		}
		// Handle translated mouse events with proper coordinates
		switch originalMsg := msg.OriginalMsg.(type) {
		case tea.MouseClickMsg:
			return m, sendMouseEvent(m.emulator, msg.X, msg.Y, int(originalMsg.Mouse().Button), true)
		case tea.MouseReleaseMsg:
			return m, sendMouseEvent(m.emulator, msg.X, msg.Y, int(originalMsg.Mouse().Button), false)
		case tea.MouseMotionMsg:
			return m, sendMouseEvent(m.emulator, msg.X, msg.Y, -1, false)
		case tea.MouseWheelMsg:
			return m, sendMouseWheel(m.emulator, msg.X, msg.Y, int(originalMsg.Mouse().Button))
		}

	case tea.WindowSizeMsg:
		// Handle terminal resize
		if msg.Width != m.width || msg.Height != m.height {
			m.width = msg.Width
			m.height = msg.Height
			return m, resizeTerminal(m.emulator, msg.Width, msg.Height)
		}

	case terminalOutputMsg:
		if msg.EmulatorID != m.emulator.ID() {
			return m, nil // Ignore messages from other emulators
		}
		if len(msg.Frame.Damage) == 0 {
			// The auto-poll loop only emits damaged frames, so this path
			// is only reachable from pollTerminalOnce or manual GetScreen
			// calls. Rescheduling a blocking poll on every undamaged
			// message would accumulate goroutines, so do nothing here.
			return m, nil
		}
		m.frame = msg.Frame
		m.cachedView = strings.Join(m.frame.Rows, "\n")
		if m.autoPoll {
			return m, pollTerminal(m.emulator)
		}
		return m, nil

	case terminalErrorMsg:
		if msg.EmulatorID != m.emulator.ID() {
			return m, nil // Ignore messages from other emulators
		}
		m.err = msg.Err
		return m, nil

	case startCommandMsg:
		if msg.EmulatorID != m.emulator.ID() {
			return m, nil // Ignore messages from other emulators
		}
		err := m.emulator.StartCommand(msg.Cmd)
		if err != nil {
			m.err = err
		}
		return m, nil
	}

	return m, nil
}

// UpdateTerminal manually polls the terminal for updates (called by external ticker)
func (m *Model) UpdateTerminal() tea.Cmd {
	return pollTerminalOnce(m.emulator)
}

// View renders the terminal output.
func (m *Model) View() tea.View {
	if m.err != nil {
		return tea.NewView("Terminal error: " + m.err.Error())
	}

	// Return cached view for maximum performance
	return tea.NewView(m.cachedView)
}

// Focus sets the bubble as focused (receives keyboard input)
func (m *Model) Focus() {
	m.focused = true
}

// Blur removes focus from the bubble
func (m *Model) Blur() {
	m.focused = false
}

// Focused returns whether the bubble is currently focused
func (m *Model) Focused() bool {
	return m.focused
}

// StartCommand starts a new command in the terminal
func (m *Model) StartCommand(cmd *exec.Cmd) tea.Cmd {
	return func() tea.Msg {
		return startCommandMsg{Cmd: cmd, EmulatorID: m.emulator.ID()}
	}
}

// SendInput sends input to the terminal
func (m *Model) SendInput(input string) tea.Cmd {
	return sendInput(m.emulator, input)
}

// Resize changes the terminal dimensions
func (m *Model) Resize(width, height int) tea.Cmd {
	m.width = width
	m.height = height
	return resizeTerminal(m.emulator, width, height)
}

// GetEmulator returns the underlying emulator (for process monitoring)
func (m *Model) GetEmulator() *emulator.Emulator {
	return m.emulator
}

// Close shuts down the terminal emulator
func (m *Model) Close() error {
	if m.emulator != nil {
		return m.emulator.Close()
	}
	return nil
}
