package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/google/uuid"
	"github.com/taigrr/bubbleterm"
)

// translatedMouseMsg wraps mouse events with translated coordinates
type translatedMouseMsg struct {
	OriginalMsg tea.Msg
	X, Y        int
}

func main() {
	p := tea.NewProgram(NewMultiWindowOS())
	if _, err := p.Run(); err != nil {
		log.Fatal(err)
	}
}

type MultiWindowOS struct {
	Dragging      bool
	DragOffsetX   int
	DragOffsetY   int
	Windows       []TerminalWindow
	CurrentZ      int
	FocusedWindow int
	InsertMode    bool // When true, all input goes to focused terminal
	width         int  // Screen width
	height        int  // Screen height
}

// centralTickMsg is sent by our centralized ticker
type centralTickMsg struct{}

type TerminalWindow struct {
	Title    string
	Width    int
	Height   int
	X        int
	Y        int
	Z        int
	ID       string
	Terminal *bubbleterm.Model
}

func NewMultiWindowOS() *MultiWindowOS {
	return &MultiWindowOS{
		FocusedWindow: -1, // No window focused initially
		Windows:       []TerminalWindow{},
		width:         80, // Default width
		height:        24, // Default height
	}
}

func (m *MultiWindowOS) Init() tea.Cmd {
	return tea.Tick(time.Millisecond*33, func(time.Time) tea.Msg { // 30 FPS centralized ticker
		return centralTickMsg{}
	})
}

func createID() string {
	return uuid.New().String()
}

