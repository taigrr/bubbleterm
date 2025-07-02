package bubbleterm

import (
	"os/exec"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea/v2"
	"github.com/taigrr/bib/emulator"
)

// Model represents the terminal bubble state
type Model struct {
	emulator *emulator.Emulator
	width    int
	height   int
	focused  bool
	err      error
	frame    emulator.EmittedFrame
}

// New creates a new terminal bubble with the specified dimensions
func New(width, height int) (*Model, error) {
	emu, err := emulator.New(width, height)
	if err != nil {
		return nil, err
	}

	return &Model{
		emulator: emu,
		width:    width,
		height:   height,
		focused:  true,
		frame:    emulator.EmittedFrame{Rows: make([]string, height)},
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

// Init initializes the bubble and starts polling for terminal updates
func (m *Model) Init() tea.Cmd {
	return tea.Batch(
		pollTerminal(m.emulator),
		tea.Tick(time.Millisecond*33, func(time.Time) tea.Msg { // ~30 FPS
			return tickMsg{}
		}),
	)
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
		// Update the frame with new terminal output
		m.frame = msg.Frame
		return m, pollTerminal(m.emulator) // Continue polling

	case terminalErrorMsg:
		m.err = msg.Err
		return m, nil

	case tickMsg:
		// Regular polling tick
		return m, tea.Batch(
			pollTerminal(m.emulator),
			tea.Tick(time.Millisecond*33, func(time.Time) tea.Msg {
				return tickMsg{}
			}),
		)

	case startCommandMsg:
		err := m.emulator.StartCommand(msg.Cmd)
		if err != nil {
			m.err = err
		}
		return m, nil
	}

	return m, nil
}

// View renders the terminal output
func (m *Model) View() string {
	if m.err != nil {
		return "Terminal error: " + m.err.Error()
	}

	// Join all rows with newlines to create the full terminal view
	return strings.Join(m.frame.Rows, "\n")
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
		return startCommandMsg{Cmd: cmd}
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