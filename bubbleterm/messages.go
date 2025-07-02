package bubbleterm

import (
	"os/exec"

	tea "github.com/charmbracelet/bubbletea/v2"
	"github.com/taigrr/bib/emulator"
)

// Messages for the Elm architecture

// terminalOutputMsg carries new terminal output
type terminalOutputMsg struct {
	Frame emulator.EmittedFrame
}

// terminalErrorMsg carries terminal errors
type terminalErrorMsg struct {
	Err error
}

// startCommandMsg requests starting a new command
type startCommandMsg struct {
	Cmd *exec.Cmd
}

// Commands (side effects)

// pollTerminal polls the emulator for new output (non-blocking)
func pollTerminal(emu *emulator.Emulator) tea.Cmd {
	return func() tea.Msg {
		// Always return current frame immediately - don't block
		frame := emu.GetScreen()
		return terminalOutputMsg{Frame: frame}
	}
}

// sendInput sends input to the terminal
func sendInput(emu *emulator.Emulator, input string) tea.Cmd {
	return func() tea.Msg {
		err := emu.SendKey(input)
		if err != nil {
			return terminalErrorMsg{Err: err}
		}
		return nil
	}
}

// resizeTerminal resizes the terminal
func resizeTerminal(emu *emulator.Emulator, width, height int) tea.Cmd {
	return func() tea.Msg {
		err := emu.Resize(width, height)
		if err != nil {
			return terminalErrorMsg{Err: err}
		}
		return nil
	}
}