func (m *MultiWindowOS) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		// Handle global keys when not in insert mode
		if !m.InsertMode {
			switch msg.String() {
			case "ctrl+c", "q", "esc":
				// Close all terminals before quitting
				for _, window := range m.Windows {
					window.Terminal.Close()
				}
				return m, tea.Quit
			case "i":
				// Enter insert mode if we have a focused window
				if m.FocusedWindow >= 0 && m.FocusedWindow < len(m.Windows) {
					m.InsertMode = true
					// Focus the terminal
					m.Windows[m.FocusedWindow].Terminal.Focus()
				}
				return m, nil
			case "=", "+":
				// Increase window size
				if m.FocusedWindow >= 0 && m.FocusedWindow < len(m.Windows) {
					cmd := m.resizeWindow(m.FocusedWindow, 2, 1)
					if cmd != nil {
						cmds = append(cmds, cmd)
					}
				}
				return m, tea.Batch(cmds...)
			case "-", "_":
				// Decrease window size
				if m.FocusedWindow >= 0 && m.FocusedWindow < len(m.Windows) {
					cmd := m.resizeWindow(m.FocusedWindow, -2, -1)
					if cmd != nil {
						cmds = append(cmds, cmd)
					}
				}
				return m, tea.Batch(cmds...)
			}
		} else {
			// In insert mode, handle escape to exit insert mode
			if msg.String() == "esc" {
				m.InsertMode = false
				// Blur the focused terminal
				if m.FocusedWindow >= 0 && m.FocusedWindow < len(m.Windows) {
					m.Windows[m.FocusedWindow].Terminal.Blur()
				}
				return m, nil
			}

			// Forward all other keys to the focused terminal
			if m.FocusedWindow >= 0 && m.FocusedWindow < len(m.Windows) {
				// Make sure terminal is focused before sending input
				m.Windows[m.FocusedWindow].Terminal.Focus()
				terminalModel, cmd := m.Windows[m.FocusedWindow].Terminal.Update(msg)
				m.Windows[m.FocusedWindow].Terminal = terminalModel.(*bubbleterm.Model)
				if cmd != nil {
					cmds = append(cmds, cmd)
				}
			}
		}

	case tea.MouseClickMsg:
		if m.InsertMode {
			// In insert mode, forward mouse events to focused terminal with coordinate translation
			if m.FocusedWindow >= 0 && m.FocusedWindow < len(m.Windows) {
				translatedMsg := m.translateMouseEvent(msg, m.Windows[m.FocusedWindow])
				if translatedMsg != nil {
					terminalModel, cmd := m.Windows[m.FocusedWindow].Terminal.Update(translatedMsg)
					m.Windows[m.FocusedWindow].Terminal = terminalModel.(*bubbleterm.Model)
					if cmd != nil {
						cmds = append(cmds, cmd)
					}
				}
			}
		} else {
			// Normal window management mode
			switch msg.Button {
			case tea.MouseRight:
				// Create new terminal window
				cmd := m.createNewTerminalWindow(msg.X, msg.Y)
				if cmd != nil {
					cmds = append(cmds, cmd)
				}
			case tea.MouseLeft:
				// Handle window selection and dragging
				m.handleWindowClick(msg.X, msg.Y)
			}
		}

	case tea.MouseMotionMsg:
		if m.InsertMode {
			// Forward mouse motion to focused terminal
			if m.FocusedWindow >= 0 && m.FocusedWindow < len(m.Windows) {
				translatedMsg := m.translateMouseEvent(msg, m.Windows[m.FocusedWindow])
				if translatedMsg != nil {
					terminalModel, cmd := m.Windows[m.FocusedWindow].Terminal.Update(translatedMsg)
					m.Windows[m.FocusedWindow].Terminal = terminalModel.(*bubbleterm.Model)
					if cmd != nil {
						cmds = append(cmds, cmd)
					}
				}
			}
		} else if m.Dragging && m.FocusedWindow >= 0 {
			// Handle window dragging
			m.Windows[m.FocusedWindow].X = msg.X - m.DragOffsetX
			m.Windows[m.FocusedWindow].Y = msg.Y - m.DragOffsetY
		}

	case tea.MouseReleaseMsg:
		if m.InsertMode {
			// Forward mouse release to focused terminal
			if m.FocusedWindow >= 0 && m.FocusedWindow < len(m.Windows) {
				translatedMsg := m.translateMouseEvent(msg, m.Windows[m.FocusedWindow])
				if translatedMsg != nil {
					terminalModel, cmd := m.Windows[m.FocusedWindow].Terminal.Update(translatedMsg)
					m.Windows[m.FocusedWindow].Terminal = terminalModel.(*bubbleterm.Model)
					if cmd != nil {
						cmds = append(cmds, cmd)
					}
				}
			}
		} else {
			if msg.Button == tea.MouseLeft {
				m.Dragging = false
			}
		}

	case tea.WindowSizeMsg:
		// Update our screen dimensions
		m.width = msg.Width
		m.height = msg.Height
		// Forward resize events to all terminals
		for i := range m.Windows {
			terminalModel, cmd := m.Windows[i].Terminal.Update(msg)
			m.Windows[i].Terminal = terminalModel.(*bubbleterm.Model)
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
		}

	case centralTickMsg:
		// Centralized terminal updates - poll all terminals
		var deadWindows []int
		for i := range m.Windows {
			cmd := m.Windows[i].Terminal.UpdateTerminal()
			if cmd != nil {
				cmds = append(cmds, cmd)
			}

			// Check if the process has exited
			if m.Windows[i].Terminal.GetEmulator().IsProcessExited() {
				deadWindows = append(deadWindows, i)
			}
		}

		// Remove dead windows (in reverse order to maintain indices)
		for i := len(deadWindows) - 1; i >= 0; i-- {
			windowIndex := deadWindows[i]
			// Close the terminal
			m.Windows[windowIndex].Terminal.Close()
			// Remove from slice
			m.Windows = append(m.Windows[:windowIndex], m.Windows[windowIndex+1:]...)
			// Adjust focused window index
			if m.FocusedWindow >= windowIndex {
				m.FocusedWindow--
			}
		}

		// Reset focus if no windows remain
		if len(m.Windows) == 0 {
			m.FocusedWindow = -1
			m.InsertMode = false // Exit insert mode when no windows remain
		} else if m.FocusedWindow < 0 {
			m.FocusedWindow = 0
		}

		// Schedule next tick
		cmds = append(cmds, tea.Tick(time.Millisecond*33, func(time.Time) tea.Msg {
			return centralTickMsg{}
		}))

	default:
		// Forward other messages to all terminals
		for i := range m.Windows {
			terminalModel, cmd := m.Windows[i].Terminal.Update(msg)
			m.Windows[i].Terminal = terminalModel.(*bubbleterm.Model)
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
		}

	}

	return m, tea.Batch(cmds...)
}

