package bubbleterm

import (
	"os/exec"
	"strings"

	tea "github.com/charmbracelet/bubbletea/v2"
	"github.com/taigrr/bib/emulator"
)

// Model represents the terminal bubble state
type Model struct {
	emulator   *emulator.Emulator
	width      int
	height     int
	focused    bool
	err        error
	frame      emulator.EmittedFrame
	cachedView string // Cache the rendered view string
}

// New creates a new terminal bubble with the specified dimensions
func New(width, height int, id string) (*Model, error) {
	emu, err := emulator.New(width, height, id)
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
	}, nil
}

// NewWithCommand creates a new terminal bubble and starts the specified command
func NewWithCommand(width, height int, id string, cmd *exec.Cmd) (*Model, error) {
	// we need at least 2 columns for
	model, err := New(width-2, height, id)
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
	// Only do initial poll, no automatic ticking
	return pollTerminal(m.emulator)
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
		// Update the frame with new terminal output
		m.frame = msg.Frame
		// Cache the rendered view for fast access
		m.cachedView = strings.Join(m.frame.Rows, "\n")
		// Don't immediately poll again - let the tick handle regular polling
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
	return pollTerminal(m.emulator)
}

// View renders the terminal output
func (m *Model) View() string {
	if m.err != nil {
		return "Terminal error: " + m.err.Error()
	}

	// Return cached view for maximum performance
	return m.cachedView
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

// Close shuts down the terminal emulator
func (m *Model) Close() error {
	if m.emulator != nil {
		return m.emulator.Close()
	}
	return nil
}
