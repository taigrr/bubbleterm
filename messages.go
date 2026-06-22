package bubbleterm

import (
	"os/exec"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/taigrr/bubbleterm/emulator"
)

const pollInterval = 8 * time.Millisecond // ~120 Hz max poll rate

// terminalOutputMsg carries new terminal output
type terminalOutputMsg struct {
	Frame      emulator.EmittedFrame
	EmulatorID string
}

// terminalErrorMsg carries terminal errors
type terminalErrorMsg struct {
	Err        error
	EmulatorID string
}

// startCommandMsg requests starting a new command
type startCommandMsg struct {
	Cmd        *exec.Cmd
	EmulatorID string
}

// Commands (side effects)

// pollTerminal polls the emulator for new output. It loops internally,
// sleeping between checks, and only returns a message when the screen
// has actually changed. This avoids sending idle messages to bubbletea
// that would trigger unnecessary View/render cycles.
func pollTerminal(emu *emulator.Emulator) tea.Cmd {
	return func() tea.Msg {
		for {
			time.Sleep(pollInterval)
			frame := emu.GetScreen()
			if len(frame.Damage) > 0 {
				return terminalOutputMsg{Frame: frame, EmulatorID: emu.ID()}
			}
		}
	}
}

// sendInput sends input to the terminal
func sendInput(emu *emulator.Emulator, input string) tea.Cmd {
	return func() tea.Msg {
		err := emu.SendKey(input)
		if err != nil {
			return terminalErrorMsg{Err: err, EmulatorID: emu.ID()}
		}
		return nil
	}
}

// sendMouseEvent sends a mouse event to the terminal
func sendMouseEvent(emu *emulator.Emulator, x, y, button int, pressed bool) tea.Cmd {
	return func() tea.Msg {
		err := emu.SendMouse(button, x, y, pressed)
		if err != nil {
			return terminalErrorMsg{Err: err, EmulatorID: emu.ID()}
		}
		return nil
	}
}

// sendMouseWheel sends a mouse wheel event to the terminal
func sendMouseWheel(emu *emulator.Emulator, x, y, button int) tea.Cmd {
	return func() tea.Msg {
		err := emu.SendMouseWheel(button, x, y)
		if err != nil {
			return terminalErrorMsg{Err: err, EmulatorID: emu.ID()}
		}
		return nil
	}
}

// resizeTerminal resizes the terminal
func resizeTerminal(emu *emulator.Emulator, width, height int) tea.Cmd {
	return func() tea.Msg {
		err := emu.Resize(width, height)
		if err != nil {
			return terminalErrorMsg{Err: err, EmulatorID: emu.ID()}
		}
		return nil
	}
}