func (m *MultiWindowOS) resizeWindow(windowIndex int, deltaWidth, deltaHeight int) tea.Cmd {
	if windowIndex < 0 || windowIndex >= len(m.Windows) {
		return nil
	}

	window := &m.Windows[windowIndex]

	// Calculate new terminal dimensions (accounting for border = 2px each axis)
	newTermWidth := window.Width - 2 + deltaWidth
	newTermHeight := window.Height - 2 + deltaHeight

	// Minimum size constraints
	if newTermWidth < 20 || newTermHeight < 5 {
		return nil
	}

	// Maximum size constraints (reasonable limits)
	if newTermWidth > 120 || newTermHeight > 40 {
		return nil
	}

	// Update window dimensions
	window.Width = newTermWidth + 2
	window.Height = newTermHeight + 2

	// Resize the terminal emulator
	return window.Terminal.Resize(newTermWidth, newTermHeight)
}

func (m *MultiWindowOS) createNewTerminalWindow(x, y int) tea.Cmd {
	// Create a completely isolated bash instance with unique environment
	cmd := exec.Command("bash")

	// Give each terminal a completely unique environment to prevent any sharing
	cmd.Env = []string{
		fmt.Sprintf("TERMINAL_ID=%d", len(m.Windows)),
		fmt.Sprintf("PS1=Terminal-%d$ ", len(m.Windows)+1), // Unique prompt
		"TERM=xterm-256color",
		"PATH=/usr/local/bin:/usr/bin:/bin",
		"HOME=" + os.Getenv("HOME"),
	}

	newID := createID()
	// Account for border (1px) + padding (2px) = 3px total on each side
	// So window width 40 = terminal width 34 (40 - 6)
	// Window height 14 = terminal height 10 (14 - 4, accounting for top/bottom border+padding)
	terminal, err := bubbleterm.NewWithCommand(34, 10, cmd)
	// disable auto-polling to avoid conflicts with our centralized tick
	terminal.SetAutoPoll(false)
	if err != nil {
		return nil
	}

	window := TerminalWindow{
		Title:    fmt.Sprintf("Terminal %d", len(m.Windows)+1),
		Width:    36, // Terminal width (34) + border (2)
		Height:   12, // Terminal height (10) + border (2)
		X:        x,
		Y:        y,
		Z:        m.CurrentZ,
		ID:       newID,
		Terminal: terminal,
	}

	m.Windows = append(m.Windows, window)
	m.FocusedWindow = len(m.Windows) - 1
	m.CurrentZ++

	// Return the terminal's init command
	return terminal.Init()
}

func (m *MultiWindowOS) handleWindowClick(x, y int) {
	// Find the topmost window that contains the click point
	// Sort windows by Z-order (descending) to find topmost first
	topWindow := -1
	topZ := -1
	for i, window := range m.Windows {
		// Check if click is within window bounds
		if x >= window.X && x < window.X+window.Width &&
			y >= window.Y && y < window.Y+window.Height {
			if window.Z > topZ {
				topZ = window.Z
				topWindow = i
			}
		}
	}

	if topWindow >= 0 {
		m.DragOffsetX = x - m.Windows[topWindow].X
		m.DragOffsetY = y - m.Windows[topWindow].Y
		m.Windows[topWindow].Z = m.CurrentZ
		m.CurrentZ++
		m.FocusedWindow = topWindow
		m.Dragging = true
	}
}

