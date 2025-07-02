package bubbleterm

import (
	"os/exec"

	tea "github.com/charmbracelet/bubbletea/v2"
	"github.com/taigrr/bib/emulator"
)

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

// pollTerminal polls the emulator for new output (non-blocking)
func pollTerminal(emu *emulator.Emulator) tea.Cmd {
	return func() tea.Msg {
		// Always return current frame immediately - don't block
		frame := emu.GetScreen()
		return terminalOutputMsg{Frame: frame, EmulatorID: emu.ID()}
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

// resizeTerminal resizes the terminal
func resizeTerminal(emu *emulator.Emulator, width, height int) tea.Cmd {
	return func() tea.Msg {
		err := emu.Resize(width-2, height)
		if err != nil {
			return terminalErrorMsg{Err: err, EmulatorID: emu.ID()}
		}
		return nil
	}
}