func (m *MultiWindowOS) translateMouseEvent(msg tea.Msg, window TerminalWindow) tea.Msg {
	// Translate mouse coordinates from screen space to window space
	switch msg := msg.(type) {
	case tea.MouseClickMsg:
		mouse := msg.Mouse()
		// Adjust coordinates relative to window position (accounting for border)
		newX := mouse.X - window.X - 1 // -1 for border
		newY := mouse.Y - window.Y - 1 // -1 for border

		// Only forward if click is within terminal bounds (34x10)
		if newX >= 0 && newX < 34 && newY >= 0 && newY < 10 {
			return translatedMouseMsg{
				OriginalMsg: msg,
				X:           newX,
				Y:           newY,
			}
		}
		return nil

	case tea.MouseMotionMsg:
		mouse := msg.Mouse()
		newX := mouse.X - window.X - 1
		newY := mouse.Y - window.Y - 1

		if newX >= 0 && newX < 34 && newY >= 0 && newY < 10 {
			return translatedMouseMsg{
				OriginalMsg: msg,
				X:           newX,
				Y:           newY,
			}
		}
		return nil

	case tea.MouseReleaseMsg:
		mouse := msg.Mouse()
		newX := mouse.X - window.X - 1
		newY := mouse.Y - window.Y - 1

		if newX >= 0 && newX < 34 && newY >= 0 && newY < 10 {
			return translatedMouseMsg{
				OriginalMsg: msg,
				X:           newX,
				Y:           newY,
			}
		}
		return nil
	}

	return msg
}

func (m *MultiWindowOS) GetLayers() []*lipgloss.Layer {
	var layers []*lipgloss.Layer

	// Sort windows by Z-order for proper layering
	type indexedWindow struct {
		index  int
		window TerminalWindow
	}
	sortedWindows := make([]indexedWindow, len(m.Windows))
	for i, w := range m.Windows {
		sortedWindows[i] = indexedWindow{i, w}
	}
	// Sort by Z-order (lower Z drawn first)
	for i := 0; i < len(sortedWindows)-1; i++ {
		for j := i + 1; j < len(sortedWindows); j++ {
			if sortedWindows[i].window.Z > sortedWindows[j].window.Z {
				sortedWindows[i], sortedWindows[j] = sortedWindows[j], sortedWindows[i]
			}
		}
	}

	for _, sw := range sortedWindows {
		window := sw.window
		isFocused := m.FocusedWindow == sw.index

		// Choose border color based on focus and insert mode
		borderColor := "#666666" // Default
		if isFocused {
			if m.InsertMode {
				borderColor = "#00FF00" // Green when in insert mode
			} else {
				borderColor = "#AFFFFF" // Cyan when focused but not in insert mode
			}
		}

		// Get terminal content
		terminalContent := window.Terminal.View()

		// Create styled box with terminal content
		// Don't set Width/Height - let lipgloss infer from content
		// (setting Width/Height causes ANSI escape code miscounting)
		box := lipgloss.NewStyle().
			BorderForeground(lipgloss.Color(borderColor)).
			Border(lipgloss.RoundedBorder()).
			Background(lipgloss.Color("#000000"))

		content := box.Render(terminalContent.Content)

		layer := lipgloss.NewLayer(content).
			X(window.X).
			Y(window.Y).
			Z(window.Z)

		layers = append(layers, layer)
	}

	return layers
}

func (m *MultiWindowOS) View() tea.View {
	layers := m.GetLayers()
	comp := lipgloss.NewCompositor(layers...)

	// Render compositor to a fixed-size canvas to prevent overflow
	canvasHeight := m.height - 1 // Leave room for status line
	if canvasHeight < 1 {
		canvasHeight = 1
	}
	canvas := lipgloss.NewCanvas(m.width, canvasHeight)
	canvas.Compose(comp)

	// Add status line
	status := "Right-click: New Terminal | Left-click: Select/Drag | 'i': Insert Mode | +/-: Resize"
	if m.InsertMode {
		status = "INSERT MODE - ESC to exit | All input goes to focused terminal"
	}

	statusStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#FFFFFF")).
		Background(lipgloss.Color("#333333")).
		Width(m.width)

	var v tea.View
	v.SetContent(canvas.Render() + "\n" + statusStyle.Render(status))
	v.AltScreen = true
	v.MouseMode = tea.MouseModeAllMotion
	return v
}
